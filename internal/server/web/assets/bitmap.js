// The landing page's m (site/assets/bitmap.js), drawn cell by cell over a
// dithered ghost of its earlier shell. Used once, on the sign-in screen.

const NS = "http://www.w3.org/2000/svg";

const stems = (mid) =>
  [".############.", "##############", "##############", ...Array(9).fill(mid)].map((r) => r.padEnd(14, "."));

const place = (rows, dx, dy, w, h) =>
  Array.from({ length: h }, (_, y) => {
    const r = rows[y - dy];
    return Array.from({ length: w }, (_, x) => (r && r[x - dx] === "#" ? "#" : ".")).join("");
  });

const M = place(stems("##....##....##"), 0, 2, 17, 14);
const N = place(stems("##..........##"), 3, 0, 17, 14);

function solid(rows) {
  let d = "";
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; x++) if (row[x] === "#") d += `M${x} ${y}h1v1h-1z`;
  });
  return d;
}

// One in four squares on a quarter-cell grid: the ghost.
function ghost(rows) {
  const sub = 0.25;
  let d = "";
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; x++) {
      if (row[x] !== "#") continue;
      for (let i = 0; i < 4; i += 2) for (let k = 0; k < 4; k += 2) d += `M${(x * 4 + k) * sub} ${(y * 4 + i) * sub}h${sub}v${sub}h-${sub}z`;
    }
  });
  return d;
}

export function mBitmap() {
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", "0 0 17 14");
  svg.setAttribute("shape-rendering", "crispEdges");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("focusable", "false");
  for (const d of [ghost(N), solid(M)]) {
    const p = document.createElementNS(NS, "path");
    p.setAttribute("d", d);
    p.setAttribute("fill", "currentColor");
    svg.append(p);
  }
  return svg;
}
