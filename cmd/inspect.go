package cmd

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wkt84/esapi-cli/internal/buildinfo"
	"github.com/wkt84/esapi-cli/internal/detect"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const rootUsage = `Usage: esapi <command> [options]

Commands:
  new <name>   Generate an ESAPI binary plug-in project
  info         Inspect environment facts and supported CLI capabilities
  doctor       Diagnose static development-environment requirements
  check        Inspect ESAPI project declarations and referenced files
  build        Run the installed MSBuild toolchain with bounded execution
  version      Display the CLI version (--version is an alias)

Use esapi <command> --help for options. Use --json for structured results.
Commands do not prompt. build executes the selected toolchain and project targets.
info/doctor/check do not execute tools or load ESAPI assemblies.
`

func inspectionUsage(command string) string {
	return fmt.Sprintf(`Usage: esapi %s [options]

Read local environment metadata without executing tools or loading assemblies.

Options:
  --esapi-version <ver>       Profile to inspect (default: 15.6)
  --esapi-path <directory>    DLL directory; overrides ESAPI_HOME and discovery
  --dotnet-path <file>        Explicit dotnet host executable
  --msbuild-path <file>       Explicit MSBuild executable
  --targeting-pack-path <dir> Explicit framework reference-assembly directory
  --json                     Emit a schema-versioned JSON result
  -h, --help                 Show help

ESAPI discovery checks the selected profile's known Varian/RTM paths only.
Custom paths are inspected without changing project or system configuration.
info reports facts even when dependencies are absent; doctor exits 3 when static
requirements fail. Passing checks do not prove build or Eclipse compatibility.
`, command)
}

type capability struct {
	Name     string `json:"name"`
	JSON     bool   `json:"json"`
	DryRun   bool   `json:"dryRun"`
	ReadOnly bool   `json:"readOnly"`
}

type capabilities struct {
	Commands       []capability             `json:"commands"`
	Profiles       []scaffold.VersionConfig `json:"profiles"`
	SchemaVersions []int                    `json:"schemaVersions"`
}

type infoData struct {
	CLI          buildinfo.Info     `json:"cli"`
	Capabilities capabilities       `json:"capabilities"`
	Environment  detect.Environment `json:"environment"`
}

func runInspection(command string, args []string) response {
	var options detect.Options
	var version string
	var help bool
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&version, "esapi-version", scaffold.DefaultVersion, "generation profile")
	flags.StringVar(&options.ESAPIPath, "esapi-path", "", "DLL directory")
	flags.StringVar(&options.DotnetPath, "dotnet-path", "", "dotnet executable")
	flags.StringVar(&options.MSBuildPath, "msbuild-path", "", "MSBuild executable")
	flags.StringVar(&options.TargetingPackPath, "targeting-pack-path", "", "reference assembly directory")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	ordered, err := orderArgs(flags, args)
	if err == nil {
		err = flags.Parse(ordered)
	}
	if err != nil {
		return failed(command, "INVALID_ARGUMENTS", err, 2)
	}
	if help {
		text := inspectionUsage(command)
		return succeeded(command, map[string]string{"usage": text}, text)
	}
	if flags.NArg() != 0 {
		return failed(command, "INVALID_ARGUMENTS", fmt.Errorf("%s accepts options only; use esapi %s --help", command, command), 2)
	}
	if err := scaffold.ValidateESAPIPath(options.ESAPIPath); err != nil {
		return failed(command, "INVALID_ESAPI_PATH", err, 2)
	}
	profile, err := scaffold.ResolveVersion(version)
	if err != nil {
		return failed(command, "INVALID_ARGUMENTS", err, 2)
	}
	environment := detect.Inspect(profile, options)
	if command == "info" {
		data := infoData{
			CLI: buildinfo.Current(), Environment: environment,
			Capabilities: capabilities{
				Commands: []capability{{"new", true, true, false}, {"info", true, false, true}, {"doctor", true, false, true}, {"check", true, false, true}, {"build", true, true, false}, {"version", true, false, true}},
				Profiles: scaffold.Profiles(), SchemaVersions: []int{1},
			},
		}
		return succeeded(command, data, formatInfo(data))
	}
	report, diagnostics := detect.Diagnose(environment)
	result := succeeded(command, report, formatDoctor(report))
	result.Diagnostics = diagnostics
	if !report.StaticChecksPassed {
		result.OK, result.exitCode = false, 3
	}
	return result
}

func formatInfo(data infoData) string {
	var text strings.Builder
	fmt.Fprintf(&text, "esapi %s\nHost: %s/%s\nSelected profile: ESAPI %s (%s, %s)\n", data.CLI.Version, data.Environment.OS, data.Environment.Arch, data.Environment.Profile.Version, data.Environment.Profile.TargetFramework, data.Environment.Profile.Platform)
	for _, item := range []struct {
		name string
		fact detect.PathFact
	}{{"dotnet", data.Environment.Dotnet.PathFact}, {"MSBuild", data.Environment.MSBuild.PathFact}, {"Targeting pack", data.Environment.TargetingPack.PathFact}, {"ESAPI", data.Environment.ESAPI}} {
		fmt.Fprintf(&text, "%s: %s (%s) %s\n", item.name, item.fact.Status, item.fact.Source, item.fact.Path)
		for _, candidate := range item.fact.Candidates {
			fmt.Fprintf(&text, "  candidate: %s\n", candidate)
		}
		if item.fact.Issue != "" {
			fmt.Fprintf(&text, "  %s\n", item.fact.Issue)
		}
	}
	fmt.Fprintf(&text, ".NET SDK markers: %s [%s]\n", data.Environment.Dotnet.SDKStatus, strings.Join(data.Environment.Dotnet.SDKs, ", "))
	for _, dll := range data.Environment.DLLs {
		fmt.Fprintf(&text, "%s: %s %s\n", dll.Name, dll.Status, dll.Path)
		if dll.PE != nil {
			fmt.Fprintf(&text, "  %s, machine=%s, CLR header declaration=%t\n", dll.PE.Format, dll.PE.Machine, dll.PE.HasCLRHeader)
		}
	}
	fmt.Fprintln(&text, "Commands: new, info, doctor, check, build, version. Use --json for capabilities and full observations.")
	fmt.Fprintln(&text, "Tool execution, assembly identity/version, and Eclipse compatibility are unverified.")
	return text.String()
}

func formatDoctor(report detect.DoctorReport) string {
	var text strings.Builder
	fmt.Fprintln(&text, "ESAPI static development-environment checks")
	fmt.Fprintf(&text, "Host: %s/%s; profile: ESAPI %s (%s, %s)\n", report.Environment.OS, report.Environment.Arch, report.Environment.Profile.Version, report.Environment.Profile.TargetFramework, report.Environment.Profile.Platform)
	for _, check := range report.Checks {
		fmt.Fprintf(&text, "[%s] %s: %s\n", strings.ToUpper(check.Status), check.ID, check.Message)
		if check.Path != "" {
			fmt.Fprintf(&text, "  %s\n", check.Path)
		}
	}
	fmt.Fprintf(&text, "Static checks passed: %t\n", report.StaticChecksPassed)
	return text.String()
}
