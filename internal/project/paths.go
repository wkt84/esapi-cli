package project

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func (i *inspector) home() (string, bool) {
	if i.options.ESAPIPath != "" {
		path, err := filepath.Abs(i.options.ESAPIPath)
		return path, err == nil && !strings.Contains(path, "%")
	}
	if i.conditional["esapi_home"] {
		return "", false
	}
	value := i.properties["esapi_home"]
	if value == "$([System.Environment]::GetEnvironmentVariable('ESAPI_HOME'))" {
		value = os.Getenv("ESAPI_HOME")
		return value, value != "" && !strings.Contains(value, "%")
	}
	if value == "" {
		value = os.Getenv("ESAPI_HOME")
		return value, value != "" && !strings.Contains(value, "%")
	}
	path, ok := i.expand(value, false)
	return path, ok && !strings.Contains(path, "%")
}

// Expand only two documented property names. Decode literal MSBuild percent
// escapes once, before substitution, so literal characters in paths are not
// mistaken for expressions or decoded a second time.
func (i *inspector) expand(value string, allowHome bool) (string, bool) {
	if value == "" || strings.ContainsAny(value, "*?;") || strings.Contains(value, "@(") || strings.Contains(value, "%(") {
		return "", false
	}
	var out strings.Builder
	for {
		before, rest, found := strings.Cut(value, "$(")
		out.WriteString(unescape(before))
		if !found {
			return out.String(), true
		}
		name, after, closed := strings.Cut(rest, ")")
		if !closed {
			return "", false
		}
		switch {
		case strings.EqualFold(name, "MSBuildProjectDirectory"):
			out.WriteString(filepath.Dir(i.report.Project))
		case allowHome && strings.EqualFold(name, "ESAPI_HOME"):
			home, ok := i.home()
			if !ok {
				return "", false
			}
			out.WriteString(home)
		default:
			return "", false
		}
		value = after
	}
}

func unescape(value string) string {
	var out strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '%' && index+2 < len(value) {
			if decoded, err := strconv.ParseUint(value[index+1:index+3], 16, 8); err == nil {
				out.WriteByte(byte(decoded))
				index += 2
				continue
			}
		}
		out.WriteByte(value[index])
	}
	return out.String()
}

func (i *inspector) resolvePath(value string) (string, bool) {
	path, ok := i.expand(value, true)
	if !ok || strings.ContainsRune(path, '\x00') {
		return "", false
	}
	path = strings.ReplaceAll(path, `\`, "/")
	// A Windows absolute path cannot be inspected as a relative Unix path.
	if runtime.GOOS != "windows" && ((len(path) > 1 && path[1] == ':') || strings.HasPrefix(path, "//")) {
		return "", false
	}
	path = filepath.FromSlash(path)
	if runtime.GOOS == "windows" && !filepath.IsAbs(path) && (filepath.VolumeName(path) != "" || strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "/")) {
		return "", false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(i.report.Project), path)
	}
	return filepath.Clean(path), true
}
