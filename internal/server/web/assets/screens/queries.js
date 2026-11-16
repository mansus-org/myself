// Queries: what this server was asked, and what it answered. Not the brain's
// history — the Log is that — but the questions that came in over the API, from
// any client, newest first.
import { api } from "../api.js";
import { h, put, time, num } from "../dom.js";
import {
  pageHeader, button, field, segmented, table, emptyState, errorBox, stamp,
} from "../ui.js";

const view = () => document.getElementById("view");
const VERBS = ["all", "search", "recall", "describe"];
const SIZES = [50, 200, 1000];
const REFRESH_MS = 10000;

export async function queriesPage(ctx) {
  const v = view();
  let verb = VERBS.includes(ctx.query.get("verb")) ? ctx.query.get("verb") : "all";
  let n = SIZES.includes(Number(ctx.query.get("n"))) ? Number(ctx.query.get("n")) : 200;
  let paused = false;
  let timer = null;
  let alive = true;
  let reads = 0;
  let updated = null;

  const pause = button("Pause", { onClick: togglePause });
  pause.setAttribute("aria-pressed", "false");

  const show = segmented({
    label: "Verb",
    options: [
      { value: "all", label: "All" },
      { value: "search", label: "Search" },
      { value: "recall", label: "Recall" },
      { value: "describe", label: "Describe" },
    ],
    value: verb,
    onChange: (next) => {
      verb = next;
      ctx.setQuery({ verb: next === "all" ? null : next });
      load();
    },
  });

  const last = h("select", {}, SIZES.map((size) =>
    h("option", { value: size, selected: size === n }, String(size))));
  last.addEventListener("change", () => {
    n = Number(last.value);
    ctx.setQuery({ n: n === 200 ? null : n });
    load();
  });

  const stateLine = h("p", { class: "queries-bar__state" });
  const bar = h("div", { class: "queries-bar" },
    field({ label: "Verb", inline: true, control: show }),
    field({ label: "Last", inline: true, control: last }),
    stateLine);

  const content = h("div", { class: "queries-content", "aria-busy": "true" });
  put(v, pageHeader({
    title: "Queries",
    actions: [pause],
    description: "What the API was asked — search, recall and describe — from any client, newest first. It is the server's own record, not the brain's: nothing here is a fact.",
  }), content);

  function paintState(total) {
    stateLine.textContent = paused
      ? "Paused"
      : "Live · updated " + (updated ? updated.toLocaleTimeString() : "…")
        + ` · ${num(total)} kept`;
  }

  function row(e) {
    const asked = h("span", {}, e.text,
      e.at ? h("span", { class: "queries__asof" }, " as of ", e.at) : null);
    const answers = e.error
      ? h("p", { class: "queries__error" }, e.error)
      : (e.facts && e.facts.length
        ? h("ul", { class: "queries__facts" }, e.facts.map((f) => h("li", {}, f)))
        : h("span", { class: "queries__none" }, "nothing"));
    const took = e.ms ? ` · ${num(e.ms)}ms` : "";
    return {
      key: `${e.time}-${e.verb}-${e.text}`,
      cells: {
        when: time(e.time),
        verb: stamp(e.verb),
        asked,
        found: e.error
          ? h("span", { class: "queries__failed" }, "failed")
          : `${num(e.hits)} ${e.hits === 1 ? "fact" : "facts"}${took}`,
        answers: h("span", {},
          answers,
          e.model ? h("span", { class: "queries__model" }, stamp(e.model)) : null),
        who: e.who || "",
      },
    };
  }

  async function load() {
    const seq = ++reads;
    content.setAttribute("aria-busy", "true");
    let res;
    try {
      res = await api("queries", { query: { n, verb: verb === "all" ? null : verb } });
    } catch (err) {
      if (err.status === 401 || !alive || seq !== reads) return;
      content.setAttribute("aria-busy", "false");
      content.replaceChildren(errorBox(err, { retry: () => load() }));
      return;
    }
    if (!alive || seq !== reads) return;
    updated = new Date();
    content.setAttribute("aria-busy", "false");
    content.replaceChildren(
      bar,
      table({
        caption: "Queries",
        columns: [
          { key: "when", label: "When" },
          { key: "verb", label: "Verb" },
          { key: "asked", label: "Asked" },
          { key: "found", label: "Found" },
          { key: "answers", label: "What came back" },
          { key: "who", label: "Who" },
        ],
        rows: res.queries.map(row),
        empty: verb === "all" ? "Nothing asked yet." : `Nothing asked with ${verb} yet.`,
        limit: 1000,
      }),
    );
    paintState(res.total);
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
