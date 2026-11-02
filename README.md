# bb-mask-sdk

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/README.tr.md)

Client SDKs for the Data Mask anonymization API. The packages never call a model; `anonymize` and `restore` go to the service.

| Language | Folder | Package | Minimum | Status |
|---|---|---|---|---|
| Python | [`python/`](python/) | PyPI `bb-mask`, import `bb_mask` | 3.11+ | ready |
| Node | [`node/`](node/) | npm `bb-mask` | 18+ | ready |
| Go | [`go/`](go/) | module `github.com/bigbrains-company/bb-mask-sdk/go`, package `bbmask` | 1.22+ | ready |
| C# | [`csharp/`](csharp/) | NuGet `bb-mask`, namespace `BigBrains` | .NET 8+ | ready |
| Java | [`java/`](java/) | Maven `tr.com.bigbrains:bb-mask`, package `com.bigbrains.bbmask` | Java 17+ | ready |

License: MIT. Version: 0.3.1 (same across all five packages). Tests and builds run in `.github/workflows/ci.yml`, one job per language.

Shared surface: `init`/`BBAnonym`, `anonymize`, `anonymize_file(path)`, `anonymize_bytes(filename, content)`, `anonymize_documents`, `restore`, `get_map`, `delete`, `session`. Default timeout 150 s (the engine waits up to 120 s). The key is masked in `repr`/`toString` output.

## Python

```bash
pip install bb-mask
```

From a clone of this repository:

```bash
pip install -e python
```

Usage is in `python/README.md`.

## Node

```bash
npm install bb-mask
```

From a clone of this repository:

```bash
npm install ./node
```

Usage is in `node/README.md`. Tests: `npm test --prefix node`.

## Go

Module path `github.com/bigbrains-company/bb-mask-sdk/go`, package name `bbmask`. Go 1.22+. Before release:

```bash
go test ./...
```

Run it from the `go/` folder.

Usage is in `go/README.md`. The first line of `go.mod` matches the repository path: `github.com/bigbrains-company/bb-mask-sdk/go`; the `go/v0.3.1` tag is pushed from the repository root.

## C#

.NET 8. NuGet name `bb-mask`. Before release:

```bash
dotnet test BbMask.Tests/BbMask.Tests.csproj
```

Run it from the `csharp/` folder. Usage is in `csharp/README.md`.

## Java

Java 17. Maven coordinates `tr.com.bigbrains:bb-mask`. Before release:

```bash
mvn test
```

Run it from the `java/` folder. Usage is in `java/README.md`. The `groupId` is the verified `tr.com.bigbrains` namespace on Central. Repository: https://github.com/bigbrains-company/bb-mask-sdk
