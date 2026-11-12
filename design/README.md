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

The signed-in app that `myself serve` embeds (`internal/server/web/`: plain ES modules and CSS, no
build step). This section is the design the app is built against. When it and the app disagree, one
of them is a bug: fix the app, or change this section in the same commit and say why. The words are
those of `GLOSSARY.md`. Screenshots: `screenshots/app-*.png`, desktop at 1440 and phone at 390 wide.

Design read: a working tool for people who run agents and want to see what their agents remember,
find what changed and fix what is wrong. Graph first: the graph is the home, and the rest of the app
is one click from it. Dense and calm, in the landing's 1-bit language. Dials: ENERGY 2 / RHYTHM 2 /
MOTION 1.

### History

The first version (BRAIN-160) was drawn at reading size: 17 px text, 44 px controls everywhere, page
titles in Anybody up to 112 px, section heads in Anybody, and screens that explained themselves in
sentences. Entities was the home: one list with no limit, filtered by a row of buttons, one per type.
It read like a poster and stopped working past a few dozen entities.

The redesign (MYSELF-74 onwards) keeps the language and changes the scale and the structure, as
bashreactor's redesign did: a sidebar, the graph as the home with a panel that shows the brain at a
glance, tables that filter, tabs, breadcrumbs, and copy in labels and numbers. Its decisions:

- **Graph first** (owner, 2026-11-12: "the most useful part"). The graph is `/app/`. Selecting an
  entity shows its facts beside the graph instead of leaving it.
- **Connect an agent** (owner, 2026-11-12). Settings connects Claude Code, pi or any MCP client to
  this server in a few copy-paste steps, with a fresh agent token filled in.
- **Look and structure, on today's API** (orchestrator; the owner can overrule it). Nothing changes
  in the REST API, MCP or the server. Every number on screen comes from SPEC section 10's endpoints.
- **Every screen** (orchestrator): sign-in, Graph, Entities, an entity, Search, Log, Add facts,
  Dream, Prime and Settings.

### What does not change: the 1-bit language

- **Two inks.** Paper `#e4e2db` and ink `#141414` (14.2:1). No other colour, no flat grey, no
  transparency used as grey. Grey only appears as dither: real 2 px squares, as a CSS tile or, on
  the graph's canvas, a 2 × 2 checkerboard pattern.
- **Square corners** everywhere. No `border-radius` and no blur. The one shadow is the hard offset
  shadow.
- **Three faces, three jobs.**
  - Anybody (width 50, weight 900) is for the one page title per screen and the graph panel's
    figures, always at 28 px or more. The wordmark keeps its own cut (Anybody 860, width 150%) at
    24 px, the one exception to that floor.
  - Atkinson Hyperlegible Next is for everything people read: facts, entity names, labels,
    buttons, tables, nav.
  - Atkinson Hyperlegible Mono is for what is typed or copied: commands, config, tokens, uuids, the
    prime text and the bulk box.
  - All three are self-hosted in `assets/fonts/`, because the CSP is `'self'` only.
- **Pixel rules.** Rows are separated by a 1 px dotted rule, panels by a 1 px solid rule.
- **Stamps.** A filled ink label marks a state (`forgotten`, `as of`, `cut`, `warning`, a token's
  scope). An outlined label is a type, and links to the entities of that type.
- **Elevation means "answer this now".** Only dialogs and the token shown once get the offset
  shadow.
- **Motion.** The e-ink flash (one frame of ink, then paper) when a row arrives while you watch, and
  the graph settling once when it is first drawn. Nothing loops. Both are off under
  `prefers-reduced-motion`, where the graph is laid out first and drawn once.
- **Off, not grey.** A disabled control keeps ink for its text, gets a dashed border on
  `--tone-faint`, and says why it is off in one line beside it.

### Scale

| | Before | Now |
| --- | --- | --- |
| Interface text (facts, tables, nav, buttons, labels, meta) | 17 px | **14 px**, line height 20 px |
| Prose (empty states, help lines, the one-line descriptions) | 15–17 px | **15 px**, line height 1.5, at most 72ch |
| Code, commands, tokens, prime | 15 px mono | **14 px** mono, line height 1.5 |
| Page title | Anybody, up to 112 px | **Anybody 32 px**, width 50, weight 900, one per screen |
| Section heading | Anybody 28–40 px | **Atkinson Next 16 px, weight 700** |
| Graph labels | 13 px | **14 px**, weight 600 |
| Controls and table rows | 44 px | **32 px** controls, **36 px** rows; **44 px** for both when the pointer is coarse |
| Borders | 2–3 px | **1 px** for panels and rules, **2 px** for controls |

Nothing is ever below 14 px, on the canvas included. WCAG 2.2 AA asks for 24 px targets and 32 px
clears it; touch screens (`@media (pointer: coarse)`) keep the Mansus 44 px. The design target is a
1280–1440 px desktop. Phones (360 and 390 wide) work with no sideways scroll, but nothing is designed
for them first.

### Tokens

Every screen uses these custom properties from `assets/base.css`. A screen stylesheet never
hardcodes a colour, a face, a size from this list or a tone.

```css
:root {
  --paper: #e4e2db;
  --ink: #141414;

  --font-display: "Anybody", system-ui, sans-serif;
  --font-text: "Atkinson Hyperlegible Next", system-ui, sans-serif;
  --font-code: "Atkinson Hyperlegible Mono", ui-monospace, monospace;

  --fs-ui: 14px;      /* line-height: 20px */
  --fs-body: 15px;    /* line-height: 1.5 */
  --fs-h2: 16px;      /* weight 700 */
  --fs-title: 32px;   /* Anybody, font-stretch 50%, weight 900, line-height 1 */
  --fs-figure: 28px;  /* Anybody, font-stretch 50%, weight 900: the graph panel's figures */

  --s1: 4px; --s2: 8px; --s3: 12px; --s4: 16px; --s5: 24px; --s6: 32px; --s7: 48px;

  --control-h: 32px;  /* 44px under (pointer: coarse) */
  --row-h: 36px;      /* 44px under (pointer: coarse) */
  --sidebar-w: 224px;
  --panel-w: 320px;   /* the graph's panel */

  --rule: 1px solid var(--ink);
  --rule-dotted: 1px dotted var(--ink);
  --control-border: 2px solid var(--ink);
  --shadow: 4px 4px 0 var(--ink);
  --shadow-dialog: 6px 6px 0 var(--ink);

  /* Tones: the only greys, as background-image tiles of real squares. */
  --tone-checker: …;  /* 50%: a 2 px checkerboard */
  --tone-sparse: …;   /* 25%: a 2 px square on a 4 px step; the dialog backdrop */
  --tone-faint: …;    /* 6%: a 2 px square on an 8 px step; "off" */
}
```

Breakpoints, the same everywhere:

- **640 px:** below it, tables stack into one block per row.
- **940 px:** below it, the sidebar becomes a top bar with a Menu button.
- **1100 px:** above it, the graph's panel sits beside the canvas (below it, under the canvas), and
  Add facts shows its two forms side by side.

### Glyphs

7×7 bitmaps, drawn cell by cell as crisp SVG at 2 px a cell (14 px), in `assets/bitmap.js` next to
the m. One per nav item:

```
Graph     Entities  Search    Log       Dream     Prime     Settings
##...##   #.#####   .###...   #######   ####...   #######   .#.....
##...##   .......   #...#..   .......   #..#...   #.....#   #######
..#.#..   #.#####   #...#..   #####..   #..####   #.###.#   .#.....
...#...   .......   #...#..   .......   ####..#   #.....#   .......
..#.#..   #.#####   .####..   ######.   ...#..#   #.###.#   ....#..
##...##   .......   .....#.   .......   ...#..#   #.....#   #######
##...##   #.#####   ......#   ####...   ...####   #######   ....#..
```

The graph's legend uses the canvas's own marks as swatches, 14 px: a solid square (entity), a hollow
square with a 2 px ink edge (type), a solid 20 × 2 px line (`is`) and a dotted one (other link).

### The shell

```
┌──────────────┬───────────────────────────────────────────────────────────────────────────┐
│ myself       │ Graph  Find [_____________]  Type [All types ▾]  [All|24 h|7 d]  [−][+][Fit] │
│ [Add facts]  ├────────────────────────────────────────────────────┬──────────────────────┤
│              │                                                    │ storefront           │
│ ⋈ Graph      │                                                    │ [project]            │
│ ≡ Entities   │                    the canvas                      │ Facts 12 · Links 5   │
│ ○ Search     │                                                    │ [Open] [Add a fact]  │
│ ≣ Log        │                                                    │ Facts                │
│ ⧉ Dream      │                                                    │ is project           │
│ ▤ Prime      │                                                    │ uses bun             │
│              │                                                    │ …                    │
│ ⚌ Settings   │                                                    │ Linked from          │
│ admin [admin]│                                                    │ billing-service rel… │
│ Sign out     │                                                    │                      │
└──────────────┴────────────────────────────────────────────────────┴──────────────────────┘
```

- **Sidebar** (940 px and up): `--sidebar-w` wide and the full height of the viewport (sticky), on
  paper, with a 1 px rule on its right and 16 px of padding.
  - **Top:** the wordmark `myself` (class `wordmark`), a link to the Graph.
  - **Add facts,** for write tokens only: a primary button, full width, under the wordmark, linking to
    `/app/add` (with `aria-current="page"` there).
  - **Nav:** Graph, Entities, Search, Log, Dream, Prime. Each item is `--control-h` tall: the 14 px
    glyph, an 8 px gap, then the label in 14 px weight 600. The current item is inverted (ink, paper
    text) and carries `aria-current="page"`; an entity's page marks Entities. Hovering an item draws
    a 3 px ink bar on its left.
  - **Bottom:** Settings (same style), then who is signed in: the token's name and a stamp for its
    scope (`admin`, `write`, `read`), and Sign out, a quiet button.
- **Top bar** (below 940 px): the wordmark on the left and a Menu button on the right
  (`aria-expanded`, `aria-controls`). Menu opens the same items as a panel under the bar. Escape
  closes it and puts focus back on Menu; following a link closes it.
- **Main area:** 24 px by 32 px of padding (16 px below 940 px). The Graph is the exception: it
  fills the main area edge to edge.
- **Announcements:** one visually hidden `role="status"` element, `#announce`, in the shell, used by
  every screen (new facts in the log, a fact forgotten, a token created).
- No footer. The fonts' licence is linked from Settings, Server.

### Components

They live in `assets/ui.js` (the DOM helpers and formatters stay in `assets/dom.js`), styled in
`assets/base.css`. Every screen builds itself out of them. A screen's own stylesheet holds only that
screen's layout: it never restyles a component, and it never makes one that is already here. The Kit
(`/app/kit`) shows every component in each of its states on one page.

- **Page header.** First the breadcrumb, only on a page below another (`Entities / storefront`): 14
  px links separated by ` / `, the last item the current page and not a link. Then the title
  (Anybody `--fs-title`, one per screen) with the page's actions right-aligned on the same row; they
  wrap under it when there is no room. Then an optional row of stamps. Then an optional meta line:
  14 px, `Label value` pairs separated by ` · `, each label weight 700. Then, on Dream and Prime only,
  a one-line description in 15 px. A 1 px rule closes the header, 16 px below it.
- **Button.** `--control-h` tall, 0 12 px of padding, `--control-border`, 14 px weight 700, on paper.
  Hover and press invert it.
  - Primary: ink with paper text; its hover inverts back. At most one per header and one per form.
  - Quiet: no border, underlined text, for Sign out, Clear and Clear filters.
  - Square: `--control-h` by `--control-h`, for − and +, always with an `aria-label`.
  - A link that looks like a button is an `<a>`.
  - Disabled: off, not grey, with its reason in one 14 px line beside it.
- **Field.** The label above in 14 px weight 700; in a toolbar or filter bar, the label sits inline,
  left of its control, on the same row. Help below in 14 px.
  - Input and select: `--control-h` tall, `--control-border`, 0 8 px of padding, 14 px. A select
    draws a pixel caret.
  - Textarea: the same border, 8 px of padding, mono 14 px.
  - Checkbox and radio, drawn 1-bit: a 16 px square box with a 2 px edge, an 8 px ink square inside
    when on, on a hit area of `--control-h`.
  - Form error: 14 px weight 700 in a 2 px ink box, `role="alert"`, under the form or field.
- **Segmented control.** A radio group drawn as joined buttons that share their borders; the chosen
  one is ink. Arrow keys move inside it. Used for Recent, the Log's Show, the agent and a token's
  scope.
- **Tabs.** A `tablist` on a 1 px ink rule. Each tab is 36 px tall (44 px coarse), 0 12 px of
  padding, 14 px weight 700, with an optional count after the label in weight 400 (`Facts 12`). The
  selected tab has a 3 px ink bar along its bottom edge and `aria-selected="true"`. Left, Right, Home
  and End move between tabs. The selected tab lives in the URL (`?tab=`), so a reload and the back
  button keep it.
- **Stamp.** 22 px tall, 0 6 px of padding, 14 px weight 700. A state stamp is ink. A type stamp is
  outlined in 1 px on paper and is an `<a>` to `/app/entities?type=<type>`; hover inverts it.
- **Table.** The workhorse, a real `<table>`.
  - Header cells: 14 px weight 700 on a 1 px ink rule.
  - Body rows: at least `--row-h` tall, 0 12 px of padding (none on the outer edges), a 1 px dotted
    rule between rows. Long text wraps and the row grows; nothing is cut.
  - Numbers are right-aligned with `tabular-nums`.
  - A row that opens something has one real link in its first cell, stretched over the whole row
    (`position: relative` on the row, an `::after` inset of 0 on the link). Hovering the row draws a
    3 px ink bar on its left; keyboard focus shows the focus ring on the link.
  - A sortable column's header is a button with `aria-sort`.
  - A long table shows 100 rows, then a "Show 100 more" button, and its count says `100 of 3,000`.
  - An empty table is one row: a sentence and at most one action.
  - Below 640 px every row becomes a block: the first cell on top, then `label value` lines read from
    each cell's `data-label`.
- **Fact table.** The one way a table of facts is shown: an entity's tabs, Search, the Log and Add
  facts' result. Each screen takes its columns from this set:
  - Subject: an entity link, weight 700.
  - Relation: weight 700.
  - Object: an entity link when the object names an entity, otherwise the text, wrapping.
  - Written: relative time, the exact time in `title`.
  - Actions: Forget, for write tokens, never under As of.

  A forgotten fact (History, the Log) is struck through, followed by a `forgotten` stamp and a second
  14 px line: `Forgotten 3 d ago · moved off go`, or `· no reason`. Rows are keyed by uuid and updated
  in place, so a refresh never drops focus or the scroll position.
- **Fact list.** The compact form, for the graph's panel: one fact per line, 14 px, a 1 px dotted
  rule between lines. A line is `relation object` under an entity, `subject relation` under Linked
  from, and `time subject relation object` in Latest; the relation is weight 700. Entity names in it
  are buttons that select that entity in the graph.
- **Notice.** A 1 px ink box, 8 px by 12 px of padding, 14 px text whose first words are weight 700,
  with an optional stamp before and an action after. For As of, warnings, "Needs you", and errors
  that leave the screen usable. It has no shadow: nothing here needs an answer now.
- **Error box.** For an error that replaces a screen's content: a notice with `role="alert"`, the
  server's message, and Try again where trying again can help.
- **Figure.** A label (14 px weight 700) over a number (Anybody `--fs-figure`); a link when it leads
  somewhere.
- **Meter.** 160 × 10 px with a 1 px ink frame, filled with solid ink up to its fraction, with the
  numbers in 14 px beside it.
- **Copy block.** For anything the reader copies.
  - A 1 px ink frame.
  - A 32 px header bar on paper with a 1 px rule under it: the block's label on the left (a file path
    like `~/.claude/settings.json`, or `Terminal`), Copy on the right.
  - The body in mono 14 px, `white-space: pre`, scrolling sideways inside the block when a line is
    long (Prime's text wraps instead).
  - Copy writes the text, says "Copied" for 1.5 s, and without the Clipboard API selects the text and
    says "Press Ctrl+C".
- **Secret box.** The token shown once: a copy block with `--control-border` and `--shadow`,
  labelled `Token for <name> · <scope>`, with one line under it, "This is the only time it is shown.",
  and Done, which closes it.
- **Dialog.** A native `<dialog>`, at most 560 px wide, with `--control-border`, `--shadow-dialog`
  and 20 px of padding. The title is 16 px weight 700. One help line at most. The actions are
  right-aligned, the primary last. The backdrop is `--tone-sparse`, never a translucent colour.
  Focus returns to the opener.
- **Empty state.** One 15 px sentence and at most one action. No illustrations.
- **Legend.** A row of swatch and word pairs in 14 px.
- **Formatters,** in `dom.js`, one of each, used everywhere:
  - **Relative time:** `just now` under 10 s, then `45 s ago`, `3 min ago`, `2 h ago`, `4 d ago`,
    and a date (`5 Nov`, or `5 Nov 2025` in another year) beyond 7 days.
  - **Exact time:** `Intl.DateTimeFormat` in the reader's locale with day, short month, year, hour,
    minute and second, e.g. `5 Nov 2026, 21:12:58`. It is the `title` of every relative time, and
    every time is a `<time datetime>`.
  - **Counts:** `Intl.NumberFormat`, with the noun spelled out: `1 fact`, `2 facts`.

### Routes

Everything is under `/app`. A screen keeps the state worth sharing in the query string and rewrites
it with `history.replaceState` as it changes.

| Path | Screen |
| --- | --- |
| `/app/` | Graph. `?focus=<entity>&type=<type>&recent=24h\|7d` |
| `/app/entities` | Entities. `?q=<text>&type=<type>&sort=name\|facts` |
| `/app/entities/<name>` | One entity. `?tab=facts\|linked\|history&depth=1..4&at=<RFC 3339>` |
| `/app/search` | Search. `?q=<text>` |
| `/app/log` | Log. `?kind=all\|remembered\|forgotten&n=50\|200\|1000` |
| `/app/add` | Add facts. `?subject=<name>` fills in the subject |
| `/app/dream` | Dream |
| `/app/prime` | Prime. `?budget=<characters>` |
| `/app/settings` | Settings. `?tab=connect\|tokens\|server&agent=claude-code\|pi\|mcp` |
| `/app/kit` | The components, not in the nav |

`<name>` is URL-encoded and may hold a `/` (`/app/entities/ci%2Fcd`).

The old addresses redirect with `replaceState`, so links and bookmarks keep working: `/app/graph`
goes to `/app/` with its query, `/app/e/<name>` to `/app/entities/<name>` with its query, and
`/app/tokens` to `/app/settings?tab=tokens`. `/app/?type=<type>`, where the old type stamps pointed,
is now the Graph filtered by that type. A login link (`#login=`) lands on `/app/`.

### Screens

#### Graph (`/app/`)

The home: what the brain holds, how it is connected and what changed.

- **Layout.** A toolbar, then the canvas and the panel. Above 1100 px the canvas fills the main
  area's width minus `--panel-w` and its height from under the toolbar to the bottom of the
  viewport, and the panel (`--panel-w`, a 1 px rule on its left) scrolls on its own. Nothing pads
  the canvas: it meets the sidebar's rule, the toolbar's rule and the panel's rule. Below 1100 px
  the panel goes under the canvas, and the canvas is `max(320px, 60dvh)` tall.
- **Toolbar,** one row where it fits (it wraps below 1100 px), 12 px by 24 px of padding, closed by a
  1 px rule:
  - the title "Graph";
  - **Find:** an inline label and a search input whose `list` is a datalist of every entity name.
    Enter, or picking a name, selects that entity and centres the view on it. When nothing matches,
    a 14 px line under the toolbar says `No entity matches "<text>".` until the next change.
  - **Type:** an inline label and a select: `All types`, then every type (`GET /ls`). Everything
    outside the type, followed transitively (`GET /ls?type=<type>`), is dithered.
  - **Recent:** a segmented control, `All`, `24 h`, `7 d`. Everything that no fact remembered in the
    window touches (as its subject, or as its object when that names an entity) is dithered. Type
    and Recent combine: lit means inside both.
  - The line under the toolbar also counts what a filter leaves lit: `7 entities are project.`
    with Type alone; `5 entities written in the last 24 h.` with Recent, alone or with Type (then
    the count is of entities inside both); `Nothing written in the last 24 h.` when nothing is.
  - at the right, zoom out, zoom in and Fit: two square buttons labelled "Zoom out" and "Zoom in",
    and Fit.
- **Canvas.** Today's force layout (Barnes-Hut repulsion, springs, a pull to the centre), with these
  changes:
  - A click or a tap selects an entity: the panel shows it and `?focus=` holds it. A double-click,
    or Enter on the canvas, opens its page. A click on empty canvas, or Escape, clears the selection.
  - Hovering with a mouse inks the entity and its neighbours, dithers the rest, and shows a tooltip:
    the name in weight 700 and its types.
  - Entities are squares sized by their facts (5 + 1.6 × √facts px, at most 16); types are hollow;
    `is` links are solid and other links dotted. The selected entity gets a 2 px ring 4 px out.
    While something is selected or hovered, it and its neighbours stay ink and the rest is dithered.
  - **Labels:** 14 px weight 600 Atkinson Next with a 4 px paper halo, right of the square. While an
    entity is selected or hovered, only it and its neighbours get labels, so a dithered square never
    carries ink that makes it look lit. Otherwise, with a Type or Recent filter on, the lit entities
    get labels; with none, every entity does when there are at most 40 or the view is zoomed in
    (k ≥ 2), and the 12 with the most facts do when there are more. Labels are placed in order of
    facts, most first (the selected entity first of all), and a label that would overlap one already
    placed is left out, except the selected entity's.
  - Opening with `?focus=` selects that entity and centres on it once the layout has settled.
  - **Keyboard:** arrows pan, + and − zoom, 0 fits (as today); Enter opens the selected entity and
    Escape clears it. The canvas's accessible name gives the number of entities and links and these
    keys. The panel makes the graph walkable: every entity in it is a button that selects it.
  - **Empty brain:** no canvas. The empty state "Nothing remembered yet.", then Add facts (write
    tokens) and Connect an agent (a link to Settings).
- **Panel, nothing selected:** the brain at a glance.
  - **Figures,** 2 × 2: Entities (links to Entities), Facts (links to the Log), Types (links to
    Entities) and Links. Facts counts live facts: the sum of the nodes' `records` minus the number
    of edges, since a link is counted at both its ends.
  - **Latest:** a 16 px weight 700 heading with a `Log` link at its right, then the 8 newest facts
    and forgets (`GET /log?n=8`) as a fact list, each line led by its relative time. A forget shows a
    `forgotten` stamp and the fact struck through. Read again every 15 s while the tab is visible;
    new lines flash once.
  - **Needs you,** only when `GET /dream` returns groups: a notice, `2 near-duplicate names`, the
    groups (`relay ≈ relays`, each name a button that selects it), and a `Dream` link.
  - The legend at the bottom: Entity, Type, `is` link, other link.
- **Panel, an entity selected:**
  - Its name (16 px weight 700, wrapping anywhere), its type stamps, and the meta line
    `Facts 12 · Links 5` (links are its neighbours).
  - Open (primary, its page), Add a fact (write tokens, `/app/add?subject=<name>`) and Clear (quiet).
  - **Facts:** its live facts (`GET /describe/<name>`) as a fact list, in the entity page's order, at
    most 12, then a link `All 30 facts` to its page.
  - **Linked from:** the facts of other entities that link to it, `subject relation`, at most 8, then
    `All 9` linking to its page's Linked from tab.
  - Entity names in both lists are buttons that select that entity and centre the view on it.
- **Data:**
  - `GET /graph` and `GET /ls`, then `GET /ls?type=<type>` while a type is chosen;
  - `GET /log?since=<now minus 24 h or 7 d>` while Recent is set;
  - `GET /log?n=8` and `GET /dream` for the panel with nothing selected;
  - `GET /describe/<name>` per entity selected, kept while the page lives.

  The graph is not redrawn on a timer, so nothing moves under the reader.

#### Entities (`/app/entities`)

- **Header:** the title "Entities".
- **Filter bar:**
  - Name: a search input with an inline label; it keeps names that contain the text.
  - Type: a select, as on the Graph.
  - At the right, the count: `43 entities`, or `5 of 43 entities`.
  - With any filter set, a quiet Clear filters.
- **Table:** Name (the link, weight 700), Types (type stamps) and Facts (a number). Sortable by Name
  (the default, A to Z) and by Facts (most first), kept in `?sort=`. 100 rows, then Show 100 more.
- **Data:** `GET /graph` (names, types, facts) and `GET /ls`, then `GET /ls?type=<type>` while a type
  is chosen.
- **Empty:** with an empty brain, "Nothing remembered yet." with Connect an agent; when nothing
  matches, "No entity matches these filters." with Clear filters.

#### One entity (`/app/entities/<name>`)

- **Header:**
  - Breadcrumb `Entities / <name>`; the title is the name, wrapping anywhere.
  - Actions: Show in graph (`/app/?focus=<name>`) and Add a fact (primary, write tokens,
    `/app/add?subject=<name>`).
  - Stamps: its types.
  - Meta: `Facts 12 · Linked from 3 · First written 5 Oct · Last written 3 min ago`. The two written
    times come from its history, and are left out when it has none.
- **View bar,** under the header:
  - Depth, on the Facts tab only: a select of `This entity`, `1 hop`, `2 hops`, `3 hops` and `4 hops`.
  - As of: a `datetime-local` input and Go, then Now once it is set.
  - Under As of, a notice holds an `as of` stamp, the time (`5 Nov 2026, 14:00`) and Back to now.
    Forget is hidden, and so is the History tab, since history ignores time.
- **Tabs** (`?tab=`, default `facts`):
  - **Facts:** a fact table (Relation, Object, Written, actions) of its live facts, ordered by
    relation (is, has, uses, does, prefers, related), then oldest first. With a depth, every entity
    within that many hops follows under a 16 px heading (its name as a link, then `1 hop` or
    `2 hops`) with its own fact table.
  - **Linked from:** a fact table (Subject, Relation, Written, actions) of the facts of other
    entities whose object is this one.
  - **History:** a fact table (Written, Relation, Object) of every fact ever written with this
    subject, newest first, the forgotten ones struck through with when and why.
- **Forget,** from a row: the dialog "Forget this fact?", with the fact in a 1 px frame
  (`subject relation object`), Reason (optional, a text input), the help line "It moves to History,
  struck through.", then Cancel and Forget (primary). Afterwards the row leaves the table and
  `#announce` says `Forgot: relay uses go`.
- **Not found:** the title is the name, then "No entity called <name>." with Search for "<name>".
  Under As of, before it existed: "<name> did not exist yet at that time." with Back to now.
- **Data:** `GET /describe/<name>?depth=&at=`; `GET /history/<name>`, not under As of (a 404 means it
  is never a subject: the History tab then says "<name> only appears as the object of other
  facts."); `GET /graph` for names and types.

#### Search (`/app/search`)

- **Header:** the title "Search".
- **Search bar:** an input with the inline label Text, and Search (primary). The text is in `?q=`.
- **Results:**
  - **Entities,** when any name contains the text: their links in a row, at most 20, then `All 31`
    (to `/app/entities?q=<text>`).
  - **Facts:** a 16 px heading with the count, then a fact table (Subject, Relation, Object, Written)
    of `GET /search?q=<text>`: live facts whose subject, relation or object contains the text, in any
    case.
- **Empty:** `No fact contains "<text>".` With no text, only the bar, focused.

#### Log (`/app/log`)

- **Header:** the title "Log". At the right, Pause (`aria-pressed`), which says Resume while paused.
- **Filter bar:** Show, a segmented control (All, Remembered, Forgotten; `?kind=`); Last, a select
  (50, 200, 1000; `?n=`, default 200); and at the right the state, `Live · updated 12:04:31` or
  `Paused`.
- **Table:** Written, Subject, Relation, Object; newest first. A forget's row shows the fact it
  forgot as the fact table draws a forgotten fact: struck through, with the `forgotten` stamp. Its
  Written is when it was forgotten, so its second line holds only the reason (`moved off go`, or
  `no reason`). A forget whose fact is older than the rows read shows that fact's uuid in mono in
  place of the fact (the first 8 characters, all of it in `title`).
- **Refresh:** the same read every 10 s while the tab is visible and not paused. New rows flash once
  and `#announce` says `3 new facts`.
- **Empty:** "Nothing written yet." With a filter: "No forgotten facts in the last 200."
- **Data:** `GET /log?n=`, and `GET /graph` to know which objects are entities.

#### Add facts (`/app/add`)

For write tokens. A read token gets the empty state "This token can only read." and nothing else.

- **Header:** the title "Add facts".
- **Two forms,** side by side above 1100 px, each under a 16 px heading:
  - **One fact:** Subject (filled in from `?subject=`), Relation (a select of is, has, uses, does,
    prefers, related), Object, Replace (a checkbox; help "Forgets the live facts with this subject
    and relation first."), and Remember (primary).
  - **Many at once:** One fact per line (a textarea, mono, 8 rows; help "subject relation object, as
    myself remember - reads them. All lines are saved, or none."), Replace, for every line, and
    Remember all (primary).
- **Result,** under the forms, `role="status"`: a notice `Remembered 3 facts` (with ` · replaced 1`
  when replacing forgot facts), a fact table of what was written (Subject, Relation, Object), and
  one notice per warning, led by a `warning` stamp. An error is the form error; the bulk box's
  errors name the line (`line 2: …`), as today.
- **Data:** `GET /relations`, `POST /remember`.

#### Dream (`/app/dream`)

- **Header:** the title "Dream", and the description "Names that are probably the same entity: equal
  once dashes and a trailing s are removed."
- **Table:** one row per group: Names (each name a link followed by its fact count:
  `relay 7 ≈ relays 1`) and Show in graph (`/app/?focus=<first name>`).
- One help line under the table: "To merge, remember the facts under one name, then forget them
  under the others."
- **Empty:** "No near-duplicate names."
- **Data:** `GET /dream`, and `GET /graph` for the counts.

#### Prime (`/app/prime`)

- **Header:** the title "Prime", and the description "What an agent reads at the start of a session
  (myself prime).", with `myself prime` in mono.
- **Bar:** Budget (a number input with an inline label and `characters` after it, applied with Enter
  or Show), the meter (`1,820 of 2,000 characters`), and a `cut` stamp when the brief was cut.
- **The brief:** a copy block labelled `myself prime --budget 2000`, mono, wrapping.
- **Empty:** "Nothing to prime yet."
- **Data:** `GET /prime?budget=`.

#### Settings (`/app/settings`)

- **Header:** the title "Settings".
- **Tabs** (`?tab=`): Connect an agent (the default), Tokens (admin only; other tokens never see it)
  and Server.

**Connect an agent**

- **Agent:** a segmented control of Claude Code, pi and Other MCP client (`?agent=`, default
  `claude-code`).
- **Token:**
  - The admin token gets Name (prefilled `claude-code`, `pi` or `mcp-client` from the agent), Scope
    (a segmented control, write or read, default write) and Create token (primary). The answer is
    shown in the secret box, and the steps below fill in the new token.
  - Every token, the admin's too, gets "Or paste a token", a password input that only fills in the
    steps; it is never sent anywhere or stored.
  - Until there is a token, the steps say `<token>`. The token the app is signed in with is never put
    in the steps: the admin token must not be handed to an agent, and a login session ends in 12 h.
- **Steps:** a numbered list. Each step is a 14 px weight 700 line, then a copy block. `<origin>` is
  this server's `location.origin`.
  - **Claude Code:**
    1. Install myself. Terminal:
       `curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh`
    2. Install the skill. Terminal:
       `mkdir -p ~/.claude/skills/myself && curl -fsSL <origin>/skill.md -o ~/.claude/skills/myself/SKILL.md`
    3. Install the hooks. Terminal:
       `mkdir -p ~/.myself/hooks && for h in session-start stop-nudge; do curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/contrib/hooks/$h.sh -o ~/.myself/hooks/$h.sh; done && chmod +x ~/.myself/hooks/*.sh`
    4. Merge into ~/.claude/settings.json. The block below, labelled `~/.claude/settings.json`.
    5. Check. Terminal: `MYSELF_URL=<origin> MYSELF_TOKEN=<token> myself whoami`

    Then the line "Or connect over MCP, with nothing to install:" and a Terminal block:
    `claude mcp add --transport http --scope user myself <origin>/mcp --header "Authorization: Bearer <token>"`

    ```json
    {
      "env": {
        "MYSELF_URL": "<origin>",
        "MYSELF_TOKEN": "<token>"
      },
      "permissions": {
        "allow": ["Bash(myself:*)"]
      },
      "hooks": {
        "SessionStart": [
          { "hooks": [{ "type": "command", "command": "~/.myself/hooks/session-start.sh", "timeout": 10 }] }
        ],
        "Stop": [
          { "hooks": [{ "type": "command", "command": "~/.myself/hooks/stop-nudge.sh", "timeout": 10 }] }
        ]
      }
    }
    ```
  - **pi:**
    1. Install myself. Terminal: the install command above.
    2. Install the pi package. Terminal: `pi install git:github.com/mansus-org/myself`
    3. Point pi at this server. A block labelled `~/.profile` holding
       `export MYSELF_URL=<origin>` and `export MYSELF_TOKEN=<token>` on two lines.
    4. Check, in a new shell. Terminal: `myself whoami`
  - **Other MCP client:**
    1. Add this server to its MCP config. A block labelled `MCP config`:
       `{"mcpServers":{"myself":{"type":"http","url":"<origin>/mcp","headers":{"Authorization":"Bearer <token>"}}}}`
    2. Give it the skill. A block labelled `Prompt`: `Read <origin>/skill.md and follow it.`

**Tokens** (admin)

- New token (primary) at the right of the tab, then the secret box once a token is created, then the
  table: Name, Scope (a stamp), Created, Last used (`never` when null) and a Revoke button per row.
- New token opens a dialog: Name, Scope (a segmented control, write or read) and Create token
  (primary). `POST /tokens`.
- Revoke opens the dialog "Revoke <name>?" with the help line "Agents using it get 401 from their
  next call.", then Cancel and Revoke. `DELETE /tokens/<id>`.
- `#announce` says `Created token laptop` and `Revoked laptop`.

**Server**

- A table of label and value: Signed in as (the name, a scope stamp, and Sign out), API
  (`<origin>/api/v1`), MCP (`<origin>/mcp`) and Skill (`<origin>/skill.md`), each value in mono with
  a Copy button, and Fonts (`Fonts and their licence`, linking to `/app/assets/fonts/OFL.txt`).

#### Sign-in

No sidebar. One column at most 420 px wide, 15vh from the top:

- the m over the ghost of its earlier shell (the landing's), 112 px wide: the one image in the app;
- the title "myself";
- one line: "Sign in with the admin token or an agent token.";
- Token, a password input;
- Remember on this device, a checkbox, with the help line "Unchecked, it is forgotten when you
  close this tab.";
- Sign in (primary).

Every message the sign-in shows today about a refused or revoked token, or a used or expired login
link, stays word for word.

#### Not found

The title "Not here", then "This page doesn't exist, or your token can't open it." with a link to
the Graph.

#### Kit (`/app/kit`)

Every component of this section in each of its states, on one page, built from `ui.js` with made-up
data. It is not in the nav. It is where a component is reviewed before screens use it.

### Copy

- The app speaks in labels, numbers and states. A sentence appears only in an empty state, an
  error, beside a disabled control to say why it is off, as one help line under a field or in a
  dialog, in the one-line descriptions of Dream and Prime, and in the steps of Connect an agent.
- Everything is in sentence case: "Add a fact", "Show in graph".
- The words are the glossary's: fact, entity, type, relation, link, linked from, forget, forgotten,
  history, log, as of, prime, dream, near-duplicate, token, scope. Never record, node, edge, triple,
  predicate or delete.
- The app never explains how it is built: no "append-only", "JSONL", "uuid order" or "endpoint".
- An error shows the server's message as it is: it is the CLI's message.

### Accessibility

- A 3 px ink focus ring, offset 2 px, on everything focusable. A skip link to the main area.
- Keyboard everywhere: tabs, segmented controls, the menu, dialogs (native, with focus returned to
  the opener), tables, and the graph (the canvas's keys and the panel's buttons).
- Targets: 32 px with a mouse, 44 px when the pointer is coarse.
- Tables are real tables; their stacked phone layout comes from `data-label`, not a second layout.
- Changes are announced in the one status region, `#announce`. The log's table is not a live region.
- No sideways scroll at 360, 390, 1280 and 1440 wide. Long text wraps; long commands scroll inside
  their copy block.
- The canvas is `role="application"` with a name and a description, and every entity it draws can
  also be reached from its panel, Entities and Search.

### Files

| File | What it holds |
| --- | --- |
| `index.html` | the shell's markup; links every stylesheet |
| `assets/app.js` | boot, the sign-in hand-off, the router and its redirects, the shell (nav, menu, who) |
| `assets/api.js` | the REST client, the token, the shared graph read |
| `assets/dom.js` | `h`, `put`, links, `normalizeName`, the formatters |
| `assets/ui.js` | the components |
| `assets/bitmap.js` | the m and the glyphs |
| `assets/base.css` | fonts, tokens, element defaults and every component |
| `assets/shell.css` | the sidebar, the top bar, the menu and the main area |
| `assets/screens/<screen>.js`, `.css` | one pair per screen: `graph`, `entities`, `entity`, `search`, `log`, `add`, `dream`, `prime`, `settings`, `signin`, `kit` |

A screen module exports one render function. Its stylesheet starts every selector with the screen's
root class (`.s-graph …`), so no screen can restyle another.

### The camera

`design/camera/screens.ts` (usage in its header) photographs every screen at 1440 and 390 wide
against a throwaway server holding the brain of `design/camera/seed.ts`. It reports page errors,
failed requests and sideways scroll, plus what a machine can check of this section: text under
14 px, Anybody under 28 px, colours other than the two inks, rounded corners, blur, transparency,
code not in mono, and controls under 32 px (44 px on the phone). It exits 1 on any finding. A finding
this section allows is fixed in the camera, in a commit of its own that says why.

### Deliberately not done

- No API change. What the API does not give directly (the number of live facts, what changed
  recently) is worked out from what it gives.
- No dark mode. An inverted 1-bit theme stays possible, since nothing hardcodes the two inks.
- No editing a fact in place (forget it and remember the new one), no bulk forget, and no merge tool
  for near-duplicates.
- No time slider on the graph, though the API takes `?at=`.
- No export or import in the app; the API has both.
