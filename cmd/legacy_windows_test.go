package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual Windows PowerShell argument serializer, not a simulated
// malformed argv. A spaced directory with a trailing backslash reaches native
// applications with a literal quote under the legacy serializer.
func TestWindowsPowerShellSDKDirectory(t *testing.T) {
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("Windows PowerShell unavailable: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ESAPI_TEST_CLI_RUN", "1")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	sdk := filepath.Join(t.TempDir(), "SDK with spaces")
	output := filepath.Join(t.TempDir(), "not-created")
	for _, trailing := range []bool{false, true} {
		value := sdk
		if trailing {
			value += `\`
		}
		script := "$sdkPath = " + quote(value) + "; & " + quote(exe) + " --json new Example --dry-run --output " + quote(output) + " --esapi-path $sdkPath; exit $LASTEXITCODE"
		command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", script)
		data, err := command.Output()
		var result decodedResponse
		if decodeErr := json.Unmarshal(data, &result); decodeErr != nil {
			t.Fatalf("invalid native-command JSON: %s / %v / %v", data, decodeErr, err)
		}
		if trailing {
			if err == nil || result.OK || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "INVALID_ESAPI_PATH" || !strings.Contains(result.Diagnostics[0].Hint, "trailing backslash") {
				t.Fatalf("legacy-corrupted path was accepted: %s / %v", data, err)
			}
		} else {
			if err != nil || !result.OK {
				t.Fatalf("path without trailing separator failed: %s / %v", data, err)
			}
			var plan newData
			if err := json.Unmarshal(result.Data, &plan); err != nil || plan.ESAPIPath != sdk {
				t.Fatalf("unexpected SDK binding: %s / %v", result.Data, err)
			}
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("dry-run or rejected directory created output")
		}
	}
}
