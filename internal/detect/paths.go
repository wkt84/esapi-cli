package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wkt84/esapi-cli/internal/scaffold"
)

func emptyFact(source string) PathFact {
	return PathFact{Source: source, Status: "missing", Candidates: []string{}}
}

func inspectPath(path, source string, directory bool) PathFact {
	fact := emptyFact(source)
	absolute, err := filepath.Abs(path)
	if err != nil {
		fact.Status, fact.Issue = "invalid", err.Error()
		return fact
	}
	fact.Path = absolute
	info, err := os.Stat(absolute)
	if err != nil {
		if !os.IsNotExist(err) {
			fact.Status, fact.Issue = "unverified", err.Error()
		}
		return fact
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		fact.Status, fact.Issue = "invalid", "path has the wrong file type"
		return fact
	}
	fact.Status = "found"
	return fact
}

func (system System) unique(paths []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		key := path
		if system.OS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, path)
		}
	}
	return result
}

func (system System) programFiles() []string {
	return system.unique([]string{system.Getenv("ProgramFiles(x86)"), system.Getenv("ProgramFiles"), system.Getenv("ProgramW6432")})
}

func (system System) choose(candidates []string, directory bool) PathFact {
	candidates = system.unique(candidates)
	sort.Strings(candidates)
	fact := emptyFact("discovery")
	fact.Candidates = candidates
	switch len(candidates) {
	case 0:
	case 1:
		fact = inspectPath(candidates[0], "discovery", directory)
		fact.Candidates = candidates
	default:
		fact.Status, fact.Issue = "ambiguous", "multiple candidates found; specify an explicit path"
	}
	return fact
}

func (system System) esapi(version, explicit string) PathFact {
	var fact PathFact
	switch {
	case explicit != "":
		fact = inspectPath(explicit, "argument", true)
	case system.Getenv("ESAPI_HOME") != "":
		fact = inspectPath(system.Getenv("ESAPI_HOME"), "environment", true)
	default:
		candidates := []string{}
		if system.OS == "windows" {
			versions := []string{version}
			if strings.HasSuffix(version, ".0") {
				versions = append(versions, strings.TrimSuffix(version, ".0"))
			}
			for _, root := range system.programFiles() {
				for _, release := range versions {
					path := filepath.Join(root, "Varian", "RTM", release, "esapi", "API")
					if inspectPath(path, "discovery", true).Status == "found" {
						candidates = append(candidates, path)
					}
				}
			}
		}
		fact = system.choose(candidates, true)
	}
	if err := scaffold.ValidateESAPIPath(fact.Path); err != nil {
		fact.Status, fact.Issue = "invalid", err.Error()
	}
	return fact
}

func (system System) targetingPack(framework, explicit string) PackFact {
	var fact PathFact
	if explicit != "" {
		fact = inspectPath(explicit, "argument", true)
	} else {
		fact = emptyFact("discovery")
		if system.OS != "windows" {
			fact.Status, fact.Issue = "unverified", "automatic .NET Framework targeting-pack discovery is Windows-only"
			return checkPack(fact)
		}
		for _, root := range system.programFiles() {
			path := filepath.Join(root, "Reference Assemblies", "Microsoft", "Framework", ".NETFramework", framework)
			candidate := inspectPath(path, "discovery", true)
			if fact.Path == "" || candidate.Status == "found" {
				fact = candidate
			}
			if candidate.Status == "found" {
				pack := checkPack(candidate)
				if pack.Status == "found" {
					return pack
				}
			}
		}
	}
	return checkPack(fact)
}

func checkPack(fact PathFact) PackFact {
	pack := PackFact{PathFact: fact, MissingFiles: []string{}}
	if fact.Status != "found" {
		return pack
	}
	var unreadable []string
	for _, name := range []string{"mscorlib.dll", filepath.Join("RedistList", "FrameworkList.xml")} {
		marker := inspectPath(filepath.Join(fact.Path, name), "discovery", false)
		if marker.Status == "unverified" {
			unreadable = append(unreadable, filepath.ToSlash(name))
		} else if marker.Status != "found" {
			pack.MissingFiles = append(pack.MissingFiles, filepath.ToSlash(name))
		}
	}
	if len(pack.MissingFiles) > 0 {
		pack.Status, pack.Issue = "invalid", fmt.Sprintf("targeting-pack files are missing or have the wrong type: %s", strings.Join(pack.MissingFiles, ", "))
	} else if len(unreadable) > 0 {
		pack.Status, pack.Issue = "unverified", fmt.Sprintf("cannot inspect targeting-pack files: %s", strings.Join(unreadable, ", "))
	}
	return pack
}
