package detect

import "github.com/wkt84/esapi-cli/internal/diagnostic"

type Check struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type DoctorReport struct {
	Environment        Environment `json:"environment"`
	Checks             []Check     `json:"checks"`
	StaticChecksPassed bool        `json:"staticChecksPassed"`
}

func Diagnose(environment Environment) (DoctorReport, []diagnostic.Diagnostic) {
	report := DoctorReport{Environment: environment, Checks: []Check{}, StaticChecksPassed: true}
	diagnostics := []diagnostic.Diagnostic{}
	add := func(id string, passed bool, code, message, hint, path string) {
		status := "pass"
		if !passed {
			status = "fail"
			report.StaticChecksPassed = false
			diagnostics = append(diagnostics, diagnostic.Diagnostic{Code: code, Severity: "error", Message: message, Hint: hint, Path: path})
		}
		report.Checks = append(report.Checks, Check{id, status, true, message, path})
	}
	observation := func(id string, found bool, message, path string) {
		status := "skip"
		if found {
			status = "pass"
		}
		report.Checks = append(report.Checks, Check{id, status, false, message, path})
	}
	add("os.windows", environment.OS == "windows", "UNSUPPORTED_OS", "Windows is required for the supported ESAPI build environment.", "Run the development-environment checks on Windows; scaffolding and info remain portable.", "")
	add("os.amd64", environment.Arch == "amd64", "UNSUPPORTED_HOST_ARCHITECTURE", "The supported ESAPI development host architecture is amd64.", "Use a Windows amd64 development host.", "")
	observation("tools.dotnet", environment.Dotnet.Status == "found", "dotnet host file inspection: "+environment.Dotnet.Status, environment.Dotnet.Path)
	observation("tools.dotnetSdk", environment.Dotnet.SDKStatus == "found", ".NET SDK MSBuild.dll markers: "+environment.Dotnet.SDKStatus, environment.Dotnet.Path)
	observation("tools.msbuild", environment.MSBuild.Status == "found", "MSBuild executable file inspection: "+environment.MSBuild.Status, environment.MSBuild.Path)
	if environment.Dotnet.Source == "argument" {
		add("tools.explicitDotnet", environment.Dotnet.Status == "found" && environment.Dotnet.SDKStatus == "found", "DOTNET_SDK_UNAVAILABLE", "The explicitly selected dotnet host must have an adjacent SDK containing MSBuild.dll.", "Correct --dotnet-path or install the .NET SDK separately.", environment.Dotnet.Path)
	}
	if environment.MSBuild.Source == "argument" {
		add("tools.explicitMSBuild", environment.MSBuild.Status == "found", "MSBUILD_UNAVAILABLE", "The explicitly selected MSBuild executable must exist.", "Correct --msbuild-path.", environment.MSBuild.Path)
	}
	toolchain := environment.MSBuild.Status == "found" || (environment.Dotnet.Status == "found" && environment.Dotnet.SDKStatus == "found")
	add("tools.toolchain", toolchain, "BUILD_TOOLCHAIN_NOT_FOUND", "An MSBuild executable or dotnet SDK candidate is required.", "Install Visual Studio Build Tools / a .NET SDK, or specify --msbuild-path / --dotnet-path. Multiple MSBuild candidates require explicit selection.", "")
	add("framework.targetingPack", environment.TargetingPack.Status == "found", "TARGETING_PACK_UNAVAILABLE", "The selected "+environment.Profile.TargetFramework+" targeting pack must contain mscorlib.dll and RedistList/FrameworkList.xml.", "Install the matching .NET Framework Developer Pack separately, or use --targeting-pack-path to inspect a custom location. Custom build paths require matching MSBuild properties.", environment.TargetingPack.Path)
	add("esapi.path", environment.ESAPI.Status == "found", "ESAPI_PATH_UNAVAILABLE", "A single accessible ESAPI DLL directory is required (inspection: "+environment.ESAPI.Status+").", "Set --esapi-path or ESAPI_HOME to the directory directly containing both DLLs. Select --esapi-version for known-path discovery.", environment.ESAPI.Path)
	for index, dll := range environment.DLLs {
		id := "esapi.api"
		if index == 1 {
			id = "esapi.types"
		}
		if environment.ESAPI.Status != "found" {
			observation(id, false, "DLL inspection skipped until an ESAPI directory is selected.", dll.Path)
			continue
		}
		message := dll.Name + ": PE DLL and CLR header declaration found."
		if dll.Status != "found" {
			message = dll.Name + ": " + dll.Issue
		}
		add(id, dll.Status == "found", dll.Code, message, "Check the selected directory and installed SDK; do not substitute unrelated DLLs.", dll.Path)
	}
	observation("verification.toolExecution", false, "Not checked: tool binaries are not executed; SDK selection/global.json and actual compilation remain unverified.", "")
	observation("verification.assemblyIdentity", false, "Not checked: assembly identity/version, CLR flags, and compatibility with the selected ESAPI profile.", "")
	observation("verification.eclipseRuntime", false, "Not checked: Eclipse execution, licenses, native dependencies, and clinical validation. DLLs are never loaded.", "")
	return report, diagnostics
}
