// The Log: every fact and forget, newest first, live while you watch.
import { api, graph } from "../api.js";
import { h, put, num } from "../dom.js";
import {
  pageHeader, button, field, segmented, factTable, emptyState, errorBox, announce,
} from "../ui.js";

const view = () => document.getElementById("view");
const KINDS = ["all", "remembered", "forgotten"];
const SIZES = [50, 200, 1000];
const REFRESH_MS = 10000;

export async function logPage(ctx) {
  const v = view();
  let kind = KINDS.includes(ctx.query.get("kind")) ? ctx.query.get("kind") : "all";
  let n = SIZES.includes(Number(ctx.query.get("n"))) ? Number(ctx.query.get("n")) : 200;
  let paused = false;
  let timer = null;
  let alive = true;
  let seen = null; // the uuids read last time; null until the first read
  let entries = [];
  const names = new Set();
  let updated = null;
  let table = null;
  let reads = 0;

  const pause = button("Pause", { onClick: togglePause });
  pause.setAttribute("aria-pressed", "false");

  const show = segmented({
    label: "Show",
    options: [
      { value: "all", label: "All" },
      { value: "remembered", label: "Remembered" },
      { value: "forgotten", label: "Forgotten" },
    ],
    value: kind,
    onChange: (next) => {
      kind = next;
      ctx.setQuery({ kind: next === "all" ? null : next });
      paint();
    },
  });

  const last = h("select", {}, SIZES.map((size) =>
    h("option", { value: size, selected: size === n }, String(size))));
  last.addEventListener("change", () => {
    n = Number(last.value);
    ctx.setQuery({ n: n === 200 ? null : n });
    seen = null;
    load();
  });

  const stateLine = h("p", { class: "log-bar__state" });
  const bar = h("div", { class: "log-bar" },
    field({ label: "Show", inline: true, control: show }),
    field({ label: "Last", inline: true, control: last }),
    stateLine);

  // A forget renders as the fact it forgot: rows are keyed by the forget's
  // uuid, and forgottenBy makes the fact table strike the row and name the
  // reason. A forget whose fact is older than the rows read has no fact to
  // show, so its cells are overridden with the uuid the forget names.
  const forgottenBy = new Map();
  const missing = new Map();
  function cell(key, rec) {
    if (!missing.has(rec.uuid)) return undefined;
    if (key === "subject" || key === "relation") return "";
    if (key === "object") {
      const uuid = missing.get(rec.uuid);
      return h("code", { title: uuid }, uuid.slice(0, 8));
    }
    return undefined;
  }

  const tableHost = h("div", { class: "log-table" });
  const emptyHost = h("div", { class: "log-empty" });
  const body = h("div", { class: "log-body" }, tableHost, emptyHost);
  const content = h("div", { class: "log-content", "aria-busy": "true" });
  put(v,
    pageHeader({ title: "Log", actions: [pause] }),
    content);

  function paintState() {
    stateLine.textContent = paused
      ? "Paused"
      : "Live · updated " + (updated ? updated.toLocaleTimeString() : "…");
  }

  function emptyText() {
    if (!entries.length) return "Nothing written yet.";
    if (kind === "remembered") return `No remembered facts in the last ${num(n)}.`;
    if (kind === "forgotten") return `No forgotten facts in the last ${num(n)}.`;
    return "Nothing written yet.";
  }

  function paint() {
    const shown = entries
      .filter((entry) => kind === "all" || entry.kind === kind)
      .map((entry) => entry.rec);
    if (!table) {
      table = factTable(shown, {
        columns: ["written", "subject", "relation", "object"],
        names,
        forgottenBy,
        cell,
      });
      tableHost.append(table.el);
    } else {
      table.update(shown);
    }
    const empty = shown.length === 0;
    tableHost.hidden = empty;
    emptyHost.hidden = !empty;
    emptyHost.replaceChildren(...(empty ? [emptyState(emptyText())] : []));
    paintState();
  }

  async function load() {
    const seq = ++reads;
    content.setAttribute("aria-busy", "true");
    let res, g;
    try {
      [res, g] = await Promise.all([api("log", { query: { n } }), graph()]);
    } catch (err) {
      if (err.status === 401 || !alive || seq !== reads) return;
      content.setAttribute("aria-busy", "false");
      content.replaceChildren(errorBox(err, { retry: () => load() }));
      return;
    }
    if (!alive || seq !== reads) return;

    names.clear();
    for (const node of g.nodes) names.add(node.name);
    const records = res.records.slice().reverse();
    const byID = new Map(records.map((rec) => [rec.uuid, rec]));
    const previous = seen;
    const fresh = new Set();
    if (previous) {
      for (const rec of records) if (!previous.has(rec.uuid)) fresh.add(rec.uuid);
    }
    seen = new Set(records.map((rec) => rec.uuid));

    forgottenBy.clear();
    missing.clear();
    entries = records.map((rec) => {
      if (rec.relation !== "forget") return { kind: "remembered", rec };
      forgottenBy.set(rec.uuid, rec);
      const forgotten = byID.get(rec.subject);
      if (forgotten) {
        return {
          kind: "forgotten",
          rec: {
            uuid: rec.uuid,
            time: rec.time,
            subject: forgotten.subject,
            relation: forgotten.relation,
            object: forgotten.object,
          },
        };
      }
      missing.set(rec.uuid, rec.subject);
      return {
        kind: "forgotten",
        rec: { uuid: rec.uuid, time: rec.time, subject: "", relation: "", object: "" },
      };
    });

    updated = new Date();
    content.setAttribute("aria-busy", "false");
    // Keep the same bar and table in the document once they are there, so
    // focus stays where the reader put it.
    if (!content.contains(bar)) content.replaceChildren(bar, body);
    paint();

    let newRows = 0;
    for (const tr of tableHost.querySelectorAll("tr.flash")) tr.classList.remove("flash");
    for (const tr of tableHost.querySelectorAll("tr[data-uuid]")) {
      if (!fresh.has(tr.dataset.uuid)) continue;
      tr.classList.add("flash");
      newRows++;
    }
    if (newRows) announce(newRows === 1 ? "1 new fact" : `${num(newRows)} new facts`);
  }

  function schedule() {
    clearInterval(timer);
    timer = paused ? null : setInterval(() => {
      if (!document.hidden) load();
    }, REFRESH_MS);
  }

  function onVisible() {
    if (!document.hidden && !paused) load();
  }

  function togglePause() {
    paused = !paused;
    pause.setAttribute("aria-pressed", String(paused));
    pause.textContent = paused ? "Resume" : "Pause";
    paintState();
    schedule();
    if (!paused) load();
  }

  document.addEventListener("visibilitychange", onVisible);
  ctx.setCleanup(() => {
    alive = false;
    clearInterval(timer);
    document.removeEventListener("visibilitychange", onVisible);
  });
  schedule();
  await load();
}
