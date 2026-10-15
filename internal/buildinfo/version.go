package buildinfo

import "runtime/debug"

// Version and Commit can be supplied through Go linker -X flags. Dates are not
// included so the default build does not embed a changing wall-clock timestamp.
var Version = "0.1.0-dev"
var Commit string

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Modified  bool   `json:"modified"`
	GoVersion string `json:"goVersion"`
}

func Current() Info {
	result := Info{Version: Version, Commit: Commit}
	if build, ok := debug.ReadBuildInfo(); ok {
		result.GoVersion = build.GoVersion
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if result.Commit == "" {
					result.Commit = setting.Value
				}
			case "vcs.modified":
				result.Modified = setting.Value == "true"
			}
		}
	}
	return result
}
