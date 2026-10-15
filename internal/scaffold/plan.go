package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

type FileAction struct {
	// Path is relative to the project directory, never the current directory.
	Path   string `json:"path"`
	Action string `json:"action"`
}

// ProjectPlan is a snapshot, not a reservation of the destination. Planning reads
// filesystem metadata but never creates directories, temporary files, or output.
type ProjectPlan struct {
	Path            string       `json:"path"`
	Name            string       `json:"name"`
	Type            string       `json:"type"`
	ESAPIVersion    string       `json:"esapiVersion"`
	TargetFramework string       `json:"targetFramework"`
	Platform        string       `json:"platform"`
	ESAPIPath       string       `json:"esapiPath"`
	ESAPIPathSource string       `json:"esapiPathSource"`
	Files           []FileAction `json:"files"`

	options Options
	files   []renderedFile
}

func pathError(code, message, path string, cause error) error {
	err := diagnostic.New(code, message, cause)
	err.Path = path
	return err
}

// Plan validates options, renders all templates, and inspects destination types.
// It does not probe write access: actual writes can still fail at execution time.
func Plan(options Options) (*ProjectPlan, error) {
	if err := validateName(options.Name); err != nil {
		return nil, err
	}
	if options.Type != "" && options.Type != "plugin" {
		return nil, diagnostic.New("UNSUPPORTED_PROJECT_TYPE", fmt.Sprintf("unsupported project type %q; choose plugin", options.Type), ErrInvalidOptions)
	}
	version, err := ResolveVersion(options.ESAPIVersion)
	if err != nil {
		return nil, err
	}
	data := projectData{Name: options.Name, VersionConfig: version}
	source := "environment"
	if options.ESAPIPath != "" {
		source = "argument"
		data.ESAPIPath, err = filepath.Abs(options.ESAPIPath)
		if err != nil {
			return nil, diagnostic.New("INVALID_ESAPI_PATH", fmt.Sprintf("invalid ESAPI path: %v", err), ErrInvalidOptions)
		}
		// The MSBuild reference/compiler pipeline can decode percent escapes twice.
		if strings.ContainsRune(data.ESAPIPath, '%') {
			return nil, diagnostic.New("INVALID_ESAPI_PATH", "ESAPI paths containing a percent sign are not supported; choose a DLL directory without it", ErrInvalidOptions)
		}
	}
	files, err := render(data)
	if err != nil {
		return nil, diagnostic.New("SCAFFOLD_RENDER_FAILED", err.Error(), err)
	}
	parent := options.Output
	if parent == "" {
		parent = "."
	}
	target, err := filepath.Abs(filepath.Join(parent, options.Name))
	if err != nil {
		return nil, pathError("OUTPUT_ACCESS_FAILED", "cannot resolve output directory", parent, err)
	}
	plan := &ProjectPlan{
		Path: target, Name: options.Name, Type: "plugin", ESAPIVersion: version.Version,
		TargetFramework: version.TargetFramework, Platform: version.Platform,
		ESAPIPath: data.ESAPIPath, ESAPIPathSource: source,
		Files: make([]FileAction, len(files)), options: options, files: files,
	}
	for i, file := range files {
		plan.Files[i] = FileAction{file.name, "create"}
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		// Find the closest existing ancestor without creating missing directories.
		for ancestor := filepath.Dir(target); ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err == nil {
				if !info.IsDir() {
					return nil, pathError("UNSAFE_DESTINATION", "output parent must be a directory", ancestor, nil)
				}
				break
			}
			if !os.IsNotExist(err) || filepath.Dir(ancestor) == ancestor {
				return nil, pathError("OUTPUT_ACCESS_FAILED", fmt.Sprintf("cannot inspect output parent: %v", err), ancestor, err)
			}
		}
		return plan, nil
	}
	if err != nil {
		return nil, pathError("OUTPUT_ACCESS_FAILED", fmt.Sprintf("cannot inspect project destination: %v", err), target, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, pathError("UNSAFE_DESTINATION", "project destination must be a real directory", target, nil)
	}
	if !options.Force {
		err := diagnostic.New("DESTINATION_EXISTS", fmt.Sprintf("directory %q already exists", target), nil)
		err.Path = target
		err.Hint = "Use --force to overwrite only generated files, or choose another destination."
		return nil, err
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, pathError("OUTPUT_ACCESS_FAILED", err.Error(), target, err)
	}
	defer root.Close()
	for i, file := range files {
		info, err := root.Lstat(file.name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, pathError("OUTPUT_ACCESS_FAILED", err.Error(), filepath.Join(target, file.name), err)
		}
		if !info.Mode().IsRegular() {
			return nil, pathError("UNSAFE_DESTINATION", "refusing to overwrite a link, directory, or special file", filepath.Join(target, file.name), nil)
		}
		plan.Files[i].Action = "overwrite"
	}
	return plan, nil
}

// Apply revalidates the filesystem snapshot before creating any output. It uses
// the original options and private rendered files, not mutable report fields.
func (plan *ProjectPlan) Apply() error {
	current, err := Plan(plan.options)
	if err != nil {
		return err
	}
	if current.Path != plan.Path {
		return diagnostic.New("DESTINATION_CHANGED", "project destination changed after planning; run the command again", nil)
	}
	if err := write(current); err != nil {
		return err
	}
	*plan = *current
	return nil
}
