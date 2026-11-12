// The camera: photographs every screen of the web app (/app) at a desktop and a phone width,
// against a throwaway `myself serve` filled by seed.ts, and checks each screen against the rules
// of design/README.md that a machine can check. A run is also a smoke test: it lists every page
// error, console error and failed request it saw, so it is useful even to someone who cannot look
// at the pictures.
//
//   cd design/camera && bun install            once
//   bun design/camera/screens.ts [out-dir]     every screen, into out-dir (default: a temp dir)
//   SCREENS=graph,entity bun design/camera/screens.ts         only those screens
//   WIDTHS=1440 bun design/camera/screens.ts                   only that width (1440 or 390)
//   BIG=1 bun design/camera/screens.ts                         also the big-brain screens
//   SEED_NOW=1794000000000 bun design/camera/screens.ts        the same brain on every run
//
// Needs Go on PATH (it builds the binary from this checkout; or set MYSELF_BIN to a built one)
// and Chromium or Chrome on PATH (or MYSELF_BROWSER). Exits 1 when it found any problem.
//
// Screens run with prefers-reduced-motion, so the graph is laid out before it is drawn and the
// e-ink flash is off: the same brain gives the same picture.

import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { launch, type Browser, type Page } from "puppeteer-core";
import { bigRecords, seedRecords, toJSONL } from "./seed.ts";

const root = join(import.meta.dir, "../..");
const stamp = new Date().toISOString().replace(/[:.]/g, "-");
const out = process.argv[2] ?? join(tmpdir(), `myself-screens-${stamp}`);
const only = (process.env.SCREENS ?? "").split(",").filter((s) => s !== "");
const widths = (process.env.WIDTHS ?? "1440,390").split(",").map(Number);
const big = process.env.BIG === "1";
const ADMIN = "myself-screens-admin-token";
const DAY = 24 * 3600 * 1000;
// SEED_NOW=<unix ms> pins the seed's clock, so two runs write the same brain and pictures that show
// absolute times can be compared byte for byte (relative times still follow the wall clock).
const now = Number(process.env.SEED_NOW) || Date.now();

type Who = "admin" | "write" | "read" | "none";
type Brain = "seed" | "empty" | "big";
type Screen = {
  name: string;
  path: string | ((t: Tokens) => string);
  who?: Who; // default admin
  brain?: Brain; // default seed
  widths?: number[]; // default both
  expect?: number[]; // response statuses that are not problems here (a 404 the screen asks for)
  act?: (page: Page) => Promise<void>; // steps before the picture; a step that fails is a problem
};
type Tokens = { write: string; read: string };

// Clicks the first element matching selector whose text is exactly text.
async function click(page: Page, selector: string, text: string): Promise<void> {
  const handle = await page.waitForFunction(
    (sel, txt) => [...document.querySelectorAll(sel)].find((el) => el.textContent?.trim() === txt) ?? null,
    { timeout: 5000 },
    selector,
    text,
  );
  const el = handle.asElement();
  if (!el) throw new Error(`no ${selector} saying "${text}"`);
  await (el as unknown as { click: () => Promise<void> }).click();
}

async function fill(page: Page, selector: string, value: string): Promise<void> {
  await page.waitForSelector(selector, { timeout: 5000 });
  await page.$eval(selector, (el) => ((el as HTMLInputElement).value = ""));
  await page.type(selector, value);
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const daysAgo = (n: number) => new Date(now - n * DAY).toISOString().replace(/\.\d{3}Z$/, "Z");

// Every screen of the design (design/README.md, "Routes" and "Screens"). Screens that write go
// last: each width gets a fresh server, so what they write never shows in another picture.
const screens: Screen[] = [
  { name: "signin", path: "/app/", who: "none" },
  {
    name: "signin-refused",
    path: "/app/",
    who: "none",
    expect: [401],
    act: async (p) => {
      await fill(p, "input[type=password]", "mys_not_a_real_token");
      await click(p, "button", "Sign in");
      await sleep(400);
    },
  },
  { name: "graph", path: "/app/" },
  { name: "graph-selected", path: "/app/?focus=storefront" },
  { name: "graph-type", path: "/app/?type=project" },
  { name: "graph-recent", path: "/app/?recent=24h" },
  { name: "graph-read", path: "/app/?focus=relay", who: "read" },
  { name: "graph-empty", path: "/app/", brain: "empty" },
  { name: "menu", path: "/app/", widths: [390], act: async (p) => click(p, "button", "Menu") },
  { name: "entities", path: "/app/entities" },
  { name: "entities-type", path: "/app/entities?type=project" },
  { name: "entities-filter", path: "/app/entities?q=zzz" },
  { name: "entities-empty", path: "/app/entities", brain: "empty" },
  { name: "entity", path: "/app/entities/storefront" },
  { name: "entity-linked", path: "/app/entities/alice?tab=linked" },
  { name: "entity-history", path: "/app/entities/self?tab=history" },
  { name: "entity-depth", path: "/app/entities/relay?depth=1" },
  { name: "entity-at", path: () => `/app/entities/self?at=${encodeURIComponent(daysAgo(5))}` },
  { name: "entity-type", path: "/app/entities/project" },
  { name: "entity-slash", path: "/app/entities/ci%2Fcd" },
  { name: "entity-long", path: "/app/entities/a-very-long-entity-name-that-an-agent-made-up-from-a-whole-sentence-about-checkout" },
  { name: "entity-markup", path: "/app/entities/relay" },
  { name: "entity-read", path: "/app/entities/storefront", who: "read" },
  { name: "entity-missing", path: "/app/entities/nobody", expect: [404] },
  { name: "search", path: "/app/search?q=bun" },
  { name: "search-none", path: "/app/search?q=zzzz" },
  { name: "search-start", path: "/app/search" },
  { name: "log", path: "/app/log" },
  { name: "log-forgotten", path: "/app/log?kind=forgotten" },
  { name: "log-empty", path: "/app/log", brain: "empty" },
  { name: "dream", path: "/app/dream" },
  { name: "dream-empty", path: "/app/dream", brain: "empty" },
  { name: "prime", path: "/app/prime" },
  { name: "prime-cut", path: "/app/prime?budget=400" },
  { name: "add", path: "/app/add?subject=relay" },
  { name: "add-read", path: "/app/add", who: "read" },
  { name: "settings", path: "/app/settings" },
  { name: "settings-pi", path: "/app/settings?tab=connect&agent=pi" },
  { name: "settings-mcp", path: "/app/settings?tab=connect&agent=mcp" },
  { name: "settings-tokens", path: "/app/settings?tab=tokens" },
  { name: "settings-server", path: "/app/settings?tab=server" },
  { name: "settings-read", path: "/app/settings", who: "read" },
  { name: "notfound", path: "/app/nope" },
  // Old addresses: each must land on its new screen.
  { name: "old-graph", path: "/app/graph?focus=relay" },
  { name: "old-entity", path: "/app/e/relay" },
  { name: "old-tokens", path: "/app/tokens" },
  // Screens that write.
  {
    name: "forget-dialog",
    path: "/app/entities/storefront",
    act: async (p) => {
      await click(p, "button", "Forget");
      await sleep(300);
    },
  },
  {
    name: "add-result",
    path: "/app/add",
    act: async (p) => {
      await fill(p, "textarea", "relay has a status page\nrelay is a place where friends post photos of their week\nrelay uses sqlite for the local cache");
      await click(p, "button", "Remember all");
      await sleep(500);
    },
  },
  {
    name: "settings-connect-created",
    path: "/app/settings?tab=connect&agent=claude-code",
    act: async (p) => {
      await click(p, "button", "Create token");
      await sleep(500);
    },
  },
  {
    name: "settings-token-created",
    path: "/app/settings?tab=tokens",
    act: async (p) => {
      await click(p, "button", "New token");
      await fill(p, "dialog input[type=text], dialog input:not([type])", "laptop");
      await click(p, "dialog button", "Create token");
      await sleep(500);
    },
  },
  { name: "big-graph", path: "/app/", brain: "big" },
  { name: "big-entities", path: "/app/entities", brain: "big" },
  { name: "big-log", path: "/app/log", brain: "big" },
];

function findBrowser(): string {
  const named = process.env.MYSELF_BROWSER;
  if (named) return named;
  for (const dir of (process.env.PATH ?? "").split(":")) {
    for (const name of ["chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"]) {
      if (dir && existsSync(join(dir, name))) return join(dir, name);
    }
  }
  throw new Error("no Chromium or Chrome on PATH; set MYSELF_BROWSER");
}

// The checks a machine can make against design/README.md, run in the page. Each finding is one
// line: what is wrong, the element, and its text.
function lint(coarse: boolean): string[] {
  const INK = "rgb(20, 20, 20)";
  const PAPER = "rgb(228, 226, 219)";
  const ok = (c: string) => c === INK || c === PAPER || c === "rgba(0, 0, 0, 0)" || c === "transparent";
  const found = new Map<string, string>();
  const say = (kind: string, el: Element, extra = "") => {
    if (found.has(kind)) return;
    const id = el.id ? `#${el.id}` : "";
    const cls = typeof el.className === "string" && el.className ? "." + el.className.trim().split(/\s+/).join(".") : "";
    const text = (el.textContent ?? "").trim().replace(/\s+/g, " ").slice(0, 40);
    found.set(kind, `${kind}: <${el.tagName.toLowerCase()}${id}${cls}>${text ? ` "${text}"` : ""}${extra}`);
  };
  const visible = (el: Element) => {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return false;
    if (el.closest(".sr, [hidden], [aria-hidden=true]")) return false;
    const s = getComputedStyle(el);
    return s.visibility !== "hidden" && s.display !== "none";
  };
  for (const el of document.querySelectorAll("body *")) {
    if (!visible(el) || el.closest("svg, canvas")) continue;
    const s = getComputedStyle(el);
    const ownText = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent!.trim() !== "");
    const size = parseFloat(s.fontSize);
    const face = s.fontFamily.split(",")[0].replace(/["']/g, "").trim();
    if (ownText) {
      if (size < 14) say(`text under 14 px (${size} px)`, el);
      if (face === "Anybody" && size < 28 && !el.closest(".wordmark")) say(`Anybody under 28 px (${size} px)`, el);
      if (!["Anybody", "Atkinson Hyperlegible Next", "Atkinson Hyperlegible Mono"].includes(face)) say(`face ${face}`, el);
      if (!ok(s.color)) say(`colour ${s.color}`, el);
    }
    if (el.matches("code, pre, kbd, samp") && face !== "Atkinson Hyperlegible Mono") say(`code not in mono (${face})`, el);
    if (!ok(s.backgroundColor)) say(`background ${s.backgroundColor}`, el);
    for (const side of ["Top", "Right", "Bottom", "Left"] as const) {
      const w = parseFloat(s.getPropertyValue(`border-${side.toLowerCase()}-width`));
      const c = s.getPropertyValue(`border-${side.toLowerCase()}-color`);
      if (w > 0 && s.getPropertyValue(`border-${side.toLowerCase()}-style`) !== "none" && !ok(c)) say(`border colour ${c}`, el);
    }
    if (s.borderRadius !== "0px" && s.borderRadius !== "") say(`rounded corner (${s.borderRadius})`, el);
    if (s.boxShadow !== "none" && !/^rgb\(20, 20, 20\) -?\d+px -?\d+px 0px( 0px)?$/.test(s.boxShadow)) say(`shadow ${s.boxShadow}`, el);
    if (parseFloat(s.opacity) < 1) say(`opacity ${s.opacity}`, el);
    if (s.filter !== "none" || (s as unknown as Record<string, string>).backdropFilter !== "none") say("filter or blur", el);
    const control = el.matches("button, select, textarea, input:not([type=checkbox]):not([type=radio]):not([type=hidden]), [role=tab], .btn");
    if (control) {
      const h = el.getBoundingClientRect().height;
      const min = coarse ? 44 : 32;
      if (h < min - 0.5) say(`control under ${min} px (${Math.round(h)} px)`, el);
    }
  }
  if (document.documentElement.scrollWidth > window.innerWidth) found.set("sideways", `scrolls sideways (${document.documentElement.scrollWidth} px wide)`);
  for (const face of ["Atkinson Hyperlegible Next", "Atkinson Hyperlegible Mono", "Anybody"]) {
    const used = [...document.querySelectorAll("body *")].some((el) => getComputedStyle(el).fontFamily.startsWith(`"${face}"`) || getComputedStyle(el).fontFamily.startsWith(face));
    if (used && !document.fonts.check(`16px "${face}"`)) found.set(`font ${face}`, `font not loaded: ${face}`);
  }
  return [...found.values()];
}

async function startServer(bin: string, brain: Brain): Promise<{ url: string; tokens: Tokens; stop: () => Promise<void> }> {
  const home = mkdtempSync(join(tmpdir(), "myself-screens-home-"));
  const file = join(home, "brain.jsonl");
  if (brain === "seed") writeFileSync(file, toJSONL(seedRecords(now)));
  if (brain === "big") writeFileSync(file, toJSONL(bigRecords(now)));
  const port = 20000 + Math.floor(Math.random() * 20000);
  const url = `http://127.0.0.1:${port}`;
  const env: Record<string, string> = {};
  for (const [k, v] of Object.entries(process.env)) if (v !== undefined && !k.startsWith("MYSELF_")) env[k] = v;
  env.MYSELF_FILE = file;
  env.MYSELF_ADMIN_TOKEN_SHA256 = createHash("sha256").update(ADMIN).digest("hex");
  const proc = Bun.spawn([bin, "serve", "--addr", `127.0.0.1:${port}`], { env, stdout: "ignore", stderr: "pipe" });
  for (let i = 0; ; i++) {
    try {
      if ((await fetch(`${url}/healthz`)).ok) break;
    } catch {
      // Not listening yet.
    }
    if (i > 100) throw new Error(`the server never answered: ${await new Response(proc.stderr).text()}`);
    await sleep(100);
  }
  const api = async (path: string, init: RequestInit & { token?: string } = {}) => {
    const res = await fetch(`${url}/api/v1/${path}`, { ...init, headers: { Authorization: `Bearer ${init.token ?? ADMIN}`, "Content-Type": "application/json" } });
    if (!res.ok) throw new Error(`${path}: ${res.status} ${await res.text()}`);
    return res.json();
  };
  const write = (await api("tokens", { method: "POST", body: JSON.stringify({ name: "claude-code-laptop", scope: "write" }) })).token;
  const read = (await api("tokens", { method: "POST", body: JSON.stringify({ name: "dashboard", scope: "read" }) })).token;
  await api("tokens", { method: "POST", body: JSON.stringify({ name: "pi-server", scope: "write" }) });
  await api("whoami", { token: write }); // so one token has a "last used"
  return {
    url,
    tokens: { write, read },
    stop: async () => {
      proc.kill();
      await proc.exited;
      rmSync(home, { recursive: true, force: true });
    },
  };
}

function build(): string {
  if (process.env.MYSELF_BIN) return process.env.MYSELF_BIN;
  const dir = mkdtempSync(join(tmpdir(), "myself-screens-bin-"));
  const bin = join(dir, "myself");
  const res = Bun.spawnSync(["go", "build", "-o", bin, "./cmd/myself"], { cwd: root, stderr: "pipe" });
  if (res.exitCode !== 0) throw new Error(`go build failed: ${res.stderr.toString()}`);
  return bin;
}

async function shoot(browser: Browser, bin: string, width: number, brain: Brain, list: Screen[], problems: string[]): Promise<void> {
  const server = await startServer(bin, brain);
  const coarse = width < 640;
  const page = await browser.newPage();
  try {
    await page.setViewport(coarse ? { width, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true } : { width, height: 900 });
    await page.emulateMediaFeatures([{ name: "prefers-reduced-motion", value: "reduce" }]);
    // One set of listeners per page, reading which screen is up (removing them breaks puppeteer).
    let current: Screen | null = null;
    let label = "";
    page.on("pageerror", (e) => current && problems.push(`${label}: page error: ${String(e)}`));
    page.on("console", (m) => current && m.type() === "error" && !/status of (401|404)/.test(m.text()) && problems.push(`${label}: console: ${m.text()}`));
    page.on("response", (r) => {
      if (!current || r.status() < 400 || (current.expect ?? []).includes(r.status())) return;
      problems.push(`${label}: ${r.status()} ${r.url().replace(server.url, "")}`);
    });
    let signedIn: Who | null = null;
    for (const s of list) {
      label = `${s.name}-${width}`;
      const who = s.who ?? "admin";
      if (who !== signedIn) {
        current = null;
        await page.goto(`${server.url}/app/`, { waitUntil: "domcontentloaded" });
        const token = who === "admin" ? ADMIN : who === "none" ? "" : server.tokens[who];
        await page.evaluate((t) => {
          sessionStorage.clear();
          localStorage.clear();
          if (t) localStorage.setItem("myself.token", t);
        }, token);
        signedIn = who;
      }
      current = s;
      const path = typeof s.path === "function" ? s.path(server.tokens) : s.path;
      try {
        await page.goto(`${server.url}${path}`, { waitUntil: "networkidle2", timeout: 15000 });
      } catch {
        // Screens that poll never go idle; what is there after the timeout is what we photograph.
      }
      await sleep(300);
      if (s.act) {
        try {
          await s.act(page);
        } catch (e) {
          problems.push(`${label}: could not do its steps: ${String(e).split("\n")[0]}`);
        }
      }
      for (const finding of await page.evaluate(lint, coarse)) problems.push(`${label}: design: ${finding}`);
      await page.screenshot({ path: join(out, `${label}.png`), fullPage: true });
      current = null;
    }
  } finally {
    await page.close();
    await server.stop();
  }
}

const problems: string[] = [];
const bin = build();
mkdirSync(out, { recursive: true });
const browser = await launch({ executablePath: findBrowser(), headless: true, args: ["--no-sandbox", "--disable-dev-shm-usage", "--font-render-hinting=none"] });
try {
  for (const width of widths) {
    for (const brain of ["seed", "empty", "big"] as Brain[]) {
      if (brain === "big" && !big && !only.some((n) => n.startsWith("big-"))) continue;
      const list = screens.filter((s) => (s.brain ?? "seed") === brain && (!s.widths || s.widths.includes(width)) && (only.length === 0 || only.includes(s.name)));
      if (list.length > 0) await shoot(browser, bin, width, brain, list, problems);
    }
  }
} finally {
  await browser.close();
}

const report = problems.length ? `${problems.length} problems:\n${problems.map((p) => `  ${p}`).join("\n")}\n` : "no problems\n";
writeFileSync(join(out, "report.txt"), report);
console.log(`screens in ${out}\n${report}`);
if (problems.length) process.exitCode = 1;
