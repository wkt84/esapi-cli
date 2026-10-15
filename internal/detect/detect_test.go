package detect

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wkt84/esapi-cli/internal/scaffold"
)

func file(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

// These are independently constructed PE headers, not vendor SDK assemblies.
// They exercise header declarations only and intentionally contain no CLR metadata.
func managedPE(t *testing.T, path string, x64, dll, managed bool) {
	t.Helper()
	var optional bytes.Buffer
	if x64 {
		header := pe.OptionalHeader64{Magic: 0x20b, NumberOfRvaAndSizes: 16}
		if managed {
			header.DataDirectory[14] = pe.DataDirectory{VirtualAddress: 0x2000, Size: 72}
		}
		if err := binary.Write(&optional, binary.LittleEndian, header); err != nil {
			t.Fatal(err)
		}
	} else {
		header := pe.OptionalHeader32{Magic: 0x10b, NumberOfRvaAndSizes: 16}
		if managed {
			header.DataDirectory[14] = pe.DataDirectory{VirtualAddress: 0x2000, Size: 72}
		}
		if err := binary.Write(&optional, binary.LittleEndian, header); err != nil {
			t.Fatal(err)
		}
	}
	header := pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_I386, SizeOfOptionalHeader: uint16(optional.Len()), Characteristics: pe.IMAGE_FILE_EXECUTABLE_IMAGE}
	if x64 {
		header.Machine = pe.IMAGE_FILE_MACHINE_AMD64
	}
	if dll {
		header.Characteristics |= pe.IMAGE_FILE_DLL
	}
	dos := make([]byte, 128)
	copy(dos, "MZ")
	binary.LittleEndian.PutUint32(dos[60:64], uint32(len(dos)))
	var image bytes.Buffer
	image.Write(dos)
	image.WriteString("PE\x00\x00")
	if err := binary.Write(&image, binary.LittleEndian, header); err != nil {
		t.Fatal(err)
	}
	image.Write(optional.Bytes())
	file(t, path, image.String())
}

func isolatedSystem() System {
	return System{
		OS: "windows", Arch: "amd64", Getenv: func(string) string { return "" },
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
}

func fixture(t *testing.T) (System, scaffold.VersionConfig, Options) {
	t.Helper()
	root := t.TempDir()
	options := Options{
		ESAPIPath:         filepath.Join(root, "sdk"),
		MSBuildPath:       file(t, filepath.Join(root, "tools", "MSBuild.exe"), "not executable code; presence-only fixture"),
		TargetingPackPath: filepath.Join(root, "pack"),
	}
	managedPE(t, filepath.Join(options.ESAPIPath, "VMS.TPS.Common.Model.API.dll"), false, true, true)
	managedPE(t, filepath.Join(options.ESAPIPath, "VMS.TPS.Common.Model.Types.dll"), true, true, true)
	file(t, filepath.Join(options.TargetingPackPath, "mscorlib.dll"), "test-only pack marker")
	file(t, filepath.Join(options.TargetingPackPath, "RedistList", "FrameworkList.xml"), "test-only pack marker")
	profile, err := scaffold.ResolveVersion("18")
	if err != nil {
		t.Fatal(err)
	}
	return isolatedSystem(), profile, options
}

func snapshot(t *testing.T, directory string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[path] = string(content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStaticChecksWithAnyCPUCompatibleHeader(t *testing.T) {
	system, profile, options := fixture(t)
	root := filepath.Dir(options.ESAPIPath)
	before := snapshot(t, root)
	environment := system.Inspect(profile, options)
	report, diagnostics := Diagnose(environment)
	if !report.StaticChecksPassed || len(diagnostics) != 0 {
		t.Fatalf("expected static fixture checks to pass: %+v", diagnostics)
	}
	if environment.DLLs[0].PE.Machine != "i386" || environment.DLLs[0].PE.Format != "PE32" || environment.DLLs[0].Status != "found" {
		t.Fatalf("I386 header was incorrectly rejected as an x86-only assembly: %+v", environment.DLLs[0])
	}
	for _, check := range report.Checks {
		if check.ID == "verification.eclipseRuntime" || check.ID == "verification.assemblyIdentity" || check.ID == "verification.toolExecution" {
			if check.Status != "skip" || check.Required {
				t.Fatalf("unperformed validation was reported as passed: %+v", check)
			}
		}
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("inspection mutated the filesystem")
	}
}

func TestDotnetSDKFallbackAndRuntimeOnlyFailure(t *testing.T) {
	system, profile, options := fixture(t)
	options.MSBuildPath = ""
	host := file(t, filepath.Join(t.TempDir(), "dotnet.exe"), "test-only host marker")
	system.LookPath = func(name string) (string, error) {
		if name == "dotnet.exe" {
			return host, nil
		}
		return "", os.ErrNotExist
	}
	report, diagnostics := Diagnose(system.Inspect(profile, options))
	if report.StaticChecksPassed || len(diagnostics) != 1 || diagnostics[0].Code != "BUILD_TOOLCHAIN_NOT_FOUND" {
		t.Fatalf("runtime-only host was treated as an SDK: %+v", diagnostics)
	}
	file(t, filepath.Join(filepath.Dir(host), "sdk", "9.0.0", "MSBuild.dll"), "test-only SDK marker")
	environment := system.Inspect(profile, options)
	report, diagnostics = Diagnose(environment)
	if !report.StaticChecksPassed || len(diagnostics) != 0 || !reflect.DeepEqual(environment.Dotnet.SDKs, []string{"9.0.0"}) {
		t.Fatalf("SDK fallback not detected: %+v %+v", environment.Dotnet, diagnostics)
	}
	options.MSBuildPath = filepath.Join(t.TempDir(), "missing.exe")
	report, diagnostics = Diagnose(system.Inspect(profile, options))
	if report.StaticChecksPassed || len(diagnostics) != 1 || diagnostics[0].Code != "MSBUILD_UNAVAILABLE" {
		t.Fatal("invalid explicit tool path was silently ignored")
	}
}

func TestESAPIPathPrecedenceAndAmbiguity(t *testing.T) {
	system := isolatedSystem()
	root1, root2 := t.TempDir(), t.TempDir()
	known1 := filepath.Join(root1, "Varian", "RTM", "18.0", "esapi", "API")
	known2 := filepath.Join(root2, "Varian", "RTM", "18.0", "esapi", "API")
	if err := os.MkdirAll(known1, 0755); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"ProgramFiles(x86)": root1}
	system.Getenv = func(name string) string { return values[name] }
	if fact := system.esapi("18.0", ""); fact.Status != "found" || fact.Source != "discovery" || fact.Path != known1 {
		t.Fatalf("known path not detected: %+v", fact)
	}
	values["ESAPI_HOME"] = filepath.Join(root1, "missing")
	if fact := system.esapi("18.0", ""); fact.Source != "environment" || fact.Status != "missing" {
		t.Fatalf("invalid environment path silently fell back to discovery: %+v", fact)
	}
	if fact := system.esapi("18.0", known1); fact.Source != "argument" || fact.Status != "found" {
		t.Fatalf("explicit path did not override environment: %+v", fact)
	}
	delete(values, "ESAPI_HOME")
	values["ProgramFiles"] = root2
	if err := os.MkdirAll(known2, 0755); err != nil {
		t.Fatal(err)
	}
	if fact := system.esapi("18.0", ""); fact.Status != "ambiguous" || fact.Path != "" || len(fact.Candidates) != 2 {
		t.Fatalf("multiple installations were silently selected: %+v", fact)
	}
	if fact := system.esapi("15.6", ""); fact.Status != "missing" {
		t.Fatal("discovery ignored the selected generation profile")
	}
}

func TestMSBuildDiscoveryPrefersX64WithinInstallation(t *testing.T) {
	system := isolatedSystem()
	root := t.TempDir()
	system.Getenv = func(name string) string {
		if name == "ProgramFiles" {
			return root
		}
		return ""
	}
	bin := filepath.Join(root, "Microsoft Visual Studio", "2022", "BuildTools", "MSBuild", "Current", "Bin")
	file(t, filepath.Join(bin, "MSBuild.exe"), "marker")
	x64 := file(t, filepath.Join(bin, "amd64", "MSBuild.exe"), "marker")
	if fact := system.msbuild(""); fact.Status != "found" || fact.Path != x64 {
		t.Fatalf("64-bit installation discovery failed: %+v", fact)
	}
	other := file(t, filepath.Join(root, "Microsoft Visual Studio", "18", "Community", "MSBuild", "Current", "Bin", "MSBuild.exe"), "marker")
	if fact := system.msbuild(""); fact.Status != "ambiguous" || len(fact.Candidates) != 2 {
		t.Fatalf("multiple MSBuild installations were silently selected: %+v", fact)
	}
	system.LookPath = func(name string) (string, error) {
		if name == "MSBuild.exe" {
			return other, nil
		}
		return "", os.ErrNotExist
	}
	if fact := system.msbuild(""); fact.Source != "path" || fact.Path != other {
		t.Fatal("PATH did not take precedence over discovery")
	}
}

func TestLinkedDotnetHostAndNonWindowsFacts(t *testing.T) {
	system := isolatedSystem()
	system.OS = "linux"
	host := file(t, filepath.Join(t.TempDir(), "dotnet", "dotnet"), "test-only host marker")
	file(t, filepath.Join(filepath.Dir(host), "sdk", "9.0.0", "MSBuild.dll"), "test-only SDK marker")
	link := filepath.Join(t.TempDir(), "dotnet")
	if err := os.Symlink(host, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	system.LookPath = func(name string) (string, error) {
		if name == "dotnet" {
			return link, nil
		}
		return "", os.ErrNotExist
	}
	if fact := system.dotnet(""); fact.SDKStatus != "found" || fact.Path != link {
		t.Fatalf("SDKs were not located beside linked host: %+v", fact)
	}
	if pack := system.targetingPack("v4.8", ""); pack.Status != "unverified" {
		t.Fatal("Windows-only detection was reported as checked on another OS")
	}
}

func TestDLLFailuresAndTargetingPackMarkers(t *testing.T) {
	for _, kind := range []string{"missing", "text", "native", "executable", "directory"} {
		t.Run(kind, func(t *testing.T) {
			system, profile, options := fixture(t)
			path := filepath.Join(options.ESAPIPath, "VMS.TPS.Common.Model.API.dll")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "text":
				file(t, path, "invalid PE data")
			case "native":
				managedPE(t, path, true, true, false)
			case "executable":
				managedPE(t, path, true, false, true)
			case "directory":
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
			}
			report, diagnostics := Diagnose(system.Inspect(profile, options))
			want := "ESAPI_API_DLL_INVALID"
			if kind == "missing" {
				want = "ESAPI_API_DLL_NOT_FOUND"
			}
			if report.StaticChecksPassed || len(diagnostics) != 1 || diagnostics[0].Code != want {
				t.Fatalf("incorrect DLL diagnosis: %+v", diagnostics)
			}
		})
	}
	system, profile, options := fixture(t)
	if err := os.Remove(filepath.Join(options.TargetingPackPath, "mscorlib.dll")); err != nil {
		t.Fatal(err)
	}
	report, diagnostics := Diagnose(system.Inspect(profile, options))
	if report.StaticChecksPassed || diagnostics[0].Code != "TARGETING_PACK_UNAVAILABLE" || !reflect.DeepEqual(report.Environment.TargetingPack.MissingFiles, []string{"mscorlib.dll"}) {
		t.Fatalf("empty targeting-pack directory was treated as installed: %+v", report.Environment.TargetingPack)
	}
}
