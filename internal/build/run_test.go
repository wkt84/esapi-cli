package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Re-executing the test binary gives both platforms real process/pipe fixtures
// without a shell, compiler, vendor SDK, or externally installed tool.
func TestProcessHelper(t *testing.T) {
	index := -1
	for n, arg := range os.Args {
		if arg == "--" {
			index = n + 1
			break
		}
	}
	if index < 0 {
		return
	}
	args := os.Args[index:]
	switch args[0] {
	case "success":
		dir, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": args[1:], "directory": dir, "telemetry": os.Getenv("DOTNET_CLI_TELEMETRY_OPTOUT")})
		fmt.Fprint(os.Stderr, "fixture stderr")
	case "failure":
		fmt.Fprint(os.Stdout, "fixture compiler error")
		os.Exit(7)
	case "flood":
		fmt.Fprint(os.Stdout, strings.Repeat("O", LogLimit+2000))
		fmt.Fprint(os.Stderr, strings.Repeat("E", LogLimit+1000))
	case "sleep":
		time.Sleep(30 * time.Second)
	case "heartbeat":
		for {
			file, err := os.OpenFile(args[1], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				os.Exit(12)
			}
			_, _ = file.WriteString("alive\n")
			_ = file.Close()
			time.Sleep(20 * time.Millisecond)
		}
	case "tree", "detach":
		child := exec.Command(os.Args[0], "-test.run=TestProcessHelper", "--", "heartbeat", args[1])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(13)
		}
		for {
			if data, _ := os.ReadFile(args[1]); len(data) != 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if args[0] == "tree" {
			time.Sleep(30 * time.Second)
		}
	}
	os.Exit(0)
}

func helperPlan(t *testing.T, mode string, args ...string) Plan {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{WorkingDirectory: t.TempDir(), Arguments: append([]string{"-test.run=TestProcessHelper", "--", mode}, args...), EnvironmentOverrides: environmentOverrides()}
	plan.Tool.Path = exe
	return plan
}

func TestExecuteArgumentsLogsAndExitCodes(t *testing.T) {
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "0")
	argument := `spaces & $(literal);'"日本語\`
	plan := helperPlan(t, "success", argument)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := Execute(ctx, plan)
	if err != nil || !result.Started || result.Status != "succeeded" || result.ExitCode == nil || *result.ExitCode != 0 || result.Stderr != "fixture stderr" || result.CleanupError != "" {
		t.Fatalf("invalid execution: %+v / %v", result, err)
	}
	var output struct {
		Args                 []string
		Directory, Telemetry string
	}
	if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil || len(output.Args) != 1 || output.Args[0] != argument || output.Directory != plan.WorkingDirectory || output.Telemetry != "1" {
		t.Fatalf("arguments/environment/directory changed: %s / %v", result.Stdout, err)
	}
	result, err = Execute(ctx, helperPlan(t, "failure"))
	if err != nil || result.Status != "failed" || result.ExitCode == nil || *result.ExitCode != 7 || !strings.Contains(result.Stdout, "compiler error") {
		t.Fatalf("tool failure lost: %+v / %v", result, err)
	}
	plan.Tool.Path = filepath.Join(t.TempDir(), "not-an-executable")
	result, err = Execute(ctx, plan)
	if err == nil || result.Started || result.ExitCode != nil || result.Status != "notStarted" {
		t.Fatalf("launcher failure invalid: %+v / %v", result, err)
	}
}

func TestBoundedOutputIsDrained(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := Execute(ctx, helperPlan(t, "flood"))
	if err != nil || result.Status != "succeeded" || len(result.Stdout) != LogLimit || len(result.Stderr) != LogLimit || !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("log limits failed: status=%s lengths=%d/%d truncated=%v/%v error=%v", result.Status, len(result.Stdout), len(result.Stderr), result.StdoutTruncated, result.StderrTruncated, err)
	}
}

func TestTimeoutAndPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result, err := Execute(ctx, helperPlan(t, "sleep"))
	if !errors.Is(err, context.DeadlineExceeded) || result.Status != "timedOut" || !result.Started || result.CleanupError != "" {
		t.Fatalf("timeout invalid: %+v / %v", result, err)
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	result, err = Execute(ctx, helperPlan(t, "success"))
	if !errors.Is(err, context.Canceled) || result.Status != "cancelled" || result.Started || result.ExitCode != nil {
		t.Fatalf("pre-cancelled context launched a tool: %+v / %v", result, err)
	}
}

func TestDescendantsTerminateOnCancellationAndParentExit(t *testing.T) {
	for _, mode := range []string{"tree", "detach"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "heartbeat")
			plan := helperPlan(t, mode, marker)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type finished struct {
				result Result
				err    error
			}
			done := make(chan finished, 1)
			go func() { result, err := Execute(ctx, plan); done <- finished{result, err} }()
			var stopped finished
			complete := false
		ready:
			for {
				if data, _ := os.ReadFile(marker); len(data) > 0 {
					break
				}
				select {
				case stopped = <-done:
					complete = true
					if data, _ := os.ReadFile(marker); len(data) > 0 {
						break ready
					}
					t.Fatalf("helper exited before spawning descendant: %+v / %v", stopped.result, stopped.err)
				case <-ctx.Done():
					t.Fatal("descendant did not start")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if mode == "tree" {
				cancel()
			}
			if !complete {
				stopped = <-done
			}
			if stopped.result.CleanupError != "" || (mode == "tree" && !errors.Is(stopped.err, context.Canceled)) || (mode == "detach" && (stopped.err != nil || stopped.result.Status != "succeeded")) {
				t.Fatalf("tree termination failed: %+v / %v", stopped.result, stopped.err)
			}
			before, _ := os.ReadFile(marker)
			time.Sleep(120 * time.Millisecond)
			after, _ := os.ReadFile(marker)
			if string(before) != string(after) {
				t.Fatal("descendant continued after build completed")
			}
		})
	}
}
