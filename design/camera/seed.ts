// The brain the camera photographs: a few weeks of an owner (alice) and her agent writing about
// their projects, with every state a screen has to draw. Times are relative to now, so "3 min ago",
// the Recent filter and the log look the same on every run; uuids and order are deterministic.
//
// What it holds on purpose:
// - types and types of types (relay is project, project is software), so `ls` and filters nest;
// - near-duplicate names for Dream: relay / relays, myself-cli / myselfcli;
// - an `is` object that is a sentence, which became a type (a-micro-social-network-for-friends);
// - forgets with a reason and one without, and molting on `self` (a fact forgotten and rewritten);
// - a name with a slash (ci/cd), unicode names (zoë, café-menu), a very long name, an object
//   holding markup that must render as text, a long paragraph, and an entity with no links;
// - hubs (alice, storefront) and writes in the last hour, day and week.
//
// bigRecords is a second, procedural brain of a few thousand entities, for checking that tables,
// filters and the graph still work at size (SEED=big).

export type Rec = { uuid: string; subject: string; relation: string; object: string; time: string };

const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

// mulberry32: a small seeded PRNG, so uuids are the same on every run.
function prng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// A UUIDv7 (RFC 9562) for ms: 48 bits of time, version 7, 12 bits of counter, variant 10, random.
function uuidAt(ms: number, seq: number, rnd: () => number): string {
  const b = new Uint8Array(16);
  let t = ms;
  for (let i = 5; i >= 0; i--) {
    b[i] = t % 256;
    t = Math.floor(t / 256);
  }
  b[6] = 0x70 | ((seq >> 8) & 0x0f);
  b[7] = seq & 0xff;
  for (let i = 8; i < 16; i++) b[i] = Math.floor(rnd() * 256);
  b[8] = 0x80 | (b[8] & 0x3f);
  const hex = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

const rfc3339 = (ms: number): string => new Date(ms).toISOString().replace(/\.\d{3}Z$/, "Z");

// One write: a fact (subject relation object, optionally keyed so a later forget can name it),
// or a forget of a keyed fact with a reason ("" for none).
type Write = [string, string, string, string?] | ["forget", string, string];

// A session is a burst of writes that starts `ago` ms before now. Sessions run oldest first.
type Session = { ago: number; writes: Write[] };

const D = (days: number, hours: number, minutes = 0): number => days * DAY + hours * HOUR + minutes * MIN;

const sessions: Session[] = [
  {
    ago: D(38, 2, 48),
    writes: [
      ["self", "is", "agent"],
      ["self", "related", "alice"],
      ["alice", "is", "person"],
      ["alice", "prefers", "short answers, no emoji"],
      ["alice", "prefers", "bun over npm for JavaScript and TypeScript"],
      ["storefront", "is", "project"],
      ["storefront", "uses", "npm", "sf-npm"],
      ["storefront", "uses", "react for the catalogue pages"],
      ["project", "is", "software"],
      ["react", "is", "library"],
      ["library", "is", "software"],
      ["npm", "is", "tool"],
      ["bun", "is", "tool"],
      ["tool", "is", "software"],
    ],
  },
  {
    ago: D(35, 7),
    writes: [
      ["relay", "is", "project"],
      ["relay", "uses", "go", "relay-go"],
      ["go", "is", "language"],
      ["relay", "does", "fan-out of posts to the timelines of followers"],
      ["relay", "has", "repo github.com/acme/relay"],
      ["relays", "is", "project"],
    ],
  },
  {
    ago: D(30, 3),
    writes: [
      ["storefront", "uses", "postgres 16 through drizzle"],
      ["postgres", "is", "database"],
      ["drizzle", "is", "library"],
      ["database", "is", "software"],
      [
        "storefront",
        "has",
        "decision: server-rendered pages over a SPA, because the catalogue is mostly static, the team is two people, and hydration bugs cost a week in March. Revisit if checkout needs offline support.",
      ],
      ["alice", "uses", "neovim with lazy.nvim"],
      ["neovim", "is", "tool"],
      ["alice", "has", "timezone Europe/Lisbon"],
    ],
  },
  {
    ago: D(24, 5),
    writes: [
      ["billing-service", "is", "service"],
      ["service", "is", "software"],
      ["billing-service", "uses", "stripe for invoices and the customer portal"],
      ["stripe", "is", "service"],
      ["billing-service", "related", "storefront"],
      ["bob", "is", "person"],
      ["billing-service", "has", "owner bob"],
      ["bob", "prefers", "small pull requests, under 300 lines"],
    ],
  },
  {
    ago: D(20, 4),
    writes: [
      ["forget", "sf-npm", "switched to bun, npm stays in CI only"],
      ["storefront", "uses", "bun"],
      ["storefront", "uses", "biome for linting and formatting"],
      ["biome", "is", "tool"],
      ["storefront", "uses", "hono for the API"],
      ["hono", "is", "library"],
    ],
  },
  {
    ago: D(15, 6),
    writes: [
      ["myself-cli", "is", "project"],
      ["myself-cli", "uses", "go"],
      ["myself-cli", "has", "repo github.com/mansus-org/myself"],
      ["myself-cli", "does", "keep memory for coding agents in one append-only JSONL file"],
      ["myselfcli", "is", "project"],
      ["web-ui", "is", "feature"],
      ["web-ui", "related", "myself-cli"],
      ["web-ui", "uses", "canvas for the graph view"],
      ["scratch", "has", "notes that link to nothing yet"],
    ],
  },
  {
    ago: D(12, 1),
    writes: [
      ["forget", "relay-go", "moved off go"],
      ["relay", "uses", "bun for the backend, biome for linting"],
      ["relay", "is", "a micro social network for friends"],
      ["carol", "is", "person"],
      ["carol", "related", "relay"],
      ["carol", "does", "design and the iOS client"],
    ],
  },
  {
    ago: D(9, 3),
    writes: [
      ["self", "does", "run the tests before every commit"],
      ["self", "prefers", "asking before deleting files"],
      ["self", "has", "a habit of explaining too much", "self-habit"],
      ["alice", "prefers", "commit messages that say what is now true"],
      ["alice", "has", "a draft talk on agent memory", "alice-talk"],
    ],
  },
  {
    ago: D(6, 2),
    writes: [
      ["garden-planner", "is", "project"],
      ["garden-planner", "uses", "python 3.13 with uv"],
      ["python", "is", "language"],
      ["uv", "is", "tool"],
      ["garden-planner", "does", "plan what to sow each month on a balcony in Lisbon"],
      ["garden-planner", "related", "alice"],
      ["typescript", "is", "language"],
      ["storefront", "uses", "typescript in strict mode"],
      ["ci/cd", "is", "practice"],
      ["storefront", "uses", "ci/cd on GitHub Actions, deploying on merge to main"],
    ],
  },
  {
    ago: D(3, 8),
    writes: [
      ["forget", "self-habit", "answers are short now"],
      ["self", "has", "answers that are short and say what changed"],
      ["forget", "alice-talk", ""],
      ["alice", "uses", "a 1-bit e-ink monitor for reading"],
      ["a-very-long-entity-name-that-an-agent-made-up-from-a-whole-sentence-about-checkout", "is", "project"],
      ["storefront", "related", "a-very-long-entity-name-that-an-agent-made-up-from-a-whole-sentence-about-checkout"],
    ],
  },
  {
    ago: D(2, 4),
    writes: [
      ["relay", "has", '<script>alert("x")</script> in a test fixture, shown as text'],
      ["zoë", "is", "person"],
      ["zoë", "related", "garden-planner"],
      ["café-menu", "is", "feature"],
      ["café-menu", "related", "storefront"],
    ],
  },
  {
    ago: D(1, 2),
    writes: [
      ["billing-service", "uses", "postgres 16, its own database"],
      ["billing-service", "has", "an alert: invoices fail when stripe webhooks time out; retries go through redis"],
      ["redis", "is", "database"],
    ],
  },
  {
    ago: D(0, 3),
    writes: [
      ["storefront", "has", "a checkout that needs offline support after all"],
      ["storefront", "does", "weekly releases on Thursdays"],
    ],
  },
  { ago: D(0, 0, 20), writes: [["web-ui", "has", "a graph-first home"]] },
  {
    ago: D(0, 0, 2),
    writes: [
      ["self", "does", "read the brain before answering questions about alice"],
      ["relay", "uses", "redis for timelines"],
    ],
  },
];

// seedRecords returns the small brain, oldest first, relative to now.
export function seedRecords(now = Date.now()): Rec[] {
  const rnd = prng(0x6d797365);
  const out: Rec[] = [];
  const keys = new Map<string, string>();
  let last = 0;
  for (const s of sessions) {
    let ms = Math.floor((now - s.ago) / 1000) * 1000;
    for (const w of s.writes) {
      ms = Math.max(ms + 7000 + Math.floor(rnd() * 20000), last + 1000);
      last = ms;
      const uuid = uuidAt(ms + Math.floor(rnd() * 999), Math.floor(rnd() * 0x7ff), rnd);
      if (w[0] === "forget") {
        const target = keys.get(w[1]);
        if (target === undefined) throw new Error(`seed: forget of unknown key ${w[1]}`);
        out.push({ uuid, subject: target, relation: "forget", object: w[2], time: rfc3339(ms) });
        continue;
      }
      const [subject, relation, object, key] = w;
      if (key !== undefined) keys.set(key, uuid);
      out.push({ uuid, subject, relation, object, time: rfc3339(ms) });
    }
  }
  if (last > now) throw new Error("seed: a write landed in the future; space the last sessions out");
  return out;
}

const WORDS = [
  "amber", "basalt", "cedar", "delta", "ember", "fjord", "garnet", "harbor", "indigo", "juniper",
  "kelp", "lumen", "maple", "nimbus", "onyx", "pollen", "quartz", "raven", "sorrel", "tundra",
  "umber", "velvet", "willow", "xenon", "yarrow", "zephyr", "acorn", "birch", "comet", "dune",
];
const KINDS = [
  "project", "service", "library", "tool", "person", "team", "database", "feature", "decision", "bug",
  "meeting", "document", "dataset", "model", "repo", "queue", "endpoint", "dashboard", "runbook", "incident",
];

// bigRecords returns a procedural brain: about n entities, each with a type and a few facts and
// links, written over 90 days, with some forgets. Deterministic for a given now and n.
export function bigRecords(now = Date.now(), n = 3000): Rec[] {
  const rnd = prng(0x62696721);
  const pick = <T,>(xs: T[]): T => xs[Math.floor(rnd() * xs.length)];
  const names: string[] = [];
  for (let i = 0; names.length < n; i++) {
    const name = `${pick(WORDS)}-${pick(WORDS)}-${(i % 97).toString(36)}`;
    if (!names.includes(name)) names.push(name);
  }
  const writes: [string, string, string][] = [];
  for (const k of KINDS) writes.push([k, "is", "thing"]);
  for (const name of names) {
    writes.push([name, "is", pick(KINDS)]);
    const facts = 1 + Math.floor(rnd() * 4);
    for (let f = 0; f < facts; f++) {
      const rel = pick(["has", "uses", "does", "prefers", "related"]);
      writes.push([name, rel, rnd() < 0.5 ? pick(names) : `${pick(WORDS)} ${pick(WORDS)} and ${pick(WORDS)}`]);
    }
  }
  // Facts over 90 days up to an hour ago, then one forget in twenty facts during the last hour.
  const out: Rec[] = [];
  const step = Math.floor((90 * DAY - HOUR) / writes.length);
  let ms = now - 90 * DAY;
  for (const [subject, relation, object] of writes) {
    ms += step;
    out.push({ uuid: uuidAt(ms, Math.floor(rnd() * 0x7ff), rnd), subject, relation, object, time: rfc3339(ms) });
  }
  const facts = out.length;
  const forgets = Math.floor(facts / 20);
  const gap = Math.floor((HOUR - MIN) / forgets);
  for (let i = 0; i < forgets; i++) {
    const target = out[Math.floor(rnd() * facts)];
    ms += gap;
    out.push({ uuid: uuidAt(ms, 0, rnd), subject: target.uuid, relation: "forget", object: pick(["", "wrong", "duplicate", "moved"]), time: rfc3339(ms) });
  }
  return out;
}

// toJSONL renders records as a brain file: one compact object per line, keys in the stored order.
export function toJSONL(recs: Rec[]): string {
  return recs.map((r) => JSON.stringify({ uuid: r.uuid, subject: r.subject, relation: r.relation, object: r.object, time: r.time })).join("\n") + "\n";
}
