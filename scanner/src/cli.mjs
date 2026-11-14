#!/usr/bin/env node
import fs from "node:fs";
import { scan } from "./scan.mjs";

const args = process.argv.slice(2);
const get = (f) => {
  const i = args.indexOf(f);
  return i >= 0 ? args[i + 1] : undefined;
};
let input;
if (get("--input")) input = JSON.parse(fs.readFileSync(get("--input"), "utf8").replace(/^\uFEFF/, ""));
else if (get("--tsconfig") && get("--endpoint") && get("--path"))
  input = { tsconfig: get("--tsconfig"), drifts: [{ endpoint: get("--endpoint"), path: get("--path") }] };
else {
  console.error('usage: cli.mjs --tsconfig <file> --endpoint "GET /api/x/{id}" --path "$.field" [--json]\n   or: cli.mjs --input drifts.json');
  process.exit(2);
}
try {
  const results = scan(input);
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