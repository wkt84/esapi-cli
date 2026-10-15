package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

type decodedResponse struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Command       string                  `json:"command"`
	OK            bool                    `json:"ok"`
	Data          json.RawMessage         `json:"data"`
	Diagnostics   []diagnostic.Diagnostic `json:"diagnostics"`
}

func runJSON(t *testing.T, args []string, wantExit int) decodedResponse {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if exit := Run(args, &stdout, &stderr); exit != wantExit {
		t.Fatalf("exit = %d, want %d; stdout = %s; stderr = %s", exit, wantExit, &stdout, &stderr)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON mode wrote to stderr: %s", &stderr)
	}
	decoder := json.NewDecoder(&stdout)
	var result decodedResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("output is not a JSON object: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected exactly one JSON object, got trailing output: %v", err)
	}
	if result.SchemaVersion != 1 || result.Diagnostics == nil {
		t.Fatalf("invalid envelope: %+v", result)
	}
	return result
}

func TestJSONDryRunThenGenerate(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing", "scripts")
	args := []string{"--json", "new", "Example", "--output", parent}
	preview := runJSON(t, append(append([]string(nil), args...), "--dry-run"), 0)
	if !preview.OK || preview.Command != "new" {
		t.Fatalf("unexpected dry-run result: %+v", preview)
	}
	if _, err := os.Stat(filepath.Dir(parent)); !os.IsNotExist(err) {
		t.Fatal("dry-run created parent directories")
	}
	var planned newData
	if err := json.Unmarshal(preview.Data, &planned); err != nil {
		t.Fatal(err)
	}
	if !planned.DryRun || planned.ESAPIVersion != "15.6" || planned.TargetFramework != "v4.5" || planned.ESAPIPathSource != "environment" || planned.ESAPIPath != "" || len(planned.Files) != 4 {
		t.Fatalf("unexpected plan: %+v", planned)
	}
	repeated := runJSON(t, append(append([]string(nil), args...), "--dry-run"), 0)
	if !bytes.Equal(preview.Data, repeated.Data) {
		t.Fatal("identical dry-runs produced different plans")
	}
	created := runJSON(t, args, 0)
	var actual newData
	if err := json.Unmarshal(created.Data, &actual); err != nil {
		t.Fatal(err)
	}
	if !created.OK || actual.DryRun || !reflect.DeepEqual(planned.ProjectPlan, actual.ProjectPlan) {
		t.Fatal("generation differs from the dry-run plan")
	}
	for _, file := range actual.Files {
		if file.Action != "create" {
			t.Errorf("unexpected action: %+v", file)
		}
		if _, err := os.Stat(filepath.Join(actual.Path, file.Path)); err != nil {
			t.Fatal(err)
		}
	}
	conflict := runJSON(t, args, 1)
	if conflict.Diagnostics[0].Code != "DESTINATION_EXISTS" || conflict.Diagnostics[0].Hint == "" {
		t.Fatalf("missing actionable conflict: %+v", conflict)
	}
}

func TestJSONErrorContract(t *testing.T) {
	for _, item := range []struct {
		args []string
		code string
	}{
		{[]string{"new"}, "INVALID_ARGUMENTS"},
		{[]string{"new", "../Outside"}, "INVALID_PROJECT_NAME"},
		{[]string{"new", "Example", "--unknown"}, "UNKNOWN_OPTION"},
		{[]string{"new", "Example", "--type=standalone"}, "UNSUPPORTED_PROJECT_TYPE"},
		{[]string{"new", "Example", "--esapi-version=20"}, "UNSUPPORTED_ESAPI_VERSION"},
		{[]string{"new", "Example", "--esapi-path=sdk%25"}, "INVALID_ESAPI_PATH"},
		{[]string{"new", "Example", "--output"}, "INVALID_ARGUMENTS"},
		{[]string{"new", "Example", "--force=invalid"}, "INVALID_ARGUMENTS"},
		{[]string{"unknown"}, "UNKNOWN_COMMAND"},
		{[]string{"new", "Example", "--json=invalid"}, "INVALID_ARGUMENTS"},
	} {
		t.Run(strings.Join(item.args, " "), func(t *testing.T) {
			args := append([]string{"--json"}, item.args...)
			result := runJSON(t, args, 2)
			if result.OK || string(result.Data) != "null" || len(result.Diagnostics) != 1 {
				t.Fatalf("invalid error envelope: %+v", result)
			}
			if d := result.Diagnostics[0]; d.Code != item.code || d.Severity != "error" || d.Message == "" {
				t.Fatalf("diagnostic = %+v, want %s", d, item.code)
			}
		})
	}
}

func TestJSONHelpAndDelimiter(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"new", "--help", "--json"}, {"--json=true", "--help"}} {
		if result := runJSON(t, args, 0); !result.OK || !bytes.Contains(result.Data, []byte("Usage:")) {
			t.Fatalf("invalid help response: %+v", result)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"new", "--", "--json"}, &stdout, &stderr); code != 2 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "error: ") {
		t.Fatalf("-- delimiter was not respected: stdout=%s stderr=%s", &stdout, &stderr)
	}
}

func TestForceDryRunReportsActionsWithoutWriting(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "Example")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Script.cs", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte("keep"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result := runJSON(t, []string{"new", "Example", "--output", parent, "--force", "--dry-run", "--json"}, 0)
	var plan newData
	if err := json.Unmarshal(result.Data, &plan); err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		want := "create"
		if file.Path == "Script.cs" {
			want = "overwrite"
		}
		if file.Action != want {
			t.Fatalf("action = %+v; want %s", file, want)
		}
	}
	entries, _ := os.ReadDir(path)
	if len(entries) != 2 {
		t.Fatal("dry-run created files")
	}
	for _, entry := range entries {
		content, _ := os.ReadFile(filepath.Join(path, entry.Name()))
		if string(content) != "keep" {
			t.Fatal("dry-run changed a file")
		}
	}
}
