# Real-machine verification

Environment inspection is ready for testing on an ESAPI development workstation.
The commands below inspect files and headers without starting Eclipse, loading SDK
assemblies, accessing patient data, or changing persistent system settings.

## 1. Check observations and diagnostics

Copy the single Windows amd64 executable to the workstation, or build it with
`go build -o esapi.exe .`. Select the installed ESAPI release explicitly; the CLI
defaults to the 15.6 generation profile.

```powershell
$esapiVersion = '18' # Use '15.6', '16.0', '16.1', '17', or '18' as appropriate.
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
  16.0/16.1 uses 4.6.1, and 17.0/18.0 uses 4.8. An installed runtime alone does not
  imply that the corresponding reference assemblies are available.

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

## 2. Optional empty plug-in build

After checking the observations, compile the generated empty plug-in using the
workstation's installed tools. This tests references and project settings without
executing the generated script. Use a new output directory; no `--force` is needed.

```powershell
$sdkPath = 'D:\path\to\ESAPI\API'
.\esapi.exe new SmokeTest --esapi-version $esapiVersion --esapi-path $sdkPath --output .\smoke-output
$msbuildPath = 'C:\path\to\MSBuild.exe' # Use the path reported by info.
& $msbuildPath .\smoke-output\SmokeTest\SmokeTest.csproj /p:Configuration=Release
```

If using the dotnet SDK alternative, invoke `dotnet msbuild` on the same project.
Confirm `bin\Release\SmokeTest.esapi.dll` exists and neither ESAPI DLL was copied
to that output directory. A failure should be compared with the preceding doctor
report; static checks intentionally do not execute compilers or choose a project SDK.

## 3. Eclipse execution is a separate verification

CLI diagnostics and compilation do not establish that Eclipse can discover or run
the plug-in. Testing that requires an appropriate non-clinical development setup and
its normal script-validation process. The CLI does not launch Eclipse or register
scripts. Do not treat a passing doctor report as evidence of clinical validation.

## Verification performed during development

On 2026-10-15, Windows checks found local ESAPI 18.0 DLLs, native Visual Studio
MSBuild, and the Framework 4.8 targeting pack. The generated empty 18.0 plug-in
compiled successfully against those DLLs, and vendor DLLs were not copied to output.
The 15.6 DLL path was also found; its doctor run correctly failed because the local
Framework 4.5 reference-assembly markers were missing. A 15.6 build was not attempted.
No Eclipse execution was performed.

CI uses independently authored filesystem/PE-header fixtures, including PE32/I386
and PE32+/amd64 declarations, invalid files, ambiguous paths, and runtime-only
dotnet hosts. Passing fixtures validate the inspector's behavior, not ESAPI compatibility.
