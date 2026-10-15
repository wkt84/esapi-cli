//go:build !windows

package build

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// The Unix backend is exercised by portable process fixtures. Product build
// planning still requires a Windows amd64 host.
type unixProcess struct{ command *exec.Cmd }

func startProcess(plan Plan, input, stdout, stderr *os.File) (process, error) {
	command := exec.Command(plan.Tool.Path, plan.Arguments...)
	command.Dir, command.Env = plan.WorkingDirectory, childEnvironment(plan.EnvironmentOverrides)
	command.Stdin, command.Stdout, command.Stderr = input, stdout, stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &unixProcess{command}, nil
}

func (p *unixProcess) Wait() (int, error) {
	err := p.command.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		err = nil // A tool failure is a completed process, not a launcher error.
	}
	return p.command.ProcessState.ExitCode(), err
}

func (p *unixProcess) Kill() error {
	err := syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
