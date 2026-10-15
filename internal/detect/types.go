// Package detect inspects local environment metadata without executing tools or
// loading assemblies. Presence of files is not proof of build/runtime compatibility.
package detect

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/wkt84/esapi-cli/internal/scaffold"
)

type Options struct {
	ESAPIPath         string
	MSBuildPath       string
	DotnetPath        string
	TargetingPackPath string
}

// System isolates OS/environment/PATH selection for cross-platform fixture tests.
// Filesystem inspection uses real files and never executes the discovered paths.
type System struct {
	OS       string
	Arch     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
}

func CurrentSystem() System {
	return System{runtime.GOOS, runtime.GOARCH, os.Getenv, exec.LookPath}
}

type PathFact struct {
	Path       string   `json:"path"`
	Source     string   `json:"source"`
	Status     string   `json:"status"`
	Candidates []string `json:"candidates"`
	Issue      string   `json:"issue,omitempty"`
}

type ToolFact struct {
	PathFact
	SDKs      []string `json:"sdks"`
	SDKStatus string   `json:"sdkStatus"`
}

type PackFact struct {
	PathFact
	MissingFiles []string `json:"missingFiles"`
}

type PEInfo struct {
	Machine      string `json:"machine"`
	Format       string `json:"format"`
	IsDLL        bool   `json:"isDll"`
	HasCLRHeader bool   `json:"hasClrHeader"`
}

type DLLFact struct {
	Name   string  `json:"name"`
	Path   string  `json:"path"`
	Status string  `json:"status"`
	Code   string  `json:"code,omitempty"`
	Issue  string  `json:"issue,omitempty"`
	PE     *PEInfo `json:"pe"`
}

type Environment struct {
	OS            string                 `json:"os"`
	Arch          string                 `json:"arch"`
	Profile       scaffold.VersionConfig `json:"profile"`
	Dotnet        ToolFact               `json:"dotnet"`
	MSBuild       ToolFact               `json:"msbuild"`
	TargetingPack PackFact               `json:"targetingPack"`
	ESAPI         PathFact               `json:"esapi"`
	DLLs          []DLLFact              `json:"dlls"`
}

func Inspect(profile scaffold.VersionConfig, options Options) Environment {
	return CurrentSystem().Inspect(profile, options)
}

func (system System) Inspect(profile scaffold.VersionConfig, options Options) Environment {
	return Environment{
		OS: system.OS, Arch: system.Arch, Profile: profile,
		Dotnet: system.dotnet(options.DotnetPath), MSBuild: system.msbuild(options.MSBuildPath),
		TargetingPack: system.targetingPack(profile.TargetFramework, options.TargetingPackPath),
		ESAPI:         system.esapi(profile.Version, options.ESAPIPath),
	}.withDLLs()
}

func (environment Environment) withDLLs() Environment {
	environment.DLLs = []DLLFact{
		inspectDLL(environment.ESAPI, "VMS.TPS.Common.Model.API.dll", "ESAPI_API_DLL"),
		inspectDLL(environment.ESAPI, "VMS.TPS.Common.Model.Types.dll", "ESAPI_TYPES_DLL"),
	}
	return environment
}
