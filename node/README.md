# bb-mask

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/node/README.tr.md)

Tags personal data in Turkish legal text and puts the real values back into the model’s answer. The package is a thin HTTP client. It never calls a model.

You get a `dm_test_` key from the Data Mask console. Your backend installs this package and passes the key to `init`. The LLM call is yours; `anonymize` goes before it and `restore` after it.

Node 18+. No dependencies; `fetch` is enough. The package is ESM; use `import`.

```js
import { init, anonymize, restore, deleteMapping } from "bb-mask";

init(process.env.BB_ANONYM_TOKEN);

const tagged = await anonymize("Davacı Hasan Demirkol, TC 62601815964");
const reply = await myLlm(tagged.text);
const restored = await restore(tagged.docId, reply);
await deleteMapping(tagged.docId);
```

## Install

```bash
npm install bb-mask
```

Never put the key in code or in the repository.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`init` runs once per process. It is not a model call. Module functions called before it throw `BBError(401)` and never reach the service.

```js
const client = init(process.env.BB_ANONYM_TOKEN, {
  baseUrl: "https://app.bbmask.co",
  timeout: 150,
});
```

| Argument | Meaning |
|---|---|
| `apiKey` | The `dm_test_` key from the console. Empty → `BBError(401)`. |
| `baseUrl` | Service address. Default `https://app.bbmask.co`. |
| `timeout` | HTTP timeout in seconds. Default `150`; the engine waits up to 120 s and the client gives up after that. |

If you need a second key, construct `BBAnonym` without `init`.

```js
import { BBAnonym } from "bb-mask";

const other = new BBAnonym("https://app.bbmask.co", token, 150);
```

## Objects

### `Tagged`

Returned by `anonymize`, `anonymizeFile` and `anonymizeDocuments`.

| Field | When it is set |
|---|---|
| `text` | Always. Personal data is replaced by tags. |
| `docId` | `persist: true` (default). The mapping’s key on the service. |
| `state` | `persist: false`. The mapping itself. Never stored on the service. |
| `entitiesFound` | Count per type: `{ PERSON: 2 }`. No plain names. |
| `balanceCredits` | Balance left after the call: `"9.00"`. |
| `requestId` | The response’s `X-Request-Id`. |
| `placeholderStyle` | `"angle"` or `"bracket_tr"`. |
| `keepLegalForm` | Applied value. |
| `healthData` | Applied value. |
| `llmReview` | If `llmReview: true` was requested, a status summary. No plain names. |

### Options

The second argument is an object. All options are off by default.

| Field | Default | Effect |
|---|---|---|
| `persist` | `true` | `false` stores nothing on the service and leaves `docId` empty. |
| `placeholderStyle` | `"angle"` | `"bracket_tr"` → `[KİŞİ_1]`. Any other value throws `BBError(400)` before the request is sent. |
| `keepLegalForm` | `false` | `true` keeps the company type suffix outside the tag. |
| `healthData` | `false` | `true` also tags health data. |
| `llmReview` | `false` | `true` requests a second LLM pass. |

The Python names are accepted too: `placeholder_style`, `keep_legal_form`, `health_data`, `llm_review`.

### `BBError`

`statusCode`, `detail`, `requestId`. `String(error)` is the detail text. Network errors are `BBError` too.

| Code | Meaning |
|---|---|
| 400 | Invalid `placeholderStyle`, file count outside 1–3, nothing anonymized in the session. |
| 401 | `init` not called, or the key is empty or unknown. |
| 402 | No balance. |
| 404 | Mapping not found. |
| 413 | File larger than 20 MB. |
| 502 | The response is not JSON. |
| 503 | Service unreachable. |
| 504 | `timeout` expired. The request may have reached the service. |

## Calls

File extensions: `.txt`, `.pdf` or `.docx`. Up to 20 MB per file.

`anonymizeFile(path)` reads a path. If you already hold the bytes, use `anonymizeBytes(filename, content)`; both names mean the same thing on the module and on `BBAnonym`. `delete` is a reserved word in JavaScript, so the delete method is `deleteMapping`.

### Mapping on the service

```js
const tagged = await anonymize(text);
const restored = await restore(tagged.docId, await myLlm(tagged.text));
await deleteMapping(tagged.docId);
```

One file, raw bytes, and several files (up to 3, one `docId`, one fee):

```js
const tagged = await anonymizeFile("petition.pdf");
const fromBytes = await anonymizeBytes("petition.pdf", buffer);
const batch = await anonymizeDocuments(["petition.pdf", "response.docx", "appendix.txt"]);
```

### Dictionary

```js
const mapping = await getMap(tagged.docId);
const readOnce = await getMap(tagged.docId, true);
```

`true` also deletes the service copy after reading. The dictionary carries plain names; only `tagged.text` goes to the model.

### Mapping on your side

```js
const tagged = await anonymize(text, { persist: false });
const restored = await restore(tagged.state, await myLlm(tagged.text));
```

### Session

The `docId` and `state` stay inside the session. `close` runs on exit: with `persist: true` every kept id is deleted.

```js
const s = session();
try {
  const tagged = await s.anonymize(text);
  const restored = await s.restore(await myLlm(tagged.text));
} finally {
  await s.close();
}
```

If your Node supports `await using`, `await using s = session()` makes the same `close` call.

```js
const s = session({ persist: false, placeholderStyle: "bracket_tr" });
```

## Reference

```js
init(apiKey, { baseUrl, timeout } = {})
anonymize(text, options) -> Promise<Tagged>
anonymizeFile(path, options) -> Promise<Tagged>
anonymizeBytes(filename, content, options) -> Promise<Tagged>
anonymizeDocuments(paths, options) -> Promise<Tagged>
restore(docIdOrState, modelText) -> Promise<string>
getMap(docId, once = false) -> Promise<object>
deleteMapping(docId) -> Promise<boolean>
session(options) -> MaskSession
```

The `BBAnonym` constructor does not need `init`; it has the same methods. The `token` field is non-enumerable: `console.log` and `JSON.stringify` never show the key. The package ships type definitions in `index.d.ts`.
