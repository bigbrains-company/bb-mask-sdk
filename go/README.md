# bb-mask

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/go/README.tr.md)

Tags personal data in Turkish legal text and puts the real values back into the model’s answer. The package is a thin HTTP client. It never calls a model.

You get a `dm_test_` key from the Data Mask console. Your backend installs this package and passes the key to `Init`. The LLM call is yours; `Anonymize` goes before it and `Restore` after it.

Go 1.22+. No dependencies; `net/http` is enough. The Python package is in `python/`. Node, C# and Java live in the other folders of the same repository.

```go
_, err := bbmask.Init(os.Getenv("BB_ANONYM_TOKEN"))
tagged, err := bbmask.Anonymize("Davacı Hasan Demirkol, TC 62601815964")
reply, err := myLLM(tagged.Text)
restored, err := bbmask.Restore(tagged.DocID, reply)
_, err = bbmask.Delete(tagged.DocID)
```

Python and Node throw. Go returns the same condition as an `error`; use `errors.As` to get `*BBError`. There is no equivalent of Python’s `aanonymize` family; calls block, and you get concurrency with `go`.

| Python | Node | Go |
|---|---|---|
| `init` | `init` | `Init` |
| `anonymize` | `anonymize` | `Anonymize` |
| `doc_id` | `docId` | `DocID` |
| `get_map` | `getMap` | `GetMap` |
| `delete` | `deleteMapping` | `Delete` |
| `entities_found` | `entitiesFound` | `EntitiesFound` |
| `llm_review` | `llmReview` | `LLMReview` |

Field names on the wire are the same as in Python: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Install

Module path `github.com/bigbrains-company/bb-mask-sdk/go`, package name `bbmask`. Repository https://github.com/bigbrains-company/bb-mask-sdk; the first line of `go.mod` matches it.

Once published:

```bash
go get github.com/bigbrains-company/bb-mask-sdk/go@v0.3.1
```

From a clone of this repository, add this to the consuming module’s `go.mod`:

```text
require github.com/bigbrains-company/bb-mask-sdk/go v0.3.1

replace github.com/bigbrains-company/bb-mask-sdk/go => /path/to/bb-mask-sdk/go
```

The path on the right of `replace` is where this `go` folder lives on disk. If the module sits at the repository root, write `./bb-mask-sdk/go`.

Tests, from this folder:

```bash
go test ./...
```

Never put the key in code or in the repository.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

```go
import "github.com/bigbrains-company/bb-mask-sdk/go"
```

`Init` runs once per process. It is not a model call. Package functions called before it return a 401 `BBError` and never reach the service.

```go
client, err := bbmask.Init(os.Getenv("BB_ANONYM_TOKEN"), bbmask.Config{
    BaseURL: "https://app.bbmask.co", // this is the default
    Timeout: 150 * time.Second,         // this is the default
})
```

| Field | Meaning |
|---|---|
| `apiKey` | The `dm_test_` key from the console. Empty or whitespace → 401. The existing client is left unchanged. |
| `BaseURL` | Service address. Empty → `https://app.bbmask.co`. |
| `Timeout` | HTTP timeout. Zero → 150 seconds; the engine waits up to 120 s and the client gives up after that. |

Returns a `*BBAnonym`. Package functions use this client. A second `Init` replaces the first.

If you need a second key, construct one with `New` instead of `Init`.

```go
other := bbmask.New("https://app.bbmask.co", token, 150*time.Second)
```

## Objects

### `Tagged`

Returned by `Anonymize`, `AnonymizeFile` and `AnonymizeDocuments`.

| Field | When it is set |
|---|---|
| `Text` | Always. Personal data is replaced by tags. |
| `DocID` | `WithPersist(true)` (default). An empty string is the equivalent of Python’s `None`. |
| `State` | `WithPersist(false)`. The mapping itself. Never stored on the service. |
| `EntitiesFound` | Count per type: `map[string]int{"PERSON": 2}`. No plain names. |
| `BalanceCredits` | Balance left after the call: `"9.00"`. |
| `RequestID` | The response’s `X-Request-Id`. |
| `PlaceholderStyle` | `"angle"` or `"bracket_tr"`. |
| `KeepLegalForm` | Applied value. |
| `HealthData` | Applied value. |
| `LLMReview` | If `WithLLMReview(true)` was requested, a status summary. No plain names. |

### Options

All options are off by default. `persist` defaults to `true` and the tag format to `"angle"`.

| Function | Default | Effect |
|---|---|---|
| `WithPersist` | `true` | `false` stores nothing on the service and leaves `DocID` empty. |
| `WithPlaceholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Any other value returns 400 before the request is sent. |
| `WithKeepLegalForm` | `false` | `true` keeps the company type suffix outside the tag. |
| `WithHealthData` | `false` | `true` also tags health data. |
| `WithLLMReview` | `false` | `true` requests a second LLM pass. |

Constants: `StyleAngle`, `StyleBracketTR`.

```go
tagged, err := bbmask.Anonymize(text,
    bbmask.WithPlaceholderStyle(bbmask.StyleBracketTR),
    bbmask.WithKeepLegalForm(true),
    bbmask.WithHealthData(true),
    bbmask.WithLLMReview(true),
)
```

### `BBError`

`StatusCode`, `Detail`, `RequestID`. `err.Error()` is the detail text. Network errors are `*BBError` too; no `net/http` error leaks out.

```go
var bb *bbmask.BBError
if errors.As(err, &bb) && bb.StatusCode == 402 {
    // no balance
}
```

| Code | Meaning |
|---|---|
| 400 | Invalid `placeholder_style`, file count outside 1–3, nothing anonymized in the session, or `Restore` got neither a string nor a map. |
| 401 | `Init` not called, or the key is empty or unknown. |
| 402 | No balance. |
| 404 | Mapping not found. |
| 413 | File larger than 20 MB. |
| 502 | The response is not JSON. |
| 503 | Service unreachable. |
| 504 | `Timeout` expired. The request may have reached the service. |

## Calls

File extensions: `.txt`, `.pdf` or `.docx`. Up to 20 MB per file.

`AnonymizeFile(path)` reads a path. If you already hold the bytes, use `AnonymizeBytes(filename, content)`; both names mean the same thing on the package and on `BBAnonym`.

### Mapping on the service

```go
tagged, err := bbmask.Anonymize(text)
restored, err := bbmask.Restore(tagged.DocID, reply)
_, err = bbmask.Delete(tagged.DocID)
```

One file and several files (up to 3, one `DocID`, one fee):

```go
tagged, err := bbmask.AnonymizeFile("petition.pdf")
batch, err := bbmask.AnonymizeDocuments([]string{"petition.pdf", "response.docx", "appendix.txt"})
```

### Dictionary

```go
mapping, err := bbmask.GetMap(tagged.DocID, false)
readOnce, err := bbmask.GetMap(tagged.DocID, true)
```

`true` also deletes the service copy after reading. The dictionary carries plain names; only `tagged.Text` goes to the model.

### Mapping on your side

```go
tagged, err := bbmask.Anonymize(text, bbmask.WithPersist(false))
restored, err := bbmask.Restore(tagged.State, reply)
```

### Session

The `DocID` and `State` stay inside the session. `Close` runs on exit: with `Persist` true every kept id is deleted. A `BBError` from the delete is swallowed.

```go
s, err := bbmask.Session()
if err != nil {
    return err
}
defer s.Close()

tagged, err := s.Anonymize(text)
restored, err := s.Restore(reply)
```

`BBAnonym` is safe for concurrent use. Use a single `MaskSession` sequentially.

```go
s, err := bbmask.Session(bbmask.WithPersist(false), bbmask.WithPlaceholderStyle("bracket_tr"))
```

## Reference

```go
Init(apiKey string, cfg ...Config) (*BBAnonym, error)
Anonymize(text string, opts ...Option) (*Tagged, error)
AnonymizeFile(path string, opts ...Option) (*Tagged, error)
AnonymizeBytes(filename string, content []byte, opts ...Option) (*Tagged, error)
AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error)
Restore(docIDOrState any, modelText string) (string, error)
GetMap(docID string, once bool) (map[string]any, error)
Delete(docID string) (bool, error)
Session(opts ...Option) (*MaskSession, error)
```

The `New` constructor does not need `Init`; it has the same methods. The key is not an exported field; `%+v` and `String()` mask it.

`MaskSession`: `Anonymize`, `AnonymizeFile`, `AnonymizeDocuments`, `Restore`, `Close`.

## Not in this package

- Your LLM call.
- Key creation and balance. Those are in the console.
- Tag repair. The service fixes broken tags.
- Python’s `aanonymize` family. Go calls block.
