package detect

import (
	"os"
	"path/filepath"
	"sort"
)

// BuildTools exposes the same bounded tool discovery used by info and doctor.
// It does not inspect SDK assemblies or execute the selected tools.
func (system System) BuildTools(msbuildPath, dotnetPath string) (ToolFact, ToolFact) {
	return system.msbuild(msbuildPath), system.dotnet(dotnetPath)
}

func (system System) tool(explicit string, names, candidates []string) PathFact {
	if explicit != "" {
		return inspectPath(explicit, "argument", false)
	}
	for _, name := range names {
		if path, err := system.LookPath(name); err == nil {
			return inspectPath(path, "path", false)
		}
	}
	found := []string{}
	for _, path := range candidates {
		if inspectPath(path, "discovery", false).Status == "found" {
			found = append(found, path)
		}
	}
	return system.choose(found, false)
}

func (system System) dotnet(explicit string) ToolFact {
	names := []string{"dotnet"}
	if system.OS == "windows" {
		names = []string{"dotnet.exe", "dotnet"}
	}
	candidates := []string{}
	if system.OS == "windows" {
		for _, root := range system.programFiles() {
			candidates = append(candidates, filepath.Join(root, "dotnet", "dotnet.exe"))
		}
	}
	fact := ToolFact{PathFact: system.tool(explicit, names, candidates), SDKs: []string{}, SDKStatus: "unverified"}
	if fact.Status != "found" {
		return fact
	}
	host := fact.Path
	info, err := os.Lstat(host)
	if err != nil {
		fact.Issue = err.Error()
		return fact
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Resolve a linked host (e.g. /usr/bin/dotnet) to locate its SDKs. Avoid
		// walking all ancestor handles for ordinary Windows executables.
		host, err = filepath.EvalSymlinks(host)
		if err != nil {
			fact.Issue = err.Error()
			return fact
		}
	}
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(host), "sdk"))
	if err != nil {
		if os.IsNotExist(err) {
			fact.SDKStatus = "missing"
		} else {
			fact.Issue = err.Error()
		}
		return fact
	}
	for _, entry := range entries {
		if entry.IsDir() && inspectPath(filepath.Join(filepath.Dir(host), "sdk", entry.Name(), "MSBuild.dll"), "discovery", false).Status == "found" {
			fact.SDKs = append(fact.SDKs, entry.Name())
		}
	}
	sort.Strings(fact.SDKs)
	fact.SDKStatus = "missing"
	if len(fact.SDKs) > 0 {
		fact.SDKStatus = "found"
	}
	return fact
}

func (system System) msbuild(explicit string) ToolFact {
	candidates := []string{}
	if system.OS == "windows" {
		for _, root := range system.programFiles() {
			for _, layout := range []string{filepath.Join("*", "*"), "*"} {
				for _, folder := range []string{"Current", "15.0"} {
					pattern := filepath.Join(root, "Microsoft Visual Studio", layout, "MSBuild", folder, "Bin", "MSBuild.exe")
					matches, _ := filepath.Glob(pattern)
					for _, path := range matches {
						// Prefer the 64-bit executable within each installation.
						x64 := filepath.Join(filepath.Dir(path), "amd64", "MSBuild.exe")
						if inspectPath(x64, "discovery", false).Status == "found" {
							path = x64
						}
						candidates = append(candidates, path)
					}
				}
			}
		}
	}
	names := []string{"MSBuild.exe", "msbuild"}
	return ToolFact{PathFact: system.tool(explicit, names, candidates), SDKs: []string{}, SDKStatus: "notApplicable"}
}
