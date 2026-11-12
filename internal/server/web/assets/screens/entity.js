// One entity: its facts, what links to it, its history and the forget dialog.
import { api, auth, enc, graph, dropGraph } from "../api.js";
import {
  h, put, entityHref, entityLink, time, exactTime, num, errorText,
} from "../dom.js";
import {
  pageHeader, button, linkButton, field, tabs, typeStamps, factTable, notice,
  errorBox, emptyState, announce, dialog,
} from "../ui.js";

const view = () => document.getElementById("view");
const RELATIONS = ["is", "has", "uses", "does", "prefers", "related"];

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

function relationRank(relation) {
  const i = RELATIONS.indexOf(relation);
  return i < 0 ? RELATIONS.length : i;
}

function sortFacts(facts) {
  return facts.slice().sort((a, b) =>
    relationRank(a.relation) - relationRank(b.relation) ||
    new Date(a.time) - new Date(b.time) ||
    a.uuid.localeCompare(b.uuid));
}

function newestFirst(facts) {
  return facts.slice().sort((a, b) =>
    new Date(b.time) - new Date(a.time) || b.uuid.localeCompare(a.uuid));
}

function reload(ctx) {
  ctx.navigate(location.pathname + location.search, { replace: true });
}

export async function entityPage(ctx, name) {
  const v = view();
  const depth = Math.max(0, Math.min(4, parseInt(ctx.query.get("depth") || "0", 10) || 0));
  const at = ctx.query.get("at");
  const crumbs = [{ label: "Entities", href: "/app/entities" }, { label: name }];

  put(v, pageHeader({ crumbs, title: name }), h("div", { "aria-busy": "true" }));

  let desc, hist, g;
  try {
    [desc, hist, g] = await Promise.all([
      api("describe/" + enc(name), { query: { depth: depth || null, at } }),
      at ? Promise.resolve(null) : api("history/" + enc(name)).catch((err) => (err.status === 404 ? null : Promise.reject(err))),
      graph(),
    ]);
  } catch (err) {
    if (err.status === 401) return;
    if (err.status === 404) {
      const missing = at
        ? emptyState(`${name} did not exist yet at that time.`, linkButton(entityHref(name), "Back to now"))
        : emptyState(`No entity called ${name}.`, linkButton("/app/search?q=" + enc(name), `Search for "${name}"`));
      v.replaceChildren(pageHeader({ crumbs, title: name }), missing);
      return;
    }
    v.replaceChildren(
      pageHeader({ crumbs, title: name }),
      errorBox(err, { retry: () => reload(ctx) }),
    );
    return;
  }

  const entity = desc.entities[0];
  const node = g.nodes.find((n) => n.name === entity.name);
  const names = new Set(g.nodes.map((n) => n.name));
  const canForget = auth.canWrite && !at;
  const historyFacts = hist ? hist.records.filter((r) => r.relation !== "forget") : [];
  const historyNewest = newestFirst(historyFacts);
  const forgottenBy = new Map();
  if (hist) for (const r of hist.records) if (r.relation === "forget") forgottenBy.set(r.subject, r);

  const factsValue = h("span", {}, num(entity.out.length));
  const linkedValue = h("span", {}, num(entity.in.length));
  const meta = [["Facts", factsValue], ["Linked from", linkedValue]];
  if (!at && historyFacts.length) {
    const oldest = historyNewest[historyNewest.length - 1];
    const newest = historyNewest[0];
    meta.push(["First written", time(oldest.time)], ["Last written", time(newest.time)]);
  }

  const actions = [linkButton("/app/?focus=" + enc(entity.name), "Show in graph")];
  if (auth.canWrite) {
    actions.push(linkButton("/app/add?subject=" + enc(entity.name), "Add a fact", { kind: "primary" }));
  }
  const head = pageHeader({
    crumbs: [{ label: "Entities", href: "/app/entities" }, { label: entity.name }],
    title: entity.name,
    actions,
    stamps: node ? typeStamps(node.types) : null,
    meta,
  });

  const depthSelect = h("select", { id: "entity-depth" }, [0, 1, 2, 3, 4].map((d) =>
    h("option", { value: String(d), selected: d === depth },
      d === 0 ? "This entity" : d === 1 ? "1 hop" : `${d} hops`)));
  const depthField = field({ label: "Depth", control: depthSelect, inline: true });
  depthSelect.addEventListener("change", () => {
    ctx.setQuery({ depth: +depthSelect.value || null });
    reload(ctx);
  });

  const atInput = h("input", { id: "entity-at", type: "datetime-local", value: at ? toLocal(at) : "" });
  const atForm = h("form", { class: "asof" },
    field({ label: "As of", control: atInput, inline: true }),
    button("Go", { type: "submit" }),
    at ? button("Now", { onClick: () => { ctx.setQuery({ at: null }); reload(ctx); } }) : null,
  );
  atForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const next = atInput.value ? toAt(atInput.value) : null;
    if (atInput.value && !next) return;
    ctx.setQuery({ at: next });
    reload(ctx);
  });

  let atNotice = null;
  if (at) {
    const q = new URLSearchParams(ctx.query.toString());
    q.delete("at");
    const s = q.toString();
    atNotice = notice({
      stamp: "as of",
      text: h("time", { datetime: at }, exactTime(at)),
      action: linkButton(entityHref(entity.name) + (s ? "?" + s : ""), "Back to now", { kind: "quiet" }),
    });
    atNotice.classList.add("asof-notice");
  }

  const tables = [];
  let historyEntry = null;

  const liveColumns = ["relation", "object", "written"].concat(canForget ? ["actions"] : []);
  const linkedColumns = ["subject", "relation", "written"].concat(canForget ? ["actions"] : []);

  function liveTable(facts) {
    const entry = { facts, table: null };
    entry.table = factTable(entry.facts, {
      columns: liveColumns,
      names,
      canForget,
      onForget: forgetFact,
    });
    tables.push(entry);
    return entry.table.el;
  }

  function linkedTable(facts) {
    const entry = { facts, table: null };
    entry.table = factTable(entry.facts, {
      columns: linkedColumns,
      names,
      canForget,
      onForget: forgetFact,
    });
    tables.push(entry);
    return entry.table.el;
  }

  const factsPanel = h("div", {},
    entity.out.length ? liveTable(sortFacts(entity.out)) : emptyState("No live facts."),
    desc.entities.slice(1).map((e) => h("section", { class: "hop" },
      h("h2", { class: "hop-head" },
        entityLink(e.name),
        h("span", {}, e.depth === 1 ? "1 hop" : `${e.depth} hops`)),
      e.out.length ? liveTable(sortFacts(e.out)) : emptyState("No live facts."))));

  const linkedPanel = h("div", {},
    entity.in.length ? linkedTable(entity.in.slice()) : emptyState("No other entity links here."));

  let historyPanel = null;
  if (!at) {
    if (!hist || !historyNewest.length) {
      historyPanel = emptyState(`${entity.name} only appears as the object of other facts.`);
    } else {
      historyEntry = { facts: historyNewest, table: null };
      historyEntry.table = factTable(historyEntry.facts, {
        columns: ["written", "relation", "object"],
        names,
        forgottenBy,
      });
      historyPanel = h("div", {}, historyEntry.table.el);
    }
  }

  const panels = { facts: factsPanel, linked: linkedPanel };
  if (historyPanel) panels.history = historyPanel;

  const tabItems = [
    { id: "facts", label: "Facts", count: entity.out.length },
    { id: "linked", label: "Linked from", count: entity.in.length },
  ];
  if (!at) tabItems.push({ id: "history", label: "History", count: historyFacts.length });

  let selected = ctx.query.get("tab");
  if (!tabItems.some((item) => item.id === selected)) selected = "facts";
  if (at && ctx.query.get("tab") === "history") ctx.setQuery({ tab: null });
  depthField.hidden = selected !== "facts";

  const tabSet = tabs({
    label: "Entity sections",
    items: tabItems,
    selected,
    onSelect: (id) => {
      ctx.setQuery({ tab: id });
      depthField.hidden = id !== "facts";
      tabSet.panel.replaceChildren(panels[id]);
    },
  });
  tabSet.panel.append(panels[selected]);

  const tabCounts = new Map();
  for (const btn of tabSet.list.querySelectorAll("[data-tab]")) {
    tabCounts.set(btn.dataset.tab, btn.querySelector(".tab__count"));
  }

  function updateCounts() {
    factsValue.textContent = num(entity.out.length);
    linkedValue.textContent = num(entity.in.length);
    if (tabCounts.get("facts")) tabCounts.get("facts").textContent = num(entity.out.length);
    if (tabCounts.get("linked")) tabCounts.get("linked").textContent = num(entity.in.length);
  }

  async function forgetFact(rec) {
    const result = await forgetRecord(rec);
    if (!result) return;
    dropGraph();
    for (const e of desc.entities) {
      e.out = e.out.filter((r) => r.uuid !== rec.uuid);
      e.in = e.in.filter((r) => r.uuid !== rec.uuid);
    }
    if (hist && rec.subject === entity.name && result.forgotten && result.forgotten[0]) {
      const forget = result.forgotten[0];
      hist.records.push(forget);
      forgottenBy.set(rec.uuid, forget);
    }
    for (const entry of tables) {
      entry.facts = entry.facts.filter((r) => r.uuid !== rec.uuid);
      entry.table.update(entry.facts);
    }
    if (historyEntry) historyEntry.table.update(historyEntry.facts);
    updateCounts();
    announce(`Forgot: ${rec.subject} ${rec.relation} ${rec.object}`);
  }

  v.replaceChildren(head);
  put(v,
    h("div", { class: "viewbar" }, depthField, atForm),
    atNotice,
    tabSet.list,
    tabSet.panel,
  );
}

// forgetRecord asks for a reason and forgets rec. It resolves with the API
// response once the forget is written, or null if the reader cancelled.
async function forgetRecord(rec) {
  return dialog("Forget this fact?", (close) => {
    const reason = h("input", { id: "forget-reason", type: "text", maxlength: "2000" });
    const err = h("p", { class: "form-error", role: "alert", hidden: true });
    const ok = button("Forget", { kind: "primary", type: "submit" });
    const form = h("form", { class: "forget-form" },
      h("p", { class: "dialog__fact" },
        h("strong", {}, rec.subject), " ", h("strong", {}, rec.relation), " ", rec.object),
      field({
        label: "Reason (optional)",
        control: reason,
        help: "It moves to History, struck through.",
      }),
      err,
      h("div", { class: "dialog__actions" },
        button("Cancel", { onClick: () => close(null) }),
        ok),
    );
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      ok.disabled = true;
      ok.textContent = "Forgetting…";
      try {
        const result = await api("forget", {
          method: "POST",
          body: { uuid: rec.uuid, reason: reason.value.trim() },
        });
        close(result);
      } catch (ex) {
        err.textContent = errorText(ex);
        err.hidden = false;
        ok.disabled = false;
        ok.textContent = "Forget";
      }
    });
    queueMicrotask(() => reason.focus());
    return form;
  }) || null;
}
