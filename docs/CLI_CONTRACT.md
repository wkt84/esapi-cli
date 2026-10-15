# CLI contract: schema version 1

This contract describes implemented behavior. Commands currently available: `new`,
`info`, `doctor`, `version` (`--version` alias), and help. `check` and `build` are planned.

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
| `data` | object or null | Command result; null for argument/new failures; doctor retains observations on environment failure |
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
| 3 | Failed static environment requirements from `doctor` |
| 4 | Reserved for build failures |

| Diagnostic code | Exit | Meaning |
| --- | --- | --- |
| `UNKNOWN_COMMAND` | 2 | Command is not supported |
| `UNKNOWN_OPTION` | 2 | Option name is not supported |
| `INVALID_ARGUMENTS` | 2 | Missing/extra arguments or invalid flag values |
| `INVALID_PROJECT_NAME` | 2 | Name is invalid or reserved on Windows |
| `UNSUPPORTED_PROJECT_TYPE` | 2 | Only `plugin` is available |
| `UNSUPPORTED_ESAPI_VERSION` | 2 | No profile exists for the requested version |
| `INVALID_ESAPI_PATH` | 2 | Explicit path is invalid or contains a percent sign |
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
