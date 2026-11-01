// The graph: GET /api/v1/graph drawn on a canvas with a small force layout
// (Barnes-Hut repulsion, springs on links, a pull to the centre). Two inks
// only: what is out of focus is drawn as dither, never as grey.
import { api, auth, graph as fetchGraph } from "./api.js";
import { h, put, entityHref, link, state } from "./dom.js";

const INK = "#141414";
const PAPER = "#e4e2db";
const reduceMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

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
  const loading = state("Loading the graph…");
  put(v, h("h1", { class: "title sr" }, "Graph"), loading);
  const [g, types] = await Promise.all([fetchGraph(), api("ls")]);
  loading.remove();
  if (!g.nodes.length) {
    put(v, h("h1", { class: "title" }, "Graph"),
      state("Nothing to draw yet. Entities appear once an agent remembers facts."),
      auth.canWrite ? h("p", {}, link("/app/add", "Add the first facts")) : null);
    return;
  }

  // Nodes and one link per connected pair; "is" wins when a pair has several.
  const byName = new Map();
  const nodes = g.nodes.map((n) => {
    const node = { ...n, deg: 0, nb: new Set(), isType: false };
    byName.set(n.name, node);
    return node;
  });
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

  // ---------- DOM ----------
  const find = h("input", { id: "g-find", type: "search", list: "g-names", autocomplete: "off", spellcheck: "false" });
  const findForm = h("form", { class: "g-find" },
    h("label", { for: "g-find", class: "label" }, "Find"),
    h("div", { class: "row" }, find, h("button", { type: "submit", class: "btn" }, "Focus")),
    h("datalist", { id: "g-names" }, nodes.map((n) => h("option", { value: n.name }))),
  );
  const typeSel = h("select", { id: "g-type" },
    h("option", { value: "" }, "All entities"),
    types.entities.map((t) => h("option", { value: t }, t)));
  const zoomIn = h("button", { type: "button", class: "btn btn--square", "aria-label": "Zoom in" }, "+");
  const zoomOut = h("button", { type: "button", class: "btn btn--square", "aria-label": "Zoom out" }, "−");
  const fitBtn = h("button", { type: "button", class: "btn" }, "Fit");
  const msg = h("p", { class: "count", "aria-live": "polite" });

  const canvas = h("canvas", {
    class: "g-canvas", tabindex: "0", role: "application",
    "aria-label": `Graph of ${nodes.length} entities and ${links.length} links. Arrow keys pan, plus and minus zoom, 0 fits. Use Find to pick an entity; its links are listed next to the graph.`,
    "aria-describedby": "g-legend",
  });
  const tip = h("div", { class: "g-tip", hidden: true, "aria-hidden": "true" });
  const stage = h("div", { class: "g-stage" }, canvas, tip);
  const panel = h("aside", { class: "g-panel", "aria-label": "Selected entity" });

  put(v, 
    h("div", { class: "g-tools" },
      findForm,
      h("div", { class: "field" }, h("label", { for: "g-type", class: "label" }, "Type"), typeSel),
      h("div", { class: "row g-zoom" }, zoomOut, zoomIn, fitBtn),
    ),
    h("div", { class: "g-body" }, stage, panel),
    h("div", { class: "g-foot" },
      h("p", { class: "count" }, `${nodes.length} entities, ${links.length} links.`),
      h("p", { id: "g-legend", class: "legend" },
        h("span", { class: "sw sw--node", "aria-hidden": "true" }), " entity ",
        h("span", { class: "sw sw--type", "aria-hidden": "true" }), " a type (something is one) ",
        h("span", { class: "sw sw--is", "aria-hidden": "true" }), " is ",
        h("span", { class: "sw sw--rel", "aria-hidden": "true" }), " other link. Hover or tap to see neighbours; click to open."),
      msg,
    ),
  );

  // ---------- state ----------
  const layout = new Layout(nodes, links);
  const view = { x: 0, y: 0, k: 1 };
  let W = 0, H = 0, dpr = 1;
  let hover = null, selected = null, filter = null;
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
    const lit = (n) => (!filter || filter.has(n.name)) && (!fs || fs.has(n));
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

    // Labels with a paper halo, so they read over links.
    c.font = "600 13px 'Atkinson Hyperlegible Next', system-ui, sans-serif";
    c.textBaseline = "middle";
    c.lineJoin = "round";
    const all = nodes.length <= 40 || view.k >= 2;
    for (const n of nodes) {
      const show = fs ? fs.has(n) : all ? lit(n) : landmarks.has(n) && lit(n);
      if (!show && !(filter && filter.size <= 30 && filter.has(n.name))) continue;
      const x = sx(n) + (n.size * zs) / 2 + 5, y = sy(n);
      if (x > W || y < -10 || y > H + 10 || x < -200) continue;
      c.strokeStyle = PAPER;
      c.lineWidth = 4;
      c.strokeText(n.name, x, y);
      c.fillStyle = INK;
      c.fillText(n.name, x, y);
    }
  }

  function loop() {
    raf = 0;
    if (layout.hot) {
      layout.tick();
      if (!userMoved) fit();
      raf = requestAnimationFrame(loop);
    }
    draw();
  }
  const kick = () => {
    if (!raf) raf = requestAnimationFrame(loop);
  };

  // ---------- selection and panel ----------
  function select(n, { center = false } = {}) {
    selected = n;
    if (n && center) {
      userMoved = true;
      view.k = Math.max(view.k, 1.4);
      view.x = W / 2 - n.x * view.k;
      view.y = H / 2 - n.y * view.k;
    }
    renderPanel();
    draw();
  }

  function renderPanel() {
    if (!selected) {
      panel.replaceChildren(h("p", { class: "help" }, "Pick an entity with Find, or tap one, to list its links here."));
      return;
    }
    const n = selected;
    const nb = [...n.nb].sort((a, b) => a.name.localeCompare(b.name));
    put(panel.replaceChildren() || panel,
      h("h2", { class: "g-panel__name" }, n.name),
      n.types.length ? h("p", { class: "types" }, n.types.map((t) => h("span", { class: "stamp stamp--small" }, t))) : null,
      h("p", { class: "meta" }, n.records === 1 ? "1 fact" : `${n.records} facts`),
      h("p", {}, h("a", { href: entityHref(n.name), "data-link": true, class: "btn btn--ink" }, "Open entity page")),
      nb.length ? h("h3", { class: "label" }, nb.length === 1 ? "1 neighbour" : `${nb.length} neighbours`) : h("p", { class: "help" }, "No links: its facts are all free text."),
      nb.length ? h("ul", { class: "g-nb" }, nb.map((m) => h("li", {},
        h("button", { type: "button", class: "linkish", onclick: () => select(m, { center: true }) }, m.name)))) : null,
      h("button", { type: "button", class: "btn btn--small", onclick: () => {
        select(null);
        canvas.focus();
      } }, "Clear selection"),
    );
  }
  renderPanel();

  findForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const q = find.value.trim().toLowerCase();
    if (!q) return;
    const n = byName.get(q) || nodes.find((x) => x.name.startsWith(q)) || nodes.find((x) => x.name.includes(q));
    if (!n) {
      msg.textContent = `No entity matches "${find.value.trim()}".`;
      return;
    }
    msg.textContent = "";
    find.value = n.name;
    select(n, { center: true });
  });

  typeSel.addEventListener("change", async () => {
    const t = typeSel.value;
    if (!t) {
      filter = null;
      msg.textContent = "";
    } else {
      try {
        filter = new Set((await api("ls", { query: { type: t } })).entities);
        msg.textContent = `${filter.size} ${filter.size === 1 ? "entity is" : "entities are"} ${t}; the rest are dithered.`;
      } catch (err) {
        msg.textContent = err.message;
        return;
      }
    }
    draw();
  });

  const zoomAt = (f, px = W / 2, py = H / 2) => {
    const k = Math.max(0.05, Math.min(8, view.k * f));
    view.x = px - ((px - view.x) * k) / view.k;
    view.y = py - ((py - view.y) * k) / view.k;
    view.k = k;
    userMoved = true;
    draw();
  };
  zoomIn.addEventListener("click", () => zoomAt(1.4));
  zoomOut.addEventListener("click", () => zoomAt(1 / 1.4));
  fitBtn.addEventListener("click", () => {
    userMoved = false;
    fit();
    draw();
  });

  // ---------- pointer ----------
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
      // Mouse: click opens. Touch and pen: first tap selects, second opens.
      if (downAt[2] === "mouse" || selected === d.node) ctx.navigate(entityHref(d.node.name));
      else {
        select(d.node);
        showTip(d.node, sx(d.node), sy(d.node));
      }
    } else if (selected) {
      select(null);
      showTip(null);
    }
  }
  canvas.addEventListener("pointerup", release);
  canvas.addEventListener("pointercancel", release);
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
    else return;
    e.preventDefault();
  });

  // ---------- run ----------
  const ro = new ResizeObserver(resize);
  ro.observe(stage);
  resize();
  if (reduceMotion) {
    // No settling animation: lay it out first, then draw once.
    for (let i = 0; i < 300 && layout.hot; i++) layout.tick();
    layout.alpha = 0;
    fit();
    draw();
  } else kick();

  // Fonts may arrive after the first frame; labels need them.
  document.fonts.ready.then(draw);

  ctx.setCleanup(() => {
    cancelAnimationFrame(raf);
    ro.disconnect();
  });
}
