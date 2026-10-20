import assert from "node:assert/strict";
import { once } from "node:events";
import http from "node:http";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  BBAnonym,
  BBError,
  anonymize,
  anonymizeDocuments,
  anonymizeFile,
  complete,
  deleteMapping,
  getMap,
  init,
  restore,
  session,
} from "../src/index.js";

function taggedBody(extra = {}) {
  return {
    doc_id: "doc-1",
    text: "Davacı <PERSON_0>",
    entities_found: { PERSON: 1 },
    balance_credits: "9.00",
    state: null,
    placeholder_style: "angle",
    keep_legal_form: false,
    health_data: false,
    llm_review: null,
    ...extra,
  };
}

async function readRaw(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return Buffer.concat(chunks);
}

async function listen(handler) {
  const server = http.createServer(async (req, res) => {
    try {
      const raw = await readRaw(req);
      await handler(req, res, raw);
    } catch (error) {
      res.statusCode = 500;
      res.end(String(error));
    }
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const { port } = server.address();
  return {
    url: `http://127.0.0.1:${port}`,
    close() {
      server.closeAllConnections?.();
      return new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
    },
  };
}

function json(res, status, body, requestId = "req-1") {
  res.statusCode = status;
  res.setHeader("content-type", "application/json");
  res.setHeader("x-request-id", requestId);
  res.end(JSON.stringify(body));
}

test("anonymize sends the wire body and maps Tagged", async () => {
  const seen = [];
  const server = await listen(async (req, res, raw) => {
    seen.push({ method: req.method, url: req.url, auth: req.headers.authorization, body: JSON.parse(raw.toString()) });
    json(res, 200, taggedBody());
  });
  try {
    const client = new BBAnonym(server.url, "dm_test_abc");
    const tagged = await client.anonymize("Davacı Hasan Demirkol", {
      placeholderStyle: "bracket_tr",
      keepLegalForm: true,
      healthData: true,
      llmReview: true,
    });
    assert.equal(seen[0].method, "POST");
    assert.equal(seen[0].url, "/v1/anonymize");
    assert.equal(seen[0].auth, "Bearer dm_test_abc");
    assert.deepEqual(seen[0].body, {
      text: "Davacı Hasan Demirkol",
      persist: true,
      placeholder_style: "bracket_tr",
      keep_legal_form: true,
      health_data: true,
      llm_review: true,
    });
    assert.equal(tagged.text, "Davacı <PERSON_0>");
    assert.equal(tagged.docId, "doc-1");
    assert.equal(tagged.state, null);
    assert.deepEqual(tagged.entitiesFound, { PERSON: 1 });
    assert.equal(tagged.balanceCredits, "9.00");
    assert.equal(tagged.requestId, "req-1");
    assert.equal(tagged.placeholderStyle, "angle");
  } finally {
    await server.close();
  }
});

test("persist false drops docId even when the service returns one", async () => {
  const server = await listen(async (req, res, raw) => {
    const body = JSON.parse(raw.toString());
    assert.equal(body.persist, false);
    json(res, 200, taggedBody({ state: { PERSON: { Ali: "<PERSON_0>" } } }));
  });
  try {
    const tagged = await new BBAnonym(server.url, "dm_test_abc").anonymize("Ali", { persist: false });
    assert.equal(tagged.docId, null);
    assert.deepEqual(tagged.state, { PERSON: { Ali: "<PERSON_0>" } });
  } finally {
    await server.close();
  }
});

test("snake_case options are accepted", async () => {
  const server = await listen(async (req, res, raw) => {
    const body = JSON.parse(raw.toString());
    assert.equal(body.placeholder_style, "bracket_tr");
    assert.equal(body.keep_legal_form, true);
    json(res, 200, taggedBody());
  });
  try {
    await new BBAnonym(server.url, "dm_test_abc").anonymize("Ali", {
      placeholder_style: "bracket_tr",
      keep_legal_form: true,
    });
  } finally {
    await server.close();
  }
});

test("invalid placeholder style fails before the request", async () => {
  await assert.rejects(
    () => new BBAnonym("http://127.0.0.1:9", "dm_test_abc").anonymize("Ali", { placeholderStyle: "stars" }),
    (error) => error instanceof BBError && error.statusCode === 400,
  );
});

test("restore sends doc_id or state", async () => {
  const bodies = [];
  const server = await listen(async (req, res, raw) => {
    bodies.push(JSON.parse(raw.toString()));
    json(res, 200, { text: "açık metin" });
  });
  try {
    const client = new BBAnonym(server.url, "dm_test_abc");
    assert.equal(await client.restore("doc-1", "<PERSON_0>"), "açık metin");
    assert.equal(await client.restore({ PERSON: {} }, "<PERSON_0>"), "açık metin");
    assert.deepEqual(bodies[0], { text: "<PERSON_0>", doc_id: "doc-1" });
    assert.deepEqual(bodies[1], { text: "<PERSON_0>", state: { PERSON: {} } });
  } finally {
    await server.close();
  }
});

test("getMap encodes the id and deleteMapping reads deleted", async () => {
  const seen = [];
  const server = await listen(async (req, res) => {
    seen.push({ method: req.method, url: req.url });
    if (req.method === "GET") json(res, 200, { state: { PERSON: { Ali: "<PERSON_0>" } } });
    else json(res, 200, { deleted: true });
  });
  try {
    const client = new BBAnonym(server.url, "dm_test_abc");
    const state = await client.getMap("a/b c", true);
    assert.deepEqual(state, { PERSON: { Ali: "<PERSON_0>" } });
    assert.equal(await client.deleteMapping("a/b c"), true);
    assert.equal(seen[0].url, "/v1/mappings/a%2Fb%20c?once=true");
    assert.equal(seen[1].method, "DELETE");
    assert.equal(seen[1].url, "/v1/mappings/a%2Fb%20c");
  } finally {
    await server.close();
  }
});

test("file upload uses multipart field names", async () => {
  const dir = await mkdtemp(join(tmpdir(), "bb-mask-"));
  const path = join(dir, "dilekce.txt");
  await writeFile(path, "Davacı Ali");
  const server = await listen(async (req, res, raw) => {
    const body = raw.toString("utf8");
    assert.equal(req.url, "/v1/anonymize/file");
    assert.match(body, /name="persist"\r\n\r\nfalse/);
    assert.match(body, /name="placeholder_style"\r\n\r\nangle/);
    assert.match(body, /name="keep_legal_form"\r\n\r\nfalse/);
    assert.match(body, /name="file"/);
    assert.match(body, /filename="dilekce.txt"/);
    assert.match(body, /Davacı Ali/);
    json(res, 200, taggedBody({ doc_id: "should-drop" }));
  });
  try {
    const tagged = await new BBAnonym(server.url, "dm_test_abc").anonymizePath(path, { persist: false });
    assert.equal(tagged.docId, null);
  } finally {
    await server.close();
    await rm(dir, { recursive: true, force: true });
  }
});

test("documents sends one to three files and rejects other counts", async () => {
  const dir = await mkdtemp(join(tmpdir(), "bb-mask-"));
  const first = join(dir, "a.txt");
  const second = join(dir, "b.txt");
  await writeFile(first, "bir");
  await writeFile(second, "iki");
  const server = await listen(async (req, res, raw) => {
    const body = raw.toString("utf8");
    assert.equal(req.url, "/v1/anonymize/documents");
    assert.equal(body.match(/name="files"/g).length, 2);
    assert.match(body, /filename="a.txt"/);
    assert.match(body, /filename="b.txt"/);
    json(res, 200, taggedBody());
  });
  try {
    const client = new BBAnonym(server.url, "dm_test_abc");
    const tagged = await client.anonymizeDocuments([first, second]);
    assert.equal(tagged.docId, "doc-1");
    await assert.rejects(
      () => client.anonymizeDocuments([]),
      (error) => error instanceof BBError && error.statusCode === 400,
    );
    await assert.rejects(
      () => client.anonymizeDocuments(["1", "2", "3", "4"], { placeholderStyle: "nope" }),
      (error) => error instanceof BBError && error.statusCode === 400 && /placeholder_style/.test(error.detail),
    );
  } finally {
    await server.close();
    await rm(dir, { recursive: true, force: true });
  }
});

test("HTTP errors keep status, detail and request id", async () => {
  const server = await listen(async (req, res) => {
    json(res, 402, { detail: "Bakiye yok" }, "req-402");
  });
  try {
    await assert.rejects(
      () => new BBAnonym(server.url, "dm_test_abc").anonymize("Ali"),
      (error) => error instanceof BBError && error.statusCode === 402 && error.detail === "Bakiye yok" && error.requestId === "req-402",
    );
  } finally {
    await server.close();
  }
});

test("a non-JSON success is 502", async () => {
  const server = await listen(async (req, res) => {
    res.statusCode = 200;
    res.setHeader("x-request-id", "req-bad");
    res.end("tamam");
  });
  try {
    await assert.rejects(
      () => new BBAnonym(server.url, "dm_test_abc").anonymize("Ali"),
      (error) => error instanceof BBError && error.statusCode === 502 && error.requestId === "req-bad",
    );
  } finally {
    await server.close();
  }
});

test("timeout is 504 and a closed port is 503", async () => {
  const hanging = await listen(() => {});
  try {
    await assert.rejects(
      () => new BBAnonym(hanging.url, "dm_test_abc", 0.2).anonymize("Ali"),
      (error) => error instanceof BBError && error.statusCode === 504,
    );
  } finally {
    await hanging.close();
  }
  const closed = await listen(() => {});
  const url = closed.url;
  await closed.close();
  await assert.rejects(
    () => new BBAnonym(url, "dm_test_abc", 1).anonymize("Ali"),
    (error) => error instanceof BBError && error.statusCode === 503,
  );
});

test("complete does not call the service", async () => {
  let calls = 0;
  const server = await listen(async (req, res) => {
    calls += 1;
    json(res, 200, taggedBody());
  });
  try {
    const client = new BBAnonym(server.url, "dm_test_abc");
    for (const call of [() => client.complete(), () => client.completeFile(), () => client.completeDocuments()]) {
      assert.throws(call, (error) => error instanceof BBError && error.statusCode === 501);
    }
    assert.equal(calls, 0);
  } finally {
    await server.close();
  }
});

test("session deletes stored ids and hides state when persist is false", async () => {
  const calls = [];
  const server = await listen(async (req, res, raw) => {
    calls.push({ method: req.method, url: req.url, body: raw.length ? raw.toString() : "" });
    if (req.method === "DELETE") {
      json(res, req.url.endsWith("missing") ? 404 : 200, req.url.endsWith("missing") ? { detail: "yok" } : { deleted: true });
      return;
    }
    const incoming = JSON.parse(raw.toString());
    json(res, 200, taggedBody({
      doc_id: incoming.persist ? `id-${calls.length}` : null,
      state: incoming.persist ? null : { PERSON: { Ali: "<PERSON_0>" } },
      text: "<PERSON_0>",
    }));
  });
  try {
    init("dm_test_abc", { baseUrl: server.url });
    const open = session();
    const first = await open.anonymize("Ali");
    const second = await open.anonymize("Veli");
    assert.equal(await open.restore("cevap"), "<PERSON_0>");
    await open.close();
    assert.ok(calls.some((call) => call.method === "DELETE" && call.url === `/v1/mappings/${first.docId}`));
    assert.ok(calls.some((call) => call.method === "DELETE" && call.url === `/v1/mappings/${second.docId}`));

    const local = session({ persist: false });
    const tagged = await local.anonymize("Ali");
    assert.equal(tagged.docId, null);
    assert.equal(await local.restore("<PERSON_0>"), "<PERSON_0>");
    const before = calls.length;
    await local.close();
    assert.equal(calls.length, before);
    await assert.rejects(() => local.restore("tekrar"), (error) => error instanceof BBError && error.statusCode === 400);

    const empty = session();
    await assert.rejects(() => empty.restore("yok"), (error) => error instanceof BBError && error.statusCode === 400);
  } finally {
    await server.close();
  }
});

test("module calls follow init, including file helpers", async () => {
  const dir = await mkdtemp(join(tmpdir(), "bb-mask-"));
  const path = join(dir, "ek.txt");
  await writeFile(path, "ek");
  const seen = [];
  const server = await listen(async (req, res) => {
    seen.push(req.url);
    json(res, 200, req.method === "POST" && req.url === "/v1/restore" ? { text: "ok" } : taggedBody());
  });
  try {
    assert.throws(() => init("  "), (error) => error instanceof BBError && error.statusCode === 401);
    init("  dm_test_abc  ", { baseUrl: server.url });
    const tagged = await anonymize("Ali");
    assert.equal(tagged.docId, "doc-1");
    assert.equal(await restore(tagged.docId, "cevap"), "ok");
    assert.deepEqual(await getMap(tagged.docId), {});
    assert.equal(await deleteMapping(tagged.docId), false);
    await anonymizeFile(path);
    await anonymizeDocuments([path]);
    assert.throws(() => complete(), (error) => error instanceof BBError && error.statusCode === 501);
    assert.deepEqual(seen, [
      "/v1/anonymize",
      "/v1/restore",
      "/v1/mappings/doc-1?once=false",
      "/v1/mappings/doc-1",
      "/v1/anonymize/file",
      "/v1/anonymize/documents",
    ]);
  } finally {
    await server.close();
    await rm(dir, { recursive: true, force: true });
  }
});
