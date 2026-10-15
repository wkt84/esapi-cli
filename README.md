# esapi-cli

An AI-friendly CLI for scaffolding, validating, and building ESAPI projects.

**Human-friendly enough, agent-friendly by design.**

ESAPI is the Eclipse Scripting API. The CLI aims to make ESAPI development steps
explicit and reproducible for both developers and coding agents.
Templates are embedded in a single executable. ESAPI binaries, SDK documentation,
and Script Wizard resources are not distributed with this tool.

## Status

Available commands: `new`, `info`, `doctor`, `check`, `build`, and `version`
(`--version` alias), with structured `--json` output. Scaffolding and build planning
also support `--dry-run`. Environment
inspection is read-only and includes limited, documented known-path discovery.
Static project validation checks conventional plug-in declarations. Windows builds
use the installed MSBuild toolchain with bounded execution and captured logs.

## Design direction

- Commands run without interactive prompts by default and accept explicit options.
- Exit codes distinguish success, invalid arguments, and operational failures;
  diagnostics go to stderr and results go to stdout.
- Generated files stay small and readable, and overwrite behavior is explicit.
- Machine-readable output (`--json`) follows a documented, versioned schema.
  Human-readable messages are not a stable parsing API.
- Read-only commands will not install dependencies or change persistent settings.
  Ambiguous project selection will require an explicit `--project` argument.
- JSON fields, diagnostic codes, and exit codes are compatibility contracts.
  External build processes support cancellation and bounded execution.
- Agent support comes from the CLI contract. Built-in LLM integration is outside
  the v1 scope.

## ESAPI generation profiles

The compatibility goal is ESAPI 15.6 and later. The generation profiles below are
available now. A generated empty 18.0 plug-in has been compiled against DLLs from
the ESAPI installer on a machine without Eclipse. Eclipse execution and other
profiles' builds remain unverified.
Unknown versions are rejected rather than assigned a framework automatically.

| ESAPI profile | Target framework | Platform | Notes |
| --- | --- | --- | --- |
| 15.6 (default) | .NET Framework 4.5 | x64 | Uses the 15.5 framework setting; 15.6 documentation does not state a separate requirement |
| 16.0, 16.1 | .NET Framework 4.6.1 | x64 | Based on reference documentation |
| 17.0, 18.0, 18.1, 19.0 | .NET Framework 4.8 | x64 | Based on reference documentation |

`16`, `17`, `18`, and `19` are aliases for their `.0` profiles. These are framework
profiles for newly generated projects, not a promise of binary compatibility
across Eclipse versions.

## Build and installation

Requires Go 1.27 or later. There are no third-party Go dependencies.

```powershell
go test ./...
go vet ./...
go build -o esapi.exe .
```

Run the resulting executable directly or put it on your `PATH`. All project
templates are embedded; no other CLI files are required at runtime.

## Quick start

```powershell
.\esapi.exe new MyScript
.\esapi.exe new MyScript18 --esapi-version 18 --output .\scripts
.\esapi.exe new MyScript --dry-run --json
.\esapi.exe info --json
.\esapi.exe doctor --esapi-version 18 --json
.\esapi.exe check --project .\MyScript\MyScript.csproj --json
.\esapi.exe build --project .\MyScript\MyScript.csproj --dry-run --json
.\esapi.exe build --project .\MyScript\MyScript.csproj --release --json
.\esapi.exe --version
```

The first command generates:

```text
MyScript/
├── MyScript.csproj
├── Script.cs
├── .gitignore
└── README.md
```

The project uses a conventional, non-SDK-style `.csproj`, targets x64, and produces
`MyScript.esapi.dll`. Its entry point is `VMS.TPS.Script.Execute(ScriptContext)`.
The generated method is empty and patient-data writes are disabled.

## `esapi new`

```text
esapi new <name> [--type plugin] [--output <parent>] [--force]
                 [--esapi-version <version>] [--esapi-path <dll-directory>]
                 [--dry-run] [--json]
```

- Names may contain 1-100 ASCII letters, digits, and underscores, starting with a
  letter or underscore. Windows device names such as `CON` and `NUL` are rejected.
- `plugin` is the only project type.
- `--output` selects the parent directory; the project is created in `<parent>/<name>`.
- An existing destination is an error, even when empty. With `--force`, only the
  four generated files are replaced. Other files and directories are preserved.
  Links and directories at generated-file destinations are refused.
- Templates and all destination types are checked before files are replaced.
  File replacement uses temporary files. A disk or permission error during a
  forced overwrite can leave a mix of old and new generated files; the operation
  is not a transaction across all four files.
- The default is explicitly ESAPI 15.6, targeting .NET Framework 4.5 using the
  15.5 framework setting. `new` does not detect installations or inspect DLLs.
- `--dry-run` validates and renders the same templates, then reports each generated
  file as `create` or `overwrite` without creating even parent directories. It is a
  filesystem snapshot; it does not reserve the destination or test write access.
- `--json` emits one JSON object for success, help, or failure, including malformed
  arguments. It can precede the command or follow its options. See the
  [CLI contract](docs/CLI_CONTRACT.md) for the schema and diagnostic codes.
- Exit codes: `0` success/help, `1` filesystem or other operational error,
  `2` invalid arguments.

Options work both before and after the project name. Use `esapi new --help` for help.

## Requirements and `ESAPI_HOME`

Generating a project does not require the ESAPI SDK or .NET tools. Building the
generated project requires Windows, Visual Studio / MSBuild, the selected .NET
Framework targeting pack, and a separately installed matching ESAPI SDK.
A modern .NET SDK alone does not replace the .NET Framework targeting pack.
Eclipse is not required on the build machine; running the plug-in is a separate
step on a compatible Eclipse installation.

Set `ESAPI_HOME` to the directory directly containing both ESAPI DLLs, typically
the SDK's `API` subdirectory. It is read by MSBuild when building, so the same
generated project can be used on different machines.

```powershell
$env:ESAPI_HOME = 'C:\path\to\ESAPI\API'
MSBuild.exe .\MyScript\MyScript.csproj /p:Configuration=Release
```

To embed an explicit default path instead:

```powershell
.\esapi.exe new MyScript --esapi-path 'C:\path\to\ESAPI\API'
```

The CLI makes an explicitly supplied path absolute. That project property takes
precedence over the build environment's `ESAPI_HOME`; an MSBuild command-line
property `/p:ESAPI_HOME="C:\other\API"` overrides either. Prefer `ESAPI_HOME` for
projects shared across machines. Paths are not required to exist during generation.

DLL-directory paths containing `%` are rejected because the MSBuild reference
pipeline can decode percent sequences more than once and resolve the wrong path.
Use a DLL directory without percent signs, including when setting `ESAPI_HOME`.

References use `Private=False` to prevent copying ESAPI DLLs to the output directory.
The generated project reports missing DLLs at build time. It never downloads an SDK.

## `esapi info` and `esapi doctor`

```powershell
.\esapi.exe info --esapi-version 18 --json
.\esapi.exe doctor --esapi-version 18 --json
.\esapi.exe doctor --esapi-version 15.6 --esapi-path 'C:\path\to\ESAPI\API'
```

Both commands inspect OS/architecture, tool files, adjacent .NET SDK markers,
the selected framework's reference-assembly markers, and the two ESAPI DLLs.
They do not execute tools, load assemblies, use registry settings, install files,
or modify projects. `info` returns observations and supported CLI capabilities even
when dependencies are absent. `doctor` evaluates static requirements and exits `3`
when they fail; its JSON retains the checks and observations on failure.

ESAPI path precedence is `--esapi-path`, then `ESAPI_HOME`, then known paths for the
selected profile: `%ProgramFiles(x86)%` / `%ProgramFiles%` / `%ProgramW6432%` under
`Varian\RTM\<version>\esapi\API`. Invalid explicit/environment paths do not silently
fall back. Multiple discovered directories require explicit selection. This search
is limited; a missing candidate does not prove ESAPI is not installed elsewhere.

Tools use `--dotnet-path` / `--msbuild-path`, then PATH, then standard Windows
installation paths. MSBuild discovery covers Visual Studio `Current` / `15.0`
layouts and prefers `Bin\amd64` within one installation. Multiple installations
are reported as ambiguous. A separate .NET SDK is optional when MSBuild is present;
a dotnet runtime host without SDK markers does not satisfy the toolchain check.

Targeting-pack discovery checks `mscorlib.dll` and `RedistList\FrameworkList.xml`
under the selected `.NETFramework\vX.Y` reference-assembly directory. A runtime or
an empty framework directory does not satisfy this check. `--targeting-pack-path`
inspects a custom directory only; it does not configure MSBuild or generated projects.

DLL inspection reports PE32 / PE32+, the machine field, DLL characteristics, and
the CLR header declaration. It does not derive an installed ESAPI version or infer
x86-only execution from PE32/I386. Assembly identity, CLR flags, actual tool
execution, compilation, licensing, and Eclipse execution remain explicitly unverified.
Passing `doctor` means its static checks passed, not that a script can run.

See [real-machine verification](docs/REAL_MACHINE_TEST.md) for the next testing steps
and the [CLI contract](docs/CLI_CONTRACT.md) for fields and diagnostics.

## `esapi check`

```powershell
.\esapi.exe check --project .\MyScript\MyScript.csproj --json
.\esapi.exe check --project .\Existing\Existing.csproj --esapi-version 15.6
.\esapi.exe check --project .\MyScript\MyScript.csproj --esapi-path 'C:\path\to\ESAPI\API'
```

Without `--project`, the current directory must contain exactly one regular
`.csproj` file. Discovery is not recursive and excludes symlinks. Multiple candidates
are returned in sorted order with `PROJECT_AMBIGUOUS`; select one explicitly.
An explicit file may be a symlink to a regular project.

The checker reads conventional non-SDK UTF-8 project XML (up to 4 MiB). It checks
the declared ESAPI profile and target framework, `PlatformTarget=x64`, `OutputType=Library`,
an assembly name ending in `.esapi`, exactly one reference to each ESAPI DLL with
`Private=False`, and the existence of both referenced DLL files and explicit
`Compile Include` source files. File existence is not assembly-identity validation.
`--esapi-version` supplies the expected profile when `ESAPIVersion` is absent;
if both are present, they must agree. It does not infer a profile from installed DLLs.

Paths are relative to the project directory. Supported path substitutions are
`$(MSBuildProjectDirectory)` and `$(ESAPI_HOME)`. `ESAPI_HOME` comes from the inspection
override `--esapi-path`, then the project declaration, then the environment. The
generated environment-reading expression is recognized without executing property
functions. Literal MSBuild percent escapes are decoded once. A relative
`--esapi-path` is resolved against the CLI's working directory; the override does
not modify the project or configure a subsequent build.

Conditional checked settings/items, globs, unknown expressions, and custom imports
return `PROJECT_EVALUATION_REQUIRED`. Such required checks remain `skip` and prevent
success. The checker does not implement MSBuild evaluation: implicit imports such
as `Directory.Build.targets`, target bodies, other dependency types, C# entry-point
semantics, compilation, and Eclipse execution remain unverified. Use MSBuild for
evaluation and compilation and `doctor` separately for environment/header inspection.
Passing means only that the documented declaration checks passed.

Exit codes are `0` for passing checks/help, `1` for operational errors, `2` for
invalid arguments or project selection, and `5` for failed or unevaluable project
requirements. Reports and candidate lists remain in JSON `data` on selection,
inspection, and validation failures; malformed arguments return `data: null`.
In human mode the report goes to stdout and diagnostics to stderr. Inspection
does not modify files, invoke tools, or load assemblies.

## `esapi build`

```powershell
.\esapi.exe build --project .\MyScript\MyScript.csproj --dry-run --json
.\esapi.exe build --project .\MyScript\MyScript.csproj --esapi-path 'C:\path\to\ESAPI\API' --json
.\esapi.exe build --project .\MyScript\MyScript.csproj --debug --clean --timeout 2m
```

Builds require Windows amd64 (Windows 10 or later for process-tree management).
Project selection follows `check`: exactly one regular `.csproj` in the current
directory, or an explicit `--project`. No recursive selection or prompt occurs.
An explicit `--msbuild-path` or `--dotnet-path` selects the tool; these flags are
mutually exclusive. Otherwise native MSBuild is preferred using the same discovery
as `info`. A missing native tool may fall back to `dotnet msbuild` with adjacent SDK
markers. An invalid or ambiguous selected tool fails without choosing a different one.
MSBuild's evaluation, including imports, targets, and dotnet SDK/global.json selection,
remains authoritative. `check` and `doctor` are independent optional checks, so a
conditional project is not rejected merely because static inspection needs evaluation.

The default configuration is Release; `--debug` selects Debug. `--release` and
`--debug` cannot both be enabled. `--clean` runs the MSBuild Rebuild target, which
cleans and builds; it does not implement custom directory deletion. The invocation
sets x64, one build node, disabled node reuse/shared compilation, and compiler
`Deterministic=true`. The toolchain/project/inputs still determine reproducibility.
Automatic response-file loading is disabled. The CLI does not request restore,
installation, or downloads; project/import/target code is executed by MSBuild and
may perform additional work. Build outputs and tool caches are normal side effects.

`--esapi-path` sets the global MSBuild `ESAPI_HOME` property for this build, overriding
project and environment values. The path is made absolute and escaped for MSBuild;
percent-containing paths remain unsupported. Without it, project/environment binding
applies; build does not silently choose an installed ESAPI release. The CLI does not
persist environment changes. Its explicit child-only environment overrides disable
dotnet telemetry/welcome messages/workload update notifications, MSBuild server
forwarding, and node reuse; `--dry-run` lists the overrides without exposing the
inherited environment. See Microsoft's [CLI environment variable reference](https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-environment-variables).

`--timeout` accepts a positive Go duration up to 24h (default 5m). Ctrl+C cancels the
build. Windows creates the tool directly in a job that terminates its descendant
processes on timeout, interruption, or completion, including if the parent tool exits
before a child. Cleanup/draining may take up to two additional 2-second waits after
the execution timeout. `BUILD_CLEANUP_FAILED` reports cleanup problems separately.
Completed/partial build outputs are retained on failure or cancellation.

`--dry-run` reports the executable, argument array, working directory, configuration,
target, timeout, and child environment overrides without running tools or writing
files. It inspects file presence only; it cannot prove a tool will start or build.
Both output streams are captured and drained, retaining the first 1 MiB of bytes
per stream with truncation flags. Logs are returned after execution, including in
human mode; `--verbose` requests detailed MSBuild logs. The CLI does not parse
localized compiler messages as a stable API.

JSON still emits exactly one object. `data` retains the plan, status, tool exit code,
and logs on failure. Exit codes: `0` success/planned, `1` launch/process I/O failure,
`2` invalid arguments/project selection, `3` unsupported host/unavailable tool,
`4` tool failure, timeout, or cancellation. See the [CLI contract](docs/CLI_CONTRACT.md).

## Version and build identity

`esapi version` and `esapi --version` report the CLI's version, not the ESAPI SDK's.
JSON also includes the commit, dirty-worktree indicator, and Go version when available.
Development builds use `0.1.0-dev`. Release builders can set the version/commit using
Go linker `-X` flags targeting `internal/buildinfo.Version` and `internal/buildinfo.Commit`
(fully qualified with the module path). No wall-clock build date is embedded.

## Development

See [docs/DESIGN.md](docs/DESIGN.md) for the intended v1 scope and later roadmap.
Version profiles are centralized in `internal/scaffold/config.go`; templates live
under `internal/scaffold/templates` for Go `embed`.

The generation, inspection, validation, and build workflow is implemented; Eclipse
execution and additional project types remain separate roadmap items.
Read-only inspections distinguish observations, failed checks, and checks that
could not be performed. Tests use independent metadata/PE-header fixtures; vendor
DLLs are never required in CI.

Internal reference material in `.local/` is ignored by Git and must not be added
to commits or release artifacts. Tests use temporary directories and do not
require proprietary ESAPI files.

## Disclaimer

NOT FOR CLINICAL USE. This CLI is a development aid. It does not guarantee clinical
validation of generated code.

This project is not affiliated with or endorsed by Varian Medical Systems or
Siemens Healthineers.

## License

The independently implemented CLI source and templates are provided under the
[MIT License](LICENSE). This does not license or redistribute the ESAPI SDK.
