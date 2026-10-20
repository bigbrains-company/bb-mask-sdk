import assert from "node:assert/strict";
import test from "node:test";

import { BBAnonym, BBError, anonymize, complete, session } from "../src/index.js";

test("module calls before init are 401 and the class complete is 501", () => {
  assert.throws(() => anonymize("Ali"), (error) => error instanceof BBError && error.statusCode === 401);
  assert.throws(() => complete(), (error) => error instanceof BBError && error.statusCode === 401);
  assert.throws(() => session(), (error) => error instanceof BBError && error.statusCode === 401);
  assert.throws(() => new BBAnonym("http://127.0.0.1:9", "dm_test_abc").complete(), (error) => error instanceof BBError && error.statusCode === 501);
});
