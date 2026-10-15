package scaffold

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*
var templateFiles embed.FS

type projectData struct {
	Name string
	VersionConfig
	ESAPIPath string
}

type renderedFile struct {
	name string
	data []byte
}

// Generate creates a project without requiring an ESAPI installation. The DLL
// references are checked by MSBuild when the generated project is built.
func Generate(options Options) (string, error) {
	if err := validateName(options.Name); err != nil {
		return "", err
	}
	if options.Type != "" && options.Type != "plugin" {
		return "", fmt.Errorf("%w: unsupported project type %q; choose plugin", ErrInvalidOptions, options.Type)
	}
	version, err := ResolveVersion(options.ESAPIVersion)
	if err != nil {
		return "", err
	}
	data := projectData{Name: options.Name, VersionConfig: version}
	if options.ESAPIPath != "" {
		data.ESAPIPath, err = filepath.Abs(options.ESAPIPath)
		if err != nil {
			return "", fmt.Errorf("%w: invalid ESAPI path: %v", ErrInvalidOptions, err)
		}
		// MSBuild's reference/compiler pipeline can unescape percent sequences
		// more than once. Refuse these paths instead of referring to a wrong DLL.
		if strings.ContainsRune(data.ESAPIPath, '%') {
			return "", fmt.Errorf("%w: ESAPI paths containing a percent sign are not supported; choose a DLL directory without it", ErrInvalidOptions)
		}
	}
	files, err := render(data)
	if err != nil {
		return "", err
	}
	parent := options.Output
	if parent == "" {
		parent = "."
	}
	target, err := filepath.Abs(filepath.Join(parent, options.Name))
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	created := false
	if err := os.Mkdir(target, 0755); err == nil {
		created = true
	} else if !os.IsExist(err) {
		return "", fmt.Errorf("create project directory: %w", err)
	} else {
		if !options.Force {
			return "", fmt.Errorf("directory %q already exists; use --force to overwrite generated files", target)
		}
		info, err := os.Lstat(target)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("project destination %q must be a real directory", target)
		}
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		if created {
			_ = os.Remove(target) // Only removes an empty directory.
		}
		return "", err
	}
	err = writeProject(root, files, created)
	closeErr := root.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		if created {
			_ = os.Remove(target) // Never recursively remove unknown files.
		}
		return "", fmt.Errorf("generate project: %w", err)
	}
	return target, nil
}

func render(data projectData) ([]renderedFile, error) {
	entries := []struct{ source, target string }{
		{"project.csproj.tmpl", data.Name + ".csproj"},
		{"Script.cs.tmpl", "Script.cs"},
		{"gitignore.tmpl", ".gitignore"},
		{"README.md.tmpl", "README.md"},
	}
	var files []renderedFile
	for _, entry := range entries {
		source, err := templateFiles.ReadFile("templates/" + entry.source)
		if err != nil {
			return nil, err
		}
		tmpl, err := template.New(entry.source).Option("missingkey=error").Funcs(template.FuncMap{
			"msbuild": escapeMSBuildXML,
		}).Parse(string(source))
		if err != nil {
			return nil, err
		}
		var content bytes.Buffer
		if err := tmpl.Execute(&content, data); err != nil {
			return nil, err
		}
		files = append(files, renderedFile{entry.target, content.Bytes()})
	}
	return files, nil
}

func escapeMSBuildXML(value string) string {
	var escaped strings.Builder
	for _, char := range value {
		if strings.ContainsRune("%$@;'()*?", char) {
			fmt.Fprintf(&escaped, "%%%02X", char)
		} else {
			escaped.WriteRune(char)
		}
	}
	var content bytes.Buffer
	_ = xml.EscapeText(&content, []byte(escaped.String()))
	return content.String()
}

func writeProject(root *os.Root, files []renderedFile, created bool) (err error) {
	// Check every destination before replacing any file. Refuse links and special
	// files even with --force. Renaming temporary files also avoids writing through
	// an existing hard link to an unrelated file.
	for _, file := range files {
		info, err := root.Lstat(file.name)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (!info.Mode().IsRegular() || created) {
			return fmt.Errorf("refusing to overwrite %q: destination is not an eligible regular file", file.name)
		}
	}
	var temporary, written []string
	defer func() {
		for _, name := range temporary {
			_ = root.Remove(name)
		}
		if err != nil && created {
			for _, name := range written {
				_ = root.Remove(name)
			}
		}
	}()
	for _, file := range files {
		name := ".esapi-" + rand.Text() + ".tmp"
		stream, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		temporary = append(temporary, name)
		_, writeErr := stream.Write(file.data)
		closeErr := stream.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	for i, file := range files {
		if err := root.Rename(temporary[i], file.name); err != nil {
			return err
		}
		written = append(written, file.name)
	}
	return nil
}
