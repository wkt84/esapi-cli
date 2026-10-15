// Package build plans and runs the installed MSBuild toolchain. MSBuild owns
// project evaluation, compilation, and output files.
package build

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wkt84/esapi-cli/internal/detect"
	"github.com/wkt84/esapi-cli/internal/diagnostic"
	"github.com/wkt84/esapi-cli/internal/project"
)

const DefaultTimeout = 5 * time.Minute

type Options struct {
	MSBuildPath string
	DotnetPath  string
	ESAPIPath   string
	Debug       bool
	Clean       bool
	Verbose     bool
	Timeout     time.Duration
}

type Tool struct {
	Kind string `json:"kind"`
	detect.ToolFact
}

type Plan struct {
	project.Selection
	Tool                 Tool     `json:"tool"`
	Arguments            []string `json:"arguments"`
	WorkingDirectory     string   `json:"workingDirectory"`
	Configuration        string   `json:"configuration"`
	Target               string   `json:"target"`
	ESAPIPath            string   `json:"esapiPath"`
	Timeout              string   `json:"timeout"`
	EnvironmentOverrides []string `json:"environmentOverrides"`
}

func NewPlan(selection project.Selection, options Options, system detect.System) (Plan, error) {
	plan := Plan{Selection: selection, Arguments: []string{}, Configuration: "Release", Target: "Build", Timeout: options.Timeout.String(), EnvironmentOverrides: environmentOverrides()}
	plan.Tool.Candidates, plan.Tool.SDKs = []string{}, []string{}
	if options.Debug {
		plan.Configuration = "Debug"
	}
	if options.Clean {
		plan.Target = "Rebuild"
	}
	plan.WorkingDirectory = filepath.Dir(selection.Project)
	if system.OS != "windows" || system.Arch != "amd64" {
		return plan, diagnostic.New("BUILD_HOST_UNSUPPORTED", "ESAPI builds require Windows amd64; use new/check for portable project operations", nil)
	}
	msbuild, dotnet := system.BuildTools(options.MSBuildPath, options.DotnetPath)
	if options.DotnetPath != "" {
		plan.Tool = Tool{"dotnet", dotnet}
	} else if options.MSBuildPath != "" || msbuild.Status != "missing" {
		plan.Tool = Tool{"msbuild", msbuild}
	} else {
		plan.Tool = Tool{"dotnet", dotnet}
	}
	if plan.Tool.Status != "found" || (plan.Tool.Kind == "dotnet" && plan.Tool.SDKStatus != "found") {
		err := diagnostic.New("BUILD_TOOL_UNAVAILABLE", "select one usable MSBuild executable or dotnet SDK; explicit/ambiguous tools do not fall back", nil)
		err.Path, err.Hint = plan.Tool.Path, "Use --msbuild-path or --dotnet-path. Run esapi info --json to inspect installations."
		return plan, err
	}
	if plan.Tool.Kind == "dotnet" {
		plan.Arguments = append(plan.Arguments, "msbuild")
	}
	verbosity := "minimal"
	if options.Verbose {
		verbosity = "detailed"
	}
	plan.Arguments = append(plan.Arguments, selection.Project, "-nologo", "-noAutoResponse", "-nodeReuse:false", "-maxCpuCount:1", "-target:"+plan.Target, "-verbosity:"+verbosity, "-property:Configuration="+plan.Configuration, "-property:Platform=x64", "-property:UseSharedCompilation=false", "-property:Deterministic=true")
	if options.ESAPIPath != "" {
		path, err := filepath.Abs(options.ESAPIPath)
		if err != nil {
			return plan, diagnostic.New("INVALID_ESAPI_PATH", "cannot resolve ESAPI path", err)
		}
		plan.ESAPIPath = path
		plan.Arguments = append(plan.Arguments, "-property:ESAPI_HOME="+escapeProperty(path))
	}
	return plan, nil
}

// MSBuild consumes percent escapes after command-line property parsing. The OS
// launcher separately quotes argv; no shell command string is evaluated.
func escapeProperty(value string) string {
	var result strings.Builder
	for _, char := range value {
		if strings.ContainsRune("%$@;?'*()", char) {
			fmt.Fprintf(&result, "%%%02X", char)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}
