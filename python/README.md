# bb-mask

**English** · [Türkçe](https://github.com/bigbrains-company/bb-mask-sdk/blob/main/python/README.tr.md)

Tags personal data in Turkish legal text and puts the real values back into the model’s answer. The package is a thin HTTP client. It never calls a model, never prints, never sleeps.

You get a `dm_test_` key from the Data Mask console. Your backend installs this package and passes the key to `init`. The LLM call is yours; `anonymize` goes before it and `restore` after it.

```python
import os
import bb_mask

bb_mask.init(api_key=os.environ["BB_ANONYM_TOKEN"])


def summarize(text: str) -> str:
    tagged = bb_mask.anonymize(text)
    reply = my_llm(tagged.text)                    # your code
    return bb_mask.restore(tagged.doc_id, reply)
```

Python 3.11+. The only dependency is `httpx`. This folder is the Python package. Node, Go, C# and Java live in the other folders of the same repository.

## Install

Once published:

```bash
pip install bb-mask
```

Before release, from the repository root:

```bash
pip install -e python
```

Never put the key in code or in the repository. An environment variable or your own config file is enough.

```bash
export BB_ANONYM_TOKEN="dm_test_..."
```

`init` runs once per process. It is not a model call. Module functions (`bb_mask.anonymize` and the rest) called before it raise `BBError(401)` and never reach the service.

```python
client = bb_mask.init(
    api_key=os.environ["BB_ANONYM_TOKEN"],
    base_url="https://app.bbmask.co",  # this is the default
    timeout=150,                       # seconds
)
```

| Argument | Type | Meaning |
|---|---|---|
| `api_key` | `str` | The `dm_test_` key from the console. Empty → `BBError(401)`. |
| `base_url` | `str` | Service address. Default `https://app.bbmask.co`. |
| `timeout` | `float` | HTTP timeout in seconds. Default `150`; the engine waits up to 120 s and the client gives up after that. |

Returns a `BBAnonym`. Module functions use this client. A second `init` replaces the first.

If you need a second key, construct `BBAnonym` without touching the process-wide client. The constructor does not need `init`.

```python
other = bb_mask.BBAnonym("https://app.bbmask.co", token, timeout=150)
```

## Objects

### `Tagged`

Returned by `anonymize`, `anonymize_file` and `anonymize_documents`.

| Field | Type | When it is set |
|---|---|---|
| `text` | `str` | Always. Personal data is replaced by tags: `<PERSON_0>`, `<TR_NATIONAL_ID_0>`. |
| `doc_id` | `str \| None` | `persist=True` (default). The mapping’s key on the service. Only a key from the same workspace can open it. |
| `state` | `dict \| None` | `persist=False`. The mapping itself. Never stored on the service. |
| `entities_found` | `dict[str, int]` | Always. Count per type: `{"PERSON": 2}`. No plain names. |
| `balance_credits` | `str \| None` | Balance left after the call: `"9.00"`. |
| `request_id` | `str \| None` | The response’s `X-Request-Id`. Matches a row on the console’s Usage page. |
| `placeholder_style` | `str` | Applied tag format: `"angle"` or `"bracket_tr"`. |
| `keep_legal_form` | `bool` | Applied value. |
| `health_data` | `bool` | Applied value. |
| `llm_review` | `dict \| None` | If `llm_review=True` was requested, a status summary: `status`, `model`, `chunks`, `chunks_failed`, `elapsed_ms`, `added_spans`, `error`. No plain names. |

```python
tagged = bb_mask.anonymize("Davacı Hasan Demirkol, TC 62601815964")
tagged.doc_id   # "a1b2..."
tagged.text     # "Davacı <PERSON_0>, TC <TR_NATIONAL_ID_0>"
tagged.state    # None
```

Tags look like `<TYPE_0>`. The same surface form gets the same number. If the model breaks a tag (`<PERSON 0>`), the service repairs it. Court and statute names are not tagged.

### Options

All four anonymize calls (`anonymize`, `anonymize_file`, `anonymize_bytes`, `anonymize_documents`), their async counterparts and `session()` take the same keyword-only parameters. All are off by default; the service never falls back to its own defaults.

| Parameter | Default | Effect |
|---|---|---|
| `placeholder_style` | `"angle"` | `"angle"` → `<PERSON_0>`. `"bracket_tr"` → `[KİŞİ_1]`, `[ŞİRKET_1]`; numbering starts at 1. Any other value raises `BBError(400)` before the request is sent. |
| `keep_legal_form` | `False` | `True` → the company type suffix stays outside the tag: `[ŞİRKET_1] A.Ş.`, `<ORGANIZATION_0> Ltd. Şti.` |
| `health_data` | `False` | `True` → also tags health data under KVKK Art. 6: diagnoses, medication, lab results, blood type, patient numbers, healthcare providers. |
| `llm_review` | `False` | `True` → an LLM makes a second pass for people, companies, addresses and IDs the rules missed. The model sees the temporarily anonymized text. If the service layer is off, it is not an error; `Tagged.llm_review["status"]` tells you. |

```python
tagged = bb_mask.anonymize(
    "Davalı Kaya İnşaat A.Ş. çalışanı Ali Kaya, tanı: diyabet",
    placeholder_style="bracket_tr",
    keep_legal_form=True,
    health_data=True,
    llm_review=True,
)
tagged.text        # "Davalı [ŞİRKET_1] A.Ş. çalışanı [KİŞİ_1], tanı: [TANI_1]"
tagged.llm_review  # {"status": "ok", "model": "qwen/qwen3.8-27b", "added_spans": 0, ...}
```

The `state` and `get_map` dictionaries carry canonical tags (`<PERSON_0>`) regardless of format. `restore` opens both formats.

### The `state` / `get_map` dictionary

Shape: `dict[str, dict[str, str]]`. The outer key is the entity type, the inner key is the plain text, the value is the tag.

```python
{
    "PERSON": {"Hasan Demirkol": "<PERSON_0>"},
    "TR_NATIONAL_ID": {"62601815964": "<TR_NATIONAL_ID_0>"},
}
```

The dictionary carries plain names. Never put it in logs, tickets or the model. `restore` uses it for you; only `tagged.text` goes to the model.

### `BBError`

Subclass of `RuntimeError`. `status_code: int`, `detail: str`, `request_id: str | None`. `str(error)` is the detail text. `request_id` is set when the service responded; include it when you contact support.

Network errors are `BBError` too; no `httpx` exception leaks out.

| Code | Meaning |
|---|---|
| 400 | Empty text, file count outside 1–3, invalid `placeholder_style`, nothing anonymized in the session, or neither `doc_id` nor `state` given. |
| 401 | `init` not called, or the key is empty or unknown. |
| 402 | No balance. Nothing is sent for anonymization. |
| 404 | Mapping not found. It was deleted, read with `once=True`, belongs to another workspace, or was never stored. |
| 413 | File larger than 20 MB. |
| 502 | The engine failed; the anonymize fee is refunded. A non-JSON response is also 502. |
| 503 | Service unreachable. The request was not sent. |
| 504 | `timeout` expired. The request may have reached the service and the fee may have been charged. Check the Usage page in the console. |

The fee is charged on a successful anonymize call. If the service fails, the fee is refunded. `restore`, `get_map` and `delete` are free.

### `MaskSession`

Returned by `with bb_mask.session()`. The `doc_id` and `state` stay inside the session.

| Method | Returns |
|---|---|
| `anonymize(text)` | `Tagged` |
| `anonymize_file(path)` | `Tagged`. Reads the path. |
| `anonymize_documents(paths)` | `Tagged` |
| `restore(model_text)` | `str` |
| `close()` | `None`. Called automatically when the block ends. |
| `persist` | `bool`. `True` for pattern 1. Fixed for the session. |
| `options` | `dict`. The options passed to `session(...)`. Sent with every anonymize call. |

Here `restore` takes only the model text. Called before any anonymize, it raises `BBError(400)`. When the block ends `close` runs: with `persist=True` every kept `doc_id` is deleted; with `persist=False` the state is dropped and no delete is sent.

## Calls

All of them need `init` first. File extensions: `.txt`, `.pdf` or `.docx`. Up to 20 MB per file.

`anonymize_file(path)` takes a path and reads the file. If you already hold the bytes, use `anonymize_bytes(filename, content)`. Both names mean the same thing on the module and on `BBAnonym`. `MaskSession.anonymize_file(path)` reads a path too.

### Pattern 1 — mapping on the service, you keep the `doc_id`

```python
tagged = bb_mask.anonymize(text)
reply = my_llm(tagged.text)
restored = bb_mask.restore(tagged.doc_id, reply)
bb_mask.delete(tagged.doc_id)          # when you are done
```

One file:

```python
tagged = bb_mask.anonymize_file("petition.pdf")
restored = bb_mask.restore(tagged.doc_id, my_llm(tagged.text))
```

Several files share one `doc_id`. The same person is `<PERSON_0>` in both files. Up to 3 files. The fee is one anonymize.

```python
tagged = bb_mask.anonymize_documents(
    ["petition.pdf", "response.docx", "appendix.txt"]
)
restored = bb_mask.restore(tagged.doc_id, my_llm(tagged.text))
```

`restore` does not take the original file. It opens the tags in the text the model wrote.

### Pattern 2 — same `doc_id`, plus the dictionary

```python
tagged = bb_mask.anonymize(text)
mapping = bb_mask.get_map(tagged.doc_id)
# mapping["PERSON"]["Hasan Demirkol"] == "<PERSON_0>"

restored = bb_mask.restore(tagged.doc_id, my_llm(tagged.text))
```

Delete the service copy as soon as you read it:

```python
mapping = bb_mask.get_map(tagged.doc_id, once=True)
# from now on restore and get_map return 404
```

If you do not need the dictionary, pattern 1 is enough.

### Pattern 3 — mapping on your side, the service stores nothing

`doc_id` is empty. `state` is the dictionary. Never send it to the model. You can store `state` on your side so `restore` still works after the process exits.

```python
tagged = bb_mask.anonymize(text, persist=False)
assert tagged.doc_id is None and tagged.state is not None

reply = my_llm(tagged.text)
restored = bb_mask.restore(tagged.state, reply)
```

Files work the same way: `anonymize_file(path, persist=False)`, `anonymize_documents(paths, persist=False)`.

### Pattern 4 — you never see an id or JSON

Pattern 1. The mapping is deleted on exit.

```python
with bb_mask.session() as s:
    tagged = s.anonymize_documents(["petition.pdf", "appendix.txt"])
    reply = my_llm(tagged.text)
    restored = s.restore(reply)
```

Pattern 3 surface. The state lives in the session object and is never stored on the service.

```python
with bb_mask.session(persist=False) as s:
    tagged = s.anonymize(text)
    restored = s.restore(my_llm(tagged.text))
```

### Async

There are four functions. Signatures match their sync counterparts. The HTTP work runs in a thread. They need `init` too.

```python
aanonymize(text, persist=True, *, <options>) -> Tagged
aanonymize_file(path, persist=True, *, <options>) -> Tagged
aanonymize_documents(paths, persist=True, *, <options>) -> Tagged
arestore(doc_id_or_state, model_text) -> str
```

```python
tagged = await bb_mask.aanonymize(text)
restored = await bb_mask.arestore(tagged.doc_id, reply)
```

`get_map`, `delete`, `session` and `anonymize_bytes` have no async version.

## Reference

Module functions need `init`. `BBAnonym` methods use the key given to the constructor.

### Module

```python
init(api_key: str, base_url: str = "https://app.bbmask.co", timeout: float = 150) -> BBAnonym
anonymize(text: str, persist: bool = True, *, <options>) -> Tagged
anonymize_file(path: str | Path, persist: bool = True, *, <options>) -> Tagged
anonymize_bytes(filename: str, content: bytes, persist: bool = True, *, <options>) -> Tagged
anonymize_documents(paths: list[str | Path], persist: bool = True, *, <options>) -> Tagged
restore(doc_id_or_state: str | dict, model_text: str) -> str
get_map(doc_id: str, once: bool = False) -> dict
delete(doc_id: str) -> bool
session(persist: bool = True, *, <options>)  # MaskSession via with
aanonymize(text: str, persist: bool = True, *, <options>) -> Tagged
aanonymize_file(path: str | Path, persist: bool = True, *, <options>) -> Tagged
aanonymize_documents(paths: list[str | Path], persist: bool = True, *, <options>) -> Tagged
arestore(doc_id_or_state: str | dict, model_text: str) -> str
```

`<options>` are the four keyword-only parameters; they are spelled out in the signature, so your editor completes them and `help(bb_mask.anonymize)` explains them:

```python
placeholder_style: Literal["angle", "bracket_tr"] = "angle",
keep_legal_form: bool = False,
health_data: bool = False,
llm_review: bool = False,
```

`persist=True` keeps the mapping on the service and sets `Tagged.doc_id`. `persist=False` stores nothing, sets `Tagged.state`, and `doc_id` is `None`.

`restore` takes a `doc_id` (`str`) or a `state` (`dict`) first. The second argument is the text the model wrote. It returns plain text.

`get_map` returns the dictionary. `once=True` deletes the service copy after reading; after that `restore` and `get_map` return `404`.

`delete` returns `True` when the service reports `deleted`.

`session` is a context manager: `with bb_mask.session() as s`. `s.close()` runs on exit.

### `BBAnonym`

No `init` needed. Same names as the module. It shares a single `httpx.Client` (connection pool); close it with `with` or `close()`. `repr()` masks the key.

```python
BBAnonym(base_url: str, token: str, timeout: float = 150)
anonymize(text: str, persist: bool = True, *, <options>) -> Tagged
anonymize_file(path: str | Path, persist: bool = True, *, <options>) -> Tagged
anonymize_bytes(filename: str, content: bytes, persist: bool = True, *, <options>) -> Tagged
anonymize_documents(paths: list[str | Path], persist: bool = True, *, <options>) -> Tagged
restore(doc_id_or_state: str | dict, model_text: str) -> str
get_map(doc_id: str, once: bool = False) -> dict
delete(doc_id: str) -> bool
close() -> None
```

The package ships `py.typed`; type checkers read the signatures.

### `MaskSession`

Not constructed directly. `persist` and the options are given when the session opens and are not passed to the methods again: `bb_mask.session(placeholder_style="bracket_tr", llm_review=True)`.

```python
anonymize(text: str) -> Tagged
anonymize_file(path: str | Path) -> Tagged
anonymize_documents(paths: list[str | Path]) -> Tagged
restore(model_text: str) -> str
close() -> None
```

## Not in this package

- Your LLM call and `@ask`.
- A FastAPI app, console output, `sleep`.
- Key creation and balance. Those are in the console.
- Tag repair. The service fixes broken tags; dictionary keys are plain text.
