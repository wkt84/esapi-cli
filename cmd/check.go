package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
	"github.com/wkt84/esapi-cli/internal/project"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const checkUsage = `Usage: esapi check [options]

Inspect conventional ESAPI binary plug-in project declarations without MSBuild.

Options:
  --project <file>       Select a .csproj (default: sole project in current directory)
  --esapi-version <ver>  Expected profile; required if ESAPIVersion is absent
  --esapi-path <dir>     Override ESAPI_HOME for this inspection only
  --json                Emit a schema-versioned JSON result
  -h, --help            Show help

Checks framework, x64, assembly naming, references, and explicit Compile files.
Conditional settings, globs, unknown expressions, SDK projects, and custom imports
require separate MSBuild evaluation. No tools or project code are executed.
Exit 5 means failed or unevaluable project requirements; 2 means selection/input
errors, and 1 means an operational error. Reports retain observations on failure.
`

func runCheck(args []string) response {
	var path string
	var options project.Options
	var help bool
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&path, "project", "", "project file")
	flags.StringVar(&options.ESAPIVersion, "esapi-version", "", "expected ESAPI profile")
	flags.StringVar(&options.ESAPIPath, "esapi-path", "", "ESAPI_HOME inspection override")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	ordered, err := orderArgs(flags, args)
	if err == nil {
		err = flags.Parse(ordered)
	}
	if err != nil {
		return failed("check", "INVALID_ARGUMENTS", err, 2)
	}
	if help {
		return succeeded("check", map[string]string{"usage": checkUsage}, checkUsage)
	}
	if flags.NArg() != 0 {
		return failed("check", "INVALID_ARGUMENTS", fmt.Errorf("check accepts options only; use --project <file>"), 2)
	}
	if options.ESAPIVersion != "" {
		profile, err := scaffold.ResolveVersion(options.ESAPIVersion)
		if err != nil {
			return failed("check", "INVALID_ARGUMENTS", err, 2)
		}
		options.ESAPIVersion = profile.Version
	}
	if err := scaffold.ValidateESAPIPath(options.ESAPIPath); err != nil {
		return failed("check", "INVALID_ESAPI_PATH", err, 2)
	}
	selection, err := project.Select(path)
	report := project.NewReport(selection)
	if err == nil {
		var diagnostics []diagnostic.Diagnostic
		report, diagnostics, err = project.Inspect(selection, options)
		if err == nil {
			result := succeeded("check", report, formatProjectReport(report))
			result.Diagnostics = diagnostics
			if !report.StaticChecksPassed {
				result.OK, result.exitCode = false, 5
			}
			return result
		}
	}
	exitCode := 1
	var detail *diagnostic.Error
	if errors.As(err, &detail) && (detail.Code == "PROJECT_NOT_FOUND" || detail.Code == "PROJECT_AMBIGUOUS" || detail.Code == "PROJECT_PATH_INVALID") {
		exitCode = 2
	}
	report.StaticChecksPassed = false
	result := failed("check", "PROJECT_ACCESS_FAILED", err, exitCode)
	result.Data, result.text = report, formatProjectReport(report)
	return result
}

func formatProjectReport(report project.Report) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Project: %s\n", report.Project)
	if report.Project == "" {
		for _, candidate := range report.Candidates {
			fmt.Fprintf(&text, "  candidate: %s\n", candidate)
		}
	}
	for _, check := range report.Checks {
		fmt.Fprintf(&text, "[%s] %s: %s\n", strings.ToUpper(check.Status), check.ID, check.Message)
	}
	fmt.Fprintf(&text, "Static checks passed: %t\n", report.StaticChecksPassed)
	return text.String()
}
