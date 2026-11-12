// The graph: GET /api/v1/graph drawn on a canvas with a small force layout
// (Barnes-Hut repulsion, springs on links, a pull to the centre). Two inks
// only: what is out of focus is drawn as dither, never as grey.
import { api, auth, enc, graph as fetchGraph } from "../api.js";
import { h, put, entityHref, link, normalizeName, num, count } from "../dom.js";
import {
  button, linkButton, field, segmented, typeStamps, factList, notice,
  errorBox, figure, emptyState, legend,
} from "../ui.js";

const INK = "#141414";
const PAPER = "#e4e2db";
const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

// The entity page's fact order: is, has, uses, does, prefers, related, then
// oldest first.
const REL_ORDER = ["is", "has", "uses", "does", "prefers", "related"];

function entityOrder(a, b) {
  const ra = REL_ORDER.indexOf(a.relation);
  const rb = REL_ORDER.indexOf(b.relation);
  const ka = ra < 0 ? REL_ORDER.length : ra;
  const kb = rb < 0 ? REL_ORDER.length : rb;
  if (ka !== kb) return ka - kb;
  return (Date.parse(a.time) - Date.parse(b.time)) || a.uuid.localeCompare(b.uuid);
}

// ---------- layout ----------

const THETA2 = 0.81; // Barnes-Hut opening angle, squared
const CHARGE = 70;
const LINK_LEN = 38;

function cell(x, y, s) {
  return { x, y, s, m: 0, cx: 0, cy: 0, kids: null, n: null };
}

function insert(c, n, depth) {
  c.cx = (c.cx * c.m + n.x) / (c.m + 1);
  c.cy = (c.cy * c.m + n.y) / (c.m + 1);
  c.m++;
  if (!c.kids) {
    if (!c.n) {
      c.n = n;
      return;
    }
    if (depth > 24) return; // points on top of each other: lump them
    const old = c.n;
    c.n = null;
    c.kids = [null, null, null, null];
    placeIn(c, old, depth);
  }
  placeIn(c, n, depth);
}

function placeIn(c, n, depth) {
  const half = c.s / 2;
  const i = (n.x >= c.x + half ? 1 : 0) + (n.y >= c.y + half ? 2 : 0);
  if (!c.kids[i]) c.kids[i] = cell(c.x + (i & 1 ? half : 0), c.y + (i & 2 ? half : 0), half);
  insert(c.kids[i], n, depth + 1);
}

function tree(nodes) {
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const n of nodes) {
    if (n.x < x0) x0 = n.x;
    if (n.y < y0) y0 = n.y;
    if (n.x > x1) x1 = n.x;
    if (n.y > y1) y1 = n.y;
  }
  const root = cell(x0, y0, Math.max(x1 - x0, y1 - y0) + 1);
  for (const n of nodes) insert(root, n, 0);
  return root;
}

function repel(c, n, alpha) {
  if (!c || !c.m || c.n === n) return;
  let dx = c.cx - n.x, dy = c.cy - n.y;
  let d2 = dx * dx + dy * dy;
  if (c.kids && (c.s * c.s) / (d2 || 1) >= THETA2) {
    for (const k of c.kids) repel(k, n, alpha);
    return;
  }
  if (d2 === 0) {
    dx = Math.random() - 0.5;
    dy = Math.random() - 0.5;
    d2 = dx * dx + dy * dy;
  }
  if (d2 < 1) d2 = 1;
  const f = (CHARGE * c.m * alpha) / d2;
  n.vx -= dx * f;
  n.vy -= dy * f;
}

class Layout {
  constructor(nodes, links) {
    this.nodes = nodes;
    this.links = links;
    this.alpha = 1;
    this.target = 0;
    // Phyllotaxis start: spread out and deterministic.
    nodes.forEach((n, i) => {
      const r = 10 * Math.sqrt(i + 0.5), a = i * Math.PI * (3 - Math.sqrt(5));
      n.x = r * Math.cos(a);
      n.y = r * Math.sin(a);
      n.vx = n.vy = 0;
    });
  }

  get hot() {
    return this.alpha > 0.004 || this.target > 0;
  }

  tick() {
    const { nodes, links } = this;
    this.alpha += (this.target - this.alpha) * 0.0228;
    const a = this.alpha;
    if (nodes.length > 1) {
      const root = tree(nodes);
      for (const n of nodes) repel(root, n, a);
    }
    for (const l of links) {
      const s = l.s, t = l.t;
      const dx = t.x + t.vx - s.x - s.vx, dy = t.y + t.vy - s.y - s.vy;
      const d = Math.sqrt(dx * dx + dy * dy) || 1;
      const k = ((d - LINK_LEN) / d) * a * l.k;
      const b = s.deg / (s.deg + t.deg);
      t.vx -= dx * k * b;
      t.vy -= dy * k * b;
      s.vx += dx * k * (1 - b);
      s.vy += dy * k * (1 - b);
    }
    for (const n of nodes) {
      n.vx -= n.x * 0.03 * a;
      n.vy -= n.y * 0.03 * a;
      if (n.fx != null) {
        n.x = n.fx;
        n.y = n.fy;
        n.vx = n.vy = 0;
      } else {
        n.vx *= 0.6;
        n.vy *= 0.6;
        n.x += n.vx;
        n.y += n.vy;
      }
    }
  }
}

// ---------- page ----------

export async function graphPage(ctx) {
  const v = document.getElementById("view");
  // The toolbar is this page's header. While the first read is pending it
  // holds only the title; the content waits, busy.
  put(v,
    h("header", { class: "g-toolbar" }, h("h1", { class: "page-title" }, "Graph")),
    h("div", { class: "g-content", "aria-busy": "true" }),
  );

  const [g, types] = await Promise.all([fetchGraph(), api("ls")]);
  v.replaceChildren();

  if (!g.nodes.length) {
    // An empty brain has no canvas and no panel: one sentence, and the two
    // ways to give it something to draw.
    put(v,
      h("header", { class: "g-toolbar" }, h("h1", { class: "page-title" }, "Graph")),
      h("div", { class: "g-content" },
        emptyState("Nothing remembered yet.", h("div", { class: "g-empty-actions" },
          auth.canWrite ? linkButton("/app/add", "Add facts", { kind: "primary" }) : null,
          link("/app/settings", "Connect an agent")))),
    );
    return;
  }

  // Nodes and one link per connected pair; "is" wins when a pair has several.
  const byName = new Map();
  const nodes = g.nodes.map((n) => {
    const node = { ...n, deg: 0, nb: new Set(), isType: false };
    byName.set(n.name, node);
    return node;
  });
  const names = new Set(byName.keys());
  const pairs = new Map();
  for (const e of g.edges) {
    const s = byName.get(e.from), t = byName.get(e.to);
    if (!s || !t || s === t) continue;
    if (e.relation === "is") t.isType = true;
    const key = s.name < t.name ? s.name + "\n" + t.name : t.name + "\n" + s.name;
    const old = pairs.get(key);
    if (old) {
      if (e.relation === "is") old.is = true;
      continue;
    }
    pairs.set(key, { s, t, is: e.relation === "is" });
    s.deg++;
    t.deg++;
    s.nb.add(t);
    t.nb.add(s);
  }
  const links = [...pairs.values()];
  for (const l of links) l.k = 1 / Math.min(l.s.deg, l.t.deg);
  for (const n of nodes) n.size = Math.min(16, 5 + Math.sqrt(n.records) * 1.6);
  // The best-connected few keep their label at any zoom, for orientation.
  const landmarks = new Set(nodes.slice().sort((a, b) => b.records - a.records).slice(0, 12));

  // ---------- the address ----------
  // An unknown focus, type or recent is ignored and dropped from the address.
  const wantType = ctx.query.get("type") || "";
  const wantRecent = ctx.query.get("recent") || "";
  const wantFocus = ctx.query.get("focus") || "";
  const initialType = types.entities.includes(wantType) ? wantType : "";
  const initialRecent = wantRecent === "24h" || wantRecent === "7d" ? wantRecent : "";
  const initialFocus = byName.get(wantFocus) || null;
  ctx.setQuery({
    type: initialType,
    recent: initialRecent,
    focus: initialFocus ? initialFocus.name : null,
  });

  // ---------- DOM ----------
  const find = h("input", {
    id: "g-find", type: "search", list: "g-names",
    autocomplete: "off", spellcheck: "false",
  });
  const findForm = h("form", { class: "g-find", role: "search" },
    field({ label: "Find", control: find, inline: true }),
    h("datalist", { id: "g-names" }, nodes.map((n) => h("option", { value: n.name }))),
  );
  const typeSel = h("select", { id: "g-type" },
    h("option", { value: "" }, "All types"),
    types.entities.map((t) => h("option", { value: t }, t)));
  typeSel.value = initialType;
  const recent = segmented({
    label: "Recent",
    options: [{ value: "", label: "All" }, { value: "24h", label: "24 h" }, { value: "7d", label: "7 d" }],
    value: initialRecent,
    onChange: (win) => {
      ctx.setQuery({ recent: win });
      applyRecent(win);
    },
  });
  const zoomOut = button("−", { kind: "square", ariaLabel: "Zoom out", onClick: () => zoomAt(1 / 1.4) });
  const zoomIn = button("+", { kind: "square", ariaLabel: "Zoom in", onClick: () => zoomAt(1.4) });
  const fitBtn = button("Fit", { onClick: () => {
    userMoved = false;
    fit();
    draw();
  } });

  let findMessage = "";
  let filterMessage = "";
  const statusText = h("p", { class: "g-status__text" });
  const statusLine = h("div", { class: "g-status", "aria-live": "polite", hidden: true }, statusText);

  const canvas = h("canvas", {
    class: "g-canvas", tabindex: "0", role: "application",
    "aria-label": `Graph of ${g.nodes.length} entities and ${g.edges.length} links. Arrow keys pan, plus and minus zoom, 0 fits. Enter opens the selected entity, Escape clears it. Pick an entity in the panel to select it.`,
  });
  const tip = h("div", { class: "g-tip", hidden: true, "aria-hidden": "true" });
  const stage = h("div", { class: "g-stage" }, canvas, tip);
  const panel = h("aside", { class: "g-panel", "aria-label": "Brain" });

  put(v,
    h("header", { class: "g-toolbar" },
      h("h1", { class: "page-title" }, "Graph"),
      findForm,
      field({ label: "Type", control: typeSel, inline: true }),
      recent,
      h("div", { class: "g-zoom" }, zoomOut, zoomIn, fitBtn),
    ),
    statusLine,
    h("div", { class: "g-body" }, stage, panel),
  );

  // ---------- state ----------
  const layout = new Layout(nodes, links);
  const view = { x: 0, y: 0, k: 1 };
  let W = 0, H = 0, dpr = 1;
  let hover = null, selected = null, typeFilter = null, recentFilter = null;
  let typeName = "", recentWindow = "";
  let pendingFocus = null;
  let userMoved = false;
  let raf = 0;
  const c2d = canvas.getContext("2d");

  // 2x2 checkerboard: the dither for anything out of focus.
  const tile = document.createElement("canvas");
  tile.width = tile.height = 2;
  const t2 = tile.getContext("2d");
  t2.fillStyle = INK;
  t2.fillRect(0, 0, 1, 1);
  t2.fillRect(1, 1, 1, 1);
  const dither = c2d.createPattern(tile, "repeat");

  function resize() {
    const r = stage.getBoundingClientRect();
    dpr = window.devicePixelRatio || 1;
    W = Math.max(1, r.width);
    H = Math.max(1, r.height);
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    if (!userMoved) fit();
    draw();
  }

  function fit() {
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const n of nodes) {
      x0 = Math.min(x0, n.x);
      y0 = Math.min(y0, n.y);
      x1 = Math.max(x1, n.x);
      y1 = Math.max(y1, n.y);
    }
    const pad = 40;
    const k = Math.min((W - pad * 2) / Math.max(x1 - x0, 1), (H - pad * 2) / Math.max(y1 - y0, 1), 2.5);
    view.k = Math.max(0.05, k);
    view.x = W / 2 - ((x0 + x1) / 2) * view.k;
    view.y = H / 2 - ((y0 + y1) / 2) * view.k;
  }

  const sx = (n) => n.x * view.k + view.x;
  const sy = (n) => n.y * view.k + view.y;

  function focusSet() {
    const f = selected || hover;
    if (!f) return null;
    const s = new Set(f.nb);
    s.add(f);
    return s;
  }

  function draw() {
    const c = c2d;
    c.setTransform(dpr, 0, 0, dpr, 0, 0);
    c.fillStyle = PAPER;
    c.fillRect(0, 0, W, H);
    const fs = focusSet();
    const lit = (n) => (!typeFilter || typeFilter.has(n.name)) && (!recentFilter || recentFilter.has(n.name)) && (!fs || fs.has(n));
    const linkLit = (l) => (fs ? fs.has(l.s) && fs.has(l.t) && (l.s === (selected || hover) || l.t === (selected || hover)) : lit(l.s) && lit(l.t));

    // Links: lit ones in ink, solid for "is" and dotted otherwise; the rest dithered.
    c.lineWidth = 1;
    for (const [style, dash, pick] of [
      [dither, [], (l) => !linkLit(l)],
      [INK, [], (l) => linkLit(l) && l.is],
      [INK, [2, 3], (l) => linkLit(l) && !l.is],
    ]) {
      c.beginPath();
      for (const l of links) {
        if (!pick(l)) continue;
        c.moveTo(sx(l.s), sy(l.s));
        c.lineTo(sx(l.t), sy(l.t));
      }
      c.strokeStyle = style;
      c.setLineDash(dash);
      c.lineWidth = fs && style === INK ? 2 : 1;
      c.stroke();
    }
    c.setLineDash([]);

    // Nodes: squares, a pixel each. Types are hollow.
    const zs = Math.max(0.6, Math.min(1.6, Math.sqrt(view.k)));
    for (const n of nodes) {
      const s = n.size * zs;
      const x = Math.round(sx(n) - s / 2), y = Math.round(sy(n) - s / 2);
      if (x > W + s || y > H + s || x < -2 * s || y < -2 * s) continue;
      const on = lit(n);
      c.fillStyle = on ? INK : dither;
      c.fillRect(x, y, s, s);
      if (n.isType) {
        c.fillStyle = PAPER;
        const b = Math.max(2, s / 4);
        c.fillRect(x + b, y + b, s - 2 * b, s - 2 * b);
      }
      if (n === selected || n === hover) {
        c.strokeStyle = INK;
        c.lineWidth = 2;
        c.strokeRect(x - 4, y - 4, s + 8, s + 8);
      }
    }

    // Labels with a paper halo, so they read over links. Placed in order of
    // facts, most first (the selected entity before everything); a label that
    // would overlap one already placed this frame is left out, except the
    // selected entity's. With something selected or hovered only its focus set
    // is labelled, so a dithered entity never keeps an ink label. A box grows
    // by 4 px on every side before the test, so two labels that only touch
    // never read as one word.
    c.font = "600 14px 'Atkinson Hyperlegible Next', system-ui, sans-serif";
    c.textBaseline = "middle";
    c.lineJoin = "round";
    const all = nodes.length <= 40 || view.k >= 2;
    const showing = new Set(fs || []);
    if (!fs) {
      if (typeFilter || recentFilter) {
        // A filter labels what it lights, never what it dithers.
        for (const n of nodes) if (lit(n)) showing.add(n);
      } else if (all) {
        for (const n of nodes) showing.add(n);
      } else {
        for (const n of nodes) if (landmarks.has(n)) showing.add(n);
      }
    }
    const order = [...showing].sort((a, b) => (a === selected ? -1 : b === selected ? 1 : b.records - a.records));
    const boxes = [];
    for (const n of order) {
      const text = n.name;
      const x = sx(n) + (n.size * zs) / 2 + 5, y = sy(n);
      if (x > W || y < -10 || y > H + 10 || x < -200) continue;
      const pad = 4;
      const box = {
        x: x - pad, y: y - 9 - pad,
        w: c.measureText(text).width + pad * 2, h: 18 + pad * 2,
      };
      const clash = boxes.some((b) => box.x < b.x + b.w && b.x < box.x + box.w && box.y < b.y + b.h && b.y < box.y + box.h);
      if (clash && n !== selected) continue;
      boxes.push(box);
      c.strokeStyle = PAPER;
      c.lineWidth = 4;
      c.strokeText(text, x, y);
      c.fillStyle = INK;
      c.fillText(text, x, y);
    }
  }

  function loop() {
    raf = 0;
    if (layout.hot) {
      layout.tick();
      if (!userMoved) fit();
      raf = requestAnimationFrame(loop);
    } else if (pendingFocus) {
      // The opening ?focus= waits for the layout, so it centres on where the
      // entity settled rather than where it started.
      centerOn(pendingFocus);
      pendingFocus = null;
    }
    draw();
  }
  const kick = () => {
    if (!raf) raf = requestAnimationFrame(loop);
  };

  // ---------- the panel ----------
  const descCache = new Map();
  let latestRecords = null;
  let latestSeen = null;
  let latestError = null;
  let latestLoading = false;
  let latestHost = null;
  let latestTimer = 0;
  let dreamGroups = null;
  let dreamError = null;
  let dreamLoading = false;
  let needsHost = null;

  function describe(name) {
    let p = descCache.get(name);
    if (!p) {
      p = api("describe/" + enc(name));
      descCache.set(name, p);
      p.catch(() => descCache.delete(name));
    }
    return p;
  }

  function stopLatest() {
    if (latestTimer) clearInterval(latestTimer);
    latestTimer = 0;
  }

  function startLatest() {
    stopLatest();
    latestTimer = setInterval(() => {
      if (!document.hidden) loadLatest();
    }, 15000);
  }

  function loadLatest() {
    if (latestLoading) return;
    latestLoading = true;
    api("log", { query: { n: 8 } }).then((res) => {
      latestLoading = false;
      latestError = null;
      // The endpoint answers oldest last; the panel reads newest first.
      latestRecords = res.records.slice().reverse();
      if (!selected) paintLatest();
    }).catch((err) => {
      latestLoading = false;
      if (err.status === 401) return;
      latestError = err;
      if (!selected) paintLatest();
    });
  }

  function paintLatest() {
    if (!latestHost || !latestHost.isConnected) return;
    if (latestError) {
      latestHost.replaceChildren(errorBox(latestError, { retry: () => {
        latestError = null;
        paintLatest();
        loadLatest();
      } }));
      return;
    }
    if (!latestRecords) {
      latestHost.replaceChildren();
      return;
    }
    const byID = new Map(latestRecords.map((r) => [r.uuid, r]));
    const items = latestRecords.map((r) => {
      if (r.relation === "forget") {
        const target = byID.get(r.subject);
        return target
          ? { time: r.time, subject: target.subject, relation: target.relation, object: target.object, forgotten: true }
          // The forgotten fact is older than these 8 records: the Log shows
          // its short uuid in mono; a fact list has no mono, so the first 8
          // characters stand in for the fact.
          : { time: r.time, subject: r.subject.slice(0, 8) + "…", relation: "forget", object: r.object || null, forgotten: true };
      }
      return { time: r.time, subject: r.subject, relation: r.relation, object: r.object };
    });
    const list = factList(items, { names, onSelect: selectByName });
    const fresh = latestSeen
      ? new Set(latestRecords.filter((r) => !latestSeen.has(r.uuid)).map((r) => r.uuid))
      : new Set();
    latestSeen = new Set(latestRecords.map((r) => r.uuid));
    latestRecords.forEach((r, i) => {
      if (fresh.has(r.uuid)) list.children[i]?.classList.add("flash");
    });
    latestHost.replaceChildren(list);
  }

  function loadDream() {
    if (dreamLoading) return;
    dreamLoading = true;
    api("dream").then((res) => {
      dreamLoading = false;
      dreamError = null;
      dreamGroups = res.near_duplicates || [];
      if (!selected) paintNeeds();
    }).catch((err) => {
      dreamLoading = false;
      if (err.status === 401) return;
      dreamError = err;
      if (!selected) paintNeeds();
    });
  }

  function paintNeeds() {
    if (!needsHost || !needsHost.isConnected) return;
    if (dreamError) {
      needsHost.replaceChildren(errorBox(dreamError, { retry: () => {
        dreamError = null;
        dreamGroups = null;
        paintNeeds();
        loadDream();
      } }));
      return;
    }
    if (!dreamGroups || !dreamGroups.length) {
      needsHost.replaceChildren();
      return;
    }
    const groups = dreamGroups.map((group) => {
      const line = [];
      group.forEach((name, i) => {
        if (i) line.push(" ≈ ");
        line.push(button(name, { kind: "quiet", onClick: () => selectByName(name) }));
      });
      return h("span", { class: "g-needs__group" }, line);
    });
    const text = h("span", { class: "g-needs__groups" }, groups.flatMap((g, i) => (i ? [", ", g] : [g])));
    needsHost.replaceChildren(notice({
      lead: count(dreamGroups.length, "near-duplicate name", "near-duplicate names"),
      text,
      action: linkButton("/app/dream", "Dream"),
    }));
  }

  function selectByName(name) {
    const n = byName.get(normalizeName(name));
    if (n) select(n, { center: true });
  }

  function renderGlance() {
    stopLatest();
    const facts = g.nodes.reduce((sum, n) => sum + n.records, 0) - g.edges.length;
    const figures = h("div", { class: "g-figures" },
      figure({ label: "Entities", value: num(g.nodes.length), href: "/app/entities" }),
      figure({ label: "Facts", value: num(facts), href: "/app/log" }),
      figure({ label: "Types", value: num(types.entities.length), href: "/app/entities" }),
      figure({ label: "Links", value: num(g.edges.length) }),
    );
    latestHost = h("div", { class: "g-latest__list" });
    needsHost = h("div", { class: "g-needs" });
    panel.setAttribute("aria-busy", "false");
    panel.replaceChildren(
      figures,
      h("section", { class: "g-latest" },
        h("div", { class: "g-panel__head" },
          h("h2", { class: "g-panel__title" }, "Latest"),
          link("/app/log", "Log")),
        latestHost),
      needsHost,
      h("div", { class: "g-legend" }, legend([
        ["entity", "Entity"],
        ["type", "Type"],
        ["is", "is link"],
        ["link", "other link"],
      ])),
    );
    paintLatest();
    paintNeeds();
    if (latestRecords === null) loadLatest();
    if (dreamGroups === null) loadDream();
    startLatest();
  }

  function renderSelection() {
    stopLatest();
    const n = selected;
    const name = n.name;
    const meta = h("p", { class: "g-panel__meta" },
      h("strong", {}, "Links"), " ", num(n.nb.size));
    const body = h("div", { class: "g-sel-body" });
    panel.setAttribute("aria-busy", "true");
    panel.replaceChildren(
      h("div", { class: "g-sel-head" },
        h("h2", { class: "g-panel__name" }, name),
        typeStamps(n.types),
        meta,
        h("div", { class: "g-panel__actions" },
          linkButton(entityHref(name), "Open", { kind: "primary" }),
          auth.canWrite ? linkButton("/app/add?subject=" + enc(name), "Add a fact") : null,
          button("Clear", { kind: "quiet", onClick: () => {
            select(null);
            canvas.focus();
          } }),
        ),
      ),
      body,
    );

    function show(desc) {
      if (selected !== n) return;
      panel.setAttribute("aria-busy", "false");
      const e = desc.entities && desc.entities[0];
      if (!e) return;
      const out = (e.out || []).slice().sort(entityOrder);
      const incoming = (e.in || []).slice().sort(entityOrder);
      meta.replaceChildren(
        h("strong", {}, "Facts"), " ", num(out.length),
        " · ",
        h("strong", {}, "Links"), " ", num(n.nb.size),
      );
      const sections = [];
      if (out.length) {
        sections.push(h("section", { class: "g-facts" },
          h("h2", { class: "g-panel__title" }, "Facts"),
          factList(out.slice(0, 12).map((r) => ({ relation: r.relation, object: r.object })),
            { names, onSelect: selectByName }),
          out.length > 12
            ? h("p", { class: "g-more" }, link(entityHref(name), "All " + count(out.length, "fact", "facts")))
            : null,
        ));
      }
      if (incoming.length) {
        sections.push(h("section", { class: "g-facts" },
          h("h2", { class: "g-panel__title" }, "Linked from"),
          factList(incoming.slice(0, 8).map((r) => ({ subject: r.subject, relation: r.relation })),
            { names, onSelect: selectByName }),
          incoming.length > 8
            ? h("p", { class: "g-more" }, link(entityHref(name) + "?tab=linked", "All " + num(incoming.length)))
            : null,
        ));
      }
      body.replaceChildren(...sections);
    }

    function fail(err) {
      if (selected !== n) return;
      panel.setAttribute("aria-busy", "false");
      body.replaceChildren(errorBox(err, { retry: () => {
        descCache.delete(name);
        renderSelection();
      } }));
    }

    describe(name).then(show).catch(fail);
  }

  function renderPanel() {
    if (selected) renderSelection();
    else renderGlance();
  }

  // ---------- selection and the address ----------
  function centerOn(n) {
    userMoved = true;
    view.k = Math.max(view.k, 1.4);
    view.x = W / 2 - n.x * view.k;
    view.y = H / 2 - n.y * view.k;
  }

  function select(n, { center = false } = {}) {
    selected = n;
    pendingFocus = null;
    if (n && center) centerOn(n);
    ctx.setQuery({ focus: n ? n.name : null });
    renderPanel();
    draw();
  }

  function say() {
    statusLine.replaceChildren(statusText);
    statusText.hidden = false;
    const parts = [findMessage, filterMessage].filter(Boolean);
    statusText.textContent = parts.join(" ");
    statusLine.hidden = parts.length === 0;
  }

  function statusError(err, retry) {
    statusLine.hidden = false;
    statusText.hidden = true;
    statusLine.replaceChildren(statusText, errorBox(err, { retry }));
  }

  function pickFind() {
    const text = find.value.trim();
    if (!text) {
      findMessage = "";
      say();
      return;
    }
    const q = text.toLowerCase();
    const n = byName.get(normalizeName(text))
      || nodes.find((x) => x.name.startsWith(q))
      || nodes.find((x) => x.name.includes(q));
    if (!n) {
      findMessage = `No entity matches "${text}".`;
      say();
      return;
    }
    findMessage = "";
    say();
    find.value = n.name;
    select(n, { center: true });
  }

  findForm.addEventListener("submit", (e) => {
    e.preventDefault();
    pickFind();
  });
  find.addEventListener("change", pickFind);
  find.addEventListener("input", () => {
    if (!findMessage) return;
    findMessage = "";
    say();
  });

  // Recent names what was written in its window; Type alone names the type.
  // With both on, the number is what they leave lit: the intersection.
  function sayFilter() {
    if (recentFilter) {
      const n = typeFilter ? [...recentFilter].filter((name) => typeFilter.has(name)).length : recentFilter.size;
      const win = recentWindow === "7d" ? "7 d" : "24 h";
      filterMessage = n
        ? `${n} ${n === 1 ? "entity" : "entities"} written in the last ${win}.`
        : `Nothing written in the last ${win}.`;
    } else if (typeFilter) {
      filterMessage = `${typeFilter.size} ${typeFilter.size === 1 ? "entity is" : "entities are"} ${typeName}.`;
    } else {
      filterMessage = "";
    }
    say();
  }

  async function applyType(t) {
    if (!t) {
      typeFilter = null;
      typeName = "";
      sayFilter();
      draw();
      return;
    }
    try {
      typeFilter = new Set((await api("ls", { query: { type: t } })).entities);
      typeName = t;
      sayFilter();
      draw();
    } catch (err) {
      if (err.status === 401) return;
      typeFilter = null;
      typeName = "";
      filterMessage = "";
      say();
      statusError(err, () => {
        filterMessage = "";
        say();
        applyType(t);
      });
    }
  }

  // since is RFC 3339 UTC without milliseconds, as the log endpoint wants it.
  const since = (win) => new Date(Date.now() - (win === "7d" ? 7 : 1) * 24 * 3600 * 1000).toISOString().replace(/\.\d{3}Z$/, "Z");

  async function applyRecent(win) {
    if (!win) {
      recentFilter = null;
      recentWindow = "";
      sayFilter();
      draw();
      return;
    }
    try {
      const res = await api("log", { query: { since: since(win) } });
      // Every fact remembered in the window touches its subject and, when it
      // names one, its object. A forget touches nothing: it is not a write.
      const touched = new Set();
      for (const r of res.records) {
        if (r.relation === "forget") continue;
        touched.add(r.subject);
        const o = normalizeName(r.object);
        if (byName.has(o)) touched.add(o);
      }
      recentFilter = touched;
      recentWindow = win;
      sayFilter();
      draw();
    } catch (err) {
      if (err.status === 401) return;
      recentFilter = null;
      recentWindow = "";
      filterMessage = "";
      say();
      statusError(err, () => {
        filterMessage = "";
        say();
        applyRecent(win);
      });
    }
  }

  typeSel.addEventListener("change", () => {
    ctx.setQuery({ type: typeSel.value });
    applyType(typeSel.value);
  });

  // ---------- zoom and pointer ----------
  function zoomAt(f, px = W / 2, py = H / 2) {
    const k = Math.max(0.05, Math.min(8, view.k * f));
    view.x = px - ((px - view.x) * k) / view.k;
    view.y = py - ((py - view.y) * k) / view.k;
    view.k = k;
    userMoved = true;
    draw();
  }

  const pointers = new Map();
  let drag = null; // { node } or { pan: true }
  let downAt = null;
  let pinch = null;

  function hit(px, py) {
    let best = null, bd = Infinity;
    const zs = Math.max(0.6, Math.min(1.6, Math.sqrt(view.k)));
    for (const n of nodes) {
      const r = (n.size * zs) / 2 + 6;
      const dx = Math.abs(sx(n) - px), dy = Math.abs(sy(n) - py);
      if (dx <= r && dy <= r && dx + dy < bd) {
        best = n;
        bd = dx + dy;
      }
    }
    return best;
  }

  const local = (e) => {
    const r = canvas.getBoundingClientRect();
    return [e.clientX - r.left, e.clientY - r.top];
  };

  function showTip(n, px, py) {
    if (!n) {
      tip.hidden = true;
      return;
    }
    put(tip.replaceChildren() || tip, h("strong", {}, n.name), n.types.length ? h("span", {}, " is " + n.types.join(", ")) : null);
    tip.hidden = false;
    tip.style.left = Math.min(px + 14, W - 220) + "px";
    tip.style.top = Math.max(py - 40, 4) + "px";
  }

  canvas.addEventListener("pointerdown", (e) => {
    canvas.setPointerCapture(e.pointerId);
    const [px, py] = local(e);
    pointers.set(e.pointerId, [px, py]);
    if (pointers.size === 2) {
      const [a, b] = [...pointers.values()];
      pinch = { d: Math.hypot(a[0] - b[0], a[1] - b[1]), k: view.k };
      if (drag && drag.node) drag.node.fx = drag.node.fy = null;
      drag = null;
      return;
    }
    downAt = [px, py, e.pointerType];
    const n = hit(px, py);
    drag = n ? { node: n } : { pan: true, x: view.x, y: view.y };
  });

  canvas.addEventListener("pointermove", (e) => {
    const [px, py] = local(e);
    if (pointers.has(e.pointerId)) pointers.set(e.pointerId, [px, py]);
    if (pinch && pointers.size === 2) {
      const [a, b] = [...pointers.values()];
      const d = Math.hypot(a[0] - b[0], a[1] - b[1]);
      zoomAt((pinch.k * d) / pinch.d / view.k, (a[0] + b[0]) / 2, (a[1] + b[1]) / 2);
      return;
    }
    if (drag && downAt) {
      const moved = Math.hypot(px - downAt[0], py - downAt[1]) > 4;
      if (drag.node && (moved || drag.node.fx != null)) {
        drag.node.fx = (px - view.x) / view.k;
        drag.node.fy = (py - view.y) / view.k;
        layout.target = 0.25;
        if (layout.alpha < 0.25) layout.alpha = 0.25;
        userMoved = true;
        kick();
      } else if (drag.pan && moved) {
        view.x = drag.x + px - downAt[0];
        view.y = drag.y + py - downAt[1];
        userMoved = true;
        draw();
      }
      return;
    }
    if (e.pointerType === "mouse") {
      const n = hit(px, py);
      if (n !== hover) {
        hover = n;
        canvas.style.cursor = n ? "pointer" : "grab";
        draw();
      }
      showTip(n, px, py);
    }
  });

  function release(e) {
    const [px, py] = local(e);
    pointers.delete(e.pointerId);
    if (pointers.size < 2) pinch = null;
    if (!drag || !downAt) return;
    const moved = Math.hypot(px - downAt[0], py - downAt[1]) > 4;
    const d = drag;
    drag = null;
    if (d.node) {
      d.node.fx = d.node.fy = null;
      layout.target = 0;
    }
    if (moved || e.type === "pointercancel") return;
    if (d.node) {
      // Every click or tap selects; a double-click opens (see dblclick).
      select(d.node);
      if (downAt[2] !== "mouse") showTip(d.node, sx(d.node), sy(d.node));
    } else if (selected) {
      select(null);
      showTip(null);
    }
  }
  canvas.addEventListener("pointerup", release);
  canvas.addEventListener("pointercancel", release);
  canvas.addEventListener("dblclick", (e) => {
    const [px, py] = local(e);
    const n = hit(px, py);
    if (n) ctx.navigate(entityHref(n.name));
  });
  canvas.addEventListener("pointerleave", (e) => {
    if (e.pointerType === "mouse" && hover) {
      hover = null;
      showTip(null);
      draw();
    }
  });

  canvas.addEventListener("wheel", (e) => {
    e.preventDefault();
    const [px, py] = local(e);
    zoomAt(Math.exp(-e.deltaY * (e.deltaMode === 1 ? 0.05 : 0.0015)), px, py);
  }, { passive: false });

  canvas.addEventListener("keydown", (e) => {
    const step = 60;
    const pan = { ArrowLeft: [step, 0], ArrowRight: [-step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] }[e.key];
    if (pan) {
      view.x += pan[0];
      view.y += pan[1];
      userMoved = true;
      draw();
    } else if (e.key === "+" || e.key === "=") zoomAt(1.4);
    else if (e.key === "-" || e.key === "_") zoomAt(1 / 1.4);
    else if (e.key === "0") {
      userMoved = false;
      fit();
      draw();
    } else if (e.key === "Enter" && selected) ctx.navigate(entityHref(selected.name));
    else if (e.key === "Escape" && selected) select(null);
    else return;
    e.preventDefault();
  });

  // ---------- the opening state ----------
  applyType(initialType);
  applyRecent(initialRecent);
  if (initialFocus) {
    select(initialFocus);
    // Centring waits for the layout to settle, or happens now under reduced motion.
    pendingFocus = initialFocus;
  } else {
    renderGlance();
  }

  // ---------- run ----------
  const ro = new ResizeObserver(resize);
  ro.observe(stage);
  resize();
  if (reduceMotion) {
    // No settling animation: lay it out first, then draw once.
    for (let i = 0; i < 300 && layout.hot; i++) layout.tick();
    layout.alpha = 0;
    fit();
    if (pendingFocus) {
      centerOn(pendingFocus);
      pendingFocus = null;
    }
    draw();
  } else kick();

  // Fonts may arrive after the first frame; labels need them.
  document.fonts.ready.then(draw);

  ctx.setCleanup(() => {
    cancelAnimationFrame(raf);
    ro.disconnect();
    stopLatest();
  });
}
