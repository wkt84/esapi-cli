package build

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const LogLimit = 1024 * 1024
const cleanupTimeout = 2 * time.Second

type Result struct {
	Plan
	Status          string `json:"status"`
	Started         bool   `json:"started"`
	ExitCode        *int   `json:"exitCode"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdoutTruncated"`
	StderrTruncated bool   `json:"stderrTruncated"`
	CleanupError    string `json:"cleanupError,omitempty"`
}

type process interface {
	Wait() (int, error)
	Kill() error
}

type capture struct {
	sync.Mutex
	data      []byte
	truncated bool
}

func (c *capture) Write(data []byte) (int, error) {
	c.Lock()
	defer c.Unlock()
	n := len(data)
	left := LogLimit - len(c.data)
	if len(data) > left {
		data, c.truncated = data[:left], true
	}
	c.data = append(c.data, data...)
	return n, nil // Always drain the pipe, including output beyond the limit.
}

func (c *capture) snapshot() (string, bool) {
	c.Lock()
	defer c.Unlock()
	return strings.ToValidUTF8(string(c.data), "\ufffd"), c.truncated
}

// Execute captures bounded logs and terminates the process tree on interruption,
// timeout, and normal completion. It never invokes a shell or restores packages.
func Execute(ctx context.Context, plan Plan) (result Result, err error) {
	result = Result{Plan: plan, Status: "notStarted"}
	if err := ctx.Err(); err != nil {
		result.Status = "cancelled"
		if errors.Is(err, context.DeadlineExceeded) {
			result.Status = "timedOut"
		}
		return result, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return result, err
	}
	defer outR.Close()
	defer outW.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		return result, err
	}
	defer errR.Close()
	defer errW.Close()
	input, err := os.Open(os.DevNull)
	if err != nil {
		return result, err
	}
	defer input.Close()
	child, err := startProcess(plan, input, outW, errW)
	if err != nil {
		return result, err
	}
	result.Started = true
	_ = outW.Close()
	_ = errW.Close()
	var stdout, stderr capture
	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); _, _ = io.Copy(&stdout, outR) }()
	go func() { defer readers.Done(); _, _ = io.Copy(&stderr, errR) }()
	type completion struct {
		code int
		err  error
	}
	done := make(chan completion, 1)
	go func() { code, err := child.Wait(); done <- completion{code, err} }()
	var completed completion
	cleanupFailure := func(failure error) {
		if failure != nil {
			result.CleanupError += failure.Error() + "\n"
			err = errors.Join(err, failure)
		}
	}
	select {
	case completed = <-done:
		err = completed.err
		result.ExitCode = &completed.code
	case <-ctx.Done():
		err = ctx.Err()
		cleanupFailure(child.Kill())
		timer := time.NewTimer(cleanupTimeout)
		select {
		case completed = <-done:
			result.ExitCode = &completed.code
		case <-timer.C:
			cleanupFailure(errors.New("build process did not exit after termination"))
		}
		timer.Stop()
	}
	cleanupFailure(child.Kill())
	drained := make(chan struct{})
	go func() { readers.Wait(); close(drained) }()
	timer := time.NewTimer(cleanupTimeout)
	select {
	case <-drained:
	case <-timer.C:
		_ = outR.Close()
		_ = errR.Close()
		cleanupFailure(errors.New("build log pipes did not close after process-tree termination"))
	}
	timer.Stop()
	result.Stdout, result.StdoutTruncated = stdout.snapshot()
	result.Stderr, result.StderrTruncated = stderr.snapshot()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Status = "timedOut"
	case errors.Is(err, context.Canceled):
		result.Status = "cancelled"
	case err != nil || completed.code != 0:
		result.Status = "failed"
	default:
		result.Status = "succeeded"
	}
	return result, err
}

func environmentOverrides() []string {
	return []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE=1", "MSBUILDDISABLENODEREUSE=1", "DOTNET_CLI_RUN_MSBUILD_OUTOFPROC=0", "DOTNET_CLI_USE_MSBUILDNOINPROCNODE=0", "MSBUILDNOINPROCNODE=0"}
}

func childEnvironment(overrides []string) []string {
	env := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		replaced := false
		for _, override := range overrides {
			key, _, _ := strings.Cut(override, "=")
			replaced = replaced || strings.EqualFold(name, key)
		}
		if !replaced {
			env = append(env, value)
		}
	}
	return append(env, overrides...)
}
