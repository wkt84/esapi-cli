package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const usage = `Usage: esapi new <name> [options]

Create a minimal ESAPI binary plug-in project.

Options:
  --type plugin          Project type (default: plugin)
  --output <path>        Parent directory (default: current directory)
  --esapi-version <ver>  15.6, 16.0, 16.1, 17.0, 18.0, 18.1, 19.0 (default: 15.6)
  --esapi-path <path>    Directory containing both ESAPI DLLs
  --force               Overwrite generated files; keep all other files
  --dry-run             Validate and list changes without writing
  --json                Emit a schema-versioned JSON result
  -h, --help            Show help

Without --esapi-path, the generated project uses ESAPI_HOME at build time.
`

type newData struct {
	*scaffold.ProjectPlan
	DryRun bool `json:"dryRun"`
}

func runNew(args []string) response {
	var options scaffold.Options
	var help, dryRun bool
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.Type, "type", "plugin", "project type")
	flags.StringVar(&options.Output, "output", ".", "parent directory")
	flags.StringVar(&options.ESAPIVersion, "esapi-version", scaffold.DefaultVersion, "ESAPI version")
	flags.StringVar(&options.ESAPIPath, "esapi-path", "", "ESAPI DLL directory")
	flags.BoolVar(&options.Force, "force", false, "overwrite generated files")
	flags.BoolVar(&dryRun, "dry-run", false, "plan without writing files")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	ordered, err := orderArgs(flags, args)
	if err == nil {
		err = flags.Parse(ordered)
	}
	if err != nil {
		return failed("new", "INVALID_ARGUMENTS", err, 2)
	}
	if help {
		return succeeded("new", map[string]string{"usage": usage}, usage)
	}
	if flags.NArg() != 1 {
		return failed("new", "INVALID_ARGUMENTS", fmt.Errorf("specify exactly one project name; use esapi new --help"), 2)
	}
	options.Name = flags.Arg(0)
	plan, err := scaffold.Plan(options)
	if err == nil && !dryRun {
		err = plan.Apply()
	}
	if err != nil {
		exitCode := 1
		if errors.Is(err, scaffold.ErrInvalidOptions) {
			exitCode = 2
		}
		return failed("new", "SCAFFOLD_WRITE_FAILED", err, exitCode)
	}
	message := fmt.Sprintf("Created %s\n", plan.Path)
	if dryRun {
		var text strings.Builder
		fmt.Fprintf(&text, "Would generate %s\n", plan.Path)
		for _, file := range plan.Files {
			fmt.Fprintf(&text, "  %s %s\n", file.Action, file.Path)
		}
		message = text.String()
	}
	return succeeded("new", newData{plan, dryRun}, message)
}

// The standard flag parser stops at the first positional argument. Move positional
// arguments to the end so the documented `new Name --output path` form works too.
func orderArgs(flags *flag.FlagSet, args []string) ([]string, error) {
	var options, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		option := flags.Lookup(name)
		if option == nil {
			return nil, diagnostic.New("UNKNOWN_OPTION", fmt.Sprintf("unknown option %q", arg), nil)
		}
		options = append(options, arg)
		boolean, isBoolean := option.Value.(interface{ IsBoolFlag() bool })
		if hasValue || (isBoolean && boolean.IsBoolFlag()) {
			continue
		}
		if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, fmt.Errorf("option %q requires a value", arg)
		}
		i++
		options = append(options, args[i])
	}
	return append(append(options, "--"), positional...), nil
}
