# myself, design notes

Why the landing page looks the way it does. It is built in the (R)Evolution language:
1-bit e-ink, two inks, one display face. Files: `site/index.html`, `site/assets/style.css`,
`site/assets/bitmap.js` (the m, its shells and the signs), `site/assets/landing.js` (the load flash and the Copy buttons),
`site/assets/player.js` (plays `demo.cast`).
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

Copy: the page opens as a product page (memory across sessions, in one file you own) and then works as the
README does: a live example, then install and setup instructions in sections with anchors, linked from an
"On this page" list, each ending with a link to the same section of the README. There is no invented proof:
no numbers, users or speed claims. Every command and output on the page was run against a v0.2.0 binary
(throwaway store), so uuids and times are real. The status is stated once, in Install: v0.2.0 is out.

## Decisions, one line each

- **Palette:** paper `#e4e2db`, ink `#141414` (14.2:1). Grey is only ever dither.
- **Typeface:** Anybody (width 50, weight 900) for display only: section and day headings, the wordmark, the spine.
  Everything people read (statements, body, lists, tables, buttons, nav) is Atkinson Hyperlegible Next, and
  commands and files are Atkinson Hyperlegible Mono, as in the web app. The page became documentation, read and
  copied from, so it took the same display/reading split. Fonts are self-hosted in `site/assets/fonts/` (no CDN).
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
- **Live example:** right after the hero, `demo.cast` plays in a terminal window with Play/Pause and Replay
  (both buttons, so keyboard works). It starts when scrolled into view and never loops. The window holds the
  finished session as plain HTML, which is what shows without JS and under `prefers-reduced-motion` (no autoplay
  there; Play still works). The screen is a fixed height and scrolls, like the terminal it was recorded in.
- **On this page:** a ruled, numbered list of the sections, plus the README and SPEC.md links.
- **Doc sections:** Install, Use with Claude Code, Connect to a shared brain (server, tokens, remote mode, MCP,
  web app with a screenshot), Commands (a table that stacks per command on phones), How it stores memory,
  Obsidian mirror, Molting and dreaming, Non-goals. On wide screens the heading holds a sticky left column and
  the body reads on the right; below 1100 px they stack. Self-hosting examples use `<your-server>`.
- **Install:** after the story. The release line (v0.1.1, install script, `go install`), then three numbered steps for Claude Code (skill, SessionStart hook,
  `Bash(myself:*)`) and one line for other harnesses. Every block has a Copy button (terminals copy commands only): 44 px tall, 3px focus
  ring, "Copied" for 1.5 s, and when the Clipboard API is missing or refuses it selects the text instead.
  Long lines scroll inside the block.
- **Why it works this way:** six ruled rows, each with its own sign, following the README's Why.
  Titles and text vary in length, so the rhythm is a list, not a grid of equals.
- **Molting section:** the sign in three shells, sparse dither, half dither, solid, under the heading.
- **Motion:** the player (once, when scrolled into view), and two moments, both e-ink: the hero painting in row by row, and one black flash on
  load. Both are off under `prefers-reduced-motion`.
- **No JS dependency for content:** all copy is in the HTML, the demo session included; JS draws bitmaps and plays the cast. Without it
  the `<noscript>` fallback shows a plain m.

## Accessibility

- `<h1>` carries the plain text "myself"; every bitmap and the spine are `aria-hidden`.
- Links are at least 44 px tall, focus ring is 3px ink.
- Checked at 360, 390 and 1440 wide, no horizontal scroll; long commands scroll inside their window.
- A skip link goes to the contents list. Section headings run h1, h2, h3 in order.
- The player's state change is announced in a status region; the screen is focusable for keyboard scrolling.

## Not done yet

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
