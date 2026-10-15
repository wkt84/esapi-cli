package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wkt84/esapi-cli/internal/build"
)

// The child accepts MSBuild-shaped argv before testing's own flag parsing.
func TestMain(m *testing.M) {
	if mode := os.Getenv("ESAPI_TEST_CLI_TOOL"); mode != "" {
		if mode == "failure" {
			fmt.Fprint(os.Stderr, "fixture compile failure")
			os.Exit(7)
		}
		if mode == "sleep" {
			time.Sleep(30 * time.Second)
		}
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestBuildArgumentsAndSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, extra := range [][]string{{"--project"}, {"--unknown"}, {"Example.csproj"}, {"--release", "--debug"}, {"--msbuild-path=one", "--dotnet-path=two"}, {"--timeout=0"}, {"--timeout=25h"}, {"--timeout=invalid"}, {"--esapi-path=bad%25"}} {
		result := runJSON(t, append([]string{"build", "--json"}, extra...), 2)
		if result.OK || string(result.Data) != "null" {
			t.Fatalf("invalid arguments accepted: %+v", result)
		}
	}
	result := runJSON(t, []string{"build", "--json"}, 2)
	if result.Diagnostics[0].Code != "PROJECT_NOT_FOUND" || string(result.Data) == "null" {
		t.Fatalf("selection failure invalid: %+v", result)
	}
	for _, name := range []string{"A.csproj", "B.csproj"} {
		if err := os.WriteFile(name, []byte("<Project/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result = runJSON(t, []string{"build", "--json"}, 2)
	if result.Diagnostics[0].Code != "PROJECT_AMBIGUOUS" || !bytes.Contains(result.Data, []byte("B.csproj")) {
		t.Fatal("ambiguity lost candidate paths")
	}
	if result := runJSON(t, []string{"build", "--help", "--json"}, 0); !result.OK || !bytes.Contains(result.Data, []byte("Usage:")) {
		t.Fatal("build help missing")
	}
}

func TestBuildJSONExecutionContract(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("product build planning requires Windows; portable runner fixtures cover Linux")
	}
	file := filepath.Join(t.TempDir(), "Example.csproj")
	if err := os.WriteFile(file, []byte("<Project/>"), 0644); err != nil {
		t.Fatal(err)
	}
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"build", "--project", file, "--msbuild-path", tool, "--json"}
	t.Setenv("ESAPI_TEST_CLI_TOOL", "success")
	preview := runJSON(t, append(append([]string{}, args...), "--dry-run"), 0)
	var report build.Result
	if err := json.Unmarshal(preview.Data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Started || report.ExitCode != nil || report.Status != "planned" {
		t.Fatal("dry-run executed a tool")
	}
	result := runJSON(t, append(append([]string{}, args...), "--debug", "--clean", "--verbose"), 0)
	if err := json.Unmarshal(result.Data, &report); err != nil {
		t.Fatal(err)
	}
	if !result.OK || !report.Started || report.Status != "succeeded" || !strings.Contains(report.Stdout, "-target:Rebuild") || report.Configuration != "Debug" || report.ExitCode == nil || *report.ExitCode != 0 {
		t.Fatalf("execution result invalid: %s", result.Data)
	}
	t.Setenv("ESAPI_TEST_CLI_TOOL", "failure")
	result = runJSON(t, args, 4)
	if result.Diagnostics[0].Code != "BUILD_FAILED" || !bytes.Contains(result.Data, []byte("fixture compile failure")) {
		t.Fatal("build failure logs lost")
	}
	t.Setenv("ESAPI_TEST_CLI_TOOL", "sleep")
	result = runJSON(t, append(append([]string{}, args...), "--timeout=150ms"), 4)
	if result.Diagnostics[0].Code != "BUILD_TIMEOUT" {
		t.Fatal("timeout not distinguished")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := RunContext(ctx, args, &stdout, &stderr); code != 4 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "BUILD_CANCELLED") {
		t.Fatalf("cancellation contract invalid: %d %s / %s", code, &stdout, &stderr)
	}
}
