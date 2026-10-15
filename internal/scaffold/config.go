package scaffold

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
)

const DefaultVersion = "15.6"

var ErrInvalidOptions = errors.New("invalid options")

// ValidateESAPIPath checks the supported SDK-directory syntax without requiring
// an installation. Never strip quotes: shell-corrupted arguments are invalid
// input, not a different directory to guess or silently repair.
func ValidateESAPIPath(path string) error {
	if strings.ContainsRune(path, '"') {
		err := diagnostic.New("INVALID_ESAPI_PATH", "ESAPI directory contains a literal double quote", ErrInvalidOptions)
		err.Path = path
		err.Hint = "Pass the directory without literal quote characters. In Windows PowerShell, remove the trailing backslash from a quoted directory argument before invoking the CLI."
		return err
	}
	for _, char := range path {
		if char < 32 || char == 127 {
			return diagnostic.New("INVALID_ESAPI_PATH", "ESAPI directory must not contain control characters", ErrInvalidOptions)
		}
	}
	if strings.ContainsRune(path, '%') {
		// The MSBuild reference/compiler pipeline may decode escapes twice.
		return diagnostic.New("INVALID_ESAPI_PATH", "ESAPI paths containing a percent sign are not supported; choose a DLL directory without it", ErrInvalidOptions)
	}
	return nil
}

type Options struct {
	Name         string
	Output       string
	Type         string
	ESAPIVersion string
	ESAPIPath    string
	Force        bool
}

type VersionConfig struct {
	Version         string `json:"version"`
	TargetFramework string `json:"targetFramework"`
	Platform        string `json:"platform"`
}

// Profiles returns a stable, independent list of supported generation profiles.
func Profiles() []VersionConfig {
	result := make([]VersionConfig, 0, len(versions))
	for _, profile := range versions {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result
}

// Keep all version-specific choices here. These are generation profiles, not a
// claim that the generated assemblies have been tested in every Eclipse release.
var versions = map[string]VersionConfig{
	// 15.6 follows the 15.5 .NET Framework 4.5 profile; no separate framework
	// requirement is stated in the available 15.6 documentation.
	"15.6": {"15.6", "v4.5", "x64"},
	"16.0": {"16.0", "v4.6.1", "x64"},
	"16.1": {"16.1", "v4.6.1", "x64"},
	"17.0": {"17.0", "v4.8", "x64"},
	"18.0": {"18.0", "v4.8", "x64"},
	"18.1": {"18.1", "v4.8", "x64"},
	"19.0": {"19.0", "v4.8", "x64"},
}

// ResolveVersion returns the generation profile for an explicitly selected release.
func ResolveVersion(version string) (VersionConfig, error) {
	if version == "" {
		version = DefaultVersion
	}
	switch version {
	case "16", "17", "18", "19":
		version += ".0"
	}
	config, ok := versions[version]
	if !ok {
		return VersionConfig{}, diagnostic.New("UNSUPPORTED_ESAPI_VERSION", fmt.Sprintf("unsupported ESAPI version %q; choose 15.6, 16.0, 16.1, 17.0, 18.0, 18.1, or 19.0 (automatic detection is not implemented)", version), ErrInvalidOptions)
	}
	return config, nil
}

var projectName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,99}$`)

func validateName(name string) error {
	if !projectName.MatchString(name) {
		return diagnostic.New("INVALID_PROJECT_NAME", "project name must be 1-100 ASCII letters, digits, or underscores and must not start with a digit", ErrInvalidOptions)
	}
	upper := strings.ToUpper(name)
	reserved := upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL"
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		reserved = reserved || (upper[3] >= '1' && upper[3] <= '9')
	}
	if reserved {
		return diagnostic.New("INVALID_PROJECT_NAME", fmt.Sprintf("project name %q is reserved on Windows", name), ErrInvalidOptions)
	}
	return nil
}
