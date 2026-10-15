# ESAPI CLI Design Document

## 1. Overview

ESAPI CLI は、Varian / Siemens Healthineers Eclipse Scripting API（ESAPI）向けの開発プロジェクトを、GUI の Script Wizard に依存せず CLI から作成・確認・ビルドできるようにする軽量な開発ツールである。

製品の紹介文:

> An AI-friendly CLI for scaffolding, validating, and building ESAPI projects.

全体の設計方針:

> Human-friendly enough, agent-friendly by design.

人間が直接扱える分かりやすさを保ちつつ、coding agent が予測可能な入出力で
自動実行できる CLI contract を中心に設計する。紹介文は目標とする機能範囲を表し、
実装済み機能と今後の予定は README の Status で区別する。

主目的は以下。

- ESAPI プロジェクトの雛形生成
- ESAPI 開発環境の簡易チェック
- 将来的なビルド・検証・プロジェクト管理
- 単一バイナリで簡単に配布
- ESAPI 本体や proprietary SDK ファイルを再配布しない

実装言語は Go とする。

CLI 自体は ESAPI DLL をリンク・埋め込みしない。
必要な ESAPI DLL はユーザーの Eclipse / ESAPI SDK 環境から参照する。

---

# 2. Goals

## Primary goals

1. 単一の実行ファイルとして配布できること
2. Windows 上で簡単に利用できること
3. ESAPI プロジェクトを CLI から生成できること
4. 生成物は人間が読める単純な構造であること
5. ESAPI SDK / DLL / documentation をリポジトリに含めないこと
6. 外部依存を最小限にすること
7. 将来的に ESAPI バージョン差異へ対応しやすい構造にすること

---

# 3. Non-goals

v1 では以下を実装しない。

- Eclipse 本体の起動
- ESAPI script の自動登録
- Eclipse DB へのアクセス
- ESAPI DLL を実際にロードした Reflection
- API documentation XML の解析
- NuGet package 化された ESAPI SDK の提供
- GUI
- MCP server
- LLM integration
- Visual Studio extension
- Eclipse Script Wizard の完全互換実装

Script Wizard の内部実装や proprietary resource をコピーしない。

---

# 4. Project Naming

仮称:

```text
esapi-cli
```

実行ファイル名:

```text
esapi
```

将来的に名称変更可能な構造とする。

CLI usage example:

```bash
esapi new MyScript
esapi doctor
esapi version
```

---

# 5. Technology

Language:

```text
Go
```

Target:

```text
Windows amd64
```

将来的には Linux / macOS でも CLI 自体は起動可能にする。

ただし ESAPI 開発環境検出やビルドは主に Windows を対象とする。

Go version は current stable version を使用する。

できるだけ標準ライブラリを使用する。

v1 では Cobra などの CLI framework を必須としない。

CLI の規模が大きくなった場合のみ導入を検討する。

---

# 6. Binary distribution

最終成果物は基本的に単一バイナリとする。

例:

```text
esapi.exe
```

template file は Go の `embed` package を使用してバイナリ内へ埋め込む。

例:

```go
//go:embed templates/*
var templates embed.FS
```

ユーザーが追加ファイルを配置しなくても動作すること。

---

# 7. Repository Structure

初期案:

```text
esapi-cli/
├── cmd/
│   ├── new.go
│   ├── doctor.go
│   └── version.go
│
├── internal/
│   ├── scaffold/
│   │   ├── scaffold.go
│   │   └── config.go
│   │
│   ├── detect/
│   │   ├── dotnet.go
│   │   ├── esapi.go
│   │   └── windows.go
│   │
│   └── fsutil/
│
├── templates/
│   ├── plugin/
│   │   ├── Script.cs.tmpl
│   │   └── project.csproj.tmpl
│   │
│   └── common/
│       ├── gitignore.tmpl
│       └── README.md.tmpl
│
├── main.go
├── go.mod
├── LICENSE
└── README.md
```

過剰な abstraction は避ける。

v1 の実装規模に対して不要ならディレクトリは簡略化してよい。

---

# 8. Version 1 Scope

v1 の目的は、

> ESAPI script project を CLI から安全・簡単に作成できること

とする。

v1 では以下の3コマンドを提供する。

```text
esapi new
esapi doctor
esapi version
```

対応目標は ESAPI 15.6 以上とする。生成設定の提供と実環境での動作確認は区別し、
未確認の将来バージョンへの互換性は保証しない。

---

# 9. `esapi new`

## Basic usage

```bash
esapi new MyScript
```

以下のようなディレクトリを生成する。

```text
MyScript/
├── MyScript.csproj
├── Script.cs
├── .gitignore
└── README.md
```

生成物はできるだけ単純にする。

---

## Initial supported project type

v1 ではまず通常の ESAPI plug-in script のみ対応する。

例:

```bash
esapi new MyScript
```

または明示的に:

```bash
esapi new MyScript --type plugin
```

`plugin` を default とする。

Standalone project は v2 以降。

---

## Options

v1:

```text
--type plugin
--output <path>
--force
--esapi-version <version>
--esapi-path <DLL directory>
--dry-run
--json
```

例:

```bash
esapi new MyScript --output ./scripts
```

生成先:

```text
./scripts/MyScript/
```

---

## Existing directory behavior

対象ディレクトリが存在する場合は default ではエラー。

```text
error: directory "MyScript" already exists
```

`--force` 指定時のみ overwrite を許可する。

ただし v1 では既存の未知ファイルを不用意に削除しない。

既存ファイルの overwrite のみとする。

---

# 10. Generated Script.cs

テンプレートは proprietary な Script Wizard のテンプレートを直接コピーしない。

必要な ESAPI interface に従い、独自に最小構成を定義する。

概念的には以下のような構造。

```csharp
using System;
using VMS.TPS.Common.Model.API;

namespace VMS.TPS
{
    public class Script
    {
        public void Execute(ScriptContext context)
        {
        }
    }
}
```

実際の namespace / assembly attribute 等は対象 ESAPI version の要求に合わせる。

初期実装は `VMS.TPS.Script` を entry point とし、
`[assembly: ESAPIScript(IsWriteable = false)]` を付与する。

余計なサンプルコードは含めない。

---

# 11. Generated project file

`.csproj` はテンプレートとして生成する。

重要な設計方針:

ESAPI DLL を repository や generated project 内へコピーしない。

ユーザー環境に存在する ESAPI DLL を参照する。

v1 では ESAPI path を以下のいずれかで扱える構造を検討する。

優先順位:

```text
1. CLI option
2. environment variable
3. known installation path
```

環境変数候補:

```text
ESAPI_HOME
```

例:

```text
ESAPI_HOME=C:\Program Files\...\ESAPI
```

project file 内では可能なら property 経由で参照する。

概念例:

```xml
<Reference Include="VMS.TPS.Common.Model.API">
  <HintPath>$(ESAPI_HOME)\VMS.TPS.Common.Model.API.dll</HintPath>
</Reference>
```

具体的な ESAPI install path をハードコードしすぎないこと。

Milestone 1 では従来形式（非 SDK 形式）の `.csproj` を生成する。
platform は x64、assembly name は `<project name>.esapi` とし、
出力拡張子は `.esapi.dll` とする。
両 ESAPI reference に `Private=False` を指定し、出力先への DLL コピーを防ぐ。
`ESAPI_HOME` は両 DLL を直接含むディレクトリ（通常は SDK の `API` 配下）を指す。
`--esapi-path` 指定時は絶対パスを project property として保存し、未指定時は
build 時の環境変数を参照する。既知パスの探索は後続実装とする。

---

# 12. Version handling

v1 では ESAPI version の完全自動判定を必須としない。

以下の option を用意できる設計とする。

```bash
esapi new MyScript --esapi-version 18
```

v1 の自動検出実装後、未指定時は以下を目指す:

```text
auto
```

ただし auto detection に失敗した場合でも、明示指定で生成可能とする。

Milestone 1 では自動検出を行わず、未指定時は明示的に `15.6` として扱う。
初期の生成 profile は以下に限定し、未知の version と `auto` はエラーとする。

| ESAPI | Target framework | Platform | 備考 |
| --- | --- | --- | --- |
| 15.6 | .NET Framework 4.5 | x64 | 15.5 と同じ設定を採用。15.6 の資料には別の要件の明記なし |
| 16.0 / 16.1 | .NET Framework 4.6.1 | x64 | reference guide に基づく設定 |
| 17.0 / 18.0 / 18.1 / 19.0 | .NET Framework 4.8 | x64 | reference guide に基づく設定 |

`16` / `17` / `18` / `19` は対応する `.0` profile の別名とする。
実 ESAPI DLL を用いた build と Eclipse 上での動作は別途確認する。

version-specific configuration はコード中に散在させず、一箇所に集約する。

概念:

```go
type ESAPIVersionConfig struct {
    Version         string
    TargetFramework string
    Platform        string
}
```

将来的に設定ファイル化できる構造にする。

---

# 13. `esapi doctor`

ESAPI 開発環境を確認する診断コマンド。

usage:

```bash
esapi doctor
```

v1 では最低限以下を確認する。

```text
OS
dotnet
MSBuild
ESAPI installation path
VMS.TPS.Common.Model.API.dll
VMS.TPS.Common.Model.Types.dll
```

出力例:

```text
ESAPI development environment

✓ Windows 11
✓ dotnet found
✓ MSBuild found
✓ ESAPI installation detected
✓ VMS.TPS.Common.Model.API.dll
✓ VMS.TPS.Common.Model.Types.dll

ESAPI path:
C:\Program Files\...\ESAPI
```

見つからない場合:

```text
✗ VMS.TPS.Common.Model.API.dll not found
```

診断失敗は適切な exit code を返す。

初期実装は読み取り専用の静的検査とする。`info` は同じ検出結果と CLI capability を返し、
環境不足があっても情報取得として成功する。`doctor` は必須項目の不足で exit code 3 を返す。
JSON の `doctor.data` は失敗時も全 observation / check を保持する。
tool や DLL は実行・load せず、PATH / 明示パス / 既知の filesystem layout を調べる。
MSBuild が見つかれば dotnet SDK は必須としない。dotnet host のみを SDK と見なさない。
選択 Framework の targeting pack は `mscorlib.dll` と `RedistList/FrameworkList.xml` で確認する。
複数の installation 候補は勝手に選択せず、明示指定を要求する。
実行・assembly identity・version 整合性・Eclipse 動作は未検査として明示する。

---

# 14. DLL inspection in v1

v1 の DLL inspection は軽量なものに限定する。

可能であれば Go 標準ライブラリ `debug/pe` を利用する。

確認候補:

```text
file exists
PE file validity
machine architecture
PE32 / PE32+
```

.NET metadata の詳細解析は v1 では必須ではない。

Assembly Version 等が簡単に安全に取得できる場合のみ実装する。

Reflection 相当の処理は行わない。

初期実装は DLL characteristic と CLR directory declaration も読み取る。
PE32 / I386 のみを理由に x86 専用と判定しない。CLR flag / metadata の解析は行わず、
構造上の検査成功から実際の SDK identity や実行可能性を推定しない。

---

# 15. `esapi version`

CLI 自身の version を表示する。

```bash
esapi version
```

例:

```text
esapi 0.1.0
```

build information を linker flag で埋め込める設計にする。

候補:

```text
version
commit
build date
```

ただし build date は reproducible build を妨げる場合は省略可能。

`--version` を `version` の別名とし、両方で `--json` を提供する。
version / commit は `internal/buildinfo` の linker flag で設定できるようにする。
利用可能な Go build metadata から commit / modified / Go version を補完する。

---

# 16. Error handling

error message は簡潔にする。

良い例:

```text
error: ESAPI installation was not found
```

可能なら対処方法を追加する。

```text
Set ESAPI_HOME or specify --esapi-path.
```

panic は原則としてユーザー入力エラーには使用しない。

---

# 17. Exit codes

基本方針:

```text
0 success
1 general error
2 invalid arguments
3 environment error
4 build error
5 project validation error / evaluation required
```

v1 では必要以上に細分化しなくてよい。

---

# 18. Security / Legal constraints

この repository には以下を含めない。

```text
VMS.TPS.Common.Model.API.dll
VMS.TPS.Common.Model.Types.dll
ESAPI documentation XML
Eclipse binaries
Script Wizard binaries
Varian / Siemens proprietary assets
```

生成テンプレートは独自実装する。

開発時のみ参照する内部資料は Git 管理外の `.local/` に配置できる。
内部資料を commit、template への同梱、配布物への追加の対象にしない。

公式 Script Wizard のファイルを直接 repository にコピーしない。

README に以下を明記する。

```text
This project is not affiliated with or endorsed by
Varian Medical Systems or Siemens Healthineers.
```

また、本ツールは clinical use を目的としない。

候補:

```text
NOT FOR CLINICAL USE
```

CLI は開発補助ツールであり、生成されたコードの clinical validation を保証しない。

---

# 19. License

OSS license は MIT を第一候補とする。

ただし repository に含まれるコード・template がすべて独自実装または互換ライセンスであることを確認する。

---

# 20. README v1

最低限以下を含める。

```text
What is esapi-cli?
Installation
Quick Start
esapi new
esapi doctor
Requirements
ESAPI_HOME
Supported ESAPI versions
Disclaimer
License
```

---

# 21. Tests

v1 から最低限の test を実装する。

主に scaffold generation を test する。

例:

```text
TestNewProject
TestExistingDirectory
TestForceOverwrite
TestInvalidProjectName
TestTemplateRendering
```

temporary directory を利用する。

生成結果について、

```text
expected files exist
project name substituted correctly
template syntax valid
```

を確認する。

実際の ESAPI DLL は CI に要求しない。

---

# 22. CI

GitHub Actions を利用する。

最低限:

```text
go test ./...
go vet ./...
go build
```

Windows build も確認する。

release 時には以下を生成できる構造にする。

```text
esapi-windows-amd64.exe
```

将来的には:

```text
linux-amd64
darwin-arm64
```

も作成可能。

---

# 23. v1 Definition of Done

v1 完了条件:

1. `esapi new MyScript` が動く
2. 最小 ESAPI project が生成される
3. ESAPI proprietary file を同梱しない
4. `esapi doctor` が基本環境を検査できる
5. `esapi version` が動く
6. Windows 用 single binary が生成できる
7. unit tests が存在する
8. GitHub Actions で test / build できる
9. README が存在する
10. MIT license と disclaimer が存在する

---

# 24. Version 2 Roadmap

v2 では「プロジェクト生成ツール」から「ESAPI developer CLI」へ拡張する。

---

## 24.1 Build command

追加:

```bash
esapi build
```

内部では既存の .NET toolchain を呼ぶ。

候補:

```text
dotnet build
MSBuild.exe
```

CLI 自身で C# compiler を実装しない。

project type や framework に応じて toolchain を選択する。

例:

```text
Detected .NET Framework project
Using MSBuild 17.x

Building MyScript...
✓ Build succeeded
```

options:

```text
--release
--debug
--clean
--verbose
```

初期実装:

- Windows amd64 を対象とし、従来形式 project に native MSBuild を優先する。
  不在時は SDK marker のある `dotnet msbuild` を選択できる。
  `--msbuild-path` / `--dotnet-path` は排他で、曖昧な候補は暗黙に fallback しない。
- `--project` と候補選択の規則は `check` と共有する。`check` / `doctor` は独立した
  補助検査とし、条件式を持つ project の build を静的検査だけで拒否しない。
- default Release、`--debug` で Debug、`--clean` で MSBuild Rebuild を呼ぶ。
  x64 / Deterministic / single node / node reuse off / shared compilation off を指定する。
  response file の自動読込を無効にし、restore は要求しない。project target 自体の処理は MSBuild に委ねる。
- `--esapi-path` は当該 build の `ESAPI_HOME` global property。未指定なら project / environment の binding を維持する。
- `--timeout` は default 5m、正の duration、最大24h。Ctrl+C / caller context の中断を伝える。
  Windows 10+ の creation-time Job assignment と kill-on-close で子プロセスも管理する。
- `--dry-run` は tool / argv / working directory / environment override を返すのみ。
  tool logs は各 stream 先頭1 MiBを保持し、超過分も読み捨てて pipe を詰まらせない。
  JSON は失敗時も plan / tool exit code / status / logs を保持し、timeout / cancelled をコードで区別する。

schema と終了コードの詳細は CLI_CONTRACT.md、実機確認手順は REAL_MACHINE_TEST.md を参照。

---

## 24.2 Standalone ESAPI project

追加:

```bash
esapi new MyApp --type standalone
```

Standalone executable 用 project template を追加する。

---

## 24.3 WPF support

例:

```bash
esapi new MyApp --type standalone --ui wpf
```

生成候補:

```text
App.xaml
MainWindow.xaml
MainWindow.xaml.cs
```

---

## 24.4 Improved DLL inspection

.NET assembly metadata を解析する。

候補:

```text
Assembly name
Assembly version
Target framework
Referenced assemblies
```

ESAPI DLL を実際にロードすることなく metadata のみ読む。

---

## 24.5 `esapi info`

例:

```bash
esapi info
```

出力:

```text
ESAPI version: 18.x
Architecture: x64
API assembly: ...
Types assembly: ...
```

---

## 24.6 Project validation

追加:

```bash
esapi check
```

確認項目:

```text
project file
target framework
platform
ESAPI references
assembly naming
missing files
```

初期の `check` は従来形式の `.csproj` に対する読み取り専用の宣言検査として実装する。
`--project <file>` がなければ current directory の通常ファイル `.csproj` を候補とし、
複数候補は選ばず一覧と診断を返す。探索は再帰せず、明示した symbolic link は参照先を検査する。
`--esapi-version` は project の `ESAPIVersion` がない場合の期待 profile を指定し、
両方ある場合は一致を要求する。`--esapi-path` は今回の検査だけの `ESAPI_HOME` override とする。

Framework / x64 / Library / `.esapi` assembly name / 各 ESAPI 参照の `Private=False` /
ESAPI DLL と明示的な Compile Include のファイル存在を確認する。
条件付きの検査対象、glob、未知の式、独自 import は MSBuild を推測実装せず、
`PROJECT_EVALUATION_REQUIRED` と必須項目の `skip` を返す。失敗・検査不能は exit 5 とし、
JSON report は保持する。候補選択の失敗は exit 2、I/O failure は exit 1 とする。
MSBuild evaluation（implicit import / target body を含む）、compile、assembly identity、
Eclipse execution は検査済みとはしない。詳しい範囲と schema は CLI_CONTRACT.md に記載する。

---

# 25. Version 3+ Roadmap

## Project upgrade

```bash
esapi upgrade
```

ESAPI version migration 補助。

例:

```text
ESAPI 17 -> 18
```

自動変更可能な項目のみ変更する。

---

## Template customization

ユーザー独自 template。

候補:

```text
~/.config/esapi/templates/
```

または:

```text
%APPDATA%\esapi\
```

---

## Config file

候補:

```text
esapi.toml
```

例:

```toml
default_esapi_version = "18"
default_project_type = "plugin"

[paths]
esapi = "C:\\..."
```

---

## Project metadata

生成時:

```bash
esapi new MyScript --author "..."
```

project metadata を README 等へ反映。

---

## Additional scaffolds

候補:

```text
plugin
standalone
wpf
console
service
web-api
```

ESAPI Web API / MCP project scaffold も将来的な候補とする。

---

# 26. Possible future MCP integration

将来的には ESAPI project を LLM orchestration / MCP server と接続する用途が考えられる。

ただし CLI 本体と MCP functionality は分離する。

例:

```text
esapi-cli
    ↓
project generation / build / validation

separate project
    ↓
ESAPI MCP server
```

CLI を clinical workflow の execution engine にしない。

---

# 27. Design principles

> Human-friendly enough, agent-friendly by design.

優先順位:

```text
Simple
Predictable
Explicit
Portable
Safe
```

Magic behavior を避ける。

agent-friendly な CLI contract として以下を重視する。

- default で対話 prompt を出さず、必要な入力は argument / option / 明示された環境変数で受け取る。
- 成功結果は stdout、診断は stderr に出力し、exit code の意味を文書化する。
- help に引数、選択肢、default、生成先と副作用を明記する。
- 上書きや既存ファイルの扱いを予測可能にし、実行時に隠れた変更を行わない。
- `--json` による構造化出力を主要 command へ展開し、schema version、安定した field と error identifier を文書化する。最初の対象は `new` とする。
- 人間向けメッセージの文字列を安定した parsing API として扱わない。

追加の原則:

- `info` / `doctor` / `check` は読み取り専用とし、依存ツールの install、環境変数の永続変更、設定の自動修正を暗黙に行わない。
- project 候補が複数ある場合は推測で選ばず、候補と診断を返し、`--project <path>` で選択可能にする。
- JSON field / schema version、診断 code、exit code を公開 contract として扱う。破壊的な schema 変更は version を上げる。
- 外部 build process へ cancellation を伝え、timeout / 中断を診断 code で区別する。待ち時間の上限も指定可能にする。
- `info --json` は実効設定と採用元に加えて、CLI の対応 command / profile / capability を返せる構造にする。
- 検査結果は確認済み・失敗・未検査を区別する。設定検査やコンパイル成功から Eclipse 上の実行成功を推定しない。
- 同じ CLI / template version と実効入力に対して生成ファイルを決定的にする。日時やランダム ID を生成物に含めない。

`new --dry-run` は通常生成と共通の validation / template rendering を使い、
生成先とファイルごとの `create` / `overwrite` を返す。親ディレクトリも作成しない。
実際の書き込み権限や実行時の I/O 成功は保証せず、計画は filesystem snapshot とする。
通常生成時には書き込み前に再検査する。

構造化出力は `schemaVersion` / `command` / `ok` / `data` / `diagnostics` を共通の
envelope とし、通常のエラーも stdout に JSON 1件で返す。
詳細は [CLI_CONTRACT.md](CLI_CONTRACT.md) に記載する。

実装順序は `new --json / --dry-run` → `doctor / info` → 基本的な `check` → v2 の `build` とする。
`check` の基本的な静的検査は agent 向けの価値を考慮し、v2 から前倒しする。

これらは CLI の入出力設計の方針であり、v1 の LLM integration / MCP server の
non-goals は維持する。Milestone 1 で未実装の機能は README に明記する。

特に ESAPI version や DLL path の自動検出は便利機能として扱い、必ず明示指定できるようにする。

例:

```bash
esapi new MyScript \
  --esapi-version 18 \
  --esapi-path "C:\..."
```

---

# 28. Implementation guidance for Codex

まず v1 の最小実装から開始すること。

最初の milestone:

```text
esapi new MyScript
```

のみを完成させる。

その後:

```text
doctor
version
tests
CI
```

を追加する。

最初から将来機能の abstraction を大量に作らない。

以下を避ける。

```text
premature plugin architecture
large dependency injection framework
complex config system
custom build engine
custom .NET parser
```

Go 標準ライブラリを優先する。

外部 package の導入は明確な利点がある場合のみとする。

---

# 29. Suggested first implementation milestone

Milestone 1:

```text
esapi new MyScript
```

成果物:

```text
MyScript/
├── MyScript.csproj
├── Script.cs
├── README.md
└── .gitignore
```

必要な実装:

```text
argument parsing
project name validation
directory creation
embedded templates
template rendering
basic tests
```

この milestone が完成・test 済みになってから `doctor` を実装すること。

初期実装では project name を `[A-Za-z_][A-Za-z0-9_]{0,99}` に制限し、
Windows の予約 device name も拒否する。
全 template の rendering と生成先の事前確認を終えてから書き込みを行う。
`--force` でも生成対象の4ファイル以外を削除せず、生成先の link / directory は拒否する。
各ファイルは一時ファイルから置き換えるが、4ファイル全体の transaction は保証しない。
上書き途中の I/O error では生成対象の一部のみ更新される可能性を README に記載する。

---

# 30. Core philosophy

ESAPI CLI は ESAPI を置き換えるものではない。

また Eclipse Script Wizard の clone を目的としない。

目的は、

> ESAPI development workflow の単純な部分を
> reproducible な CLI operation として提供する

ことである。

可能な限り標準的な .NET / Go toolchain を再利用し、独自実装を最小限にする。
