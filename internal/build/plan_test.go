package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wkt84/esapi-cli/internal/detect"
	"github.com/wkt84/esapi-cli/internal/project"
)

func TestPlanToolSelectionAndArguments(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(tool, []byte("not executable; planning must not run it"), 0644); err != nil {
		t.Fatal(err)
	}
	system := detect.System{OS: "windows", Arch: "amd64", Getenv: func(string) string { return "" }, LookPath: func(name string) (string, error) {
		if name == "MSBuild.exe" {
			return tool, nil
		}
		return "", os.ErrNotExist
	}}
	selection := project.Selection{Project: filepath.Join(dir, "Project & name.csproj"), Candidates: []string{}}
	plan, err := NewPlan(selection, Options{Timeout: time.Minute, ESAPIPath: filepath.Join(dir, "sdk & $(Literal);@value"), Debug: true, Clean: true, Verbose: true}, system)
	if err != nil || plan.Tool.Kind != "msbuild" || plan.Tool.Source != "path" || plan.Arguments[0] != selection.Project || plan.Configuration != "Debug" || plan.Target != "Rebuild" || plan.Timeout != "1m0s" {
		t.Fatalf("bad plan: %+v / %v", plan, err)
	}
	args := strings.Join(plan.Arguments, "\n")
	for _, want := range []string{"-target:Rebuild", "-noAutoResponse", "-nodeReuse:false", "-property:UseSharedCompilation=false", "-property:Platform=x64", "-verbosity:detailed", "%24%28Literal%29%3B%40value"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %s in %v", want, plan.Arguments)
		}
	}
	if strings.Contains(args, "-restore") || len(plan.EnvironmentOverrides) == 0 {
		t.Fatal("unplanned restore or missing environment overrides")
	}
	_, err = NewPlan(selection, Options{MSBuildPath: filepath.Join(dir, "missing.exe"), Timeout: DefaultTimeout}, system)
	if err == nil {
		t.Fatal("explicit missing tool fell back")
	}
	if err := os.MkdirAll(filepath.Join(dir, "sdk", "fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sdk", "fixture", "MSBuild.dll"), []byte("marker only"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err = NewPlan(selection, Options{DotnetPath: tool, Timeout: DefaultTimeout}, system)
	if err != nil || plan.Tool.Kind != "dotnet" || plan.Arguments[0] != "msbuild" || plan.Configuration != "Release" {
		t.Fatalf("explicit dotnet not selected: %+v / %v", plan, err)
	}
	system.LookPath = func(name string) (string, error) {
		if name == "dotnet.exe" {
			return tool, nil
		}
		return "", os.ErrNotExist
	}
	plan, err = NewPlan(selection, Options{Timeout: DefaultTimeout}, system)
	if err != nil || plan.Tool.Kind != "dotnet" {
		t.Fatalf("missing native tool did not use SDK: %+v / %v", plan, err)
	}
	if err := os.Remove(filepath.Join(dir, "sdk", "fixture", "MSBuild.dll")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlan(selection, Options{DotnetPath: tool, Timeout: DefaultTimeout}, system); err == nil {
		t.Fatal("runtime-only dotnet accepted")
	}
	system.OS = "linux"
	if _, err := NewPlan(selection, Options{Timeout: DefaultTimeout}, system); err == nil {
		t.Fatal("unsupported build host accepted")
	}
}
