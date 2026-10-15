package scaffold

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type projectXML struct {
	XMLName xml.Name
	Groups  []struct {
		Framework string `xml:"TargetFrameworkVersion"`
		Platform  string `xml:"PlatformTarget"`
		Assembly  string `xml:"AssemblyName"`
		Home      string `xml:"ESAPI_HOME"`
	} `xml:"PropertyGroup"`
	Items []struct {
		References []struct {
			Name    string `xml:"Include,attr"`
			Hint    string `xml:"HintPath"`
			Private string `xml:"Private"`
		} `xml:"Reference"`
		Compile []struct {
			Name string `xml:"Include,attr"`
		} `xml:"Compile"`
	} `xml:"ItemGroup"`
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewProject(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "new", "scripts")
	t.Setenv("ESAPI_HOME", filepath.Join(parent, "do-not-capture"))
	path, err := Generate(Options{Name: "MyScript", Output: parent})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(parent, "MyScript") {
		t.Fatalf("unexpected output path: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if want := []string{".gitignore", "MyScript.csproj", "README.md", "Script.cs"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("files = %v; want %v", names, want)
	}
	script := readFile(t, filepath.Join(path, "Script.cs"))
	for _, want := range []string{"namespace VMS.TPS", "public class Script", "public void Execute(ScriptContext context)", "IsWriteable = false"} {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing %q", want)
		}
	}
	content := readFile(t, filepath.Join(path, "MyScript.csproj"))
	var project projectXML
	if err := xml.Unmarshal([]byte(content), &project); err != nil {
		t.Fatal(err)
	}
	if project.XMLName.Local != "Project" || project.XMLName.Space != "http://schemas.microsoft.com/developer/msbuild/2003" {
		t.Fatalf("invalid project root: %v", project.XMLName)
	}
	if group := project.Groups[0]; group.Framework != "v4.5" || group.Platform != "x64" || group.Assembly != "MyScript.esapi" || group.Home != "$([System.Environment]::GetEnvironmentVariable('ESAPI_HOME'))" {
		t.Fatalf("unexpected project settings: %+v", group)
	}
	apiReferences, compileItems := 0, 0
	for _, group := range project.Items {
		for _, ref := range group.References {
			if strings.HasPrefix(ref.Name, "VMS.TPS.Common.Model.") {
				apiReferences++
				if ref.Private != "False" || ref.Hint != "$(ESAPI_HOME)\\"+ref.Name+".dll" {
					t.Errorf("incorrect external DLL reference: %+v", ref)
				}
			}
		}
		for _, item := range group.Compile {
			if item.Name == "Script.cs" {
				compileItems++
			}
		}
	}
	if apiReferences != 2 || compileItems != 1 {
		t.Fatalf("references = %d; Script.cs items = %d", apiReferences, compileItems)
	}
	readme := readFile(t, filepath.Join(path, "README.md"))
	for _, want := range []string{"MyScript.esapi.dll", "provisional", "NOT FOR CLINICAL USE", "not affiliated with or endorsed by"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README is missing %q", want)
		}
	}
	if strings.Contains(content, "{{") || strings.Contains(readme, "{{") || strings.Contains(content, "do-not-capture") {
		t.Error("unrendered template or captured environment value")
	}
}

func TestVersionProfiles(t *testing.T) {
	for _, item := range []struct{ version, framework string }{
		{"15.6", "v4.5"}, {"16.0", "v4.6.1"}, {"16.1", "v4.6.1"},
		{"17.0", "v4.8"}, {"18.0", "v4.8"}, {"18", "v4.8"},
	} {
		t.Run(item.version, func(t *testing.T) {
			path, err := Generate(Options{Name: "Example", Output: t.TempDir(), ESAPIVersion: item.version})
			if err != nil {
				t.Fatal(err)
			}
			content := readFile(t, filepath.Join(path, "Example.csproj"))
			if !strings.Contains(content, "<TargetFrameworkVersion>"+item.framework+"</TargetFrameworkVersion>") {
				t.Fatal("incorrect target framework")
			}
		})
	}
}

func TestInvalidOptionsDoNotCreateOutput(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../Outside", `..\Outside`, "A/B", `A\B`, "A B", "A.B", "1Script", "A:stream", "CON", "nul", "Com1", "LPT9", "日本語", strings.Repeat("A", 101)} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "not-created")
			_, err := Generate(Options{Name: name, Output: parent})
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("expected invalid options, got %v", err)
			}
			if _, err := os.Stat(parent); !os.IsNotExist(err) {
				t.Error("invalid name changed the filesystem")
			}
		})
	}
	for _, options := range []Options{
		{Name: "Example", Type: "standalone"},
		{Name: "Example", ESAPIVersion: "15.5"},
		{Name: "Example", ESAPIVersion: "19.0"},
		{Name: "Example", ESAPIVersion: "auto"},
		{Name: "Example", ESAPIPath: "sdk%25"},
	} {
		options.Output = filepath.Join(t.TempDir(), "not-created")
		if _, err := Generate(options); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("expected invalid options, got %v", err)
		}
		if _, err := os.Stat(options.Output); !os.IsNotExist(err) {
			t.Error("invalid options changed the filesystem")
		}
	}
}

func TestExistingDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "Example")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{Name: "Example", Output: parent}); err == nil {
		t.Fatal("expected an error even for an empty existing directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("existing directory was changed: %v, %v", entries, err)
	}
}

func TestForceOverwritePreservesUnknownFiles(t *testing.T) {
	options := Options{Name: "Example", Output: t.TempDir()}
	path, err := Generate(options)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(path, "Script.cs"), "old source")
	writeFile(t, filepath.Join(path, "notes.txt"), "keep me")
	options.Force = true
	options.ESAPIVersion = "18"
	if _, err := Generate(options); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(path, "notes.txt")); got != "keep me" {
		t.Fatal("unknown file was changed")
	}
	if got := readFile(t, filepath.Join(path, "Script.cs")); got == "old source" {
		t.Fatal("generated file was not overwritten")
	}
	if got := readFile(t, filepath.Join(path, "Example.csproj")); !strings.Contains(got, "<ESAPIVersion>18.0</ESAPIVersion>") {
		t.Fatal("project settings were not overwritten")
	}
	entries, _ := os.ReadDir(path)
	if len(entries) != 5 {
		t.Fatalf("unexpected files or leftover temporary files: %v", entries)
	}
}

func TestForcePreflightDoesNotPartiallyOverwrite(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "Example")
	if err := os.MkdirAll(filepath.Join(path, "README.md"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(path, "Example.csproj"), "keep project")
	writeFile(t, filepath.Join(path, "Script.cs"), "keep source")
	if _, err := Generate(Options{Name: "Example", Output: parent, Force: true}); err == nil {
		t.Fatal("expected directory collision error")
	}
	if readFile(t, filepath.Join(path, "Example.csproj")) != "keep project" || readFile(t, filepath.Join(path, "Script.cs")) != "keep source" {
		t.Fatal("files were overwritten before all destinations were checked")
	}
	if _, err := os.Stat(filepath.Join(path, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("partial output was created")
	}
}

func TestForceRejectsSymlinks(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			parent := t.TempDir()
			outside := t.TempDir()
			writeFile(t, filepath.Join(outside, "README.md"), "outside")
			link, target := filepath.Join(parent, "Example"), outside
			if !directory {
				if err := os.Mkdir(link, 0755); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, "README.md")
				target = filepath.Join(outside, "README.md")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			if _, err := Generate(Options{Name: "Example", Output: parent, Force: true}); err == nil {
				t.Fatal("expected symlink error")
			}
			if readFile(t, filepath.Join(outside, "README.md")) != "outside" {
				t.Fatal("symlink target was overwritten")
			}
		})
	}
}

func TestForceDoesNotWriteThroughHardLinks(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "Example")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "source.cs")
	writeFile(t, outside, "keep outside")
	if err := os.Link(outside, filepath.Join(path, "Script.cs")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := Generate(Options{Name: "Example", Output: parent, Force: true}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, outside) != "keep outside" {
		t.Fatal("hard-linked file outside the project was overwritten")
	}
}

func TestExplicitESAPIPathIsEscapedAndDLLsAreNotCopied(t *testing.T) {
	parent := t.TempDir()
	sdk := filepath.Join(parent, "sdk & '$(Property);@value")
	if err := os.Mkdir(sdk, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"VMS.TPS.Common.Model.API.dll", "VMS.TPS.Common.Model.Types.dll"} {
		writeFile(t, filepath.Join(sdk, name), "test-only placeholder; not an ESAPI DLL")
	}
	path, err := Generate(Options{Name: "Example", Output: parent, ESAPIPath: sdk})
	if err != nil {
		t.Fatal(err)
	}
	content := readFile(t, filepath.Join(path, "Example.csproj"))
	var project projectXML
	if err := xml.Unmarshal([]byte(content), &project); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	home := project.Groups[0].Home
	if strings.Contains(home, "$(Property)") || !strings.Contains(home, "%24%28Property%29") || !strings.Contains(home, "sdk & %27") {
		t.Fatalf("path is not escaped for MSBuild: %s", home)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 4 {
		t.Fatalf("DLLs copied or unexpected output: %v %v", entries, err)
	}
}
