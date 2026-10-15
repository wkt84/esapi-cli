package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

// Independent fixture, not an embedded generation template. DLL placeholders
// establish existence only; the checker must never attempt to load assemblies.
const fixture = `<Project xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
 <PropertyGroup>
  <ESAPIVersion>15.6</ESAPIVersion>
  <TargetFrameworkVersion>v4.5</TargetFrameworkVersion>
  <PlatformTarget>x64</PlatformTarget><OutputType>Library</OutputType>
  <AssemblyName>Independent.esapi</AssemblyName>
  <ESAPI_HOME>$(MSBuildProjectDirectory)\sdk</ESAPI_HOME>
 </PropertyGroup>
 <ItemGroup>
  <Reference Include="VMS.TPS.Common.Model.API"><HintPath>$(ESAPI_HOME)\VMS.TPS.Common.Model.API.dll</HintPath><Private>False</Private></Reference>
  <Reference Include="VMS.TPS.Common.Model.Types"><HintPath>$(ESAPI_HOME)\VMS.TPS.Common.Model.Types.dll</HintPath><Private>False</Private></Reference>
  <Compile Include="Source.cs" />
 </ItemGroup>
 <Import Project="$(MSBuildToolsPath)\Microsoft.CSharp.targets" />
</Project>`

func setup(t *testing.T, content string) Selection {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sdk"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"Independent.csproj", "Source.cs", "sdk/VMS.TPS.Common.Model.API.dll", "sdk/VMS.TPS.Common.Model.Types.dll"} {
		data := []byte("independent placeholder; not executable")
		if path == "Independent.csproj" {
			data = []byte(content)
		}
		if err := os.WriteFile(filepath.Join(dir, path), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	selection, err := Select(filepath.Join(dir, "Independent.csproj"))
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func hasCode(diagnostics []diagnostic.Diagnostic, code string) bool {
	for _, d := range diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestIndependentProjectIsReadOnlyAndDeterministic(t *testing.T) {
	selection := setup(t, fixture)
	t.Setenv("ESAPI_HOME", "must-not-override-project-property")
	snapshot := func() map[string]string {
		files := map[string]string{}
		err := filepath.WalkDir(filepath.Dir(selection.Project), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = string(data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	var first string
	for n := 0; n < 2; n++ {
		report, diagnostics, err := Inspect(selection, Options{})
		if err != nil || !report.StaticChecksPassed || len(diagnostics) != 0 || len(report.References) != 2 || len(report.Files) != 1 {
			t.Fatalf("unexpected inspection: %+v / %+v / %v", report, diagnostics, err)
		}
		if report.Checks[len(report.Checks)-1].Status != "skip" || report.Checks[len(report.Checks)-1].Required {
			t.Fatal("runtime verification claimed or required")
		}
		data, _ := json.Marshal(report)
		if n > 0 && string(data) != first {
			t.Fatal("inspection is not deterministic")
		}
		first = string(data)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("inspection changed project files")
	}
}

func TestInvalidAndUnevaluableDeclarations(t *testing.T) {
	for _, item := range []struct{ name, before, after, code string }{
		{"framework", "v4.5", "v4.8", "PROJECT_FRAMEWORK_MISMATCH"},
		{"platform", ">x64<", ">AnyCPU<", "PROJECT_PLATFORM_INVALID"},
		{"assembly", "Independent.esapi", "Independent.dll", "PROJECT_ASSEMBLY_NAME_INVALID"},
		{"output", ">Library<", ">Exe<", "PROJECT_OUTPUT_TYPE_INVALID"},
		{"copy-local", ">False<", ">True<", "PROJECT_REFERENCE_COPY_LOCAL"},
		{"version-missing", "<ESAPIVersion>15.6</ESAPIVersion>", "", "PROJECT_ESAPI_VERSION_REQUIRED"},
		{"version-unsupported", ">15.6<", ">20.0<", "PROJECT_ESAPI_VERSION_UNSUPPORTED"},
		{"conditional", "<PlatformTarget>", `<PlatformTarget Condition="'$(Configuration)' == 'Debug'">`, "PROJECT_EVALUATION_REQUIRED"},
		{"import", `$(MSBuildToolsPath)\Microsoft.CSharp.targets`, "custom.targets", "PROJECT_EVALUATION_REQUIRED"},
		{"expression", ">x64<", ">$(SomePlatform)<", "PROJECT_EVALUATION_REQUIRED"},
		{"glob", `Include="Source.cs"`, `Include="**\*.cs"`, "PROJECT_EVALUATION_REQUIRED"},
		{"missing-source", "Source.cs", "Missing.cs", "PROJECT_SOURCE_UNAVAILABLE"},
		{"missing-api", `$(ESAPI_HOME)\VMS.TPS.Common.Model.API.dll`, `$(ESAPI_HOME)\Missing.dll`, "PROJECT_REFERENCE_UNAVAILABLE"},
		{"wrong-dll", `$(ESAPI_HOME)\VMS.TPS.Common.Model.API.dll`, `$(ESAPI_HOME)\VMS.TPS.Common.Model.Types.dll`, "PROJECT_REFERENCE_PATH_INVALID"},
		{"quoted-sdk", `$(MSBuildProjectDirectory)\sdk`, `$(MSBuildProjectDirectory)\sdk&quot;`, "PROJECT_REFERENCE_PATH_INVALID"},
		{"unresolved-path", `$(ESAPI_HOME)\VMS.TPS.Common.Model.API.dll`, `$(Unknown)\API.dll`, "PROJECT_EVALUATION_REQUIRED"},
		{"missing-reference", `Include="VMS.TPS.Common.Model.API"`, `Include="Other.API"`, "PROJECT_REFERENCE_INVALID"},
		{"missing-compile", `<Compile Include="Source.cs" />`, "", "PROJECT_SOURCE_REQUIRED"},
		{"foreign-namespace", "<PlatformTarget>", `<PlatformTarget xmlns="other">`, "PROJECT_EVALUATION_REQUIRED"},
		{"malformed", "</Project>", "", "PROJECT_XML_INVALID"},
		{"trailing-root", "</Project>", "</Project><Project/>", "PROJECT_XML_INVALID"},
		{"leading-text", "<Project xmlns=", "unexpected text<Project xmlns=", "PROJECT_XML_INVALID"},
		{"sdk", "<Project xmlns=", `<Project Sdk="Microsoft.NET.Sdk" xmlns=`, "PROJECT_FORMAT_UNSUPPORTED"},
	} {
		t.Run(item.name, func(t *testing.T) {
			selection := setup(t, strings.Replace(fixture, item.before, item.after, 1))
			report, diagnostics, err := Inspect(selection, Options{})
			if err != nil || report.StaticChecksPassed || !hasCode(diagnostics, item.code) {
				t.Fatalf("expected %s: %+v / %v", item.code, diagnostics, err)
			}
			if item.code == "PROJECT_EVALUATION_REQUIRED" {
				found := false
				for _, check := range report.Checks {
					found = found || check.Status == "skip" && check.Required
				}
				if !found {
					t.Fatal("unevaluated requirement not reported as required skip")
				}
			}
		})
	}
}

func TestXMLBoundariesAndAccessFailure(t *testing.T) {
	for _, content := range []string{fixture + "<!-- valid trailing comment -->", "\ufeff" + fixture} {
		selection := setup(t, content)
		report, diagnostics, err := Inspect(selection, Options{})
		if err != nil || !report.StaticChecksPassed || len(diagnostics) != 0 {
			t.Fatalf("valid XML rejected: %+v / %v", diagnostics, err)
		}
	}
	selection := setup(t, strings.Repeat(" ", maxProjectBytes+1))
	report, diagnostics, err := Inspect(selection, Options{})
	if err != nil || report.StaticChecksPassed || !hasCode(diagnostics, "PROJECT_XML_INVALID") {
		t.Fatalf("oversized input accepted: %+v / %v", diagnostics, err)
	}
	if err := os.Remove(selection.Project); err != nil {
		t.Fatal(err)
	}
	report, _, err = Inspect(selection, Options{})
	if err == nil || report.StaticChecksPassed {
		t.Fatal("I/O failure reported as successful inspection")
	}
}

func TestExplicitProfileAndPathPrecedence(t *testing.T) {
	malformed := setup(t, strings.Replace(fixture, `$(MSBuildProjectDirectory)\sdk`, `$(MSBuildProjectDirectory)\sdk&quot;`, 1))
	fixed, details, accessErr := Inspect(malformed, Options{ESAPIPath: filepath.Join(filepath.Dir(malformed.Project), "sdk")})
	if accessErr != nil || !fixed.StaticChecksPassed || len(details) != 0 {
		t.Fatalf("explicit valid override did not repair malformed project binding: %+v / %v", details, accessErr)
	}
	selection := setup(t, strings.Replace(fixture, "<ESAPIVersion>15.6</ESAPIVersion>", "", 1))
	report, diagnostics, err := Inspect(selection, Options{ESAPIVersion: "15.6"})
	if err != nil || !report.StaticChecksPassed || len(diagnostics) != 0 || report.Properties.ESAPIVersionSource != "argument" {
		t.Fatalf("explicit profile not used: %+v / %v", diagnostics, err)
	}
	selection = setup(t, fixture)
	_, diagnostics, err = Inspect(selection, Options{ESAPIVersion: "19.0", ESAPIPath: filepath.Join(t.TempDir(), "missing")})
	if err != nil || !hasCode(diagnostics, "PROJECT_ESAPI_VERSION_MISMATCH") || !hasCode(diagnostics, "PROJECT_REFERENCE_UNAVAILABLE") {
		t.Fatalf("explicit selections ignored: %+v / %v", diagnostics, err)
	}
}

func TestProjectSelection(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	selection, err := Select("")
	if err == nil || len(selection.Candidates) != 0 {
		t.Fatal("missing project accepted")
	}
	for _, name := range []string{"B.csproj", "A.CSPROJ"} {
		if err := os.WriteFile(name, []byte("<Project/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	selection, err = Select("")
	if err == nil || selection.Project != "" || len(selection.Candidates) != 2 || filepath.Base(selection.Candidates[0]) != "A.CSPROJ" {
		t.Fatalf("ambiguous project selected or unordered: %+v / %v", selection, err)
	}
	selection, err = Select("B.csproj")
	if err != nil || selection.Project != filepath.Join(dir, "B.csproj") {
		t.Fatalf("explicit selection failed: %+v / %v", selection, err)
	}
	if err := os.Remove("A.CSPROJ"); err != nil {
		t.Fatal(err)
	}
	selection, err = Select("")
	if err != nil || filepath.Base(selection.Project) != "B.csproj" {
		t.Fatal("single project not selected")
	}
	for _, path := range []string{"missing.csproj", dir, "Source.cs"} {
		if _, err := Select(path); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
}

func TestSymlinkNeedsExplicitSelection(t *testing.T) {
	selection := setup(t, fixture)
	t.Chdir(t.TempDir())
	if err := os.Symlink(selection.Project, "Linked.csproj"); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if selected, err := Select(""); err == nil || len(selected.Candidates) != 0 {
		t.Fatal("discovery followed a symlink")
	}
	if selected, err := Select("Linked.csproj"); err != nil || filepath.Base(selected.Project) != "Linked.csproj" {
		t.Fatalf("explicit symlink was not followed: %+v / %v", selected, err)
	}
}
