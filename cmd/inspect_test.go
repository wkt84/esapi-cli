package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wkt84/esapi-cli/internal/detect"
)

func TestInfoJSONIncludesCapabilitiesAndPartialFacts(t *testing.T) {
	missing := t.TempDir() + "/missing-sdk"
	result := runJSON(t, []string{"info", "--json", "--esapi-version=18", "--esapi-path", missing}, 0)
	var data infoData
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Command != "info" || len(result.Diagnostics) != 0 || data.CLI.Version == "" || data.Environment.ESAPI.Status != "missing" || data.Environment.ESAPI.Source != "argument" {
		t.Fatalf("invalid info response: %s", result.Data)
	}
	if len(data.Capabilities.Commands) != 6 || len(data.Capabilities.Profiles) != 7 || data.Capabilities.SchemaVersions[0] != 1 || data.Environment.Profile.Version != "18.0" {
		t.Fatalf("capabilities/profile selection incomplete: %s", result.Data)
	}
}

func TestDoctorJSONKeepsInspectionOnEnvironmentFailure(t *testing.T) {
	directory := t.TempDir()
	result := runJSON(t, []string{"doctor", "--esapi-path", directory, "--json"}, 3)
	var report detect.DoctorReport
	if err := json.Unmarshal(result.Data, &report); err != nil {
		t.Fatal(err)
	}
	if result.OK || report.StaticChecksPassed || len(report.Checks) == 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("missing failed-doctor report: %s", result.Data)
	}
	codes := map[string]bool{}
	for _, diagnostic := range result.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if !codes["ESAPI_API_DLL_NOT_FOUND"] || !codes["ESAPI_TYPES_DLL_NOT_FOUND"] {
		t.Fatalf("missing precise DLL diagnostics: %+v", result.Diagnostics)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"doctor", "--esapi-path", directory}, &stdout, &stderr); code != 3 || !strings.Contains(stdout.String(), "Static checks passed: false") || !strings.Contains(stderr.String(), "ESAPI_API_DLL_NOT_FOUND") {
		t.Fatalf("invalid human-readable doctor streams: %s / %s", &stdout, &stderr)
	}
}

func TestInspectionArgumentsAndHelp(t *testing.T) {
	for _, command := range []string{"doctor", "info"} {
		for _, extra := range [][]string{{"unexpected"}, {"--esapi-path"}, {"--unknown"}, {"--esapi-version=20"}} {
			args := append([]string{command, "--json"}, extra...)
			result := runJSON(t, args, 2)
			if result.OK || string(result.Data) != "null" {
				t.Fatalf("invalid arguments were not rejected: %+v", result)
			}
		}
		if result := runJSON(t, []string{command, "--help", "--json"}, 0); !result.OK || !bytes.Contains(result.Data, []byte("Usage:")) {
			t.Fatalf("missing command help: %+v", result)
		}
	}
}

func TestVersionAndAlias(t *testing.T) {
	var version json.RawMessage
	for _, command := range []string{"version", "--version"} {
		result := runJSON(t, []string{"--json", command}, 0)
		if !result.OK || result.Command != "version" || !bytes.Contains(result.Data, []byte(`"version":`)) {
			t.Fatalf("invalid version result: %+v", result)
		}
		if version != nil && !bytes.Equal(version, result.Data) {
			t.Fatal("version and --version disagree")
		}
		version = result.Data
	}
	runJSON(t, []string{"version", "extra", "--json"}, 2)
}
