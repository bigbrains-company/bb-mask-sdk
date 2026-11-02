"""bb_mask HTTP client. This module never calls a model.

doc_id stays on the service. get_map returns the dictionary. persist=False returns state.
session() hides the doc_id or state.

anonymize_file(path) reads a path; anonymize_bytes(filename, content) takes bytes.
Both names mean the same thing on the module and on BBAnonym.
"""

from __future__ import annotations

import asyncio
import threading
from contextlib import contextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterator, Literal
from urllib.parse import quote

import httpx

# Customer code talks to the Data Mask container on the server (8081).
DEFAULT_BASE_URL = "https://app.bbmask.co"
# The engine waits 120 s; the client gives up after it so the refund/502 decision comes from the server.
DEFAULT_TIMEOUT = 150.0
VERSION = "0.3.1"

PlaceholderStyle = Literal["angle", "bracket_tr"]
PLACEHOLDER_STYLES = ("angle", "bracket_tr")


class BBError(RuntimeError):
    def __init__(self, status_code: int, detail: str, request_id: str | None = None):
        super().__init__(detail)
        self.status_code = status_code
        self.detail = detail
        self.request_id = request_id


@dataclass
class Tagged:
    text: str
    doc_id: str | None = None
    state: dict | None = None
    entities_found: dict = field(default_factory=dict)
    balance_credits: str | None = None
    request_id: str | None = None
    placeholder_style: str = "angle"
    keep_legal_form: bool = False
    health_data: bool = False
    llm_review: dict | None = None


def _options(
    placeholder_style: PlaceholderStyle,
    keep_legal_form: bool,
    health_data: bool,
    llm_review: bool,
) -> dict:
    if placeholder_style not in PLACEHOLDER_STYLES:
        raise BBError(400, "placeholder_style must be angle or bracket_tr")
    return {
        "placeholder_style": placeholder_style,
        "keep_legal_form": bool(keep_legal_form),
        "health_data": bool(health_data),
        "llm_review": bool(llm_review),
    }


def _form(persist: bool, options: dict) -> dict:
    data = {"persist": str(persist).lower()}
    for key, value in options.items():
        data[key] = str(value).lower() if isinstance(value, bool) else value
    return data


def _mask(token: str) -> str:
    return f"{token[:8]}…{token[-4:]}" if len(token) > 12 else "…"


class BBAnonym:
    """Client for one key. The connection pool lives with the object; close it with `with` or close()."""

    def __init__(self, base_url: str, token: str, timeout: float = DEFAULT_TIMEOUT):
        self.base_url = (base_url or DEFAULT_BASE_URL).rstrip("/")
        self._token = token
        self.timeout = timeout
        self._http: httpx.Client | None = None
        self._lock = threading.Lock()

    def __repr__(self) -> str:
        return f"BBAnonym(base_url={self.base_url!r}, token={_mask(self._token)!r}, timeout={self.timeout})"

    def __enter__(self) -> BBAnonym:
        return self

    def __exit__(self, *_exc) -> None:
        self.close()

    def close(self) -> None:
        with self._lock:
            if self._http is not None:
                self._http.close()
                self._http = None

    def _client(self) -> httpx.Client:
        with self._lock:
            if self._http is None:
                self._http = httpx.Client(
                    base_url=self.base_url,
                    timeout=self.timeout,
                    headers={"Authorization": f"Bearer {self._token}", "User-Agent": f"bb-mask-python/{VERSION}"},
                )
            return self._http

    def _send(self, method: str, path: str, **kwargs) -> httpx.Response:
        try:
            return self._client().request(method, path, **kwargs)
        except httpx.TimeoutException as exc:
            raise BBError(504, f"Timed out ({self.timeout:g} s). The fee may have been charged.") from exc
        except httpx.HTTPError as exc:
            raise BBError(503, f"Could not reach the service ({self.base_url})") from exc

    def _parse(self, response: httpx.Response) -> dict:
        request_id = response.headers.get("x-request-id")
        if response.status_code >= 400:
            detail = response.text
            try:
                body = response.json()
                if isinstance(body, dict) and body.get("detail"):
                    detail = str(body["detail"])
            except Exception:
                pass
            raise BBError(response.status_code, detail, request_id)
        try:
            body = response.json()
        except ValueError as exc:
            raise BBError(502, "The service did not return JSON", request_id) from exc
        if not isinstance(body, dict):
            raise BBError(502, "The service did not return JSON", request_id)
        body["_request_id"] = request_id
        return body

    def _tagged(self, body: dict, persist: bool) -> Tagged:
        doc_id = body.get("doc_id") or None
        if not persist:
            doc_id = None
        return Tagged(
            text=body.get("text") or "",
            doc_id=doc_id,
            state=body.get("state"),
            entities_found=body.get("entities_found") or {},
            balance_credits=body.get("balance_credits"),
            request_id=body.get("_request_id"),
            placeholder_style=body.get("placeholder_style") or "angle",
            keep_legal_form=bool(body.get("keep_legal_form")),
            health_data=bool(body.get("health_data")),
            llm_review=body.get("llm_review"),
        )

    def anonymize(
        self,
        text: str,
        persist: bool = True,
        *,
        placeholder_style: PlaceholderStyle = "angle",
        keep_legal_form: bool = False,
        health_data: bool = False,
        llm_review: bool = False,
    ) -> Tagged:
        """Anonymizes text. 1 credit.

        persist: True → mapping on the service, Tagged.doc_id set. False → Tagged.state set.
        placeholder_style: "angle" → <PERSON_0>. "bracket_tr" → [KİŞİ_1]. Any other value: BBError(400).
        keep_legal_form: True → company type suffix stays outside the tag: [ŞİRKET_1] A.Ş.
        health_data: True → health data such as diagnoses, drugs, lab results and blood type is tagged too.
        llm_review: True → an LLM searches a second time for what the rules missed. Status in Tagged.llm_review.
        """
        options = _options(placeholder_style, keep_legal_form, health_data, llm_review)
        response = self._send("POST", "/v1/anonymize", json={"text": text, "persist": persist, **options})
        return self._tagged(self._parse(response), persist)

    def anonymize_bytes(
        self,
        filename: str,
        content: bytes,
        persist: bool = True,
        *,
        placeholder_style: PlaceholderStyle = "angle",
        keep_legal_form: bool = False,
        health_data: bool = False,
        llm_review: bool = False,
    ) -> Tagged:
        """One file from bytes (.txt, .pdf, .docx). 1 credit. Options as in anonymize."""
        options = _options(placeholder_style, keep_legal_form, health_data, llm_review)
        response = self._send(
            "POST",
            "/v1/anonymize/file",
            data=_form(persist, options),
            files={"file": (filename, content, "application/octet-stream")},
        )
        return self._tagged(self._parse(response), persist)

    def anonymize_file(
        self,
        path: str | Path,
        persist: bool = True,
        *,
        placeholder_style: PlaceholderStyle = "angle",
        keep_legal_form: bool = False,
        health_data: bool = False,
        llm_review: bool = False,
    ) -> Tagged:
        """One file from a path. 1 credit. Options as in anonymize."""
        file_path = Path(path)
        return self.anonymize_bytes(
            file_path.name,
            file_path.read_bytes(),
            persist,
            placeholder_style=placeholder_style,
            keep_legal_form=keep_legal_form,
            health_data=health_data,
            llm_review=llm_review,
        )

    def anonymize_documents(
        self,
        paths: list[str | Path],
        persist: bool = True,
        *,
        placeholder_style: PlaceholderStyle = "angle",
        keep_legal_form: bool = False,
        health_data: bool = False,
        llm_review: bool = False,
    ) -> Tagged:
        """1–3 files, one doc_id. 1 credit. Options as in anonymize."""
        options = _options(placeholder_style, keep_legal_form, health_data, llm_review)
        if not 1 <= len(paths) <= 3:
            raise BBError(400, "1–3 files per request")
        files = []
        for path in paths:
            file_path = Path(path)
            files.append(("files", (file_path.name, file_path.read_bytes(), "application/octet-stream")))
        response = self._send("POST", "/v1/anonymize/documents", data=_form(persist, options), files=files)
        return self._tagged(self._parse(response), persist)

    def restore(self, doc_id_or_state: str | dict, model_text: str) -> str:
        payload: dict = {"text": model_text}
        if isinstance(doc_id_or_state, dict):
            payload["state"] = doc_id_or_state
        else:
            payload["doc_id"] = doc_id_or_state
        response = self._send("POST", "/v1/restore", json=payload)
        return self._parse(response).get("text") or ""

    def get_map(self, doc_id: str, once: bool = False) -> dict:
        response = self._send("GET", f"/v1/mappings/{quote(doc_id, safe='')}", params={"once": str(once).lower()})
        return self._parse(response).get("state") or {}

    def delete(self, doc_id: str) -> bool:
        response = self._send("DELETE", f"/v1/mappings/{quote(doc_id, safe='')}")
        return bool(self._parse(response).get("deleted"))


class MaskSession:
    """persist=True keeps the mapping on the service and deletes it on exit.
    persist=False keeps the state in the session object. Options are fixed for the whole session."""

    def __init__(
        self,
        client: BBAnonym,
        persist: bool = True,
        *,
        placeholder_style: PlaceholderStyle = "angle",
        keep_legal_form: bool = False,
        health_data: bool = False,
        llm_review: bool = False,
    ):
        self._client = client
        self.persist = persist
        self.options = _options(placeholder_style, keep_legal_form, health_data, llm_review)
        self._ids: list[str] = []
        self._doc_id: str | None = None
        self._state: dict | None = None

    def _remember(self, tagged: Tagged) -> Tagged:
        self._doc_id = tagged.doc_id
        self._state = tagged.state
        if tagged.doc_id:
            self._ids.append(tagged.doc_id)
        return tagged

    def anonymize(self, text: str) -> Tagged:
        return self._remember(self._client.anonymize(text, self.persist, **self.options))

    def anonymize_file(self, path: str | Path) -> Tagged:
        return self._remember(self._client.anonymize_file(path, self.persist, **self.options))

    def anonymize_documents(self, paths: list[str | Path]) -> Tagged:
        return self._remember(self._client.anonymize_documents(paths, self.persist, **self.options))

    def restore(self, model_text: str) -> str:
        if self.persist:
            if not self._doc_id:
                raise BBError(400, "Nothing anonymized in this session")
            return self._client.restore(self._doc_id, model_text)
        if not self._state:
            raise BBError(400, "No state in this session")
        return self._client.restore(self._state, model_text)

    def close(self) -> None:
        if not self.persist:
            self._state = None
            return
        for doc_id in self._ids:
            try:
                self._client.delete(doc_id)
            except BBError:
                pass
        self._ids.clear()


_current: BBAnonym | None = None


def init(api_key: str, base_url: str = DEFAULT_BASE_URL, timeout: float = DEFAULT_TIMEOUT) -> BBAnonym:
    """Once per process. Not a model call."""
    global _current
    if not api_key.strip():
        raise BBError(401, "api_key is empty")
    _current = BBAnonym(base_url, api_key.strip(), timeout)
    return _current


def _require() -> BBAnonym:
    if _current is None:
        raise BBError(401, "bb_mask.init was not called")
    return _current


def anonymize(
    text: str,
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """Anonymizes text. 1 credit. Requires init.

    persist: True → mapping on the service, Tagged.doc_id set. False → Tagged.state set.
    placeholder_style: "angle" → <PERSON_0>. "bracket_tr" → [KİŞİ_1]. Any other value: BBError(400).
    keep_legal_form: True → company type suffix stays outside the tag: [ŞİRKET_1] A.Ş.
    health_data: True → health data such as diagnoses, drugs, lab results and blood type is tagged too.
    llm_review: True → an LLM searches a second time for what the rules missed. Status in Tagged.llm_review.
    """
    return _require().anonymize(
        text,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


def anonymize_file(
    path: str | Path,
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """One file from a path (.txt, .pdf, .docx). 1 credit. Options as in anonymize."""
    return _require().anonymize_file(
        path,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


def anonymize_bytes(
    filename: str,
    content: bytes,
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """One file from bytes. 1 credit. Options as in anonymize."""
    return _require().anonymize_bytes(
        filename,
        content,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


def anonymize_documents(
    paths: list[str | Path],
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """1–3 files, one doc_id. 1 credit. Options as in anonymize."""
    return _require().anonymize_documents(
        paths,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


def restore(doc_id_or_state: str | dict, model_text: str) -> str:
    return _require().restore(doc_id_or_state, model_text)


def get_map(doc_id: str, once: bool = False) -> dict:
    return _require().get_map(doc_id, once=once)


def delete(doc_id: str) -> bool:
    return _require().delete(doc_id)


@contextmanager
def session(
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Iterator[MaskSession]:
    """MaskSession for use with `with`. Options as in anonymize; applied to every call in the session."""
    handle = MaskSession(
        _require(),
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )
    try:
        yield handle
    finally:
        handle.close()


async def aanonymize(
    text: str,
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """Async version of anonymize. Same options."""
    return await asyncio.to_thread(
        anonymize,
        text,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


async def aanonymize_file(
    path: str | Path,
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """Async version of anonymize_file. Same options."""
    return await asyncio.to_thread(
        anonymize_file,
        path,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


async def aanonymize_documents(
    paths: list[str | Path],
    persist: bool = True,
    *,
    placeholder_style: PlaceholderStyle = "angle",
    keep_legal_form: bool = False,
    health_data: bool = False,
    llm_review: bool = False,
) -> Tagged:
    """Async version of anonymize_documents. Same options."""
    return await asyncio.to_thread(
        anonymize_documents,
        paths,
        persist,
        placeholder_style=placeholder_style,
        keep_legal_form=keep_legal_form,
        health_data=health_data,
        llm_review=llm_review,
    )


async def arestore(doc_id_or_state: str | dict, model_text: str) -> str:
    return await asyncio.to_thread(restore, doc_id_or_state, model_text)
