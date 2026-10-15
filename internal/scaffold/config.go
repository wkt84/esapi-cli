package scaffold

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const DefaultVersion = "15.6"

var ErrInvalidOptions = errors.New("invalid options")

type Options struct {
	Name         string
	Output       string
	Type         string
	ESAPIVersion string
	ESAPIPath    string
	Force        bool
}

type VersionConfig struct {
	Version         string
	TargetFramework string
	Platform        string
	Provisional     bool
}

// Keep all version-specific choices here. These are generation profiles, not a
// claim that the generated assemblies have been tested in every Eclipse release.
var versions = map[string]VersionConfig{
	"15.6": {"15.6", "v4.5", "x64", true},
	"16.0": {"16.0", "v4.6.1", "x64", false},
	"16.1": {"16.1", "v4.6.1", "x64", false},
	"17.0": {"17.0", "v4.8", "x64", false},
	"18.0": {"18.0", "v4.8", "x64", false},
}

// ResolveVersion returns the generation profile for an explicitly selected release.
func ResolveVersion(version string) (VersionConfig, error) {
	if version == "" {
		version = DefaultVersion
	}
	switch version {
	case "16", "17", "18":
		version += ".0"
	}
	config, ok := versions[version]
	if !ok {
		return VersionConfig{}, fmt.Errorf("%w: unsupported ESAPI version %q; choose 15.6, 16.0, 16.1, 17.0, or 18.0 (automatic detection is not implemented)", ErrInvalidOptions, version)
	}
	return config, nil
}

var projectName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,99}$`)

func validateName(name string) error {
	if !projectName.MatchString(name) {
		return fmt.Errorf("%w: project name must be 1-100 ASCII letters, digits, or underscores and must not start with a digit", ErrInvalidOptions)
	}
	upper := strings.ToUpper(name)
	reserved := upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL"
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		reserved = reserved || (upper[3] >= '1' && upper[3] <= '9')
	}
	if reserved {
		return fmt.Errorf("%w: project name %q is reserved on Windows", ErrInvalidOptions, name)
	}
	return nil
}
