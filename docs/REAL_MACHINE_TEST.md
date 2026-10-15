# Real-machine verification

Environment inspection is ready for testing on an ESAPI development workstation.
The commands below inspect files and headers without starting Eclipse, loading SDK
assemblies, accessing patient data, or changing persistent system settings.
Inspection and compilation can run on an SDK-only machine with DLLs installed by
the ESAPI installer. Eclipse is not required for those steps. Runtime verification
on an Eclipse workstation is tracked separately.

## 1. Check observations and diagnostics

Copy the single Windows amd64 executable to the workstation, or build it with
`go build -o esapi.exe .`. Select the installed ESAPI release explicitly; the CLI
defaults to the 15.6 generation profile.

```powershell
$esapiVersion = '18' # Choose 15.6, 16.0, 16.1, 17, 18, 18.1, or 19 as installed.
.\esapi.exe --version --json
.\esapi.exe info --esapi-version $esapiVersion --json
.\esapi.exe doctor --esapi-version $esapiVersion --json
$LASTEXITCODE
```

Expected: `info` exits 0 and reports facts even if dependencies are absent.
`doctor` exits 0 when required static checks pass, or 3 with precise diagnostic
codes when they fail. Checks that were not performed remain `skip`.

Confirm these observations against the workstation:

- ESAPI directory and its source (`argument`, `environment`, or `discovery`).
- Both DLL paths and PE header observations.
- MSBuild/dotnet paths and SDK markers. Native MSBuild can satisfy the toolchain
  requirement without a separate .NET SDK.
- Targeting-pack path for the selected profile: 15.6 uses Framework 4.5,
  16.0/16.1 uses 4.6.1, and 17.0/18.0/18.1/19.0 uses 4.8. An installed runtime
  alone does not imply that the corresponding reference assemblies are available.

If known-path discovery misses a custom installation or reports multiple candidates:

```powershell
.\esapi.exe info --esapi-version $esapiVersion --esapi-path 'D:\path\to\ESAPI\API' --json
.\esapi.exe doctor --esapi-version $esapiVersion --esapi-path 'D:\path\to\ESAPI\API' --json
```

The directory must directly contain both DLLs. Explicit paths override `ESAPI_HOME`;
an invalid explicit/environment path does not silently select a different SDK.
Use `--msbuild-path` / `--dotnet-path` for custom tools. `--targeting-pack-path`
inspects a custom reference directory only and does not configure the subsequent build.

Useful feedback is the CLI version/commit, selected release, expected versus actual
paths, exit code, and diagnostic codes. JSON contains local filesystem paths;
review it before sharing. Do not add vendor DLLs or documentation to this repository.

## 2. Empty plug-in build

After checking the observations, compile the generated empty plug-in using the
workstation's installed tools. This tests references and project settings without
executing the generated script. Use a new output directory; no `--force` is needed.

```powershell
$sdkPath = 'D:\path\to\ESAPI\API'
.\esapi.exe new SmokeTest --esapi-version $esapiVersion --esapi-path $sdkPath --output .\smoke-output
.\esapi.exe check --project .\smoke-output\SmokeTest\SmokeTest.csproj --json
.\esapi.exe build --project .\smoke-output\SmokeTest\SmokeTest.csproj --dry-run --json
.\esapi.exe build --project .\smoke-output\SmokeTest\SmokeTest.csproj --release --timeout 2m --json
$LASTEXITCODE
```

Use `--msbuild-path` / `--dotnet-path` to select a tool explicitly if needed.
Expected: build exit 0 with `status: succeeded`, `started: true`, tool `exitCode: 0`,
and captured logs. Native MSBuild is preferred. A custom/project conditional setting
may be unevaluable by `check` while MSBuild can still build it.
`check` should exit 0 for the generated project with existing DLLs and source files.
It does not verify the targeting pack or compile code; compare `doctor` separately.
Confirm `bin\Release\SmokeTest.esapi.dll` exists and neither ESAPI DLL was copied
to that output directory. A failure should be compared with the preceding doctor
report; static checks intentionally do not execute compilers or choose a project SDK.

## 3. Eclipse execution is a separate verification

CLI diagnostics and compilation do not establish that Eclipse can discover or run
the plug-in. Testing that requires an appropriate non-clinical development setup and
its normal script-validation process. The CLI does not launch Eclipse or register
scripts. Do not treat a passing doctor report as evidence of clinical validation.

## Verification performed during development

The local development machine has ESAPI 15.6 and 18.0 DLLs installed using the
ESAPI installer, and does not have Eclipse installed. The available Eclipse 15.6
environment is on a separate Windows 10/11 64-bit workstation; its reported doctor
results are recorded below. Build and Eclipse execution there remain unverified.

On 2026-10-15, local Windows checks found ESAPI 18.0 DLLs, native Visual Studio
MSBuild, and the Framework 4.8 targeting pack. The generated empty 18.0 plug-in
compiled successfully against those DLLs, and vendor DLLs were not copied to output.
The 15.6 DLL path was also found; its doctor run correctly failed because the local
Framework 4.5 reference-assembly markers were missing. The later `esapi build` run
for 15.6 failed with CLI exit 4 / `BUILD_FAILED`, preserving MSBuild's `MSB3644`
missing-reference-assemblies error in the tool logs.
No Eclipse execution was performed.
The `check` command also passed on generated 15.6 and 18.0 projects referencing
the installed DLL files. That checks declarations and file existence only; it
does not change the missing 4.5 targeting-pack result or establish build readiness.
The 18.1 and 19.0 generation profiles follow their reference guides' Framework 4.8
requirements; matching SDK builds and Eclipse execution have not been tested.

`esapi build` also compiled the empty 18.0 project using native MSBuild in Release
and Debug/Rebuild modes and explicit `dotnet msbuild` in Release/Rebuild mode.
A temporary local SDK directory with spaces, ampersand, apostrophe,
property-like text, semicolon, and at-sign characters exercised the global path
override without shell interpretation. Two native Release/Rebuild outputs had
identical DLL SHA-256 hashes with deterministic compilation enabled. These tests
did not redistribute SDK files or run the generated plug-in.

CI uses independently authored filesystem/PE-header fixtures, including PE32/I386
and PE32+/amd64 declarations, invalid files, ambiguous paths, and runtime-only
dotnet hosts. Passing fixtures validate the inspector's behavior, not ESAPI compatibility.

## Reported Eclipse 15.6 workstation verification

On 2026-10-15, the user supplied `doctor --esapi-version 15.6 --json` output from
the separate Eclipse workstation. This is a user-reported result, not a remotely
executed test. The report showed:

- Windows amd64, ESAPI 15.6 profile targeting Framework 4.5 / x64.
- Visual Studio 2019 Community native amd64 MSBuild discovered as a single candidate.
- The Framework 4.5 targeting-pack markers present, with no missing files.
- The ESAPI 15.6 API directory discovered as a single candidate; both API and Types
  DLLs reported PE32+ / amd64 with DLL and CLR-header declarations.
- A dotnet host on PATH with SDK 2.1.801 markers. Native MSBuild remains the default
  build choice, so that SDK is not selected merely because it is on PATH.
- All required static checks passed: `ok: true`, `staticChecksPassed: true`, and
  `diagnostics: []`. Tool execution, assembly identity, and Eclipse runtime checks
  remained optional `skip` observations.

The CLI version/commit and shell exit code were not included in the submitted
report. No successful 15.6 build or Eclipse execution is established by this result.
Machine-specific user directories and the raw report are not committed.

The next step on that workstation is to record CLI identity and run generation,
check, and build with the discovered DLL directory. Use a new project name if the
example destination already exists; do not overwrite an existing project for this test.

```powershell
.\esapi.exe --version --json
$sdkPath = 'C:\Program Files (x86)\Varian\RTM\15.6\esapi\API'
.\esapi.exe new Smoke156 --esapi-version 15.6 --esapi-path $sdkPath
.\esapi.exe check --project .\Smoke156\Smoke156.csproj --json
.\esapi.exe build --project .\Smoke156\Smoke156.csproj --release --json
$LASTEXITCODE
```

Expected: check/build exit 0, build `status: succeeded`, tool `exitCode: 0`, and
`Smoke156\bin\Release\Smoke156.esapi.dll` present without copied vendor DLLs.
If compilation fails, retain the build JSON including tool logs and diagnostic
codes. Eclipse execution remains the separate verification in step 3 above.
