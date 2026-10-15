package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewArgumentOrder(t *testing.T) {
	for _, args := range [][]string{
		{"new", "Example", "--output", "OUTPUT", "--esapi-version", "18"},
		{"new", "--output", "OUTPUT", "Example", "--esapi-version=18.0"},
		{"new", "--esapi-version", "18", "--output=OUTPUT", "--", "Example"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			parent := t.TempDir()
			args = append([]string(nil), args...)
			for i := range args {
				args[i] = strings.ReplaceAll(args[i], "OUTPUT", parent)
			}
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit = %d; stderr = %s", code, &stderr)
			}
			if _, err := os.Stat(filepath.Join(parent, "Example", "Example.csproj")); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), "Created ") || stderr.Len() != 0 {
				t.Fatalf("stdout = %s; stderr = %s", &stdout, &stderr)
			}
		})
	}
}

func TestInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"new"}, {"new", "One", "Two"},
		{"new", "Example", "--unknown"}, {"new", "Example", "--output"},
		{"new", "Example", "--output", "--force"}, {"new", "Example", "--force=invalid"},
		{"new", "../Outside"}, {"new", "Example", "--type", "standalone"},
		{"new", "Example", "--esapi-version", "auto"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit = %d; stderr = %s", code, &stderr)
			}
			if !strings.HasPrefix(stderr.String(), "error: ") || stdout.Len() != 0 {
				t.Fatalf("stdout = %s; stderr = %s", &stdout, &stderr)
			}
		})
	}
}

func TestHelpDoesNotGenerateFiles(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"new", "--help"}, {"new", "Example", "-h"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Usage:") || stderr.Len() != 0 {
			t.Fatalf("args = %v; stdout = %s; stderr = %s", args, &stdout, &stderr)
		}
	}
}

func TestFilesystemErrorExitCode(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "Example"), 0755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"new", "Example", "--output", parent}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d; stderr = %s", code, &stderr)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("incorrect error: %s", &stderr)
	}
}
