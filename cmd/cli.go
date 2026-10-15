package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

type response struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Command       string                  `json:"command"`
	OK            bool                    `json:"ok"`
	Data          any                     `json:"data"`
	Diagnostics   []diagnostic.Diagnostic `json:"diagnostics"`
	text          string
	exitCode      int
}

func succeeded(command string, data any, text string) response {
	return response{SchemaVersion: 1, Command: command, OK: true, Data: data, Diagnostics: []diagnostic.Diagnostic{}, text: text}
}

func failed(command, code string, err error, exitCode int) response {
	d := diagnostic.Diagnostic{Code: code, Severity: "error", Message: err.Error()}
	var detailed *diagnostic.Error
	if errors.As(err, &detailed) {
		d = detailed.Diagnostic
	}
	return response{SchemaVersion: 1, Command: command, Diagnostics: []diagnostic.Diagnostic{d}, exitCode: exitCode}
}

// Run executes the CLI without prompting or exiting the calling process.
func Run(args []string, stdout, stderr io.Writer) int {
	args, jsonMode, formatErr := extractJSON(args)
	command := "help"
	if len(args) > 0 {
		command = args[0]
	}
	var result response
	switch {
	case formatErr != nil:
		result = failed(command, "INVALID_ARGUMENTS", formatErr, 2)
	case len(args) == 0 || command == "--help" || command == "-h" || command == "help":
		result = succeeded("help", map[string]string{"usage": rootUsage}, rootUsage)
	case command == "new":
		result = runNew(args[1:])
	case command == "check":
		result = runCheck(args[1:])
	case command == "info" || command == "doctor":
		result = runInspection(command, args[1:])
	case command == "version" || command == "--version":
		result = runVersion(args[1:])
	default:
		result = failed(command, "UNKNOWN_COMMAND", fmt.Errorf("unknown command %q; use esapi --help", command), 2)
	}
	if jsonMode {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "error: cannot write JSON output: %v\n", err)
			return 1
		}
	} else {
		if _, err := io.WriteString(stdout, result.text); err != nil {
			fmt.Fprintf(stderr, "error: cannot write output: %v\n", err)
			return 1
		}
	}
	if !jsonMode && !result.OK {
		for _, d := range result.Diagnostics {
			fmt.Fprintf(stderr, "error: %s [%s]\n", d.Message, d.Code)
			if d.Hint != "" {
				fmt.Fprintln(stderr, d.Hint)
			}
		}
	}
	return result.exitCode
}

// Resolve the output format before parsing other options so malformed requests
// also produce JSON. The -- delimiter ends option interpretation as usual.
func extractJSON(args []string) ([]string, bool, error) {
	var remaining []string
	var jsonMode bool
	var invalid error
	for i, arg := range args {
		if arg == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		name, value, assigned := strings.Cut(arg, "=")
		if name != "--json" {
			remaining = append(remaining, arg)
			continue
		}
		jsonMode = true
		if assigned {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				invalid = fmt.Errorf("invalid boolean value %q for --json", value)
			} else {
				jsonMode = parsed
			}
		}
	}
	return remaining, jsonMode, invalid
}
