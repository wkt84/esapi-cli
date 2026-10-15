package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

func TestApplyRevalidatesDestination(t *testing.T) {
	parent := t.TempDir()
	plan, err := Plan(Options{Name: "Example", Output: parent})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(plan.Path, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(plan.Path, "notes.txt"), "keep")
	err = plan.Apply()
	var detail *diagnostic.Error
	if !errors.As(err, &detail) || detail.Code != "DESTINATION_EXISTS" {
		t.Fatalf("expected a conflict after planning, got %v", err)
	}
	entries, _ := os.ReadDir(plan.Path)
	if len(entries) != 1 || readFile(t, filepath.Join(plan.Path, "notes.txt")) != "keep" {
		t.Fatal("apply changed a newly occupied destination")
	}
}

func TestGeneratedFilesAreDeterministic(t *testing.T) {
	var baseline map[string]string
	for _, environment := range []string{"first-sdk", "second-sdk"} {
		t.Setenv("ESAPI_HOME", environment)
		path, err := Generate(Options{Name: "Example", Output: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			files[entry.Name()] = readFile(t, filepath.Join(path, entry.Name()))
		}
		if baseline == nil {
			baseline = files
			continue
		}
		for name, content := range baseline {
			if files[name] != content {
				t.Errorf("%s depends on the current SDK environment or output parent", name)
			}
		}
	}
}
