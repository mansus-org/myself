// Search: the same question two ways — the word you typed, or what you meant.
// They are siblings, not one command: words answers from the log alone and will
// answer the same way forever, meaning answers with a model and says which one.
import { api, enc, graph } from "../api.js";
import { h, put, entityLink, time, state } from "../dom.js";
import { object, segmented, notice, stamp, emptyState } from "../ui.js";

const view = () => document.getElementById("view");

const MODES = [
  { value: "words", label: "Words" },
  { value: "meaning", label: "Meaning" },
];

export async function searchPage(ctx) {
  const v = view();
  const q = ctx.query.get("q") || "";
  const mode = ctx.query.get("mode") === "meaning" ? "meaning" : "words";

  const go = (text, next) => {
    const params = new URLSearchParams();
    if (text) params.set("q", text);
    if (next !== "words") params.set("mode", next);
    const s = params.toString();
    ctx.navigate("/app/search" + (s ? "?" + s : ""));
  };

  const input = h("input", { id: "q", type: "search", value: q, required: true, autocomplete: "off", spellcheck: "false" });
  const modes = segmented({
    label: "Match",
    options: MODES,
    value: mode,
    onChange: (next) => go(input.value.trim(), next),
  });
  const form = h("form", { class: "searchbar", role: "search" },
    h("label", { for: "q", class: "label" },
      mode === "meaning" ? "What you mean, in your own words" : "Text in a subject, relation or object"),
    h("div", { class: "row" }, input, h("button", { type: "submit", class: "btn btn--ink" }, "Search")),
    h("div", { class: "searchbar__modes" }, modes, h("p", { class: "searchbar__hint" }, mode === "meaning"
      ? "Reads the question with an embedding model, and brings the facts around each match: what the things it names are."
      : "A plain substring match. It will answer the same way forever, which is what makes it dependable.")),
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    if (input.value.trim()) go(input.value.trim(), modes.value);
  });
  put(v, h("h1", { class: "title" }, "Search"), form);
  if (!q) {
    input.focus();
    return;
  }
  const g = await graph();
  const names = new Set(g.nodes.map((n) => n.name));
  if (mode === "meaning") await meaning(v, q, names); else await words(v, q, names);
}

const factItem = (r, names) => h("li", { class: "fact fact--triple" },
  h("span", { class: "subj" }, entityLink(r.subject)),
  h("span", { class: "rel" }, r.relation),
  h("span", { class: "obj" }, object(r, names)),
  h("span", { class: "meta" }, time(r.time)));

async function words(v, q, names) {
  const loading = state(`Searching for "${q}"…`);
  put(v, loading);
  const res = await api("search", { query: { q } });
  loading.remove();
  if (!res.matches.length) {
    put(v, state(`No live fact contains "${q}". Search is a plain substring match, so try a shorter word.`));
    return;
  }
  put(v,
    h("p", { class: "count" }, res.matches.length === 1 ? "1 fact" : `${res.matches.length} facts`),
    h("ul", { class: "facts facts--triple" }, res.matches.map((r) => h("li", { class: "fact" },
      h("span", { class: "subj" }, entityLink(r.subject)),
      h("span", { class: "rel" }, r.relation),
      h("span", { class: "obj" }, object(r, names)),
      h("span", { class: "meta" }, time(r.time)),
    ))),
  );
}

async function meaning(v, q, names) {
  const loading = state(`Reading what "${q}" means…`);
  put(v, loading);
  let res;
  try {
    res = await api("recall", { query: { q } });
  } catch (err) {
    if (err.status === 401) throw err;
    loading.remove();
    // The server says what is missing — no vectors in this brain, or no endpoint
    // to read with. That is not a fault to retry, and it is not "nothing is
    // close", so it is said in the server's own words and left there.
    put(v, notice({ stamp: "Recall", text: err.message }));
    return;
  }
  loading.remove();
  if (!res.matches.length) {
    put(v, emptyState(`Nothing in the brain is close to "${q}" — to ${res.model}, at least. Words may still find the fact: try a shorter word.`));
    return;
  }
  put(v,
    h("p", { class: "count" },
      res.matches.length === 1 ? "1 fact" : `${res.matches.length} facts`,
      " · read by ", res.model,
      res.graph ? ` · with ${res.graph} hop${res.graph > 1 ? "s" : ""} of the graph` : ""),
    h("ul", { class: "recall" }, res.matches.map((m) => h("li", { class: "recall__match" },
      h("div", { class: "recall__fact" },
        h("span", { class: "recall__score", title: "how close this is, as read by " + res.model }, m.score.toFixed(2)),
        h("span", { class: "subj" }, entityLink(m.subject)),
        h("span", { class: "rel" }, m.relation),
        h("span", { class: "obj" }, object(m, names)),
        h("span", { class: "meta" }, time(m.time))),
      m.context && m.context.length ? h("div", { class: "recall__context" },
        h("p", { class: "recall__why" }, "around this fact"),
        h("ul", {}, m.context.map((c) => factItem(c, names)))) : null,
    ))),
  );
}
