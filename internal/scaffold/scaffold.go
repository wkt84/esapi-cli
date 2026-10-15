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

// Generate creates a project without requiring an ESAPI installation.
func Generate(options Options) (string, error) {
	plan, err := Plan(options)
	if err != nil {
		return "", err
	}
	if err := plan.Apply(); err != nil {
		return "", err
	}
	return plan.Path, nil
}

func write(plan *ProjectPlan) error {
	target, options, files := plan.Path, plan.options, plan.files
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return pathError("SCAFFOLD_WRITE_FAILED", fmt.Sprintf("create output directory: %v", err), target, err)
	}
	created := false
	if err := os.Mkdir(target, 0755); err == nil {
		created = true
	} else if !os.IsExist(err) {
		return pathError("SCAFFOLD_WRITE_FAILED", fmt.Sprintf("create project directory: %v", err), target, err)
	} else {
		if !options.Force {
			return pathError("DESTINATION_CHANGED", "destination appeared after planning; run the command again", target, nil)
		}
		info, err := os.Lstat(target)
		if err != nil {
			return pathError("OUTPUT_ACCESS_FAILED", err.Error(), target, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return pathError("UNSAFE_DESTINATION", "project destination must be a real directory", target, nil)
		}
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		if created {
			_ = os.Remove(target) // Only removes an empty directory.
		}
		return pathError("OUTPUT_ACCESS_FAILED", err.Error(), target, err)
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
		return pathError("SCAFFOLD_WRITE_FAILED", fmt.Sprintf("generate project: %v", err), target, err)
	}
	return nil
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
