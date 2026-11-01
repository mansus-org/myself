// Read pages: entities, one entity, search, log, prime, dream.
import { api, auth, enc, graph } from "./api.js";
import { h, put, link, entityHref, entityLink, normalizeName, time, state, copyButton, errorText } from "./dom.js";
import { forgetRecord } from "./write.js";

const view = () => document.getElementById("view");

function typeStamps(types) {
  return types.length ? h("span", { class: "types" }, types.map((t) => h("a", {
    class: "stamp", href: "/app/?type=" + enc(t), "data-link": true, title: "Every " + t,
  }, t))) : null;
}

// object renders a record's object, linked when it names an entity.
function object(rec, names) {
  const n = normalizeName(rec.object);
  return names.has(n) ? link(entityHref(n), rec.object) : rec.object;
}

// ---------- Entities ----------

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

// ---------- One entity ----------

// at values: the input is local time; the API wants RFC 3339 in UTC.
function toAt(local) {
  const d = new Date(local);
  return isNaN(d) ? null : d.toISOString().replace(/\.\d{3}Z$/, "Z");
}
function toLocal(at) {
  const d = new Date(at);
  if (isNaN(d)) return "";
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

export async function entityPage(ctx, name) {
  const v = view();
  const depth = Math.max(0, Math.min(4, parseInt(ctx.query.get("depth") || "0", 10) || 0));
  const at = ctx.query.get("at");
  const go = (d, a) => {
    const p = new URLSearchParams();
    if (d) p.set("depth", d);
    if (a) p.set("at", a);
    const s = p.toString();
    ctx.navigate(entityHref(name) + (s ? "?" + s : ""), { replace: true });
  };

  const title = h("h1", { class: "title title--entity" }, name);
  const loading = state("Loading…");
  put(v, title, loading);

  let desc, hist, g;
  try {
    [desc, hist, g] = await Promise.all([
      api("describe/" + enc(name), { query: { depth: depth || null, at } }),
      at ? null : api("history/" + enc(name)).catch((err) => (err.status === 404 ? null : Promise.reject(err))),
      graph(),
    ]);
  } catch (err) {
    loading.remove();
    if (err.status === 404) {
      put(v, state(at ? `${name} did not exist yet at that time.` : `No entity called ${name}. It may have been forgotten, or the name is spelled differently.`),
        h("p", {}, link("/app/search?q=" + enc(name), `Search for "${name}"`)),
        at ? h("p", {}, link(entityHref(name), "See it as of now")) : null);
      return;
    }
    if (err.status === 400) {
      put(v, state(err.message, "error"), h("p", {}, link(entityHref(name), "Back to now")));
      return;
    }
    throw err;
  }
  loading.remove();
  const names = new Set(g.nodes.map((n) => n.name));
  const node = g.nodes.find((n) => n.name === desc.entities[0].name);
  title.textContent = desc.entities[0].name;
  const canForget = auth.canWrite && !at;

  // Controls: depth and time travel.
  const depthSel = h("select", { id: "depth" }, [0, 1, 2, 3, 4].map((d) =>
    h("option", { value: d, selected: d === depth }, d === 0 ? "This entity only" : d === 1 ? "1 hop" : `${d} hops`)));
  depthSel.addEventListener("change", () => go(+depthSel.value, at));
  const atIn = h("input", { id: "at", type: "datetime-local", value: at ? toLocal(at) : "" });
  const atForm = h("form", { class: "at-form" },
    h("div", { class: "field" }, h("label", { for: "at", class: "label" }, "As of (your local time)"), atIn),
    h("button", { type: "submit", class: "btn" }, "Travel"),
    at ? h("button", { type: "button", class: "btn", onclick: () => go(depth, null) }, "Back to now") : null,
  );
  atForm.addEventListener("submit", (e) => {
    e.preventDefault();
    go(depth, atIn.value ? toAt(atIn.value) : null);
  });

  put(v, 
    h("div", { class: "entity-head" },
      node ? typeStamps(node.types) : null,
      at ? h("p", { class: "banner" }, h("span", { class: "stamp" }, "As of"), " ", time(at), ". Facts written later, and forgets written later, are hidden.") : null,
    ),
    h("div", { class: "tools tools--entity" },
      h("div", { class: "field" }, h("label", { for: "depth", class: "label" }, "Depth"), depthSel),
      atForm),
  );

  const refresh = () => ctx.navigate(location.pathname + location.search, { replace: true });
  for (const e of desc.entities) put(v, describedEntity(e, names, canForget, refresh, e === desc.entities[0]));
  if (hist) put(v, historyView(hist.records, names));
  else if (!at) put(v, h("section", { class: "section" }, h("h2", { class: "h2" }, "History"),
    state(`${desc.entities[0].name} is never a subject: it only appears as the object of other facts.`)));
}

function describedEntity(e, names, canForget, refresh, main) {
  const rows = [];
  for (const r of e.out) {
    rows.push(h("li", { class: "fact" },
      h("span", { class: "rel" }, r.relation),
      h("span", { class: "obj" }, object(r, names)),
      h("span", { class: "meta" }, time(r.time)),
      canForget ? h("button", {
        type: "button", class: "btn btn--small", "aria-label": `Forget: ${r.subject} ${r.relation} ${r.object}`,
        onclick: async () => (await forgetRecord(r)) && refresh(),
      }, "Forget") : null,
    ));
  }
  for (const r of e.in) {
    rows.push(h("li", { class: "fact fact--in" },
      h("span", { class: "rel" }, h("span", { "aria-hidden": "true" }, "← "), h("span", { class: "sr" }, "linked from "), entityLink(r.subject)),
      h("span", { class: "obj" }, r.relation),
      h("span", { class: "meta" }, time(r.time)),
      canForget ? h("button", {
        type: "button", class: "btn btn--small", "aria-label": `Forget: ${r.subject} ${r.relation} ${r.object}`,
        onclick: async () => (await forgetRecord(r)) && refresh(),
      }, "Forget") : null,
    ));
  }
  const heading = main
    ? h("h2", { class: "h2" }, "Facts")
    : h("h2", { class: "h2 h2--hop" }, entityLink(e.name), h("span", { class: "hop" }, e.depth === 1 ? "1 hop away" : `${e.depth} hops away`));
  return h("section", { class: "section" + (main ? "" : " section--hop") },
    heading,
    rows.length ? h("ul", { class: "facts" }, rows) : state("No live facts."),
  );
}

// historyView strikes through forgotten records and shows why.
function historyView(records, names) {
  const forgets = new Map();
  for (const r of records) if (r.relation === "forget") forgets.set(r.subject, r);
  const rows = records.filter((r) => r.relation !== "forget").map((r) => {
    const f = forgets.get(r.uuid);
    const fact = [h("span", { class: "rel" }, r.relation), " ", h("span", { class: "obj" }, object(r, names))];
    return h("li", { class: "hist" + (f ? " hist--gone" : "") },
      h("span", { class: "meta" }, time(r.time)),
      f ? h("del", { class: "hist__fact" }, h("span", { class: "sr" }, "forgotten: "), fact) : h("span", { class: "hist__fact" }, fact),
      f ? h("span", { class: "hist__why" }, "Forgotten ", time(f.time), f.object ? ": " + f.object : ", no reason given.") : null,
    );
  });
  return h("section", { class: "section" },
    h("h2", { class: "h2" }, "History"),
    h("p", { class: "help" }, "Every fact ever written with this subject, oldest first. Forgotten ones are struck through."),
    h("ol", { class: "history" }, rows),
  );
}

// ---------- Search ----------

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

// ---------- Log ----------

export async function logPage(ctx) {
  const v = view();
  const sizes = [50, 200, 1000];
  let n = sizes.includes(+ctx.query.get("n")) ? +ctx.query.get("n") : 200;
  let paused = false;
  let timer = null;
  let seen = null;

  const pause = h("button", { type: "button", class: "btn", "aria-pressed": "false" }, "Pause");
  const nSel = h("select", { id: "log-n" }, sizes.map((s) => h("option", { value: s, selected: s === n }, `Last ${s}`)));
  const status = h("p", { class: "count" });
  const news = h("p", { class: "sr", "aria-live": "polite" });
  const list = h("ol", { class: "log" });

  async function load() {
    let res, g;
    try {
      [res, g] = await Promise.all([api("log", { query: { n } }), graph()]);
    } catch (err) {
      if (err.status === 401) return;
      status.textContent = "Refresh failed: " + errorText(err) + " Trying again in 10 s.";
      return;
    }
    const names = new Set(g.nodes.map((x) => x.name));
    const byID = new Map(res.records.map((r) => [r.uuid, r]));
    const recs = res.records.slice().reverse();
    const fresh = seen ? recs.filter((r) => !seen.has(r.uuid)) : [];
    seen = new Set(recs.map((r) => r.uuid));
    if (!recs.length) {
      list.replaceChildren();
      status.textContent = "The log is empty. Records appear here as agents write them.";
      return;
    }
    const isNew = new Set(fresh.map((r) => r.uuid));
    list.replaceChildren(...recs.map((r) => logRow(r, byID, names, isNew.has(r.uuid))));
    const t = new Date().toLocaleTimeString();
    status.textContent = `${recs.length} newest records, newest first. Updated ${t}` + (paused ? ", paused." : ", refreshing every 10 s.");
    if (fresh.length) news.textContent = fresh.length === 1 ? "1 new record." : `${fresh.length} new records.`;
  }

  function schedule() {
    clearInterval(timer);
    timer = paused ? null : setInterval(() => {
      if (!document.hidden) load();
    }, 10000);
  }

  pause.addEventListener("click", () => {
    paused = !paused;
    pause.setAttribute("aria-pressed", String(paused));
    pause.textContent = paused ? "Resume" : "Pause";
    status.textContent = status.textContent.replace(/, (paused|refreshing every 10 s)\.$/, paused ? ", paused." : ", refreshing every 10 s.");
    schedule();
    if (!paused) load();
  });
  nSel.addEventListener("change", () => {
    n = +nSel.value;
    history.replaceState(null, "", "/app/log?n=" + n);
    seen = null;
    load();
  });

  put(v, 
    h("h1", { class: "title" }, "Log"),
    h("p", { class: "lead" }, "What agents write, as they write it."),
    h("div", { class: "tools tools--row" },
      h("div", { class: "field" }, h("label", { for: "log-n", class: "label" }, "Show"), nSel),
      pause),
    status, news, list,
  );
  status.textContent = "Loading the log…";
  await load();
  schedule();
  ctx.setCleanup(() => clearInterval(timer));
}

function logRow(r, byID, names, isNew) {
  const head = [h("span", { class: "meta" }, time(r.time)), isNew ? h("span", { class: "stamp stamp--small" }, "new") : null];
  if (r.relation === "forget") {
    const t = byID.get(r.subject);
    return h("li", { class: "logrow logrow--forget" + (isNew ? " is-new" : "") }, ...head,
      h("span", { class: "logrow__fact" },
        h("span", { class: "rel" }, "forget"), " ",
        t ? h("del", {}, entityLink(t.subject), " ", t.relation, " ", t.object) : h("code", { title: r.subject }, r.subject.slice(0, 13) + "…"),
        r.object ? h("span", { class: "quiet" }, ": " + r.object) : null,
      ));
  }
  return h("li", { class: "logrow" + (isNew ? " is-new" : "") }, ...head,
    h("span", { class: "logrow__fact" }, entityLink(r.subject), " ", h("span", { class: "rel" }, r.relation), " ", object(r, names)));
}

// ---------- Prime ----------

export async function primePage(ctx) {
  const v = view();
  const budget = Math.max(1, parseInt(ctx.query.get("budget") || "2000", 10) || 2000);
  const input = h("input", { id: "budget", type: "number", min: 1, step: 100, value: budget, inputmode: "numeric" });
  const form = h("form", { class: "tools tools--row" },
    h("div", { class: "field" }, h("label", { for: "budget", class: "label" }, "Budget (characters)"), input),
    h("button", { type: "submit", class: "btn" }, "Show"),
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    ctx.navigate("/app/prime?budget=" + enc(input.value || "2000"), { replace: true });
  });
  put(v, h("h1", { class: "title" }, "Prime"),
    h("p", { class: "lead" }, "The brief an agent reads at the start of a session (", h("code", {}, "myself prime"), ")."),
    form);
  const res = await api("prime", { query: { budget } });
  const pre = h("pre", { class: "prime", tabindex: "0" }, res.text || "");
  if (!res.text) {
    put(v, state("The brain is empty, so the brief is empty too."));
    return;
  }
  put(v, 
    h("div", { class: "row row--between" },
      h("p", { class: "count" }, `${res.text.length} of ${res.budget} characters` + (res.truncated ? ", cut at a line to fit." : ".")),
      copyButton("Copy", () => res.text, pre)),
    pre,
  );
}

// ---------- Dream ----------

export async function dreamPage() {
  const v = view();
  put(v, h("h1", { class: "title" }, "Dream"),
    h("p", { class: "lead" }, "Subjects that are probably the same thing: equal once dashes and a trailing s are removed."));
  const res = await api("dream");
  if (!res.near_duplicates.length) {
    put(v, state("No near-duplicates. Every subject is distinct."));
    return;
  }
  put(v, 
    h("p", { class: "count" }, res.near_duplicates.length === 1 ? "1 group" : `${res.near_duplicates.length} groups`),
    h("ul", { class: "dream" }, res.near_duplicates.map((g) => h("li", { class: "dream__group" },
      g.map((name, i) => [i ? h("span", { class: "dream__eq", "aria-hidden": "true" }, "≈") : null, i ? h("span", { class: "sr" }, " and ") : null, entityLink(name)])))),
    h("p", { class: "help" }, "To merge, remember the facts under one name and forget the others from their entity pages."),
  );
}
