package project

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wkt84/esapi-cli/internal/diagnostic"
	"github.com/wkt84/esapi-cli/internal/scaffold"
)

const maxProjectBytes = 4 * 1024 * 1024

type Options struct {
	ESAPIVersion string
	ESAPIPath    string
}

type Check struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type Properties struct {
	ESAPIVersion       string `json:"esapiVersion"`
	ESAPIVersionSource string `json:"esapiVersionSource"`
	TargetFramework    string `json:"targetFramework"`
	Platform           string `json:"platform"`
	AssemblyName       string `json:"assemblyName"`
	OutputType         string `json:"outputType"`
}

type File struct {
	Include string `json:"include"`
	Path    string `json:"path"`
	Status  string `json:"status"`
}

type Report struct {
	Selection
	Properties         Properties `json:"properties"`
	References         []File     `json:"references"`
	Files              []File     `json:"files"`
	Checks             []Check    `json:"checks"`
	StaticChecksPassed bool       `json:"staticChecksPassed"`
}

func NewReport(selection Selection) Report {
	return Report{Selection: selection, References: []File{}, Files: []File{}, Checks: []Check{}}
}

type element struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []element  `xml:",any"`
}

func (e element) attr(name string) string {
	for _, attr := range e.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

type inspector struct {
	report      Report
	diagnostics []diagnostic.Diagnostic
	properties  map[string]string
	conditional map[string]bool
	options     Options
}

// Inspect reads at most 4 MiB and checks declarations only. Returned errors are
// I/O failures; invalid or unevaluable project declarations remain in the report.
func Inspect(selection Selection, options Options) (Report, []diagnostic.Diagnostic, error) {
	i := inspector{NewReport(selection), []diagnostic.Diagnostic{}, map[string]string{}, map[string]bool{}, options}
	f, err := os.Open(selection.Project)
	if err != nil {
		return i.report, i.diagnostics, selectionError("PROJECT_ACCESS_FAILED", "cannot open project", selection.Project, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return i.report, i.diagnostics, selectionError("PROJECT_ACCESS_FAILED", "project is not an accessible regular file", selection.Project, err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxProjectBytes+1))
	if err != nil {
		return i.report, i.diagnostics, selectionError("PROJECT_ACCESS_FAILED", "cannot read project", selection.Project, err)
	}
	i.report.StaticChecksPassed = true
	root, err := parseProject(data)
	if len(data) > maxProjectBytes || err != nil {
		message := "Project must be UTF-8 XML no larger than 4 MiB."
		if err != nil {
			message += " " + err.Error()
		}
		i.add("project.xml", false, "PROJECT_XML_INVALID", message, selection.Project)
		return i.report, i.diagnostics, nil
	}
	legacy := root.XMLName.Local == "Project" && (root.XMLName.Space == "" || root.XMLName.Space == "http://schemas.microsoft.com/developer/msbuild/2003") && root.attr("Sdk") == ""
	i.add("project.format", legacy, "PROJECT_FORMAT_UNSUPPORTED", "Static checks support conventional non-SDK .csproj files only.", selection.Project)
	if !legacy {
		return i.report, i.diagnostics, nil
	}
	for _, child := range root.Children {
		if !sameNamespace(child, root.XMLName.Space) {
			i.unevaluated("project.namespace", "Foreign XML namespaces are outside the supported project format.")
		}
		if child.XMLName.Local == "PropertyGroup" {
			for _, property := range child.Children {
				key := strings.ToLower(property.XMLName.Local)
				i.properties[key] = strings.TrimSpace(property.Text)
				if child.attr("Condition") != "" || property.attr("Condition") != "" || len(property.Children) != 0 {
					i.conditional[key] = true
				}
			}
		}
	}
	i.checkProperties()
	i.checkItems(root)
	i.report.Checks = append(i.report.Checks, Check{ID: "verification.msbuild", Status: "skip", Message: "MSBuild evaluation (including implicit imports and target bodies), compilation, assembly identity, and Eclipse execution were not performed."})
	return i.report, i.diagnostics, nil
}

func parseProject(data []byte) (element, error) {
	var root element
	if len(data) > maxProjectBytes {
		return root, fmt.Errorf("project exceeds size limit")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(bytes.NewReader(data))
	found := false
	for {
		token, err := decoder.Token()
		if err == io.EOF && found {
			return root, nil
		}
		if err != nil {
			return root, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if found {
				return root, fmt.Errorf("multiple project roots")
			}
			if err := decoder.DecodeElement(&root, &value); err != nil {
				return root, err
			}
			found = true
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return root, fmt.Errorf("unexpected text outside project root")
			}
		case xml.Directive:
			if found {
				return root, fmt.Errorf("unexpected directive after project root")
			}
		}
	}
}

func sameNamespace(node element, namespace string) bool {
	if node.XMLName.Space != namespace {
		return false
	}
	for _, child := range node.Children {
		if !sameNamespace(child, namespace) {
			return false
		}
	}
	return true
}

func (i *inspector) add(id string, passed bool, code, message, path string) {
	status := "pass"
	if !passed {
		status = "fail"
		i.report.StaticChecksPassed = false
		i.diagnostics = append(i.diagnostics, diagnostic.Diagnostic{Code: code, Severity: "error", Message: message, Path: path, Hint: "Inspect the project declarations; use MSBuild separately for evaluation and compilation."})
	}
	i.report.Checks = append(i.report.Checks, Check{id, status, true, message, path})
}

func (i *inspector) unevaluated(id, message string) {
	i.add(id, false, "PROJECT_EVALUATION_REQUIRED", message, i.report.Project)
	i.report.Checks[len(i.report.Checks)-1].Status = "skip"
}

func (i *inspector) literal(key, id string) (string, bool) {
	value := i.properties[strings.ToLower(key)]
	if i.conditional[strings.ToLower(key)] || strings.ContainsAny(value, "$@%") {
		i.unevaluated(id, key+" requires MSBuild evaluation; conditional or expression-based settings are not evaluated.")
		return value, false
	}
	return value, true
}

func (i *inspector) checkProperties() {
	version, versionKnown := i.literal("ESAPIVersion", "project.version")
	i.report.Properties.ESAPIVersionSource = "project"
	if i.options.ESAPIVersion != "" {
		i.report.Properties.ESAPIVersionSource = "argument"
		if versionKnown && version != "" {
			declared, err := scaffold.ResolveVersion(version)
			i.add("project.versionMatch", err == nil && declared.Version == i.options.ESAPIVersion, "PROJECT_ESAPI_VERSION_MISMATCH", "Explicit ESAPI profile must match the declared ESAPIVersion.", i.report.Project)
		}
		version = i.options.ESAPIVersion
	}
	i.report.Properties.ESAPIVersion = version
	var profile scaffold.VersionConfig
	profileKnown := versionKnown && version != ""
	if versionKnown {
		var err error
		if version != "" {
			profile, err = scaffold.ResolveVersion(version)
		}
		profileKnown = version != "" && err == nil
		code := "PROJECT_ESAPI_VERSION_REQUIRED"
		if version != "" && err != nil {
			code = "PROJECT_ESAPI_VERSION_UNSUPPORTED"
		}
		i.add("project.version", profileKnown, code, "A supported ESAPIVersion declaration or --esapi-version is required.", i.report.Project)
		if profileKnown {
			i.report.Properties.ESAPIVersion = profile.Version
		}
	}
	framework, known := i.literal("TargetFrameworkVersion", "project.framework")
	i.report.Properties.TargetFramework = framework
	if known && profileKnown {
		i.add("project.framework", strings.EqualFold(framework, profile.TargetFramework), "PROJECT_FRAMEWORK_MISMATCH", "TargetFrameworkVersion must match "+profile.TargetFramework+" for ESAPI "+profile.Version+".", i.report.Project)
	}
	platform, known := i.literal("PlatformTarget", "project.platform")
	i.report.Properties.Platform = platform
	if known {
		i.add("project.platform", strings.EqualFold(platform, "x64"), "PROJECT_PLATFORM_INVALID", "PlatformTarget must be x64.", i.report.Project)
	}
	assembly, known := i.literal("AssemblyName", "project.assembly")
	i.report.Properties.AssemblyName = assembly
	if known {
		i.add("project.assembly", len(assembly) > len(".esapi") && strings.HasSuffix(strings.ToLower(assembly), ".esapi") && !strings.ContainsAny(assembly, `/\:*?"<>|;`), "PROJECT_ASSEMBLY_NAME_INVALID", "AssemblyName must be a file name ending in .esapi (without .dll).", i.report.Project)
	}
	output, known := i.literal("OutputType", "project.outputType")
	i.report.Properties.OutputType = output
	if known {
		i.add("project.outputType", strings.EqualFold(output, "Library"), "PROJECT_OUTPUT_TYPE_INVALID", "Binary plug-ins require OutputType Library.", i.report.Project)
	}
}

func (i *inspector) checkItems(root element) {
	counts := map[string]int{}
	compileCount := 0
	for _, child := range root.Children {
		switch child.XMLName.Local {
		case "Import":
			path := strings.ReplaceAll(child.attr("Project"), "/", `\`)
			if child.attr("Condition") != "" || (path != `$(MSBuildToolsPath)\Microsoft.CSharp.targets` && path != `$(MSBuildBinPath)\Microsoft.CSharp.targets`) {
				i.unevaluated("project.import", "Custom or conditional imports require MSBuild evaluation; imported files are not read.")
			}
		case "ImportGroup", "Choose", "Sdk":
			i.unevaluated("project.structure", child.XMLName.Local+" requires MSBuild evaluation.")
		case "ItemGroup":
			for _, item := range child.Children {
				conditional := child.attr("Condition") != "" || item.attr("Condition") != ""
				switch item.XMLName.Local {
				case "Compile":
					compileCount++
					file := i.inspectFile(item.attr("Include"), conditional || item.attr("Exclude") != "" || item.attr("Remove") != "" || item.attr("Update") != "", false)
					i.report.Files = append(i.report.Files, file)
				case "Reference":
					name, _, _ := strings.Cut(item.attr("Include"), ",")
					name = strings.TrimSpace(name)
					if name != "VMS.TPS.Common.Model.API" && name != "VMS.TPS.Common.Model.Types" {
						continue
					}
					counts[name]++
					var hint, private string
					for _, metadata := range item.Children {
						if metadata.XMLName.Local == "HintPath" {
							hint = strings.TrimSpace(metadata.Text)
							conditional = conditional || metadata.attr("Condition") != "" || len(metadata.Children) != 0
						}
						if metadata.XMLName.Local == "Private" {
							private = strings.TrimSpace(metadata.Text)
							conditional = conditional || metadata.attr("Condition") != "" || len(metadata.Children) != 0 || strings.ContainsAny(private, "$@%")
						}
					}
					file := i.inspectFile(hint, conditional, true)
					file.Include = name
					if file.Status == "found" {
						i.add("project.referenceName", strings.EqualFold(filepath.Base(file.Path), name+".dll"), "PROJECT_REFERENCE_PATH_INVALID", "HintPath must identify "+name+".dll.", file.Path)
					}
					i.report.References = append(i.report.References, file)
					if !conditional {
						i.add("project.referencePrivate", strings.EqualFold(private, "False"), "PROJECT_REFERENCE_COPY_LOCAL", name+" must declare Private=False to avoid copying vendor DLLs.", i.report.Project)
					}
				}
			}
		}
	}
	for _, name := range []string{"VMS.TPS.Common.Model.API", "VMS.TPS.Common.Model.Types"} {
		i.add("project.reference", counts[name] == 1, "PROJECT_REFERENCE_INVALID", "Exactly one direct reference to "+name+" is required.", i.report.Project)
	}
	i.add("project.compileItems", compileCount > 0, "PROJECT_SOURCE_REQUIRED", "At least one explicit Compile Include source file is required.", i.report.Project)
}

func (i *inspector) inspectFile(value string, conditional, reference bool) File {
	file := File{Include: value, Status: "unverified"}
	path, ok := i.resolvePath(value)
	if conditional || !ok {
		i.unevaluated("project.file", "File path requires MSBuild evaluation or an explicit ESAPI path: "+value)
		return file
	}
	file.Path = path
	if reference {
		if err := scaffold.ValidateESAPIPath(path); err != nil {
			file.Status = "invalid"
			i.add("project.referencePath", false, "PROJECT_REFERENCE_PATH_INVALID", err.Error(), path)
			if detail, ok := err.(*diagnostic.Error); ok {
				i.diagnostics[len(i.diagnostics)-1].Hint = detail.Hint
			}
			return file
		}
	}
	info, err := os.Stat(path)
	file.Status = "missing"
	if err == nil && info.Mode().IsRegular() {
		file.Status = "found"
	} else if err != nil && !os.IsNotExist(err) {
		file.Status = "unverified"
	} else if err == nil {
		file.Status = "invalid"
	}
	code := "PROJECT_SOURCE_UNAVAILABLE"
	if reference {
		code = "PROJECT_REFERENCE_UNAVAILABLE"
	}
	i.add("project.file", file.Status == "found", code, "Referenced file inspection: "+file.Status+" ("+value+").", path)
	return file
}
