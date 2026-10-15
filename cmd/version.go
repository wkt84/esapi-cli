package cmd

import (
	"fmt"

	"github.com/wkt84/esapi-cli/internal/buildinfo"
)

func runVersion(args []string) response {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		text := "Usage: esapi version [--json]\nDisplay the CLI version; esapi --version is an alias.\n"
		return succeeded("version", map[string]string{"usage": text}, text)
	}
	if len(args) != 0 {
		return failed("version", "INVALID_ARGUMENTS", fmt.Errorf("version accepts only --json; use esapi version --help"), 2)
	}
	data := buildinfo.Current()
	return succeeded("version", data, fmt.Sprintf("esapi %s\n", data.Version))
}
