package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wkt84/esapi-cli/internal/build"
	"github.com/wkt84/esapi-cli/internal/detect"
	"github.com/wkt84/esapi-cli/internal/diagnostic"
	"github.com/wkt84/esapi-cli/internal/project"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const buildUsage = `Usage: esapi build [options]

Run MSBuild for a selected ESAPI project on Windows amd64.

Options:
  --project <file>       Select a .csproj (default: sole project in current directory)
  --msbuild-path <file>  Explicit native MSBuild; mutually exclusive with --dotnet-path
  --dotnet-path <file>   Explicit dotnet SDK host (uses dotnet msbuild)
  --esapi-path <dir>     Set the build's ESAPI_HOME global property
  --release             Release configuration (default)
  --debug               Debug configuration; mutually exclusive with --release
  --clean               Rebuild (clean then build using MSBuild targets)
  --verbose             Capture detailed MSBuild output
  --timeout <duration>  Positive Go duration, at most 24h (default: 5m)
  --dry-run             Report invocation without executing tools or writing files
  --json                Emit one JSON result including bounded tool logs
  -h, --help            Show help

Native MSBuild is preferred; missing MSBuild may fall back to a dotnet SDK.
Ambiguous or invalid selected tools require explicit correction. No restore is
requested. MSBuild evaluates project/import/target code and writes build outputs.
check/doctor are separate optional inspections, not mandatory build gates.
`

func runBuild(parent context.Context, args []string) response {
	var path string
	var release, dryRun, help bool
	options := build.Options{Timeout: build.DefaultTimeout}
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&path, "project", "", "project file")
	flags.StringVar(&options.MSBuildPath, "msbuild-path", "", "MSBuild executable")
	flags.StringVar(&options.DotnetPath, "dotnet-path", "", "dotnet executable")
	flags.StringVar(&options.ESAPIPath, "esapi-path", "", "build ESAPI_HOME override")
	flags.BoolVar(&release, "release", false, "Release configuration")
	flags.BoolVar(&options.Debug, "debug", false, "Debug configuration")
	flags.BoolVar(&options.Clean, "clean", false, "Rebuild target")
	flags.BoolVar(&options.Verbose, "verbose", false, "detailed logs")
	flags.DurationVar(&options.Timeout, "timeout", build.DefaultTimeout, "build time limit")
	flags.BoolVar(&dryRun, "dry-run", false, "report invocation only")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	ordered, err := orderArgs(flags, args)
	if err == nil {
		err = flags.Parse(ordered)
	}
	if err != nil {
		return failed("build", "INVALID_ARGUMENTS", err, 2)
	}
	if help {
		return succeeded("build", map[string]string{"usage": buildUsage}, buildUsage)
	}
	if flags.NArg() != 0 || (release && options.Debug) || (options.MSBuildPath != "" && options.DotnetPath != "") || options.Timeout <= 0 || options.Timeout > 24*time.Hour {
		return failed("build", "INVALID_ARGUMENTS", fmt.Errorf("invalid project arguments, conflicting options, or timeout; use esapi build --help"), 2)
	}
	if err := scaffold.ValidateESAPIPath(options.ESAPIPath); err != nil {
		return failed("build", "INVALID_ESAPI_PATH", err, 2)
	}
	selection, err := project.Select(path)
	if err != nil {
		exit := 1
		var detail *diagnostic.Error
		if errors.As(err, &detail) && (detail.Code == "PROJECT_NOT_FOUND" || detail.Code == "PROJECT_PATH_INVALID" || detail.Code == "PROJECT_AMBIGUOUS") {
			exit = 2
		}
		result := failed("build", "PROJECT_ACCESS_FAILED", err, exit)
		result.Data = build.Result{Plan: build.Plan{Selection: selection, Arguments: []string{}}, Status: "notStarted"}
		result.text = fmt.Sprintln(err)
		for _, candidate := range selection.Candidates {
			result.text += "  candidate: " + candidate + "\n"
		}
		return result
	}
	plan, err := build.NewPlan(selection, options, detect.CurrentSystem())
	if err != nil {
		var detail *diagnostic.Error
		if errors.As(err, &detail) && detail.Code == "INVALID_ESAPI_PATH" {
			return failed("build", "INVALID_ESAPI_PATH", err, 2)
		}
		result := failed("build", "BUILD_TOOL_UNAVAILABLE", err, 3)
		result.Data = build.Result{Plan: plan, Status: "notStarted"}
		result.text = formatBuild(build.Result{Plan: plan, Status: "notStarted"})
		return result
	}
	if dryRun {
		return succeeded("build", build.Result{Plan: plan, Status: "planned"}, formatBuild(build.Result{Plan: plan, Status: "planned"}))
	}
	ctx, cancel := context.WithTimeout(parent, options.Timeout)
	defer cancel()
	report, err := build.Execute(ctx, plan)
	result := succeeded("build", report, formatBuild(report))
	code, message, exit := "", "", 4
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code, message = "BUILD_TIMEOUT", "Build exceeded its time limit: "+options.Timeout.String()
	case errors.Is(err, context.Canceled):
		code, message = "BUILD_CANCELLED", "Build was interrupted."
	case !report.Started:
		code, message, exit = "BUILD_START_FAILED", fmt.Sprintf("Cannot start build tool: %v", err), 1
	case err != nil:
		code, message, exit = "BUILD_PROCESS_FAILED", err.Error(), 1
	case report.ExitCode != nil && *report.ExitCode != 0:
		code, message = "BUILD_FAILED", fmt.Sprintf("MSBuild exited with code %d; inspect the captured tool logs.", *report.ExitCode)
	}
	if code != "" {
		result.OK, result.exitCode = false, exit
		result.Diagnostics = []diagnostic.Diagnostic{{Code: code, Severity: "error", Message: message, Path: plan.Project}}
	}
	if report.CleanupError != "" {
		result.OK = false
		if result.exitCode == 0 {
			result.exitCode = 1
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{Code: "BUILD_CLEANUP_FAILED", Severity: "error", Message: report.CleanupError, Path: plan.Tool.Path})
	}
	return result
}

func formatBuild(report build.Result) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Project: %s\nTool: %s (%s, %s)\nConfiguration: %s; target: %s; timeout: %s\n", report.Project, report.Tool.Path, report.Tool.Kind, report.Tool.Source, report.Configuration, report.Target, report.Timeout)
	if report.Status == "planned" {
		fmt.Fprintf(&text, "Working directory: %s\n", report.WorkingDirectory)
		for _, arg := range report.Arguments {
			fmt.Fprintf(&text, "  argument: %s\n", arg)
		}
		for _, value := range report.EnvironmentOverrides {
			fmt.Fprintf(&text, "  environment override: %s\n", value)
		}
	}
	text.WriteString(report.Stdout)
	text.WriteString(report.Stderr)
	if report.StdoutTruncated || report.StderrTruncated {
		fmt.Fprintln(&text, "\nTool logs truncated (1 MiB per stream).")
	}
	fmt.Fprintf(&text, "\nBuild status: %s\n", report.Status)
	return text.String()
}
