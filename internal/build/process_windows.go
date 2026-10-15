package build

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var createJob = kernel.NewProc("CreateJobObjectW")
var setJob = kernel.NewProc("SetInformationJobObject")
var initializeAttributes = kernel.NewProc("InitializeProcThreadAttributeList")
var updateAttributes = kernel.NewProc("UpdateProcThreadAttribute")
var deleteAttributes = kernel.NewProc("DeleteProcThreadAttributeList")

type jobLimits struct {
	ProcessTime, JobTime int64
	Flags                uint32
	Minimum, Maximum     uintptr
	ActiveProcesses      uint32
	Affinity             uintptr
	Priority, Scheduling uint32
}

type extendedJobLimits struct {
	Basic                                          jobLimits
	IO                                             [6]uint64
	ProcessMemory, JobMemory, PeakProcess, PeakJob uintptr
}

type windowsProcess struct {
	handle  syscall.Handle
	job     syscall.Handle
	once    sync.Once
	killErr error
}

func apiError(name string, err error) error {
	if err == syscall.Errno(0) {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, err)
}

// JOB_LIST assigns the process to the job at creation, before its first thread
// runs. No child can be spawned in an assignment race. Windows 10+ is required.
// Only the three duplicated standard handles are inherited by the build tool.
// https://learn.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute
func startProcess(plan Plan, input, stdout, stderr *os.File) (process, error) {
	rawJob, _, err := createJob.Call(0, 0)
	if rawJob == 0 {
		return nil, apiError("CreateJobObjectW", err)
	}
	job := syscall.Handle(rawJob)
	owned := true
	defer func() {
		if owned {
			_ = syscall.CloseHandle(job)
		}
	}()
	limits := extendedJobLimits{Basic: jobLimits{Flags: 0x2000}} // KILL_ON_JOB_CLOSE
	if ok, _, err := setJob.Call(uintptr(job), 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 {
		return nil, apiError("SetInformationJobObject", err)
	}
	parent, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	handles := make([]syscall.Handle, 3)
	for index, file := range []*os.File{input, stdout, stderr} {
		if err := syscall.DuplicateHandle(parent, syscall.Handle(file.Fd()), parent, &handles[index], 0, true, syscall.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		defer syscall.CloseHandle(handles[index])
	}
	var size uintptr
	_, _, queryErr := initializeAttributes.Call(0, 2, 0, uintptr(unsafe.Pointer(&size)))
	if queryErr != syscall.ERROR_INSUFFICIENT_BUFFER || size == 0 {
		return nil, apiError("InitializeProcThreadAttributeList(size)", queryErr)
	}
	storage := make([]uintptr, (size+unsafe.Sizeof(uintptr(0))-1)/unsafe.Sizeof(uintptr(0)))
	attributes := unsafe.Pointer(&storage[0])
	if ok, _, err := initializeAttributes.Call(uintptr(attributes), 2, 0, uintptr(unsafe.Pointer(&size))); ok == 0 {
		return nil, apiError("InitializeProcThreadAttributeList", err)
	}
	defer deleteAttributes.Call(uintptr(attributes))
	for _, attribute := range []struct {
		key   uintptr
		value unsafe.Pointer
		size  uintptr
	}{
		{0x0002000d, unsafe.Pointer(&job), unsafe.Sizeof(job)},                                       // JOB_LIST
		{0x00020002, unsafe.Pointer(&handles[0]), uintptr(len(handles)) * unsafe.Sizeof(handles[0])}, // HANDLE_LIST
	} {
		if ok, _, err := updateAttributes.Call(uintptr(attributes), 0, attribute.key, uintptr(attribute.value), attribute.size, 0, 0); ok == 0 {
			return nil, apiError("UpdateProcThreadAttribute", err)
		}
	}
	app, err := syscall.UTF16PtrFromString(plan.Tool.Path)
	if err != nil {
		return nil, err
	}
	args := []string{syscall.EscapeArg(plan.Tool.Path)}
	for _, arg := range plan.Arguments {
		args = append(args, syscall.EscapeArg(arg))
	}
	command, err := syscall.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return nil, err
	}
	directory, err := syscall.UTF16PtrFromString(plan.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	env := childEnvironment(plan.EnvironmentOverrides)
	sort.SliceStable(env, func(i, j int) bool { return strings.ToUpper(env[i]) < strings.ToUpper(env[j]) })
	block := []uint16{}
	for _, entry := range env {
		value, err := syscall.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, value...)
	}
	block = append(block, 0)
	startup := struct {
		syscall.StartupInfo
		Attributes unsafe.Pointer
	}{Attributes: attributes}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = syscall.STARTF_USESTDHANDLES | syscall.STARTF_USESHOWWINDOW
	startup.ShowWindow = syscall.SW_HIDE
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	var child syscall.ProcessInformation
	const flags = 0x08000000 | 0x00080000 | syscall.CREATE_UNICODE_ENVIRONMENT // NO_WINDOW | EXTENDED_STARTUPINFO
	err = syscall.CreateProcess(app, command, nil, nil, true, flags, &block[0], directory, &startup.StartupInfo, &child)
	runtime.KeepAlive(storage)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(&job)
	if err != nil {
		return nil, err
	}
	_ = syscall.CloseHandle(child.Thread)
	owned = false
	return &windowsProcess{handle: child.Process, job: job}, nil
}

func (p *windowsProcess) Wait() (int, error) {
	defer syscall.CloseHandle(p.handle)
	if _, err := syscall.WaitForSingleObject(p.handle, syscall.INFINITE); err != nil {
		return -1, err
	}
	var code uint32
	err := syscall.GetExitCodeProcess(p.handle, &code)
	return int(code), err
}

func (p *windowsProcess) Kill() error {
	p.once.Do(func() { p.killErr = syscall.CloseHandle(p.job) })
	return p.killErr
}
