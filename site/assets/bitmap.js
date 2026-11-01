// 1-bit drawing for the page: the m, its ghost shells, the signs and the dither
// that stands in for grey. Cell by cell, rendered as crisp SVG. Exposed as window.Bitmap.
window.Bitmap = (() => {
  const NS = "http://www.w3.org/2000/svg";
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // The sign: an m. Three stems are three nodes, the arches are the edges.
  const stems = (mid) => [
    ".############.",
    "##############",
    "##############",
    ...Array(9).fill(mid),
  ].map((r) => r.padEnd(14, "."));
  const M = stems("##....##....##");
  // An earlier shell: the m before it grew its middle stem.
  const N = stems("##..........##");
  // Place rows on a w x h sheet at (dx, dy), so a shell can sit off-register.
  const place = (rows, dx, dy, w, h) =>
    Array.from({ length: h }, (_, y) => {
      const r = rows[y - dy];
      return Array.from({ length: w }, (_, x) => (r && r[x - dx] === "#" ? "#" : ".")).join("");
    });
  const M_SHEET = place(M, 0, 2, 17, 14);
  const N_SHEET = place(N, 3, 0, 17, 14);

  // Shells in order of age: oldest first.
  const SHELLS = [
    ["#####.", "#...##", "#....#", "#....#", "#....#", "#....#"],
    N,
    M,
  ];

  // Signs on a 7x7 grid, one per idea.
  const GLYPHS = {
    agent: ["..###..", ".#####.", "..###..", "...#...", "..###..", ".#...#.", "#.....#"],
    cli: ["#......", ".#.....", "..#....", ".#.....", "#......", ".......", ".#####."],
    triple: ["##.....", "##.....", "..#....", "...#...", "....#..", ".....##", ".....##"],
    molt: ["#######", "#.....#", "#.###.#", "#.#.#.#", "#.###.#", "#.....#", "#######"],
    dream: ["#.#.#..", ".......", "..#.#.#", ".......", "#.#.#..", "..#....", "....###"],
    file: ["#####..", "#...##.", "#...###", "#.....#", "#.....#", "#.....#", "#######"],
    mirror: ["...#...", "..#.#..", ".#...#.", "#..#..#", ".#...#.", "..#.#..", "...#..."],
    // One brain in the middle, agents on four sides.
    hub: ["#.....#", ".#...#.", "..###..", "..###..", "..###..", ".#...#.", "#.....#"],
  };

  const cellsToPath = (rows) => {
    let d = "";
    rows.forEach((row, y) => {
      for (let x = 0; x < row.length; x++) if (row[x] === "#") d += `M${x} ${y}h1v1h-1z`;
    });
    return d;
  };

  let uid = 0;
  const el = (name, attrs) => {
    const n = document.createElementNS(NS, name);
    for (const k in attrs) n.setAttribute(k, attrs[k]);
    return n;
  };

  // Ordered dither as real squares on a sub-grid, so no SVG pattern seams.
  // "half" is a checkerboard, "ghost" is one in four.
  function ditherPath(rows, sub, mode) {
    const n = Math.round(1 / sub);
    let d = "";
    rows.forEach((row, y) => {
      for (let x = 0; x < row.length; x++) {
        if (row[x] !== "#") continue;
        for (let i = 0; i < n; i++) {
          for (let k = 0; k < n; k++) {
            const gx = x * n + k, gy = y * n + i;
            const on = mode === "ghost" ? gx % 2 === 0 && gy % 2 === 0 : (gx + gy) % 2 === 0;
            if (on) d += `M${gx * sub} ${gy * sub}h${sub}v${sub}h-${sub}z`;
          }
        }
      }
    });
    return d;
  }

  // Ink over an optional ghost. With `sweep`, the ink arrives row by row, the
  // way an e-ink panel repaints: half-tone first, then solid.
  function draw(host, ink, ghost, { sweep = false, sub = 0.25, tone = "" } = {}) {
    const w = Math.max(ink[0].length, ghost ? ghost[0].length : 0);
    const h = Math.max(ink.length, ghost ? ghost.length : 0);
    const svg = el("svg", {
      viewBox: `0 0 ${w} ${h}`, "shape-rendering": "crispEdges", focusable: "false", "aria-hidden": "true",
    });
    if (ghost) svg.append(el("path", { d: ditherPath(ghost, sub, "ghost"), fill: "currentColor" }));
    const inkPath = tone ? ditherPath(ink, sub, tone) : cellsToPath(ink);
    if (sweep && !reduceMotion) {
      const id = ++uid;
      const clip = el("clipPath", { id: `sweep${id}` });
      const bar = el("rect", { x: 0, y: 0, width: w, height: 0 });
      clip.append(bar);
      svg.append(el("defs", {}));
      svg.querySelector("defs").append(clip);
      svg.append(el("path", { d: ditherPath(ink, sub, "half"), fill: "currentColor" }));
      svg.append(el("path", { d: inkPath, fill: "currentColor", "clip-path": `url(#sweep${id})` }));
      let row = 0;
      const tick = () => { bar.setAttribute("height", ++row); if (row < h) setTimeout(tick, 55); };
      setTimeout(tick, 250);
    } else {
      svg.append(el("path", { d: inkPath, fill: "currentColor" }));
    }
    host.replaceChildren(svg);
  }

  //   data-bitmap="front"   the m over a ghost of its earlier shell (data-sweep paints it in)
  //   data-bitmap="back"    the shell facing the other way: the old one inked, the m a ghost
  //   data-shell="0|1|2"    one shell on its own; 0 and 1 are dithered, 2 is solid
  //   data-glyph="agent"    a 7x7 sign
  document.querySelectorAll("[data-bitmap]").forEach((h) => {
    if (h.dataset.bitmap === "front") draw(h, M_SHEET, N_SHEET, { sweep: h.hasAttribute("data-sweep") });
    else draw(h, N_SHEET, M_SHEET);
  });
  document.querySelectorAll("[data-shell]").forEach((h) => {
    const i = +h.dataset.shell;
    draw(h, SHELLS[i], null, { sub: 0.5, tone: i === 0 ? "ghost" : i === 1 ? "half" : "" });
  });
  document.querySelectorAll("[data-glyph]").forEach((h) => draw(h, GLYPHS[h.dataset.glyph], null, { sub: 0.5 }));

  return { reduceMotion };
})();
