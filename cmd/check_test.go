package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wkt84/esapi-cli/internal/project"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

func TestCheckGeneratedProjectJSONAndHuman(t *testing.T) {
	parent := t.TempDir()
	sdk := filepath.Join(parent, "sdk & '$(Literal);@value")
	if err := os.Mkdir(sdk, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"VMS.TPS.Common.Model.API.dll", "VMS.TPS.Common.Model.Types.dll"} {
		if err := os.WriteFile(filepath.Join(sdk, name), []byte("not a real assembly"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "project-default"}[explicit], func(t *testing.T) {
			options := scaffold.Options{Name: "Example", Output: t.TempDir(), ESAPIVersion: "19"}
			t.Setenv("ESAPI_HOME", sdk)
			if explicit {
				options.ESAPIPath = sdk
				t.Setenv("ESAPI_HOME", "wrong-environment-path")
			}
			dir, err := scaffold.Generate(options)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "Example.csproj")
			result := runJSON(t, []string{"check", "--project", file, "--json"}, 0)
			var report project.Report
			if err := json.Unmarshal(result.Data, &report); err != nil {
				t.Fatal(err)
			}
			if !result.OK || !report.StaticChecksPassed || report.Properties.ESAPIVersion != "19.0" || report.Properties.TargetFramework != "v4.8" {
				t.Fatalf("generated project failed: %s", result.Data)
			}
			var stdout, stderr bytes.Buffer
			if code := Run([]string{"check", "--project", file}, &stdout, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "Static checks passed: true") {
				t.Fatalf("human output failure: %d %s / %s", code, &stdout, &stderr)
			}
			if err := os.Remove(filepath.Join(dir, "Script.cs")); err != nil {
				t.Fatal(err)
			}
			failed := runJSON(t, []string{"check", "--project", file, "--json"}, 5)
			if failed.OK || string(failed.Data) == "null" || failed.Diagnostics[0].Code != "PROJECT_SOURCE_UNAVAILABLE" {
				t.Fatalf("missing source failure lost report: %+v", failed)
			}
		})
	}
}

func TestCheckSelectionAndArgumentContract(t *testing.T) {
	t.Chdir(t.TempDir())
	result := runJSON(t, []string{"check", "--json"}, 2)
	if result.Diagnostics[0].Code != "PROJECT_NOT_FOUND" || string(result.Data) == "null" {
		t.Fatalf("missing project error invalid: %+v", result)
	}
	for _, name := range []string{"One.csproj", "Two.csproj"} {
		if err := os.WriteFile(name, []byte("<Project/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result = runJSON(t, []string{"check", "--json"}, 2)
	if result.Diagnostics[0].Code != "PROJECT_AMBIGUOUS" || !bytes.Contains(result.Data, []byte("Two.csproj")) {
		t.Fatalf("ambiguous candidates not retained: %+v", result)
	}
	for _, extra := range [][]string{{"--project"}, {"--unknown"}, {"One.csproj"}, {"--esapi-version=20"}, {"--esapi-path=sdk%25"}} {
		result := runJSON(t, append([]string{"check", "--json"}, extra...), 2)
		if result.OK || string(result.Data) != "null" {
			t.Fatalf("invalid input accepted: %+v", result)
		}
	}
	if result := runJSON(t, []string{"check", "--json", "--help"}, 0); !result.OK || !bytes.Contains(result.Data, []byte("Usage:")) {
		t.Fatal("check help unavailable")
	}
}
