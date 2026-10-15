# CLI contract: schema version 1

This contract describes implemented behavior. Commands currently available: `new`,
`info`, `doctor`, `check`, `build`, `version` (`--version` alias), and help.

## Invocation and streams

Commands do not prompt. Required inputs come from arguments and options. Generated
projects may read `ESAPI_HOME` when MSBuild runs; project generation does not inspect
or capture that environment variable.

Use `--json` before the command or among its options. `--json=true` and
`--json=false` are supported; the last value selects the format. The `--` delimiter
ends option interpretation. A value embedded in another option, such as
`--output=--json`, does not select JSON mode.

In JSON mode, stdout contains exactly one JSON object followed by a newline. This
includes argument errors and command failures. No progress messages, ANSI formatting,
or extra human-readable text are emitted. Diagnostics are in the JSON object and
stderr is empty. If writing stdout itself fails, the process exits with code 1 and
may explain the stream failure on stderr; a complete JSON object cannot be guaranteed.

In human-readable mode, success/help goes to stdout and errors/hints to stderr.
Human-readable wording is not a stable parsing interface.

## Common envelope

| Field | Type | Meaning |
| --- | --- | --- |
| `schemaVersion` | integer | Currently `1` |
| `command` | string | Requested command; root help uses `help` |
| `ok` | boolean | Whether the operation succeeded; agrees with exit code 0 |
| `data` | object or null | Command result; null for argument/new failures; doctor/check/build retain reports on command failure |
| `diagnostics` | array | Empty on current successful operations; structured errors on failure |

Help uses `data.usage` containing the help text, including `new --help --json`.
The help text itself is human-readable and may change.

Each diagnostic has required string fields `code`, `severity`, and `message`.
Current error severity is `error`. Optional string fields `path` and `hint` identify
the affected path and a possible next step. Agents should branch on codes rather
than parse messages, hints, or operating-system error text.

Consumers should ignore additional object fields and handle unknown diagnostic
codes as a general failure. Existing field types, meanings, and diagnostic codes
are stable within this schema version; incompatible changes increment the version.
Array order for `new.data.files` is stable for the current template version.

## `new` and dry-run

```powershell
esapi new MyScript --dry-run --json
```

Example result (the absolute output path depends on the working directory):

```json
{
  "schemaVersion": 1,
  "command": "new",
  "ok": true,
  "data": {
    "path": "C:\\work\\MyScript",
    "name": "MyScript",
    "type": "plugin",
    "esapiVersion": "15.6",
    "targetFramework": "v4.5",
    "platform": "x64",
    "esapiPath": "",
    "esapiPathSource": "environment",
    "files": [
      {"path": "MyScript.csproj", "action": "create"},
      {"path": "Script.cs", "action": "create"},
      {"path": ".gitignore", "action": "create"},
      {"path": "README.md", "action": "create"}
    ],
    "dryRun": true
  },
  "diagnostics": []
}
```

`data.path` is the absolute project directory. File paths are relative to that
directory. Actions are `create` or `overwrite`; unrelated files are not listed.
Version aliases are normalized, e.g. `18` becomes `18.0`.

`esapiPathSource` is `argument` when `--esapi-path` supplies a default absolute DLL
directory. Otherwise it is `environment`, `esapiPath` is empty, and lookup is
deferred to MSBuild. This describes the generated reference binding, not detection
of an installed SDK. No SDK is required for generation or dry-run.

Dry-run performs the same input validation, template rendering, and destination
inspection as normal generation, without writing files, creating directories, or
probing write access with temporary files. A conflict fails with `data: null` and
a diagnostic, rather than returning a partially inspected plan. A successful plan
does not reserve paths or guarantee that execution will have permission to write.
Normal execution revalidates before writing.

Successful normal execution returns the same result shape with `dryRun: false`.
File contents are deterministic for the CLI/template version, project name,
selected profile, and explicit DLL path. A relative explicit DLL path is resolved
against the working directory. Files do not depend on generation time or random
identifiers. Filesystem changes between planning and execution can change actions.

## Exit codes and diagnostic codes

| Exit | Meaning |
| --- | --- |
| 0 | Success or requested help, including a successful dry-run |
| 1 | Filesystem / operational failure, including destination conflicts |
| 2 | Invalid command, arguments, project name, type, version, or DLL path |
| 3 | Failed static environment requirements from `doctor`, or build host/tool selection failure |
| 4 | Build tool failure, timeout, or cancellation |
| 5 | Failed or unevaluable static project requirements from `check` |

| Diagnostic code | Exit | Meaning |
| --- | --- | --- |
| `UNKNOWN_COMMAND` | 2 | Command is not supported |
| `UNKNOWN_OPTION` | 2 | Option name is not supported |
| `INVALID_ARGUMENTS` | 2 | Missing/extra arguments or invalid flag values |
| `INVALID_PROJECT_NAME` | 2 | Name is invalid or reserved on Windows |
| `UNSUPPORTED_PROJECT_TYPE` | 2 | Only `plugin` is available |
| `UNSUPPORTED_ESAPI_VERSION` | 2 | No profile exists for the requested version |
| `INVALID_ESAPI_PATH` | 2 | SDK argument is invalid or contains percent signs, literal double quotes, or control characters |
| `DESTINATION_EXISTS` | 1 | Existing directory requires explicit overwrite permission |
| `DESTINATION_CHANGED` | 1 | Destination changed during execution; inspect and retry |
| `UNSAFE_DESTINATION` | 1 | Destination is a link, directory collision, or special file |
| `OUTPUT_ACCESS_FAILED` | 1 | Cannot resolve or inspect the output location |
| `SCAFFOLD_RENDER_FAILED` | 1 | Embedded template could not be rendered |
| `SCAFFOLD_WRITE_FAILED` | 1 | File creation/replacement failed |

Normal `--force` overwrites only the four generated files and preserves unrelated
files. Replacement is atomic per file, not across the whole project. On a write
failure, previously replaced generated files may already be updated. Inspect the
destination before retrying; `--force` is always an explicit choice.

## `info`, `doctor`, and version

`info`/`doctor` accept `--esapi-version` (default `15.6`), `--esapi-path`,
`--dotnet-path`, `--msbuild-path`, and `--targeting-pack-path`, plus help/JSON flags.
Profiles are normalized using the same table as `new`. Tool/path flags identify
local files for inspection only; they do not execute or configure the tools.
SDK directory arguments with literal double quotes, control characters, or percent
signs return `INVALID_ESAPI_PATH` / exit 2 with null data. A malformed `ESAPI_HOME`
environment value remains an `invalid` path observation rather than an argument
error; it does not silently fall back to discovery.
Known installation discovery is Windows-only, bounded to the layouts documented
in the README, and does not inspect registry settings or scan arbitrary disks.

`info.data` contains:

- `cli`: `version`, `commit`, `modified` (boolean), and `goVersion`.
- `capabilities`: `commands` (`name`, `json`, `dryRun`, `readOnly`), sorted `profiles`
  (`version`, `targetFramework`, `platform`), and `schemaVersions` (currently `[1]`).
- `environment`: `os`, `arch`, selected `profile`, `dotnet`, `msbuild`,
  `targetingPack`, `esapi`, and ordered `dlls` (API then Types).

The environment's profile is the requested generation profile, not a detected DLL
version. `info` exits 0 when it can report observations, including missing/invalid
components. Invalid arguments still exit 2 with `data: null`.

Path facts contain `path` (an absolute selected/attempted path when available;
empty when no candidate is selected), `source`
(`argument`, `environment`, `path`, or `discovery`), `status`, `candidates` (array),
and optional `issue`. Status values are `found`, `missing`, `invalid`, `ambiguous`,
and `unverified`. `missing` means the limited probes found no candidate; it is not
an exhaustive assertion that no installation exists. Unusable explicit/environment
paths do not fall back. Ambiguous discovery leaves `path` empty and lists candidates.

Tool facts add `sdks` (array of directory names with MSBuild.dll markers) and
`sdkStatus` (`found`, `missing`, `unverified`, or `notApplicable` for native MSBuild).
SDK markers do not prove which SDK a project/global.json would select. Targeting-pack
facts add `missingFiles`; unreadable marker metadata is reported as unverified.

DLL facts contain `name`, `path`, `status`, `pe` (object or null), and optional
`code`/`issue`. `pe` contains `machine` (`amd64`, `i386`, `arm64`, or hexadecimal),
`format` (`PE32`/`PE32+`), `isDll`, and `hasClrHeader`. Header inspection checks only
the declaration of the CLR directory; it does not validate its metadata, assembly
identity/version, CLR flags, or runtime bitness. `found` is a structural observation,
not an assertion of SDK authenticity or compatibility. Assemblies are never loaded.

`doctor.data` contains `environment`, `checks`, and `staticChecksPassed` (boolean).
Each check has `id`, `status` (`pass`, `fail`, `skip`), `required` (boolean), `message`,
and optional `path`. A failed required check sets `ok: false`, exit 3, and adds a
diagnostic. The complete report remains in `data` on environment failure; this is
specific to the newly introduced doctor command and does not change `new` results.
Unperformed tool execution, assembly identity, and Eclipse execution checks are
`skip` with `required: false`. A passing report cannot establish runtime readiness.
In human mode the report goes to stdout even on exit 3; error diagnostics go to stderr.

Environment diagnostics use these codes (exit 3):

| Code | Meaning |
| --- | --- |
| `UNSUPPORTED_OS` | Supported development host requires Windows |
| `UNSUPPORTED_HOST_ARCHITECTURE` | Supported development host requires amd64 |
| `BUILD_TOOLCHAIN_NOT_FOUND` | No native MSBuild or dotnet SDK candidate selected |
| `MSBUILD_UNAVAILABLE` | Explicit MSBuild executable is unavailable |
| `DOTNET_SDK_UNAVAILABLE` | Explicit dotnet path lacks a usable host/SDK marker |
| `TARGETING_PACK_UNAVAILABLE` | Selected framework's reference markers are unavailable |
| `ESAPI_PATH_UNAVAILABLE` | ESAPI directory is missing, unusable, or ambiguous |
| `ESAPI_API_DLL_NOT_FOUND` / `ESAPI_TYPES_DLL_NOT_FOUND` | Required file missing |
| `ESAPI_API_DLL_INVALID` / `ESAPI_TYPES_DLL_INVALID` | Wrong file type or PE/CLR-header declaration |
| `ESAPI_API_DLL_UNREADABLE` / `ESAPI_TYPES_DLL_UNREADABLE` | File could not be inspected |

`version`/`--version` return the same CLI identity fields as `info.data.cli`, without
environment inspection. `commit`/`goVersion` may be empty when unavailable. No build
timestamp is returned. Human output is `esapi <version>`.

## `check`

`check` accepts `--project <file>`, optional `--esapi-version <profile>` and
`--esapi-path <directory>`, plus help/JSON flags. There are no positional arguments.
No profile defaults to 15.6 here: the project must declare `ESAPIVersion` or the
caller must select one. An explicit profile must match a present declaration.

Without `--project`, selection considers only regular `.csproj` files (extension
case-insensitive) in the current directory. Symlinks are excluded from discovery;
an explicit path may resolve through a symlink to a regular file. Zero or multiple
candidates exit 2; multiple candidates never select a project. Candidate order is
filename order, and paths are absolute. Selection does not recurse.

`check.data` contains:

- `project`: absolute selected path, or empty when selection failed.
- `candidates`: array of absolute candidate paths; explicit selection lists its file.
- `properties`: `esapiVersion`, `esapiVersionSource` (`project` or `argument`),
  `targetFramework`, `platform`, `assemblyName`, and `outputType`. These are top-level
  declarations, not evaluated MSBuild properties; unsupported expressions may remain
  as raw strings. A supported profile is normalized to its `.0` alias when appropriate.
  Fields may be empty when parsing/inspection could not reach them.
- `references`: ESAPI references in declaration order; each has `include` (assembly
  name), `path` (resolved path or empty), and `status`.
- `files`: explicit Compile items in declaration order, with `include` (original
  expression), `path`, and `status`.
- `checks`: ordered checks using `id`, `status`, `required`, `message`, optional `path`.
  Status is `pass`, `fail`, or `skip`. IDs may repeat for file/reference checks;
  identify occurrences by their path/message and diagnostics, not unique IDs.
- `staticChecksPassed`: boolean. Required `fail` or `skip` prevents success.

File statuses are `found` (regular file), `missing`, `invalid` (non-regular file),
and `unverified` (unresolved expression, unreadable path, or a Windows absolute path
on a non-Windows host). File existence does not establish assembly identity/version.
Inspection errors and selection errors retain the report in `data`, including empty
arrays and `staticChecksPassed: false`. Malformed options/profile/path arguments
return `data: null`. JSON stderr stays empty. In human mode reports go to stdout and
diagnostics to stderr, including on failure.

The scope and path substitutions are documented in the README. The checker does
not evaluate MSBuild or read imported files; it does not execute property functions,
tools, target bodies, or assemblies. The generated `ESAPI_HOME` environment expression
is matched as text and reads the process environment only. `--esapi-path` is an
inspection override, not a persistent setting or a build argument. Actual build
evaluation, implicit imports, assembly metadata, and runtime verification remain
an optional `skip` check. Unsupported required declarations cause exit 5, never a
false passing report.

| Diagnostic code | Exit | Meaning |
| --- | --- | --- |
| `PROJECT_NOT_FOUND` | 2 | No candidate or selected file does not exist |
| `PROJECT_AMBIGUOUS` | 2 | Multiple current-directory candidates require `--project` |
| `PROJECT_PATH_INVALID` | 2 | Selected path is not a regular `.csproj` file |
| `PROJECT_ACCESS_FAILED` | 1 | Project directory/file cannot be resolved, inspected, or read |
| `PROJECT_XML_INVALID` | 5 | XML cannot be parsed as UTF-8 or exceeds 4 MiB |
| `PROJECT_FORMAT_UNSUPPORTED` | 5 | Root/namespace or SDK format is unsupported |
| `PROJECT_EVALUATION_REQUIRED` | 5 | Required setting/item needs MSBuild evaluation |
| `PROJECT_ESAPI_VERSION_REQUIRED` | 5 | No project profile or explicit profile was supplied |
| `PROJECT_ESAPI_VERSION_UNSUPPORTED` | 5 | Declared profile is unsupported |
| `PROJECT_ESAPI_VERSION_MISMATCH` | 5 | Explicit profile disagrees with declaration |
| `PROJECT_FRAMEWORK_MISMATCH` | 5 | Declared framework differs from the profile |
| `PROJECT_PLATFORM_INVALID` | 5 | PlatformTarget is not x64 |
| `PROJECT_ASSEMBLY_NAME_INVALID` | 5 | AssemblyName does not have the plug-in output convention |
| `PROJECT_OUTPUT_TYPE_INVALID` | 5 | OutputType is not Library |
| `PROJECT_REFERENCE_INVALID` | 5 | Required direct ESAPI reference is missing or duplicated |
| `PROJECT_REFERENCE_PATH_INVALID` | 5 | Reference path has invalid SDK syntax or points to a different filename |
| `PROJECT_REFERENCE_UNAVAILABLE` | 5 | ESAPI reference path is not an accessible regular file |
| `PROJECT_REFERENCE_COPY_LOCAL` | 5 | ESAPI reference lacks explicit Private=False |
| `PROJECT_SOURCE_REQUIRED` | 5 | No explicit Compile item exists |
| `PROJECT_SOURCE_UNAVAILABLE` | 5 | Compile path is not an accessible regular file |

## `build`

`build` accepts `--project`, `--msbuild-path`, `--dotnet-path`, `--esapi-path`,
`--release`, `--debug`, `--clean`, `--verbose`, `--timeout`, `--dry-run`, and help/JSON.
There are no positional arguments or arbitrary pass-through MSBuild switches.
Native/dotnet explicit paths conflict, as do enabled release/debug flags. Timeout
must be greater than zero and at most 24h (default 5m). Invalid flags return exit 2
with `data: null`. Project selection uses the same rules/codes as `check` and retains
candidate paths in `data`; other plan fields may be empty/null before selection.

Build planning requires Windows amd64. Windows process creation requires JOB_LIST
support (Windows 10+); unavailable process-management APIs fail launch with exit 1.
Native MSBuild is preferred; missing native MSBuild permits a dotnet SDK fallback.
Explicit invalid tools, ambiguous native discovery, runtime-only dotnet hosts, and
unsupported hosts fail planning with exit 3 and retain partial plan/tool facts.
Discovered files are observations, not proof of successful tool execution. There is
no automatic retry using another tool. `check`/`doctor` do not gate this command.

`build.data` contains:

- `project` / `candidates`: project selection facts, as in `check`.
- `tool`: `kind` (`msbuild`/`dotnet`), plus the path/tool facts used by `info`
  (`path`, `source`, `status`, `candidates`, `sdks`, `sdkStatus`, optional `issue`).
- `arguments`: ordered argv strings, excluding the executable. Dotnet's first
  argument is `msbuild`. Paths are arguments, not a shell command.
- `workingDirectory`: the selected project's directory.
- `configuration`: `Release` by default or `Debug`.
- `target`: `Build`, or `Rebuild` for `--clean`.
- `esapiPath`: absolute global-property override from `--esapi-path`, otherwise empty.
  The corresponding argument uses MSBuild percent escaping; the path field is literal.
- `timeout`: normalized Go duration string for execution, excluding discovery/cleanup.
- `environmentOverrides`: child-only `NAME=value` overrides. Other process environment
  values are inherited and are not enumerated. No persistent settings change.
- `status`: `planned`, `notStarted`, `succeeded`, `failed`, `timedOut`, or `cancelled`.
- `started`: whether a tool process was successfully created.
- `exitCode`: tool exit integer, or null when none was observed. This is distinct
  from the CLI exit code, including on Windows forced process termination.
- `stdout` / `stderr`: captured tool logs. Each retains at most 1 MiB of source bytes;
  invalid UTF-8 sequences are replaced with U+FFFD. JSON escaping/replacement may
  make the encoded JSON larger. No compiler-message parsing contract is provided.
- `stdoutTruncated` / `stderrTruncated`: whether bytes beyond the limit were discarded.
- optional `cleanupError`: details when tree termination or pipe draining failed.

A successful dry-run has status `planned`, `started: false`, `exitCode: null`, empty
logs, and no tool execution/file writes. Actual execution retains the same invocation
fields. Tool failures/timeouts/cancellation retain observations and partial logs.
The wrapper requests no restore; MSBuild/project code can still have its own side
effects. x64, deterministic compilation, single-node builds, disabled automatic
response files, node reuse, and shared compilation are explicit invocation arguments.
The child-only overrides shown in the plan suppress dotnet telemetry/welcome
messages and workload update notifications, and disable MSBuild server forwarding
and node reuse. SDK/global.json,
imports, targets, and project dependencies are still evaluated by the selected tool.

Ctrl+C cancels the execution context. The process and descendants are terminated,
and logs are drained; the process wait and log drain each have a 2-second cleanup
bound. Normal completion also terminates remaining descendants. Windows uses
creation-time job assignment and kill-on-close, avoiding a process-assignment race.
An additional `BUILD_CLEANUP_FAILED` diagnostic reports termination/drain errors.
Exit 4 is retained on timeout/cancellation even if cleanup also fails; infrastructure
errors otherwise exit 1. Completed/partial build outputs are not rolled back.
The product build command is Windows-only; Unix process fixtures run in Linux CI.

In human mode a report and both captured tool streams go to stdout after completion;
wrapper diagnostics go to stderr. JSON mode always contains exactly one object on
stdout and no tool output on the CLI stderr stream.

| Diagnostic code | Exit | Meaning |
| --- | --- | --- |
| `BUILD_HOST_UNSUPPORTED` | 3 | Build host is not Windows amd64 |
| `BUILD_TOOL_UNAVAILABLE` | 3 | Selected executable/SDK is unavailable or ambiguous |
| `BUILD_START_FAILED` | 1 | Tool/process-tree setup could not start a process |
| `BUILD_PROCESS_FAILED` | 1 | Process waiting or log/termination infrastructure failed |
| `BUILD_FAILED` | 4 | Tool returned a nonzero exit code; inspect logs |
| `BUILD_TIMEOUT` | 4 | Execution exceeded the selected duration |
| `BUILD_CANCELLED` | 4 | Caller context or Ctrl+C interrupted execution |
| `BUILD_CLEANUP_FAILED` | 1 or 4 | Additional termination/drain error; accompanies primary failure |
