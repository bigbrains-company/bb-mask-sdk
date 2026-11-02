# bb-mask

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/csharp/README.tr.md)

Tags personal data in Turkish legal text and puts the real values back into the model’s answer. The package is a thin HTTP client. It never calls a model.

You get a `dm_test_` key from the Data Mask console. Your .NET or ASP.NET Core service installs this package and passes the key to `Init`. The LLM call is yours; `Anonymize` goes before it and `Restore` after it. The package does not hook into any framework.

.NET 8+. No dependencies; `HttpClient` is enough. Python, Node, Go and Java live in the other folders of the same repository.

```csharp
using BigBrains;

BbMask.Init(Environment.GetEnvironmentVariable("BB_ANONYM_TOKEN"));

var tagged = await BbMask.AnonymizeAsync("Davacı Hasan Demirkol, TC 62601815964");
var reply = await MyLlm(tagged.Text);
var restored = await BbMask.RestoreAsync(tagged.DocId, reply);
await BbMask.DeleteAsync(tagged.DocId);
```

Every method has a sync counterpart without the `Async` suffix. Use `Async` in ASP.NET Core. Errors throw `BBError`; `Message` is the detail text.

| Python | C# |
|---|---|
| `init` | `Init` |
| `anonymize` | `Anonymize` / `AnonymizeAsync` |
| `doc_id` | `DocId` |
| `get_map` | `GetMap` |
| `delete` | `Delete` |
| `entities_found` | `EntitiesFound` |
| `llm_review` | `LlmReview` |

Field names on the wire are the same as in Python: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Install

NuGet name `bb-mask`, namespace `BigBrains`. Once published:

```bash
dotnet add package bb-mask
```

From a clone of this repository, add this to your project file:

```xml
<ProjectReference Include="path/to/bb-mask-sdk/csharp/BbMask.csproj" />
```

Tests, from the `csharp` folder:

```bash
dotnet test BbMask.Tests/BbMask.Tests.csproj
```

Never put the key in code or in the repository.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`Init` runs once per process. It is not a model call. Static methods called before it throw a 401 `BBError` and never reach the service.

```csharp
var client = BbMask.Init(
    Environment.GetEnvironmentVariable("BB_ANONYM_TOKEN"),
    "https://app.bbmask.co",
    TimeSpan.FromSeconds(150));
```

| Argument | Meaning |
|---|---|
| `apiKey` | The `dm_test_` key from the console. Empty or whitespace → 401. The existing client is left unchanged. |
| `baseUrl` | Service address. Empty → `https://app.bbmask.co`. |
| `timeout` | HTTP timeout. Empty → 150 seconds; the engine waits up to 120 s and the client gives up after that. |

Returns a `BBAnonym`. A second `Init` replaces the first.

If you need a second key, construct a client without `Init`. In ASP.NET Core it can be a singleton.

```csharp
var other = new BBAnonym("https://app.bbmask.co", token, TimeSpan.FromSeconds(150));
builder.Services.AddSingleton(other);
```

## Objects

### `Tagged`

Returned by `Anonymize`, `AnonymizeFile` and `AnonymizeDocuments`.

| Field | When it is set |
|---|---|
| `Text` | Always. Personal data is replaced by tags. |
| `DocId` | `Persist` true (default). Otherwise `null`. |
| `State` | `Persist` false. The mapping itself. Never stored on the service. |
| `EntitiesFound` | Count per type. No plain names. |
| `BalanceCredits` | Balance left after the call: `"9.00"`. |
| `RequestId` | The response’s `X-Request-Id`. |
| `PlaceholderStyle` | `"angle"` or `"bracket_tr"`. |
| `KeepLegalForm` | Applied value. |
| `HealthData` | Applied value. |
| `LlmReview` | If `LlmReview` true was requested, a status summary. No plain names. |

### Options

`AnonymizeOptions`. `Persist` defaults to `true` and the tag format to `"angle"`.

| Field | Default | Effect |
|---|---|---|
| `Persist` | `true` | `false` stores nothing on the service and leaves `DocId` null. |
| `PlaceholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Any other value throws 400 before the request is sent. |
| `KeepLegalForm` | `false` | `true` keeps the company type suffix outside the tag. |
| `HealthData` | `false` | `true` also tags health data. |
| `LlmReview` | `false` | `true` requests a second LLM pass. |

Constants: `BbMask.StyleAngle`, `BbMask.StyleBracketTr`.

```csharp
var tagged = await BbMask.AnonymizeAsync(text, new AnonymizeOptions
{
    PlaceholderStyle = BbMask.StyleBracketTr,
    KeepLegalForm = true,
    HealthData = true,
    LlmReview = true,
});
```

### `BBError`

`StatusCode`, `Detail`, `RequestId`. Network errors are `BBError` too.

| Code | Meaning |
|---|---|
| 400 | Invalid `placeholder_style`, file count outside 1–3, nothing anonymized in the session, or `Restore` got neither a string nor a dictionary. |
| 401 | `Init` not called, or the key is empty or unknown. |
| 402 | No balance. |
| 404 | Mapping not found. |
| 413 | File larger than 20 MB. |
| 502 | The response is not JSON. |
| 503 | Service unreachable. |
| 504 | Timeout expired. The request may have reached the service. |

## Calls

File extensions: `.txt`, `.pdf` or `.docx`. Up to 20 MB per file.

`AnonymizeFile(path)` reads a path. If you already hold the bytes, use `AnonymizeBytes(filename, content)`; both names mean the same thing on `BbMask` and `BBAnonym`.

### Mapping on the service

```csharp
var tagged = await BbMask.AnonymizeAsync(text);
var restored = await BbMask.RestoreAsync(tagged.DocId, reply);
await BbMask.DeleteAsync(tagged.DocId);
```

One file and several files (up to 3, one `DocId`, one fee):

```csharp
var tagged = await BbMask.AnonymizeFileAsync("petition.pdf");
var batch = await BbMask.AnonymizeDocumentsAsync(new[] { "petition.pdf", "response.docx", "appendix.txt" });
```

### Dictionary

```csharp
var mapping = await BbMask.GetMapAsync(tagged.DocId, once: false);
var readOnce = await BbMask.GetMapAsync(tagged.DocId, once: true);
```

`once: true` also deletes the service copy after reading. The dictionary carries plain names; only `tagged.Text` goes to the model.

### Mapping on your side

```csharp
var tagged = await BbMask.AnonymizeAsync(text, new AnonymizeOptions { Persist = false });
var restored = await BbMask.RestoreAsync(tagged.State, reply);
```

### Session

The `DocId` and `State` stay inside the session. `Dispose` runs on exit: with `Persist` true every kept id is deleted. A `BBError` from the delete is swallowed.

```csharp
await using var s = BbMask.Session();
var tagged = await s.AnonymizeAsync(text);
var restored = await s.RestoreAsync(reply);
```

`BBAnonym` is safe for concurrent use. Use a single `MaskSession` sequentially.

## Reference

```csharp
Init(apiKey, baseUrl = null, timeout = null) -> BBAnonym
Anonymize / AnonymizeAsync(text, options) -> Tagged
AnonymizeFile / AnonymizeFileAsync(path, options) -> Tagged
AnonymizeBytes / AnonymizeBytesAsync(filename, content, options) -> Tagged
AnonymizeDocuments / AnonymizeDocumentsAsync(paths, options) -> Tagged
Restore / RestoreAsync(docIdOrState, modelText) -> string
GetMap / GetMapAsync(docId, once = false) -> Dictionary<string, object?>
Delete / DeleteAsync(docId) -> bool
Session(options) -> MaskSession
```

Async methods take a `CancellationToken` last. The `BBAnonym` constructor does not need `Init`; it has the same methods. The key is not a public property; `ToString()` masks it.

`MaskSession`: `Anonymize`, `AnonymizeFile`, `AnonymizeDocuments`, `Restore`, `Close`. `Dispose` and `DisposeAsync` call `Close`.

## Not in this package

- Your LLM call.
- Key creation and balance. Those are in the console.
- Tag repair. The service fixes broken tags.
- An ASP.NET controller or DI extension. You register the client yourself.
