# esapi-cli

An AI-friendly CLI for scaffolding, validating, and building ESAPI projects.

**Human-friendly enough, agent-friendly by design.**

ESAPI is the Eclipse Scripting API. The CLI aims to make ESAPI development steps
explicit and reproducible for both developers and coding agents.
Templates are embedded in a single executable. ESAPI binaries, SDK documentation,
and Script Wizard resources are not distributed with this tool.

## Status

The current milestone implements `esapi new`, `--json`, `--dry-run`, and their tests.
`doctor`, `version`, automatic environment detection, and a build command are not
implemented yet.
Validation and build commands are planned capabilities.

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
  External build processes will support cancellation and bounded execution.
- Agent support comes from the CLI contract. Built-in LLM integration is outside
  the v1 scope.

## ESAPI generation profiles

The compatibility goal is ESAPI 15.6 and later. The generation profiles below are
available now; builds and execution against real ESAPI installations are unverified.
Unknown versions are rejected rather than assigned a framework automatically.

| ESAPI profile | Target framework | Platform | Notes |
| --- | --- | --- | --- |
| 15.6 (default) | .NET Framework 4.5 | x64 | Uses the 15.5 framework setting; 15.6 documentation does not state a separate requirement |
| 16.0, 16.1 | .NET Framework 4.6.1 | x64 | Based on reference documentation |
| 17.0, 18.0 | .NET Framework 4.8 | x64 | Based on reference documentation |

`16`, `17`, and `18` are aliases for their `.0` profiles. These are framework
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
  15.5 framework setting. No ESAPI version or installation-path detection is performed yet.
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

## Development

See [docs/DESIGN.md](docs/DESIGN.md) for the intended v1 scope and later roadmap.
Version profiles are centralized in `internal/scaffold/config.go`; templates live
under `internal/scaffold/templates` for Go `embed`.

Next steps are environment diagnostics and inspection (`doctor` / `info`), basic
static project validation (`check`), and the v2 build command. `info --json` will
also report supported capabilities. Read-only inspections must distinguish facts,
failed checks, and checks that could not be performed.

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
