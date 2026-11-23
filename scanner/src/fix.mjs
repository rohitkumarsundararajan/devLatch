import fs from "node:fs";
import path from "node:path";
import { Project } from "ts-morph";

const readText = (f) => {
  const raw = fs.readFileSync(f, "utf8");
  return raw.charCodeAt(0) === 0xfeff ? { bom: true, text: raw.slice(1) } : { bom: false, text: raw };
};

// Turn scanner results into per-file edits, skipping low-confidence ones unless asked.
export function planFix(results, { includeNameOnly = false } = {}) {
  const byFile = new Map();
  const seen = new Map();
  for (const r of results) {
    for (const e of r.edits ?? []) {
      if (e.confidence === "name-only" && !includeNameOnly) continue;
      const k = `${e.abs}:${e.start}`;
      if (seen.has(k)) continue; // same spot from two drifts: first one wins
      seen.set(k, e);
      if (!byFile.has(e.abs)) byFile.set(e.abs, []);
      byFile.get(e.abs).push(e);
    }
  }
  return byFile;
}

// Apply edits to text (back to front so offsets stay valid). Throws if the file changed since the scan.
export function applyToText(text, edits) {
  let out = text;
  for (const e of [...edits].sort((a, b) => b.start - a.start)) {
    if (out.slice(e.start, e.end) !== e.old) throw new Error(`file changed since scan near offset ${e.start}`);
    out = out.slice(0, e.start) + e.text + out.slice(e.end);
  }
  return out;
}

function diagCount(tsconfig) {
  return new Project({ tsConfigFilePath: path.resolve(tsconfig) }).getPreEmitDiagnostics().length;
}

// Plan (and optionally apply) a rename. With apply+verify, reverts everything if TypeScript errors increase.
export function runFix(results, { apply = false, verify = true, includeNameOnly = false, tsconfig } = {}) {
  const plan = planFix(results, { includeNameOnly });
  const changes = [];
  const originals = new Map();
  const updated = new Map();
  for (const [abs, edits] of plan) {
    const { bom, text } = readText(abs);
    originals.set(abs, { bom, text });
    const next = applyToText(text, edits);
    updated.set(abs, next);
    const a = text.split("\n");
    const b = next.split("\n");
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) {
        const e = edits.find((x) => x.line === i + 1) ?? edits[0];
        changes.push({ file: e.file, line: i + 1, confidence: e.confidence, before: a[i].trim(), after: b[i].trim() });
      }
    }
  }
  changes.sort((x, y) => x.file.localeCompare(y.file) || x.line - y.line);
  const out = { edits: [...plan.values()].reduce((n, l) => n + l.length, 0), files: plan.size, changes, applied: false, reverted: false };
  if (!apply || plan.size === 0) return out;

  const before = verify && tsconfig ? diagCount(tsconfig) : null;
  for (const [abs, next] of updated) fs.writeFileSync(abs, (originals.get(abs).bom ? "\uFEFF" : "") + next);
  out.applied = true;
  if (verify && tsconfig) {
    const after = diagCount(tsconfig);
    out.diagnostics = { before, after };
    if (after > before) {
      for (const [abs, o] of originals) fs.writeFileSync(abs, (o.bom ? "\uFEFF" : "") + o.text);
      out.applied = false;
      out.reverted = true;
    }
  }
  return out;
}