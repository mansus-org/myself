# myself, design notes

Why the landing page looks the way it does. It is built in the (R)Evolution / Consenso language:
1-bit e-ink, two inks, one typeface. Files: `site/index.html`, `site/assets/style.css`,
`site/assets/bitmap.js` (the m, its shells and the signs), `site/assets/landing.js` (the load flash).
Screenshots at 1440 and 390 wide are in `screenshots/`.

## The idea

The page is one letter shedding a shell. The big m is the sign of the project: three stems are
three nodes, the arches are the edges between them. Behind it stands a dithered ghost of the shell
it grew out of, the m before it had a middle stem. That is molting, the one idea the page keeps
returning to: nothing is overwritten, the old shell stays readable. At the bottom the same pair
swaps places: the old shell is inked, the new m is the ghost, and the layout mirrors as in
(R)Evolution.

Design read: a tool announcement for people who run agents, in the Mansus 1-bit language.
Dials: ENERGY 2 / RHYTHM 3 / MOTION 1.

## Decisions, one line each

- **Palette:** paper `#e4e2db`, ink `#141414` (14.2:1). Grey is only ever dither.
- **Typeface:** Anybody only. Width 50 / weight 900 for display, width 150 / weight 330 for
  statements, width 108 for reading, width 80 for code (no monospace font).
- **Bitmaps:** the m, three shells and seven 7×7 signs are drawn cell by cell and rendered as
  crisp SVG. Dither is real squares on a sub-grid, so it stays 1-bit at any size.
- **Terminal:** a bordered block with an inked label bar and a hard offset shadow. Prompts are
  heavy, comments dotted-underlined, output marked by a bar. No dark theme, no window dots.
- **The seven decisions:** ruled rows, not cards. Each has its own sign and a different title
  length, so the rhythm is a list, not a grid of equals.
- **Molting section:** the sign in three shells, sparse dither, half dither, solid.
- **Commands:** the fourteen commands in two ruled columns, in reading order down each column.
- **Motion:** two moments, both e-ink: the hero painting in row by row, and one black flash on
  load. Both are off under `prefers-reduced-motion`.
- **No JS dependency for content:** all copy is in the HTML; JS only draws bitmaps. Without it
  the `<noscript>` fallback shows a plain m.

## Accessibility

- `<h1>` carries the plain text "myself"; every bitmap and the spine are `aria-hidden`.
- Links are at least 44 px tall, focus ring is 3px ink.
- Checked at 390 and 1440 wide, no horizontal scroll.

## Not done yet

- The tool is not built; the page says so.
- Real Mansus wordmark still to be supplied.
