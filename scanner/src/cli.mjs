#!/usr/bin/env node
import fs from "node:fs";
import { scan } from "./scan.mjs";
import { runFix } from "./fix.mjs";

const args = process.argv.slice(2);
const get = (f) => {
  const i = args.indexOf(f);
  return i >= 0 ? args[i + 1] : undefined;
};
let input;
if (get("--input")) input = JSON.parse(fs.readFileSync(get("--input"), "utf8").replace(/^\uFEFF/, ""));
else if (get("--tsconfig") && get("--endpoint") && get("--path"))
  input = { tsconfig: get("--tsconfig"), drifts: [{ endpoint: get("--endpoint"), path: get("--path"), to: get("--to") }] };
else {
  console.error(
    'usage: cli.mjs --tsconfig <file> --endpoint "GET /api/x/{id}" --path "$.field" [--json]\n   or: cli.mjs --input drifts.json\n   fix: add --fix with a "to" name per drift (or --to NAME) [--apply] [--include-name-only] [--no-verify]'
  );
  process.exit(2);
}
try {
  const results = scan(input);
  if (args.includes("--fix")) {
    if (!input.drifts.every((d) => d.to)) {
      console.error('scanner error: --fix needs a new name for every drift ("to" in the input file, or --to NAME)');
      process.exit(2);
    }
    const res = runFix(results, {
      apply: args.includes("--apply"),
      verify: !args.includes("--no-verify"),
      includeNameOnly: args.includes("--include-name-only"),
      tsconfig: input.tsconfig,
    });
    if (args.includes("--json")) console.log(JSON.stringify(res, null, 2));
    else {
      console.log(`${res.applied ? "applied" : res.reverted ? "REVERTED (TypeScript errors increased)" : "preview"}: ${res.edits} edits in ${res.files} files`);
      for (const c of res.changes) console.log(`  ${c.file}:${c.line}  (${c.confidence})\n    - ${c.before}\n    + ${c.after}`);
      if (res.diagnostics) console.log(`TypeScript errors: ${res.diagnostics.before} before, ${res.diagnostics.after} after`);
    }
    process.exit(res.reverted ? 3 : 0);
  }
  if (args.includes("--json")) console.log(JSON.stringify({ results }, null, 2));
  else
    for (const r of results) {
      console.log(`${r.path}  (${r.endpoint})  affects ${new Set(r.hits.map((h) => h.file)).size} files, endpoint calls found: ${r.endpointCalls}`);
      for (const h of r.hits) console.log(`  ${h.file}:${h.line}  ${h.snippet}  (${h.confidence}, ${h.kind})`);
    }
} catch (e) {
  console.error("scanner error:", e.message);
  process.exit(2);
}