import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { scan } from "../src/scan.mjs";

const dir = path.dirname(fileURLToPath(import.meta.url));
const fixture = path.join(dir, "..", "fixtures", "basic");
const [r] = scan({
  tsconfig: path.join(fixture, "tsconfig.json"),
  cwd: fixture,
  drifts: [{ endpoint: "GET /api/users/{id} 200", path: "$.user_name" }],
});
const by = (f) => r.hits.filter((h) => h.file === f).map((h) => h.confidence);

assert.ok(r.endpointCalls >= 2, "endpoint calls found");
assert.deepEqual(by("src/types.ts"), ["type-linked"], "declaration is type-linked");
assert.deepEqual(by("src/header.ts"), ["type-linked"], "typed param read");
assert.deepEqual(by("src/profile.ts"), ["flow-linked"], "destructure from untyped fetch");
assert.deepEqual(by("src/decoy.ts"), ["name-only", "name-only"], "unrelated Product is name-only");
console.log("scanner fixture test: PASS");