// Precision / recall benchmark for the impact scanner.
// Ground truth: lines in fixtures/bench/src/*.ts ending with "// @hit <caseId>".
// usage: node test/bench.mjs [--fixture DIR] [--json] [--verbose] [--min-recall 0.9] [--min-precision 0.8]
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { scan } from "../src/scan.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const opt = (f, d) => {
  const i = args.indexOf(f);
  return i >= 0 ? args[i + 1] : d;
};
const fixture = path.resolve(opt("--fixture", path.join(here, "..", "fixtures", "bench")));
const asJson = args.includes("--json");
const verbose = args.includes("--verbose");
const minRecall = opt("--min-recall") === undefined ? null : Number(opt("--min-recall"));
const minPrecision = opt("--min-precision") === undefined ? null : Number(opt("--min-precision"));

const cases = JSON.parse(fs.readFileSync(path.join(fixture, "cases.json"), "utf8"));
const srcDir = path.join(fixture, "src");

// ground truth
const expected = new Map(cases.map((c) => [c.id, new Map()]));
const lineText = new Map();
for (const f of fs.readdirSync(srcDir).filter((n) => /\.tsx?$/.test(n))) {
  fs.readFileSync(path.join(srcDir, f), "utf8")
    .split(/\r?\n/)
    .forEach((line, i) => {
      lineText.set(`src/${f}:${i + 1}`, line.trim());
      const m = line.match(/\/\/\s*@hit\s+(\w+)/);
      if (m && expected.has(m[1])) expected.get(m[1]).set(`src/${f}:${i + 1}`, line.trim());
    });
}

const results = scan({
  tsconfig: path.join(fixture, "tsconfig.json"),
  cwd: fixture,
  drifts: cases.map((c) => ({ endpoint: c.endpoint, path: c.path })),
});

const pct = (x) => (x * 100).toFixed(1) + "%";
const modes = [
  { name: "all hits", keep: () => true },
  { name: "no name-only", keep: (h) => h.confidence !== "name-only" },
];
const report = { cases: [], overall: {} };
const totals = Object.fromEntries(modes.map((m) => [m.name, { tp: 0, fp: 0, fn: 0 }]));

cases.forEach((c, i) => {
  const exp = expected.get(c.id);
  const row = { id: c.id, path: c.path, expected: exp.size, modes: {} };
  for (const m of modes) {
    const found = new Map();
    for (const h of results[i].hits.filter(m.keep)) found.set(`${h.file}:${h.line}`, h);
    const tp = [...found.keys()].filter((k) => exp.has(k));
    const fp = [...found.keys()].filter((k) => !exp.has(k));
    const fn = [...exp.keys()].filter((k) => !found.has(k));
    const t = totals[m.name];
    t.tp += tp.length;
    t.fp += fp.length;
    t.fn += fn.length;
    row.modes[m.name] = {
      tp: tp.length,
      fp: fp.length,
      fn: fn.length,
      precision: tp.length + fp.length ? tp.length / (tp.length + fp.length) : 1,
      recall: exp.size ? tp.length / exp.size : 1,
      missed: fn.map((k) => ({ at: k, code: exp.get(k) })),
      extra: fp.map((k) => ({ at: k, code: lineText.get(k) ?? "" })),
    };
  }
  report.cases.push(row);
});

for (const m of modes) {
  const t = totals[m.name];
  report.overall[m.name] = {
    ...t,
    precision: t.tp + t.fp ? t.tp / (t.tp + t.fp) : 1,
    recall: t.tp + t.fn ? t.tp / (t.tp + t.fn) : 1,
  };
}

if (asJson) console.log(JSON.stringify(report, null, 2));
else {
  console.log("Impact scanner benchmark (line-level, ground truth = // @hit markers)\n");
  for (const m of modes) {
    console.log(`Mode: ${m.name}`);
    console.log("  case  expected  TP  FP  FN  precision  recall");
    for (const r of report.cases) {
      const x = r.modes[m.name];
      console.log(`  ${r.id.padEnd(5)} ${String(r.expected).padStart(8)} ${String(x.tp).padStart(3)} ${String(x.fp).padStart(3)} ${String(x.fn).padStart(3)}  ${pct(x.precision).padStart(9)}  ${pct(x.recall).padStart(6)}`);
    }
    const o = report.overall[m.name];
    console.log(`  ALL   ${String(o.tp + o.fn).padStart(8)} ${String(o.tp).padStart(3)} ${String(o.fp).padStart(3)} ${String(o.fn).padStart(3)}  ${pct(o.precision).padStart(9)}  ${pct(o.recall).padStart(6)}\n`);
  }
  if (verbose) {
    for (const m of modes) {
      console.log(`Missed (${m.name}):`);
      for (const r of report.cases) for (const x of r.modes[m.name].missed) console.log(`  [${r.id}] ${x.at}  ${x.code}`);
      console.log(`Extra (${m.name}):`);
      for (const r of report.cases) for (const x of r.modes[m.name].extra) console.log(`  [${r.id}] ${x.at}  ${x.code}`);
      console.log("");
    }
  }
}

const conf = report.overall["no name-only"];
let fail = false;
if (minRecall !== null && conf.recall < minRecall) { console.error(`FAIL: recall ${pct(conf.recall)} < ${pct(minRecall)}`); fail = true; }
if (minPrecision !== null && conf.precision < minPrecision) { console.error(`FAIL: precision ${pct(conf.precision)} < ${pct(minPrecision)}`); fail = true; }
if (fail) process.exit(1);