// Search: a plain substring search over live facts.
import { api, enc, graph } from "../api.js";
import { h, put, entityLink, time, state } from "../dom.js";
import { object } from "../ui.js";

const view = () => document.getElementById("view");

export async function searchPage(ctx) {
  const v = view();
  const q = ctx.query.get("q") || "";
  const input = h("input", { id: "q", type: "search", value: q, required: true, autocomplete: "off", spellcheck: "false" });
  const form = h("form", { class: "searchbar", role: "search" },
    h("label", { for: "q", class: "label" }, "Text in a subject, relation or object"),
    h("div", { class: "row" }, input, h("button", { type: "submit", class: "btn btn--ink" }, "Search")),
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    if (input.value.trim()) ctx.navigate("/app/search?q=" + enc(input.value.trim()));
  });
  put(v, h("h1", { class: "title" }, "Search"), form);
  if (!q) {
    input.focus();
    return;
  }
  const loading = state(`Searching for "${q}"…`);
  put(v, loading);
  const [res, g] = await Promise.all([api("search", { query: { q } }), graph()]);
  const names = new Set(g.nodes.map((n) => n.name));
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
