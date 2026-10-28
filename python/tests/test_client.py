"""Yerel sahte sunucuya karşı istemci davranışı. Gerçek servise gidilmez."""

from __future__ import annotations

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

import bb_mask
from bb_mask import BBAnonym, BBError


def tagged_body(**extra):
    body = {
        "doc_id": "doc-1",
        "text": "Davacı <PERSON_0>",
        "entities_found": {"PERSON": 1},
        "balance_credits": "9.00",
        "state": None,
        "placeholder_style": "angle",
        "keep_legal_form": False,
        "health_data": False,
        "llm_review": None,
    }
    body.update(extra)
    return body


class Fake:
    """handler(method, path, headers, raw) -> (status, body | str, request_id)."""

    def __init__(self, handler):
        self.calls: list[dict] = []
        outer = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def _serve(self):
                length = int(self.headers.get("content-length") or 0)
                raw = self.rfile.read(length) if length else b""
                outer.calls.append({"method": self.command, "path": self.path, "headers": self.headers, "raw": raw})
                result = handler(self.command, self.path, self.headers, raw)
                if result is None:  # hang
                    time.sleep(2)
                    return
                status, body, request_id = result
                payload = body.encode() if isinstance(body, str) else json.dumps(body).encode()
                self.send_response(status)
                if not isinstance(body, str):
                    self.send_header("content-type", "application/json")
                self.send_header("x-request-id", request_id)
                self.send_header("content-length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            do_GET = do_POST = do_DELETE = _serve

            def log_message(self, *_args):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


@pytest.fixture
def fake():
    servers = []

    def make(handler):
        server = Fake(handler)
        servers.append(server)
        return server

    yield make
    for server in servers:
        server.close()


def test_anonymize_sends_wire_body_and_maps_tagged(fake):
    server = fake(lambda m, p, h, raw: (200, tagged_body(), "req-1"))
    with BBAnonym(server.url, "dm_test_abc") as client:
        tagged = client.anonymize(
            "Davacı Hasan Demirkol",
            placeholder_style="bracket_tr",
            keep_legal_form=True,
            health_data=True,
            llm_review=True,
        )
    call = server.calls[0]
    assert (call["method"], call["path"]) == ("POST", "/v1/anonymize")
    assert call["headers"]["authorization"] == "Bearer dm_test_abc"
    assert call["headers"]["user-agent"].startswith("bb-mask-python/")
    assert json.loads(call["raw"]) == {
        "text": "Davacı Hasan Demirkol",
        "persist": True,
        "placeholder_style": "bracket_tr",
        "keep_legal_form": True,
        "health_data": True,
        "llm_review": True,
    }
    assert tagged.text == "Davacı <PERSON_0>"
    assert tagged.doc_id == "doc-1"
    assert tagged.state is None
    assert tagged.entities_found == {"PERSON": 1}
    assert tagged.balance_credits == "9.00"
    assert tagged.request_id == "req-1"


def test_persist_false_drops_doc_id(fake):
    server = fake(lambda m, p, h, raw: (200, tagged_body(state={"PERSON": {"Ali": "<PERSON_0>"}}), "r"))
    tagged = BBAnonym(server.url, "dm_test_abc").anonymize("Ali", persist=False)
    assert json.loads(server.calls[0]["raw"])["persist"] is False
    assert tagged.doc_id is None
    assert tagged.state == {"PERSON": {"Ali": "<PERSON_0>"}}


def test_invalid_style_fails_before_request():
    with pytest.raises(BBError) as exc:
        BBAnonym("http://127.0.0.1:9", "dm_test_abc").anonymize("Ali", placeholder_style="stars")  # type: ignore[arg-type]
    assert exc.value.status_code == 400


def test_restore_sends_doc_id_or_state(fake):
    server = fake(lambda m, p, h, raw: (200, {"text": "açık metin"}, "r"))
    client = BBAnonym(server.url, "dm_test_abc")
    assert client.restore("doc-1", "<PERSON_0>") == "açık metin"
    assert client.restore({"PERSON": {}}, "<PERSON_0>") == "açık metin"
    assert json.loads(server.calls[0]["raw"]) == {"text": "<PERSON_0>", "doc_id": "doc-1"}
    assert json.loads(server.calls[1]["raw"]) == {"text": "<PERSON_0>", "state": {"PERSON": {}}}


def test_get_map_encodes_id_and_delete_reads_deleted(fake):
    def handler(method, path, headers, raw):
        if method == "GET":
            return 200, {"state": {"PERSON": {"Ali": "<PERSON_0>"}}}, "r"
        return 200, {"deleted": True}, "r"

    server = fake(handler)
    client = BBAnonym(server.url, "dm_test_abc")
    assert client.get_map("a/b c", once=True) == {"PERSON": {"Ali": "<PERSON_0>"}}
    assert client.delete("a/b c") is True
    assert server.calls[0]["path"] == "/v1/mappings/a%2Fb%20c?once=true"
    assert (server.calls[1]["method"], server.calls[1]["path"]) == ("DELETE", "/v1/mappings/a%2Fb%20c")


def test_file_upload_uses_multipart_fields(fake, tmp_path):
    path = tmp_path / "dilekce.txt"
    path.write_text("Davacı Ali", encoding="utf-8")
    server = fake(lambda m, p, h, raw: (200, tagged_body(doc_id="should-drop"), "r"))
    tagged = BBAnonym(server.url, "dm_test_abc").anonymize_file(path, persist=False)
    body = server.calls[0]["raw"].decode("utf-8")
    assert server.calls[0]["path"] == "/v1/anonymize/file"
    assert 'name="persist"\r\n\r\nfalse' in body
    assert 'name="placeholder_style"\r\n\r\nangle' in body
    assert 'name="keep_legal_form"\r\n\r\nfalse' in body
    assert 'name="file"' in body and 'filename="dilekce.txt"' in body and "Davacı Ali" in body
    assert tagged.doc_id is None


def test_anonymize_bytes_matches_file(fake):
    server = fake(lambda m, p, h, raw: (200, tagged_body(), "r"))
    tagged = BBAnonym(server.url, "dm_test_abc").anonymize_bytes("ek.pdf", b"%PDF")
    body = server.calls[0]["raw"]
    assert b'filename="ek.pdf"' in body and b"%PDF" in body
    assert tagged.doc_id == "doc-1"


def test_documents_sends_one_to_three_files(fake, tmp_path):
    first, second = tmp_path / "a.txt", tmp_path / "b.txt"
    first.write_text("bir")
    second.write_text("iki")
    server = fake(lambda m, p, h, raw: (200, tagged_body(), "r"))
    client = BBAnonym(server.url, "dm_test_abc")
    assert client.anonymize_documents([first, second]).doc_id == "doc-1"
    body = server.calls[0]["raw"].decode()
    assert server.calls[0]["path"] == "/v1/anonymize/documents"
    assert body.count('name="files"') == 2
    assert 'filename="a.txt"' in body and 'filename="b.txt"' in body
    with pytest.raises(BBError) as exc:
        client.anonymize_documents([])
    assert exc.value.status_code == 400
    with pytest.raises(BBError) as exc:
        client.anonymize_documents(["1", "2", "3", "4"], placeholder_style="nope")  # type: ignore[arg-type]
    assert exc.value.status_code == 400 and "placeholder_style" in exc.value.detail
    assert len(server.calls) == 1


def test_http_errors_keep_status_detail_request_id(fake):
    server = fake(lambda m, p, h, raw: (402, {"detail": "Bakiye yok"}, "req-402"))
    with pytest.raises(BBError) as exc:
        BBAnonym(server.url, "dm_test_abc").anonymize("Ali")
    assert (exc.value.status_code, exc.value.detail, exc.value.request_id) == (402, "Bakiye yok", "req-402")


def test_non_json_success_is_502(fake):
    server = fake(lambda m, p, h, raw: (200, "tamam", "req-bad"))
    with pytest.raises(BBError) as exc:
        BBAnonym(server.url, "dm_test_abc").anonymize("Ali")
    assert exc.value.status_code == 502 and exc.value.request_id == "req-bad"


def test_timeout_is_504_and_closed_port_is_503(fake):
    hanging = fake(lambda m, p, h, raw: None)
    with pytest.raises(BBError) as exc:
        BBAnonym(hanging.url, "dm_test_abc", timeout=0.2).anonymize("Ali")
    assert exc.value.status_code == 504
    with pytest.raises(BBError) as exc:
        BBAnonym("http://127.0.0.1:9", "dm_test_abc", timeout=1).anonymize("Ali")
    assert exc.value.status_code == 503


def test_repr_masks_token():
    text = repr(BBAnonym("https://x", "dm_test_abcdefghijklmnop"))
    assert "dm_test_abcdefghijklmnop" not in text and "dm_test_" in text


def test_session_deletes_ids_and_hides_state(fake):
    def handler(method, path, headers, raw):
        if method == "DELETE":
            return (404, {"detail": "yok"}, "r") if path.endswith("missing") else (200, {"deleted": True}, "r")
        if path == "/v1/restore":
            return 200, {"text": "<PERSON_0>"}, "r"
        incoming = json.loads(raw)
        n = len(server.calls)
        return 200, tagged_body(
            doc_id=f"id-{n}" if incoming["persist"] else None,
            state=None if incoming["persist"] else {"PERSON": {"Ali": "<PERSON_0>"}},
            text="<PERSON_0>",
        ), "r"

    server = fake(handler)
    bb_mask.init("dm_test_abc", base_url=server.url)
    with bb_mask.session() as open_session:
        first = open_session.anonymize("Ali")
        second = open_session.anonymize("Veli")
        assert open_session.restore("cevap") == "<PERSON_0>"
    deletes = [c["path"] for c in server.calls if c["method"] == "DELETE"]
    assert f"/v1/mappings/{first.doc_id}" in deletes and f"/v1/mappings/{second.doc_id}" in deletes

    with bb_mask.session(persist=False) as local:
        tagged = local.anonymize("Ali")
        assert tagged.doc_id is None
        assert local.restore("<PERSON_0>") == "<PERSON_0>"
        before = len(server.calls)
    assert len(server.calls) == before
    with pytest.raises(BBError) as exc:
        local.restore("tekrar")
    assert exc.value.status_code == 400

    with bb_mask.session() as empty:
        with pytest.raises(BBError) as exc:
            empty.restore("yok")
        assert exc.value.status_code == 400


def test_module_calls_follow_init(fake, tmp_path):
    path = tmp_path / "ek.txt"
    path.write_text("ek")

    def handler(method, p, headers, raw):
        if method == "POST" and p == "/v1/restore":
            return 200, {"text": "ok"}, "r"
        return 200, tagged_body(), "r"

    server = fake(handler)
    with pytest.raises(BBError) as exc:
        bb_mask.init("  ")
    assert exc.value.status_code == 401
    bb_mask.init("  dm_test_abc  ", base_url=server.url)
    tagged = bb_mask.anonymize("Ali")
    assert tagged.doc_id == "doc-1"
    assert bb_mask.restore(tagged.doc_id, "cevap") == "ok"
    assert bb_mask.get_map(tagged.doc_id) == {}
    assert bb_mask.delete(tagged.doc_id) is False
    bb_mask.anonymize_file(path)
    bb_mask.anonymize_bytes("ek.txt", b"ek")
    bb_mask.anonymize_documents([path])
    assert [c["path"] for c in server.calls] == [
        "/v1/anonymize",
        "/v1/restore",
        "/v1/mappings/doc-1?once=false",
        "/v1/mappings/doc-1",
        "/v1/anonymize/file",
        "/v1/anonymize/file",
        "/v1/anonymize/documents",
    ]


def test_async_wrappers(fake):
    import asyncio

    server = fake(lambda m, p, h, raw: (200, tagged_body() if p != "/v1/restore" else {"text": "ok"}, "r"))
    bb_mask.init("dm_test_abc", base_url=server.url)

    async def run():
        tagged = await bb_mask.aanonymize("Ali")
        return tagged.doc_id, await bb_mask.arestore(tagged.doc_id, "cevap")

    assert asyncio.run(run()) == ("doc-1", "ok")


def test_uninitialised_module_is_401(monkeypatch):
    monkeypatch.setattr(bb_mask.client, "_current", None)
    with pytest.raises(BBError) as exc:
        bb_mask.anonymize("Ali")
    assert exc.value.status_code == 401
    with pytest.raises(BBError):
        with bb_mask.session():
            pass
