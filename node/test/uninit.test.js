import assert from "node:assert/strict";
import test from "node:test";

import { BBError, anonymize, anonymizeBytes, session } from "../src/index.js";

test("module calls before init are 401", () => {
  assert.throws(() => anonymize("Ali"), (error) => error instanceof BBError && error.statusCode === 401);
  assert.throws(() => anonymizeBytes("a.txt", Buffer.from("a")), (error) => error instanceof BBError && error.statusCode === 401);
  assert.throws(() => session(), (error) => error instanceof BBError && error.statusCode === 401);
});
