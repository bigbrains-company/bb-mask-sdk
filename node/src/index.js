/**
 * bb-mask HTTP client. This package never calls a model.
 *
 * docId stays on the service. getMap returns the dictionary. persist false returns state.
 * session() hides the docId or state.
 *
 * anonymizeFile(path) reads a path; anonymizeBytes(filename, content) takes bytes.
 * Both names mean the same thing on the module and on BBAnonym.
 */

import { readFile } from "node:fs/promises";
import { basename } from "node:path";

export const DEFAULT_BASE_URL = "https://app.bbmask.co";
// The engine waits 120 s; the client gives up after it so the refund/502 decision comes from the server.
export const DEFAULT_TIMEOUT = 150;
export const VERSION = "0.3.1";
const STYLES = new Set(["angle", "bracket_tr"]);

export class BBError extends Error {
  constructor(statusCode, detail, requestId = null) {
    super(detail);
    this.name = "BBError";
    this.statusCode = statusCode;
    this.detail = detail;
    this.requestId = requestId;
  }
}

export class Tagged {
  constructor({
    text,
    docId = null,
    state = null,
    entitiesFound = {},
    balanceCredits = null,
    requestId = null,
    placeholderStyle = "angle",
    keepLegalForm = false,
    healthData = false,
    llmReview = null,
  }) {
    this.text = text;
    this.docId = docId;
    this.state = state;
    this.entitiesFound = entitiesFound;
    this.balanceCredits = balanceCredits;
    this.requestId = requestId;
    this.placeholderStyle = placeholderStyle;
    this.keepLegalForm = keepLegalForm;
    this.healthData = healthData;
    this.llmReview = llmReview;
  }
}

function quote(value) {
  return encodeURIComponent(value).replace(/[!'()*]/g, (char) =>
    `%${char.charCodeAt(0).toString(16).toUpperCase()}`,
  );
}

function pick(options, camel, snake) {
  if (options[camel] !== undefined) return options[camel];
  if (options[snake] !== undefined) return options[snake];
  return undefined;
}

function wireOptions(options = {}) {
  const style = pick(options, "placeholderStyle", "placeholder_style") ?? "angle";
  if (!STYLES.has(style)) {
    throw new BBError(400, "placeholder_style must be angle or bracket_tr");
  }
  return {
    placeholder_style: style,
    keep_legal_form: Boolean(pick(options, "keepLegalForm", "keep_legal_form")),
    health_data: Boolean(pick(options, "healthData", "health_data")),
    llm_review: Boolean(pick(options, "llmReview", "llm_review")),
  };
}

function formFields(persist, options) {
  const data = { persist: String(persist) };
  for (const [key, value] of Object.entries(options)) {
    data[key] = typeof value === "boolean" ? String(value) : value;
  }
  return data;
}

function blob(content) {
  const bytes = Buffer.isBuffer(content) ? content : Buffer.from(content);
  return new Blob([bytes], { type: "application/octet-stream" });
}

async function readBody(response) {
  const requestId = response.headers.get("x-request-id");
  const raw = await response.text();
  let body = null;
  if (raw) {
    try {
      body = JSON.parse(raw);
    } catch {
      body = null;
    }
  }
  if (response.status >= 400) {
    let detail = raw;
    if (body && typeof body === "object" && body.detail != null) {
      detail = typeof body.detail === "string" ? body.detail : JSON.stringify(body.detail);
    }
    throw new BBError(response.status, detail, requestId);
  }
  if (!body || typeof body !== "object" || Array.isArray(body)) {
    throw new BBError(502, "The service did not return JSON", requestId);
  }
  body._request_id = requestId;
  return body;
}

function toTagged(body, persist) {
  let docId = body.doc_id || null;
  if (!persist) docId = null;
  return new Tagged({
    text: body.text || "",
    docId,
    state: body.state ?? null,
    entitiesFound: body.entities_found || {},
    balanceCredits: body.balance_credits ?? null,
    requestId: body._request_id ?? null,
    placeholderStyle: body.placeholder_style || "angle",
    keepLegalForm: Boolean(body.keep_legal_form),
    healthData: Boolean(body.health_data),
    llmReview: body.llm_review ?? null,
  });
}

export class BBAnonym {
  constructor(baseUrl, token, timeout = DEFAULT_TIMEOUT) {
    this.baseUrl = String(baseUrl || DEFAULT_BASE_URL).replace(/\/+$/, "");
    // Keep the key out of console.log / JSON.stringify output.
    Object.defineProperty(this, "token", { value: token, enumerable: false, writable: false });
    this.timeout = timeout;
  }

  headers() {
    return { Authorization: `Bearer ${this.token}`, "User-Agent": `bb-mask-node/${VERSION}` };
  }

  async send(method, path, { json, form, query } = {}) {
    const url = new URL(this.baseUrl + path);
    if (query) {
      for (const [key, value] of Object.entries(query)) url.searchParams.set(key, value);
    }
    const headers = this.headers();
    let body;
    if (json !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(json);
    } else if (form) {
      body = form;
    }
    try {
      return await fetch(url, {
        method,
        headers,
        body,
        signal: AbortSignal.timeout(this.timeout * 1000),
      });
    } catch (error) {
      if (error?.name === "TimeoutError" || error?.name === "AbortError") {
        throw new BBError(504, `Timed out (${this.timeout} s). The fee may have been charged.`);
      }
      throw new BBError(503, `Could not reach the service (${this.baseUrl})`);
    }
  }

  async anonymize(text, options = {}) {
    const persist = options.persist !== false;
    const wire = wireOptions(options);
    const response = await this.send("POST", "/v1/anonymize", {
      json: { text, persist, ...wire },
    });
    return toTagged(await readBody(response), persist);
  }

  async anonymizeBytes(filename, content, options = {}) {
    const persist = options.persist !== false;
    const wire = wireOptions(options);
    const form = new FormData();
    for (const [key, value] of Object.entries(formFields(persist, wire))) form.append(key, value);
    form.append("file", blob(content), filename);
    const response = await this.send("POST", "/v1/anonymize/file", { form });
    return toTagged(await readBody(response), persist);
  }

  async anonymizeFile(path, options = {}) {
    const content = await readFile(path);
    return this.anonymizeBytes(basename(path), content, options);
  }

  async anonymizeDocuments(paths, options = {}) {
    const persist = options.persist !== false;
    const wire = wireOptions(options);
    if (paths.length < 1 || paths.length > 3) throw new BBError(400, "1–3 files per request");
    const form = new FormData();
    for (const [key, value] of Object.entries(formFields(persist, wire))) form.append(key, value);
    for (const path of paths) {
      form.append("files", blob(await readFile(path)), basename(path));
    }
    const response = await this.send("POST", "/v1/anonymize/documents", { form });
    return toTagged(await readBody(response), persist);
  }

  async restore(docIdOrState, modelText) {
    const payload = { text: modelText };
    if (docIdOrState && typeof docIdOrState === "object") payload.state = docIdOrState;
    else payload.doc_id = docIdOrState;
    const response = await this.send("POST", "/v1/restore", { json: payload });
    return (await readBody(response)).text || "";
  }

  async getMap(docId, once = false) {
    const response = await this.send("GET", `/v1/mappings/${quote(docId)}`, {
      query: { once: String(once) },
    });
    return (await readBody(response)).state || {};
  }

  async deleteMapping(docId) {
    const response = await this.send("DELETE", `/v1/mappings/${quote(docId)}`);
    return Boolean((await readBody(response)).deleted);
  }
}

export class MaskSession {
  constructor(client, options = {}) {
    this.client = client;
    this.persist = options.persist !== false;
    this.options = { ...options, persist: this.persist };
    this.ids = [];
    this.docId = null;
    this.state = null;
  }

  remember(tagged) {
    this.docId = tagged.docId;
    this.state = tagged.state;
    if (tagged.docId) this.ids.push(tagged.docId);
    return tagged;
  }

  anonymize(text) {
    return this.client.anonymize(text, this.options).then((tagged) => this.remember(tagged));
  }

  anonymizeFile(path) {
    return this.client.anonymizeFile(path, this.options).then((tagged) => this.remember(tagged));
  }

  anonymizeDocuments(paths) {
    return this.client.anonymizeDocuments(paths, this.options).then((tagged) => this.remember(tagged));
  }

  restore(modelText) {
    if (this.persist) {
      if (!this.docId) return Promise.reject(new BBError(400, "Nothing anonymized in this session"));
      return this.client.restore(this.docId, modelText);
    }
    if (!this.state) return Promise.reject(new BBError(400, "No state in this session"));
    return this.client.restore(this.state, modelText);
  }

  async close() {
    if (!this.persist) {
      this.state = null;
      return;
    }
    for (const docId of this.ids) {
      try {
        await this.client.deleteMapping(docId);
      } catch (error) {
        if (!(error instanceof BBError)) throw error;
      }
    }
    this.ids = [];
  }
}

if (typeof Symbol.asyncDispose === "symbol") {
  MaskSession.prototype[Symbol.asyncDispose] = function dispose() {
    return this.close();
  };
}

let current = null;

export function init(apiKey, { baseUrl = DEFAULT_BASE_URL, timeout = DEFAULT_TIMEOUT } = {}) {
  if (!String(apiKey ?? "").trim()) throw new BBError(401, "api_key is empty");
  current = new BBAnonym(baseUrl, String(apiKey).trim(), timeout);
  return current;
}

function requireClient() {
  if (!current) throw new BBError(401, "init was not called");
  return current;
}

export function anonymize(text, options) {
  return requireClient().anonymize(text, options);
}

export function anonymizeFile(path, options) {
  return requireClient().anonymizeFile(path, options);
}

export function anonymizeBytes(filename, content, options) {
  return requireClient().anonymizeBytes(filename, content, options);
}

export function anonymizeDocuments(paths, options) {
  return requireClient().anonymizeDocuments(paths, options);
}

export function restore(docIdOrState, modelText) {
  return requireClient().restore(docIdOrState, modelText);
}

export function getMap(docId, once) {
  return requireClient().getMap(docId, once);
}

export function deleteMapping(docId) {
  return requireClient().deleteMapping(docId);
}

export function session(options) {
  return new MaskSession(requireClient(), options);
}
