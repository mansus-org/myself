// Recall: ask what you mean, not what you typed. It has its own page because it
// is its own verb — it answers with a model and names it, it brings the facts
// around each match, and when the brain keeps no vectors it says what to
// configure instead of answering badly. Search is the word; this is the sense.
import { api, graph } from "../api.js";
import { h, put, entityLink, time, exactTime, state } from "../dom.js";
import {
  pageHeader, field, segmented, button, notice, emptyState, errorBox, object,
  linkButton,
} from "../ui.js";

const view = () => document.getElementById("view");

const COUNTS = [5, 10, 20, 50];
const HOPS = [
  { value: "0", label: "None" },
  { value: "1", label: "1 hop" },
  { value: "2", label: "2 hops" },
  { value: "3", label: "3 hops" },
];

const toLocal = (iso) => (iso || "").slice(0, 19);
const toAt = (local) => (local ? new Date(local).toISOString() : null);

function numberOrNull(text) {
  if (text === null || text === "") return null;
  const n = Number(text);
  return Number.isFinite(n) ? n : null;
}

const factItem = (r, names) => h("li", { class: "fact fact--triple" },
  h("span", { class: "subj" }, entityLink(r.subject)),
  h("span", { class: "rel" }, r.relation),
  h("span", { class: "obj" }, object(r, names)),
  h("span", { class: "meta" }, time(r.time)));

export async function recallPage(ctx) {
  const v = view();
  const q = ctx.query.get("q") || "";
  const n = COUNTS.includes(Number(ctx.query.get("n"))) ? Number(ctx.query.get("n")) : 10;
  const min = numberOrNull(ctx.query.get("min"));
  const hops = HOPS.some((o) => o.value === ctx.query.get("graph")) ? ctx.query.get("graph") : "1";
  const at = ctx.query.get("at") || "";

  // Every knob is in the URL, so a recall is a link you can send someone: the
  // question, how close it had to be, and how far the graph was walked.
  const set = (params) => {
    ctx.setQuery(params);
    if (q) ctx.navigate(location.pathname + location.search, { replace: true });
  };

  const input = h("input", {
    id: "q", type: "search", value: q, required: true, autocomplete: "off",
    spellcheck: "false", placeholder: "what does relay run on",
  });
  const form = h("form", { class: "recallbar", role: "search" },
    h("label", { for: "q", class: "label" }, "What you mean, in your own words"),
    h("div", { class: "row" }, input, h("button", { type: "submit", class: "btn btn--ink" }, "Recall")),
    h("p", { class: "recallbar__hint" },
      "An embedding model reads the question and brings the live facts closest to it, best first — and the facts around each one in the graph, so you can see what the things it names are."));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (text) set({ q: text });
  });

  const countSelect = h("select", {}, COUNTS.map((c) =>
    h("option", { value: c, selected: c === n }, String(c))));
  countSelect.addEventListener("change", () => {
    const next = Number(countSelect.value);
    set({ n: next === 10 ? null : next });
  });

  const minInput = h("input", {
    id: "recall-min", type: "number", step: "0.05", min: "-1", max: "1",
    value: min === null ? "" : String(min), placeholder: "anything",
  });
  minInput.addEventListener("change", () => set({ min: numberOrNull(minInput.value) }));

  const hopsPick = segmented({
    label: "Around each match", options: HOPS, value: hops,
    onChange: (next) => set({ graph: next === "1" ? null : next }),
  });

  const atInput = h("input", { id: "recall-at", type: "datetime-local", value: toLocal(at) });
  const atForm = h("form", { class: "recallbar__asof" },
    field({ label: "As of", control: atInput, inline: true }),
    at ? button("Now", { onClick: () => set({ at: null }) }) : null);
  atForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const next = atInput.value ? toAt(atInput.value) : null;
    if (atInput.value && !next) return;
    set({ at: next });
  });

  const bar = h("div", { class: "recall-bar" },
    field({ label: "How many", control: countSelect, inline: true }),
    field({ label: "Close enough", control: minInput, inline: true }),
    field({ label: "Around each match", control: hopsPick, inline: true }),
    atForm);

  put(v,
    pageHeader({
      title: "Recall",
      description: "Meaning, not words. The facts are the record and the vectors are a projection over them, rebuilt by `myself index`; the model that read your question is named with the answer.",
    }),
    form, bar);

  if (!q) {
    input.focus();
    v.append(emptyState(
      "Ask a question in your own words. Recall needs a brain that keeps vectors and a server that can read them; if either is missing it will say which, rather than answer badly.",
      linkButton("/app/search", "Match the words instead")));
    return;
  }

  const loading = state(`Reading what "${q}" means…`);
  v.append(loading);
  const g = await graph();
  const names = new Set(g.nodes.map((node) => node.name));

  let res;
  try {
    res = await api("recall", { query: { q, n, min, graph: Number(hops), at: at || null } });
  } catch (err) {
    if (err.status === 401) throw err;
    loading.remove();
    // A 400 is the server explaining itself — no vectors in this brain, no
    // endpoint to read with, a min out of range. That is not a fault to retry,
    // so it is said in the server's own words and left there. Anything else is.
    v.append(err.status === 400
      ? notice({ stamp: "Recall", text: err.message })
      : errorBox(err, { retry: () => recallPage(ctx) }));
    return;
  }
  loading.remove();

  if (!res.matches.length) {
    v.append(emptyState(
      `Nothing in the brain is close to "${q}" — to ${res.model}, at least.`,
      linkButton("/app/search?" + new URLSearchParams({ q }), "Match the words instead")));
    return;
  }

  v.append(
    h("p", { class: "count" },
      res.matches.length === 1 ? "1 fact" : `${res.matches.length} facts`,
      " · read by ", res.model,
      res.graph ? ` · ${res.graph} hop${res.graph > 1 ? "s" : ""} around each` : " · nothing around them",
      res.at ? ` · as of ${exactTime(res.at)}` : ""),
    h("ul", { class: "recall" }, res.matches.map((m) => h("li", { class: "recall__match" },
      h("div", { class: "recall__fact" },
        h("span", { class: "recall__score", title: "how close this is, as read by " + res.model }, m.score.toFixed(2)),
        h("span", { class: "subj" }, entityLink(m.subject)),
        h("span", { class: "rel" }, m.relation),
        h("span", { class: "obj" }, object(m, names)),
        h("span", { class: "meta" }, time(m.time))),
      m.context && m.context.length ? h("div", { class: "recall__context" },
        h("p", { class: "recall__why" }, "reached from this fact, not matched by it"),
        h("ul", {}, m.context.map((c) => factItem(c, names)))) : null,
    ))),
  );
}
