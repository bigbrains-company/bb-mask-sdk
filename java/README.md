# bb-mask

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/java/README.tr.md)

Tags personal data in Turkish legal text and puts the real values back into the model’s answer. The package is a thin HTTP client. It never calls a model.

You get a `dm_test_` key from the Data Mask console. Your Java or Spring service installs this package and passes the key to `init`. The LLM call is yours; `anonymize` goes before it and `restore` after it. The package does not hook into Spring.

Java 17+. Runtime dependency `jackson-databind`. Python, Node, Go and C# live in the other folders of the same repository.

```java
import com.bigbrains.bbmask.BbMask;
import com.bigbrains.bbmask.Tagged;

BbMask.init(System.getenv("BB_ANONYM_TOKEN"));

Tagged tagged = BbMask.anonymize("Davacı Hasan Demirkol, TC 62601815964");
String reply = myLlm(tagged.text());
String restored = BbMask.restore(tagged.docId(), reply);
BbMask.delete(tagged.docId());
```

Errors throw `BBError`; `getMessage()` is the detail text. Calls block; if you use Spring WebFlux, move them to your own scheduler.

| Python | Java |
|---|---|
| `init` | `BbMask.init` |
| `anonymize` | `anonymize` |
| `doc_id` | `docId()` |
| `get_map` | `getMap` |
| `delete` | `delete` |
| `entities_found` | `entitiesFound()` |
| `llm_review` | `llmReview()` |

Field names on the wire are the same as in Python: `doc_id`, `placeholder_style`, `keep_legal_form`.

## Install

Maven coordinates `tr.com.bigbrains:bb-mask:0.3.1`. The group ID is the verified `tr.com.bigbrains` namespace on Central Portal; the Java package name stays `com.bigbrains.bbmask`.

Once published:

```xml
<dependency>
  <groupId>tr.com.bigbrains</groupId>
  <artifactId>bb-mask</artifactId>
  <version>0.3.1</version>
</dependency>
```

From a clone of this repository, run the tests from the `java` folder:

```bash
mvn test
```

Never put the key in code or in the repository.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`init` runs once per process. It is not a model call. Static methods called before it throw a 401 `BBError` and never reach the service.

```java
BBAnonym client = BbMask.init(
    System.getenv("BB_ANONYM_TOKEN"),
    "https://app.bbmask.co",
    Duration.ofSeconds(150));
```

| Argument | Meaning |
|---|---|
| `apiKey` | The `dm_test_` key from the console. Empty or whitespace → 401. The existing client is left unchanged. |
| `baseUrl` | Service address. Empty → `https://app.bbmask.co`. |
| `timeout` | HTTP timeout. Empty → 150 seconds; the engine waits up to 120 s and the client gives up after that. |

Returns a `BBAnonym`. A second `init` replaces the first.

If you need a second key, construct a client without `init`. In Spring it can be a `@Bean`.

```java
@Bean
BBAnonym bbAnonym(@Value("${bb.anonym.token}") String token) {
    return new BBAnonym("https://app.bbmask.co", token, Duration.ofSeconds(150));
}
```

## Objects

### `Tagged`

Returned by `anonymize`, `anonymizeFile` and `anonymizeDocuments`. Fields are accessors: `text()`, `docId()`.

| Field | When it is set |
|---|---|
| `text` | Always. Personal data is replaced by tags. |
| `docId` | `persist` true (default). Otherwise `null`. |
| `state` | `persist` false. The mapping itself. Never stored on the service. |
| `entitiesFound` | Count per type. No plain names. |
| `balanceCredits` | Balance left after the call: `"9.00"`. |
| `requestId` | The response’s `X-Request-Id`. |
| `placeholderStyle` | `"angle"` or `"bracket_tr"`. |
| `keepLegalForm` | Applied value. |
| `healthData` | Applied value. |
| `llmReview` | If `llmReview` true was requested, a status summary. No plain names. |

### Options

`Options`. `persist` defaults to `true` and the tag format to `"angle"`.

| Field | Default | Effect |
|---|---|---|
| `persist` | `true` | `false` stores nothing on the service and leaves `docId` null. |
| `placeholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Any other value throws 400 before the request is sent. |
| `keepLegalForm` | `false` | `true` keeps the company type suffix outside the tag. |
| `healthData` | `false` | `true` also tags health data. |
| `llmReview` | `false` | `true` requests a second LLM pass. |

Constants: `BbMask.STYLE_ANGLE`, `BbMask.STYLE_BRACKET_TR`.

```java
Tagged tagged = BbMask.anonymize(text, new Options()
    .placeholderStyle(BbMask.STYLE_BRACKET_TR)
    .keepLegalForm(true)
    .healthData(true)
    .llmReview(true));
```

### `BBError`

`getStatusCode()`, `getDetail()`, `getRequestId()`. Network errors are `BBError` too.

| Code | Meaning |
|---|---|
| 400 | Invalid `placeholder_style`, file count outside 1–3, nothing anonymized in the session, or `restore` got neither a string nor a map. |
| 401 | `init` not called, or the key is empty or unknown. |
| 402 | No balance. |
| 404 | Mapping not found. |
| 413 | File larger than 20 MB. |
| 502 | The response is not JSON. |
| 503 | Service unreachable. |
| 504 | Timeout expired. The request may have reached the service. |

## Calls

File extensions: `.txt`, `.pdf` or `.docx`. Up to 20 MB per file.

`anonymizeFile(path)` reads a path. If you already hold the bytes, use `anonymizeBytes(filename, content)`; both names mean the same thing on `BbMask` and `BBAnonym`.

### Mapping on the service

```java
Tagged tagged = BbMask.anonymize(text);
String restored = BbMask.restore(tagged.docId(), reply);
BbMask.delete(tagged.docId());
```

One file and several files (up to 3, one `docId`, one fee):

```java
Tagged tagged = BbMask.anonymizeFile("petition.pdf");
Tagged batch = BbMask.anonymizeDocuments(List.of("petition.pdf", "response.docx", "appendix.txt"));
```

### Dictionary

```java
Map<String, Object> mapping = BbMask.getMap(tagged.docId(), false);
Map<String, Object> readOnce = BbMask.getMap(tagged.docId(), true);
```

`true` also deletes the service copy after reading. The dictionary carries plain names; only `tagged.text()` goes to the model.

### Mapping on your side

```java
Tagged tagged = BbMask.anonymize(text, new Options().persist(false));
String restored = BbMask.restore(tagged.state(), reply);
```

### Session

The `docId` and `state` stay inside the session. `close` runs on exit: with `persist` true every kept id is deleted. A `BBError` from the delete is swallowed. `try-with-resources` makes the same `close` call.

```java
try (MaskSession s = BbMask.session()) {
    Tagged tagged = s.anonymize(text);
    String restored = s.restore(reply);
}
```

```java
try (MaskSession s = BbMask.session(new Options().persist(false).placeholderStyle("bracket_tr"))) {
    Tagged tagged = s.anonymize(text);
}
```

## Reference

```java
BbMask.init(apiKey)
BbMask.init(apiKey, baseUrl, timeout)
anonymize(text, options) -> Tagged
anonymizeFile(path, options) -> Tagged
anonymizeBytes(filename, content, options) -> Tagged
anonymizeDocuments(paths, options) -> Tagged
restore(docIdOrState, modelText) -> String
getMap(docId, once) -> Map<String, Object>
delete(docId) -> boolean
session(options) -> MaskSession
```

The `BBAnonym` constructor does not need `init`; it has the same methods. The key is not exposed; `toString()` masks it.

`MaskSession`: `anonymize`, `anonymizeFile`, `anonymizeDocuments`, `restore`, `close`.

## Not in this package

- Your LLM call.
- Key creation and balance. Those are in the console.
- Tag repair. The service fixes broken tags.
- A Spring Boot starter. You create the client in your own `@Bean`.
