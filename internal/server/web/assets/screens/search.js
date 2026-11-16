// Search: the word, not the sense. Asking by meaning is Recall — another verb,
// another answer, another page. This one matches text in a subject, relation or
// object, answers from the log alone, and will answer the same way forever,
// which is what makes it dependable.
import { api, graph } from "../api.js";
import { h, put, link, entityLink, time, state } from "../dom.js";
import { object, emptyState } from "../ui.js";

const view = () => document.getElementById("view");

export async function searchPage(ctx) {
  const v = view();
  const q = ctx.query.get("q") || "";

  const recallHref = "/app/recall" + (q ? "?" + new URLSearchParams({ q }) : "");
  const input = h("input", { id: "q", type: "search", value: q, required: true, autocomplete: "off", spellcheck: "false" });
  const form = h("form", { class: "searchbar", role: "search" },
    h("label", { for: "q", class: "label" }, "Text in a subject, relation or object"),
    h("div", { class: "row" }, input, h("button", { type: "submit", class: "btn btn--ink" }, "Search")),
    h("p", { class: "searchbar__hint" },
      "A plain substring match. It will answer the same way forever, which is what makes it dependable. ",
      link(recallHref, "Ask Recall for what you mean.")),
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (text) ctx.navigate("/app/search?" + new URLSearchParams({ q: text }));
  });
  put(v, h("h1", { class: "title" }, "Search"), form);
  if (!q) {
    input.focus();
    return;
  }

  const loading = state(`Searching for "${q}"…`);
  put(v, loading);
  const g = await graph();
  const names = new Set(g.nodes.map((n) => n.name));
  const res = await api("search", { query: { q } });
  loading.remove();
  if (!res.matches.length) {
    put(v, emptyState(`No live fact contains "${q}". Search matches words, so try a shorter one.`,
      link(recallHref, `Ask Recall what "${q}" means`)));
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
