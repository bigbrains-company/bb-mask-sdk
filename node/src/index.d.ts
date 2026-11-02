/** bb-mask: Data Mask client. This package never calls a model. */

export const DEFAULT_BASE_URL: string;
export const DEFAULT_TIMEOUT: number;
export const VERSION: string;

export type PlaceholderStyle = "angle" | "bracket_tr";

export interface AnonymizeOptions {
  /** true → mapping on the service, docId set. false → state set. Default true. */
  persist?: boolean;
  /** "angle" → <PERSON_0>. "bracket_tr" → [KİŞİ_1]. */
  placeholderStyle?: PlaceholderStyle;
  placeholder_style?: PlaceholderStyle;
  /** Company type suffix stays outside the tag: [ŞİRKET_1] A.Ş. */
  keepLegalForm?: boolean;
  keep_legal_form?: boolean;
  /** Health data layer. */
  healthData?: boolean;
  health_data?: boolean;
  /** Second LLM pass. Still 1 credit. */
  llmReview?: boolean;
  llm_review?: boolean;
}

export type State = Record<string, unknown>;

export interface LlmReview {
  status?: string;
  model?: string;
  chunks?: number;
  chunks_failed?: number;
  elapsed_ms?: number;
  added_spans?: number;
  error?: string | null;
  [key: string]: unknown;
}

export class BBError extends Error {
  readonly name: "BBError";
  readonly statusCode: number;
  readonly detail: string;
  readonly requestId: string | null;
  constructor(statusCode: number, detail: string, requestId?: string | null);
}

export class Tagged {
  text: string;
  docId: string | null;
  state: State | null;
  entitiesFound: Record<string, number>;
  balanceCredits: string | null;
  requestId: string | null;
  placeholderStyle: string;
  keepLegalForm: boolean;
  healthData: boolean;
  llmReview: LlmReview | null;
}

export type Content = Buffer | Uint8Array | string;

/** Client for one key. No init needed. */
export class BBAnonym {
  readonly baseUrl: string;
  /** Non-enumerable; console.log and JSON.stringify do not show it. */
  readonly token: string;
  timeout: number;
  constructor(baseUrl: string, token: string, timeout?: number);
  anonymize(text: string, options?: AnonymizeOptions): Promise<Tagged>;
  /** Reads a path. */
  anonymizeFile(path: string, options?: AnonymizeOptions): Promise<Tagged>;
  /** When you already hold the bytes. */
  anonymizeBytes(filename: string, content: Content, options?: AnonymizeOptions): Promise<Tagged>;
  /** 1–3 files, one docId. */
  anonymizeDocuments(paths: string[], options?: AnonymizeOptions): Promise<Tagged>;
  restore(docIdOrState: string | State, modelText: string): Promise<string>;
  getMap(docId: string, once?: boolean): Promise<State>;
  deleteMapping(docId: string): Promise<boolean>;
}

export class MaskSession {
  readonly persist: boolean;
  docId: string | null;
  state: State | null;
  constructor(client: BBAnonym, options?: AnonymizeOptions);
  anonymize(text: string): Promise<Tagged>;
  anonymizeFile(path: string): Promise<Tagged>;
  anonymizeDocuments(paths: string[]): Promise<Tagged>;
  restore(modelText: string): Promise<string>;
  close(): Promise<void>;
  [Symbol.asyncDispose](): Promise<void>;
}

export interface InitOptions {
  baseUrl?: string;
  timeout?: number;
}

export function init(apiKey: string, options?: InitOptions): BBAnonym;
export function anonymize(text: string, options?: AnonymizeOptions): Promise<Tagged>;
export function anonymizeFile(path: string, options?: AnonymizeOptions): Promise<Tagged>;
export function anonymizeBytes(filename: string, content: Content, options?: AnonymizeOptions): Promise<Tagged>;
export function anonymizeDocuments(paths: string[], options?: AnonymizeOptions): Promise<Tagged>;
export function restore(docIdOrState: string | State, modelText: string): Promise<string>;
export function getMap(docId: string, once?: boolean): Promise<State>;
export function deleteMapping(docId: string): Promise<boolean>;
export function session(options?: AnonymizeOptions): MaskSession;
