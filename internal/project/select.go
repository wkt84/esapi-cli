// Package project performs bounded, read-only inspection of conventional ESAPI
// project declarations. It never evaluates MSBuild or executes project code.
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

type Selection struct {
	Project    string   `json:"project"`
	Candidates []string `json:"candidates"`
}

// Select considers only regular .csproj files in the current directory unless
// an explicit file is supplied. Candidates follow os.ReadDir's sorted order.
func Select(explicit string) (Selection, error) {
	selection := Selection{Candidates: []string{}}
	if explicit == "" {
		entries, err := os.ReadDir(".")
		if err != nil {
			return selection, selectionError("PROJECT_ACCESS_FAILED", "cannot list current directory", ".", err)
		}
		for _, entry := range entries {
			if !strings.EqualFold(filepath.Ext(entry.Name()), ".csproj") || !entry.Type().IsRegular() {
				continue
			}
			path, err := filepath.Abs(entry.Name())
			if err != nil {
				return selection, selectionError("PROJECT_ACCESS_FAILED", "cannot resolve project path", entry.Name(), err)
			}
			selection.Candidates = append(selection.Candidates, path)
		}
		if len(selection.Candidates) == 0 {
			return selection, selectionError("PROJECT_NOT_FOUND", "no .csproj file in the current directory; use --project <file>", "", nil)
		}
		if len(selection.Candidates) > 1 {
			return selection, selectionError("PROJECT_AMBIGUOUS", "multiple projects found; select one with --project <file>", "", nil)
		}
		explicit = selection.Candidates[0]
	}
	path, err := filepath.Abs(explicit)
	if err != nil {
		return selection, selectionError("PROJECT_ACCESS_FAILED", "cannot resolve project path", explicit, err)
	}
	if !strings.EqualFold(filepath.Ext(path), ".csproj") {
		return selection, selectionError("PROJECT_PATH_INVALID", "--project must name a .csproj file", path, nil)
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return selection, selectionError("PROJECT_NOT_FOUND", "selected project does not exist", path, err)
	}
	if err != nil {
		return selection, selectionError("PROJECT_ACCESS_FAILED", "cannot inspect selected project", path, err)
	}
	if !info.Mode().IsRegular() {
		return selection, selectionError("PROJECT_PATH_INVALID", "selected project must be a regular file", path, nil)
	}
	selection.Project = path
	if len(selection.Candidates) == 0 {
		selection.Candidates = append(selection.Candidates, path)
	}
	return selection, nil
}

func selectionError(code, message, path string, cause error) error {
	if cause != nil {
		message = fmt.Sprintf("%s: %v", message, cause)
	}
	err := diagnostic.New(code, message, cause)
	err.Path = path
	return err
}
