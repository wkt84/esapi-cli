package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const usage = `Usage: esapi new <name> [options]

Create a minimal ESAPI binary plug-in project.

Options:
  --type plugin          Project type (default: plugin)
  --output <path>        Parent directory (default: current directory)
  --esapi-version <ver>  15.6, 16.0, 16.1, 17.0, 18.0 (default: 15.6)
  --esapi-path <path>    Directory containing both ESAPI DLLs
  --force               Overwrite generated files; keep all other files
  -h, --help            Show help

The ESAPI 15.6 framework profile is provisional, pending environment validation.
Without --esapi-path, the generated project uses ESAPI_HOME at build time.
`

// Run executes the CLI and returns an exit code without exiting the process.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if args[0] != "new" {
		fmt.Fprintf(stderr, "error: unknown command %q; use esapi --help\n", args[0])
		return 2
	}

	var options scaffold.Options
	var help bool
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.Type, "type", "plugin", "project type")
	flags.StringVar(&options.Output, "output", ".", "parent directory")
	flags.StringVar(&options.ESAPIVersion, "esapi-version", scaffold.DefaultVersion, "ESAPI version")
	flags.StringVar(&options.ESAPIPath, "esapi-path", "", "ESAPI DLL directory")
	flags.BoolVar(&options.Force, "force", false, "overwrite generated files")
	flags.BoolVar(&help, "help", false, "show help")
	flags.BoolVar(&help, "h", false, "show help")
	ordered, err := orderArgs(flags, args[1:])
	if err == nil {
		err = flags.Parse(ordered)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if help {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "error: specify exactly one project name; use esapi new --help")
		return 2
	}
	options.Name = flags.Arg(0)
	profile, err := scaffold.ResolveVersion(options.ESAPIVersion)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	path, err := scaffold.Generate(options)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		if errors.Is(err, scaffold.ErrInvalidOptions) {
			return 2
		}
		return 1
	}
	fmt.Fprintf(stdout, "Created %s\n", path)
	if profile.Provisional {
		fmt.Fprintf(stdout, "ESAPI %s: using .NET Framework %s provisionally; validate in your environment.\n", profile.Version, strings.TrimPrefix(profile.TargetFramework, "v"))
	}
	return 0
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
			return nil, fmt.Errorf("unknown option %q", arg)
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
