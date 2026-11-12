// The Entities page: one list filtered by name and type.
import { api, auth, graph } from "../api.js";
import { h, put, link, entityHref, state, errorText } from "../dom.js";
import { typeStamps } from "../ui.js";

const view = () => document.getElementById("view");

export async function entitiesPage(ctx) {
  const v = view();
  const q = ctx.query;
  let type = q.get("type");
  const loading = state("Loading entities…");
  put(v, h("h1", { class: "title" }, "Entities"), loading);
  const [g, types] = await Promise.all([graph(), api("ls")]);
  let members = null;
  if (type) {
    try {
      members = new Set((await api("ls", { query: { type } })).entities);
    } catch (err) {
      if (err.status !== 404) throw err;
      type = null;
    }
  }
  loading.remove();
  if (!g.nodes.length) {
    put(v, state("Nothing remembered yet. When an agent runs myself remember, its entities show up here."),
      auth.canWrite ? h("p", {}, link("/app/add", "Add the first facts")) : null);
    return;
  }

  const filter = h("input", { id: "filter", type: "search", value: q.get("q") || "", autocomplete: "off", spellcheck: "false" });
  const count = h("p", { class: "count", "aria-live": "polite" });
  const list = h("ul", { class: "entities" });
  const facets = h("div", { class: "facets", role: "group", "aria-labelledby": "facets-l" });

  const setURL = () => {
    const p = new URLSearchParams();
    if (type) p.set("type", type);
    if (filter.value) p.set("q", filter.value);
    const s = p.toString();
    history.replaceState(null, "", "/app/" + (s ? "?" + s : ""));
  };

  function draw() {
    const f = filter.value.trim().toLowerCase();
    const shown = g.nodes.filter((n) => (!members || members.has(n.name)) && (!f || n.name.includes(f)));
    list.replaceChildren(...shown.map((n) => h("li", { class: "entity" },
      h("a", { class: "entity__name", href: entityHref(n.name), "data-link": true }, n.name),
      typeStamps(n.types),
      h("span", { class: "meta" }, n.records === 1 ? "1 fact" : `${n.records} facts`),
    )));
    const of = type ? ` of type ${type}` : "";
    count.textContent = shown.length
      ? `${shown.length} ${shown.length === 1 ? "entity" : "entities"}${of}${f ? ` matching "${f}"` : ""}`
      : `No entity${of} matches "${f}".`;
    for (const b of facets.querySelectorAll("button")) b.setAttribute("aria-pressed", String((b.value || null) === type));
  }

  async function pick(t) {
    if (t === type) t = null;
    if (t) {
      try {
        members = new Set((await api("ls", { query: { type: t } })).entities);
      } catch (err) {
        count.textContent = errorText(err);
        return;
      }
    } else members = null;
    type = t;
    setURL();
    draw();
  }

  put(facets, 
    h("span", { id: "facets-l", class: "label" }, "Type"),
    h("button", { type: "button", class: "facet", value: "", onclick: () => pick(null) }, "All"),
    types.entities.map((t) => h("button", { type: "button", class: "facet", value: t, onclick: () => pick(t) }, t)),
  );
  filter.addEventListener("input", () => {
    setURL();
    draw();
  });
  put(v, 
    h("div", { class: "tools" },
      h("div", { class: "field" }, h("label", { for: "filter", class: "label" }, "Filter by name"), filter),
      facets),
    count,
    list,
  );
  draw();
}
