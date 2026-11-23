import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { scan } from "../src/scan.mjs";
import { runFix } from "../src/fix.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "devlatch-fix-"));
fs.cpSync(path.join(here, "..", "fixtures", "bench"), tmp, { recursive: true });
const tsconfig = path.join(tmp, "tsconfig.json");
const read = (f) => fs.readFileSync(path.join(tmp, "src", f), "utf8");
const endpoint = "GET /api/users/{id} 200";
const run = (drifts, opts = {}) => runFix(scan({ tsconfig, cwd: tmp, drifts }), { tsconfig, ...opts });

// 1. preview changes nothing
const good = [{ endpoint, path: "$.user_name", to: "username" }];
const before = read("users.ts");
const preview = run(good);
assert.equal(preview.applied, false);
assert.ok(preview.edits >= 15, "found edits");
assert.equal(read("users.ts"), before, "preview must not write");

// 2. apply renames every linked usage and TypeScript stays clean
const res = run(good, { apply: true });
assert.equal(res.applied, true);
assert.equal(res.reverted, false);
assert.deepEqual(res.diagnostics, { before: 0, after: 0 });
assert.ok(read("users.ts").includes("{ username: user_name }: User"), "shorthand destructure keeps the local name");
assert.ok(read("users.ts").includes("u.username"));
assert.ok(read("types.ts").includes('Pick<User, "username" | "id">'), "Pick key renamed");
assert.ok(read("decoy.ts").includes("x.user_name"), "unrelated Product field untouched");

// 3. running again finds nothing to do
assert.equal(run(good).edits, 0);

// 4. a rename that breaks compilation is reverted
const snapshot = read("users.ts");
const bad = run([{ endpoint, path: "$.username", to: "bad name" }], { apply: true });
assert.equal(bad.reverted, true);
assert.equal(bad.applied, false);
assert.equal(read("users.ts"), snapshot, "files restored");

fs.rmSync(tmp, { recursive: true, force: true });
console.log("scanner fix test: PASS");