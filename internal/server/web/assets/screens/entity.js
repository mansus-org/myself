// One entity: its facts, hops, history and the forget dialog.
import { api, auth, enc, graph, dropGraph } from "../api.js";
import { h, put, link, entityHref, entityLink, time, state, dialog, errorText } from "../dom.js";
import { object, typeStamps } from "../ui.js";

const view = () => document.getElementById("view");

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

// forgetRecord asks for a reason and forgets rec. It resolves true once
// the forget record is written, false if the reader cancelled.
async function forgetRecord(rec) {
  const result = await dialog("Forget this fact?", (close) => {
    const reason = h("textarea", { id: "forget-reason", rows: 3, maxlength: 2000 });
    const err = h("p", { class: "form-error", role: "alert", hidden: true });
    const ok = h("button", { type: "submit", class: "btn btn--ink" }, "Forget");
    const form = h("form", { class: "stack", method: "dialog" },
      h("p", { class: "dialog__fact" }, h("strong", {}, rec.subject), " ", h("span", { class: "rel" }, rec.relation), " ", rec.object),
      h("p", { class: "help" }, "Nothing is erased: a forget record is appended and this fact moves to the history, struck through."),
      h("label", { for: "forget-reason", class: "label" }, "Reason (optional)"),
      reason,
      err,
      h("div", { class: "row" },
        ok,
        h("button", { type: "button", class: "btn", onclick: () => close(false) }, "Cancel"),
      ),
    );
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      ok.disabled = true;
      ok.textContent = "Forgetting…";
      try {
        await api("forget", { method: "POST", body: { uuid: rec.uuid, reason: reason.value.trim() } });
        dropGraph();
        close(true);
      } catch (ex) {
        err.textContent = errorText(ex);
        err.hidden = false;
        ok.disabled = false;
        ok.textContent = "Forget";
      }
    });
    queueMicrotask(() => reason.focus());
    return form;
  });
  return result === true;
}
