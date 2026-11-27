import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
import { createRequire } from "node:module";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const { chromium } = createRequire(import.meta.url)("@playwright/test");
const run = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const exe = process.platform === "win32" ? ".exe" : "";
const PATHS = ["/api/v1/users/7", "/api/v1/orders", "/api/v1/stats/daily"];
const P = { clean: 18000, mut: 18001, rec: 19000, ui: 19001, dash: 17070 };
const procs = [];
let traffic = null;
const t0 = Date.now();
const log = (m) => console.log(`[e2e +${Math.round((Date.now() - t0) / 1000)}s] ${m}`);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function start(cmd, args, opts = {}) {
  const p = spawn(cmd, args, { stdio: ["ignore", "inherit", "inherit"], ...opts });
  procs.push(p);
  return p;
}

async function get(url) {
  const r = await fetch(url, { signal: AbortSignal.timeout(2000) });
  await r.arrayBuffer();
  return r.status;
}

async function waitUp(url, ms = 20000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    try {
      await get(url);
      return;
    } catch {
      await sleep(150);
    }
  }
  throw new Error("did not come up: " + url);
}

async function hit(base, rounds = 1) {
  for (let i = 0; i < rounds; i++) {
    for (const p of PATHS) {
      try {
        await get(base + p);
      } catch {
        /* proxy may be busy */
      }
    }
  }
}

async function setup() {
  const scanner = path.join(root, "scanner", "src", "cli.mjs");
  if (!fs.existsSync(path.join(root, "scanner", "node_modules"))) {
    throw new Error("run `npm ci` in scanner/ first (the impact view needs it)");
  }
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "devlatch-e2e-"));
  const bin = path.join(work, "devlatch" + exe);
  const demo = path.join(work, "demo-backend" + exe);

  log("building devlatch and the demo backend");
  await run("go", ["build", "-o", bin, "./cmd/devlatch"], { cwd: root, timeout: 120000 });
  await run("go", ["build", "-o", demo, "."], { cwd: path.join(root, "examples", "demo-backend"), timeout: 120000 });

  log("clean backend + record");
  const cleanURL = `http://127.0.0.1:${P.clean}`;
  const clean = start(demo, ["-addr", `127.0.0.1:${P.clean}`]);
  await waitUp(cleanURL + "/api/v1/orders");
  const rec = start(
    bin,
    ["record", "--target", cleanURL, "--port", String(P.rec), "--min-samples", "3", "--duration", "6s"],
    { cwd: work },
  );
  const recDone = new Promise((r) => rec.on("exit", r));
  await waitUp(`http://127.0.0.1:${P.rec}/api/v1/orders`);
  await hit(`http://127.0.0.1:${P.rec}`, 5);
  await Promise.race([recDone, sleep(20000)]);
  log("approve");
  await run(bin, ["approve"], { cwd: work, timeout: 30000 });
  clean.kill();

  log("mutated backend + devlatch ui");
  const mutURL = `http://127.0.0.1:${P.mut}`;
  start(demo, ["-addr", `127.0.0.1:${P.mut}`, "-mutate", "rename_user_name"]);
  await waitUp(mutURL + "/api/v1/orders");
  start(
    bin,
    [
      "ui", "--target", mutURL, "--port", String(P.ui), "--ui-port", String(P.dash),
      "--min-samples", "3", "--no-notify",
      "--frontend", path.join(root, "examples", "demo-frontend"),
      "--scanner", scanner,
    ],
    { cwd: work },
  );
  await waitUp(`http://127.0.0.1:${P.dash}/api/health`);
  traffic = setInterval(() => {
    hit(`http://127.0.0.1:${P.ui}`);
  }, 300);
  log("setup done");
}

function cleanup() {
  if (traffic) clearInterval(traffic);
  for (const p of procs) {
    try {
      p.kill();
    } catch {
      /* already gone */
    }
  }
}

async function main() {
  await setup();
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    log("opening the dashboard");
    await page.goto(`http://127.0.0.1:${P.dash}/`, { timeout: 15000 });

    await page.getByText(/users\/\{id\}/).first().waitFor({ timeout: 30000 });
    log("endpoints visible");

    const row = page.getByText(/user_name/).first();
    await row.waitFor({ timeout: 30000 });
    log("drift row visible");
    await row.click();

    await page.getByText(/Header\.tsx/).first().waitFor({ timeout: 60000 });
    log("affected files visible");

    fs.mkdirSync(path.join(here, "out"), { recursive: true });
    await page.screenshot({ path: path.join(here, "out", "dashboard.png"), fullPage: true });
    log("PASS - screenshot saved to web/e2e/out/dashboard.png");
  } finally {
    await browser.close();
  }
}

const killer = setTimeout(() => {
  console.error("[e2e] FAIL: overall 4 minute limit reached");
  cleanup();
  process.exit(1);
}, 240000);

main()
  .then(() => {
    clearTimeout(killer);
    cleanup();
    process.exit(0);
  })
  .catch((e) => {
    console.error("[e2e] FAIL:", e.message);
    clearTimeout(killer);
    cleanup();
    process.exit(1);
  });