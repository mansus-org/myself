# myself, design notes

Why the landing page looks the way it does. It is built in the (R)Evolution language:
1-bit e-ink, two inks, one typeface. Files: `site/index.html`, `site/assets/style.css`,
`site/assets/bitmap.js` (the m, its shells and the signs), `site/assets/landing.js` (the load flash and the Copy buttons).
Screenshots at 1440 and 390 wide are in `screenshots/`.

## The idea

The page is one letter shedding a shell. The big m is the sign of the project: three stems are
three nodes, the arches are the edges between them. Behind it stands a dithered ghost of the shell
it grew out of, the m before it had a middle stem. That is molting, the one idea the page keeps
returning to: nothing is overwritten, the old shell stays readable. At the bottom the same pair
swaps places: the old shell is inked, the new m is the ghost, and the layout mirrors as in
(R)Evolution.

Design read: a tool announcement for people who run agents, in the Mansus 1-bit language.
Dials: ENERGY 3 / RHYTHM 3 / MOTION 2.

Copy: the page reads as a product page. It says what the agent and you get (memory across sessions, in
one file you own), and each section heading names a benefit. There is no invented proof: no numbers,
users or speed claims. The status is stated once, in the Install section: v0.1.1 is out.

## Decisions, one line each

- **Palette:** paper `#e4e2db`, ink `#141414` (14.2:1). Grey is only ever dither.
- **Typeface:** Anybody only. Width 50 / weight 900 for display, width 150 / weight 330 for
  statements, width 108 for reading, width 80 for code (no monospace font).
- **Bitmaps:** the m, three shells and seven 7×7 signs are drawn cell by cell and rendered as
  crisp SVG. Dither is real squares on a sub-grid, so it stays 1-bit at any size.
- **Terminal windows:** kept by owner decision (overrides antislop R-05); myself is a CLI and the terminal is the product.
  A window is a 3px ink frame with an ink title bar (`bash`, plus the Copy button) over a paper body, with a small
  offset shadow for the elevation of "this is where you type". No traffic-light dots, no dark theme. Copy takes only
  the lines after `$`, never output or comments.
- **Files are not terminals:** the `~/.claude/settings.json` snippets use a lighter panel: 2px frame, paper title bar with the
  path and a dotted rule, no shadow. Ink bar means "you type this", paper bar means "you paste this into a file".
- **Cross-session example:** the hero example is a story in three windows, staggered down the page (stacked on mobile):
  Monday the agent saves facts, Thursday a fresh session starts from `myself prime`, later the user asks with
  `describe`, `history` and `path`. All three show the same facts (storefront, bun, npm, alice), so the value is visible: nothing is re-explained.
- **Install:** after the first example. The release line (v0.1.1, install script, `go install`), then three numbered steps for Claude Code (skill, SessionStart hook,
  `Bash(myself:*)`) and one line for other harnesses. Every block has a Copy button (terminals copy commands only): 44 px tall, 3px focus
  ring, "Copied" for 1.5 s, and when the Clipboard API is missing or refuses it selects the text instead.
  Long lines scroll inside the block.
- **Connect your agents:** after Install, in the same window language: the server command, then three numbered ways in (the CLI in remote mode, MCP with `claude mcp add`, the web app at `/app`). URLs are `myself.example.com` placeholders and the token is `<token>` or `mys_...`, never a real one.
- **What you get:** the eight ruled rows (the third, "One brain for all your agents", has the hub sign: one node, four spokes), each with its own sign, written from the user's side.
  Titles and text vary in length, so the rhythm is a list, not a grid of equals.
- **Molting section:** the sign in three shells, sparse dither, half dither, solid.
- **Commands:** the fifteen user-facing commands (not `merge-driver` or `version`) in two ruled columns, in reading order down each column.
- **Motion:** two moments, both e-ink: the hero painting in row by row, and one black flash on
  load. Both are off under `prefers-reduced-motion`.
- **No JS dependency for content:** all copy is in the HTML; JS only draws bitmaps. Without it
  the `<noscript>` fallback shows a plain m.

## Accessibility

- `<h1>` carries the plain text "myself"; every bitmap and the spine are `aria-hidden`.
- Links are at least 44 px tall, focus ring is 3px ink.
- Checked at 390 and 1440 wide, no horizontal scroll.

## Not done yet

- Connect uses placeholder URLs; there is no public shared brain to point at.
- Real Mansus wordmark still to be supplied.

## Web app (/app)

The signed-in app that `myself serve` embeds (`internal/server/web/`). Screenshots: `screenshots/app-*.png`, desktop at
1440 and phone at 390 wide.

Design read: a working tool for people who run agents and want to see and fix what the agents remember, in the same
1-bit language as the landing, calmer. Dials: ENERGY 2 / RHYTHM 2 / MOTION 1.

- **Palette:** the landing's paper and ink only. Anything out of focus (graph nodes outside the type filter or away
  from the hovered entity, the backdrop behind a dialog) is dithered, never grey.
- **Type:** Anybody (width 50, weight 900) only for page titles, entity names at the top of their page and section
  heads, all 28 px and up. Everything people read (facts, lists, forms, buttons, nav, meta) is Atkinson Hyperlegible
  Next; prime text, tokens and the bulk box are Atkinson Hyperlegible Mono. The landing is a statement read once; the
  app is read for an hour, so reading text gets the legible face (the revolution split).
- **Fonts are self-hosted** in `assets/fonts/` (Latin woff2, OFL), so the CSP can stay `'self'` only.
- **Rows, not cards:** entities, facts, history and the log are ruled lists with the landing's square-pixel dotted
  rule. A fact is `relation object`, as `describe` prints it, so the CLI and the app read the same.
- **Stamps:** a filled ink label marks a state (token scope, "new" in the log, "As of", warnings); an outlined one is
  a type and links to the entities of that type.
- **Sign-in:** the landing's m over the ghost of its earlier shell is the one large image in the app, on the one
  screen that has nothing else to show.
- **Graph:** entities are square pixels sized by their number of facts; types are hollow squares. `is` links are
  solid, other links dotted. Hover (or a first tap on touch) inks the entity and its neighbours and dithers the
  rest; a click (or second tap) opens the entity. The side panel lists the selected entity's neighbours as buttons,
  so the graph can be walked with the keyboard after Find.
- **Elevation:** only things that need an action now get the landing's offset shadow: dialogs and the new-token box.
- **Motion:** the graph settles live (it shows the layout working; off under reduced motion, where it is laid out
  first and drawn once), and a record that arrives in the log flashes once, ink then paper, as an e-ink panel
  repaints. Nothing loops.
- **Nav:** one row on wide screens, behind a labelled Menu button below 940 px. Tokens shows only for the admin token
  and Add facts only for write tokens, so no link leads to a page the token can't use.
