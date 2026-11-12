// The Kit: every shared component in each state, with made-up data.
import { h, link, entityHref, relTime, exactTime, num, count, time } from "../dom.js";
import {
  pageHeader, button, linkButton, field, checkbox, formError, segmented, tabs,
  stamp, typeStamp, typeStamps, table, factTable, factList, notice, errorBox,
  figure, meter, copyBlock, secretBox, emptyState, legend, announce, dialog,
} from "../ui.js";
import { glyph } from "../bitmap.js";

const MIN = 60 * 1000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

function group(title, ...children) {
  return h("section", { class: "kit-group" },
    h("h2", { class: "kit-group__title" }, title),
    ...children);
}

function sampleRows(names) {
  return names.map((name, i) => ({
    key: name,
    cells: {
      name: link(entityHref(name), name),
      types: typeStamps(i % 3 === 0 ? ["project", "software"] : ["project"]),
      facts: num(2 + ((i * 17) % 180)),
    },
  }));
}

export function kitPage(ctx) {
  const view = document.getElementById("view");
  const now = Date.now();
  const at = (ms) => new Date(ms).toISOString();

  const page = pageHeader({
    crumbs: [{ label: "Entities", href: "/app/entities" }, { label: "storefront" }],
    title: "Kit",
    actions: [
      button("Quiet action", { kind: "quiet", onClick: () => announce("Quiet action pressed.") }),
      button("Add a fact", { kind: "primary", onClick: () => announce("Primary action pressed.") }),
    ],
    stamps: [stamp("as of"), typeStamp("project")],
    meta: [["Facts", "12"], ["Links", "5"], ["Last written", time(at(now - 3 * MIN))]],
    description: "Every component, in each state, with made-up data.",
  });

  const names = Array.from({ length: 230 }, (_, i) => "entity-" + String(i + 1).padStart(3, "0"));
  const rows = sampleRows(names);
  let sort = { key: "name", dir: "asc" };
  const sortedRows = () => rows.slice().sort((a, b) => {
    const av = a.cells[sort.key] instanceof Node ? a.key : Number(a.cells[sort.key].replace(/,/g, ""));
    const bv = b.cells[sort.key] instanceof Node ? b.key : Number(b.cells[sort.key].replace(/,/g, ""));
    const order = typeof av === "string" ? av.localeCompare(bv) : av - bv;
    return sort.dir === "asc" ? order : -order;
  });
  const tableHost = h("div", { class: "kit-table" });
  const drawTable = () => {
    tableHost.replaceChildren(table({
      caption: "230 made-up entities",
      columns: [
        { key: "name", label: "Name", sortable: true },
        { key: "types", label: "Types" },
        { key: "facts", label: "Facts", numeric: true, sortable: true },
      ],
      rows: sortedRows(),
      sort,
      onSort: (key) => {
        sort = { key, dir: sort.key === key && sort.dir === "asc" ? "desc" : "asc" };
        drawTable();
      },
      empty: emptyState("No entity matches these filters.", button("Clear filters", { kind: "quiet" })),
    }));
  };
  drawTable();

  const emptyTable = table({
    caption: "An empty table",
    columns: [{ key: "name", label: "Name" }, { key: "facts", label: "Facts", numeric: true }],
    rows: [],
    empty: emptyState("No entity matches these filters.", button("Clear filters", { kind: "quiet" })),
  });

  const factNames = new Set(["storefront", "bun", "relay", "project", "billing-service", "web-ui"]);
  let liveFacts = [
    { uuid: "fact-1", subject: "storefront", relation: "uses", object: "bun", time: at(now - 3 * MIN) },
    { uuid: "fact-2", subject: "relay", relation: "is", object: "project", time: at(now - 2 * HOUR) },
    { uuid: "fact-3", subject: "storefront", relation: "has", object: "a checkout that needs offline support", time: at(now - 4 * DAY) },
    { uuid: "fact-4", subject: "storefront", relation: "does", object: "weekly releases on Thursdays", time: at(now - 30 * MIN) },
  ];
  const forgottenBy = new Map([["fact-4", {
    uuid: "forget-1", subject: "fact-4", relation: "forget", object: "moved off go",
    time: at(now - 2 * DAY),
  }]]);
  const facts = factTable(liveFacts, {
    columns: ["subject", "relation", "object", "written", "actions"],
    names: factNames,
    canForget: true,
    forgottenBy,
    onForget: (rec) => {
      announce(`Forgot: ${rec.subject} ${rec.relation} ${rec.object}`);
      liveFacts = liveFacts.filter((r) => r.uuid !== rec.uuid);
      facts.update(liveFacts);
    },
  });

  const choose = (name) => announce(`Selected ${name}.`);
  const underEntity = factList([
    { relation: "uses", object: "bun" },
    { relation: "is", object: "project" },
    { relation: "has", object: "a checkout that needs offline support" },
  ], { names: factNames, onSelect: choose });
  const linkedFrom = factList([
    { subject: "billing-service", relation: "related" },
    { subject: "web-ui", relation: "related" },
  ], { names: factNames, onSelect: choose });
  const latest = factList([
    { time: at(now - 3 * MIN), subject: "storefront", relation: "uses", object: "bun" },
    { time: at(now - 2 * HOUR), subject: "relay", relation: "is", object: "project" },
    { time: at(now - 4 * DAY), subject: "storefront", relation: "has", object: "offline checkout", forgotten: true },
  ], { names: factNames, onSelect: choose });

  const segmentValue = h("p", { class: "kit-value" }, "Chosen: 24 h");
  const recent = segmented({
    label: "Recent",
    options: [{ value: "all", label: "All" }, { value: "24h", label: "24 h" }, { value: "7d", label: "7 d" }],
    value: "24h",
    onChange: (value) => {
      segmentValue.textContent = "Chosen: " + ({ all: "All", "24h": "24 h", "7d": "7 d" })[value];
    },
  });

  const tabPanelContent = (id) => h("p", { class: "kit-value" },
    id === "facts" ? "Facts panel." : id === "linked" ? "Linked from panel." : "History panel.");
  let tabSet;
  tabSet = tabs({
    label: "Entity sections",
    items: [
      { id: "facts", label: "Facts", count: 12 },
      { id: "linked", label: "Linked from", count: 3 },
      { id: "history", label: "History", count: 18 },
    ],
    selected: "facts",
    onSelect: (id) => tabSet.panel.replaceChildren(tabPanelContent(id)),
  });
  tabSet.panel.append(tabPanelContent("facts"));

  const copyText = "myself remember relay uses redis for timelines\nmyself describe relay";
  const profileText = "export MYSELF_URL=https://brain.example.com\nexport MYSELF_TOKEN=mys_one_very_long_token_that_wraps_instead_of_scrolling_sideways";

  const secretHost = h("div", { class: "kit-secret" });
  secretHost.append(secretBox({
    name: "claude-code",
    scope: "write",
    token: "mys_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    onDone: () => {
      announce("Token saved.");
      secretHost.replaceChildren(notice({ text: "Token saved." }));
    },
  }));

  const openDialog = button("Open dialog", {
    kind: "primary",
    onClick: async () => {
      const value = await dialog("Forget this fact?", (close) => h("div", { class: "stack" },
        h("p", { class: "dialog__fact" },
          h("strong", {}, "storefront"), " ", h("span", { class: "rel" }, "uses"), " bun"),
        h("p", { class: "help" }, "It moves to History, struck through."),
        h("div", { class: "dialog__actions" },
          button("Cancel", { onClick: () => close(false) }),
          button("Forget", { kind: "primary", onClick: () => close(true) }))));
      if (value === true) announce("Forgot: storefront uses bun");
    },
  });

  const samples = [
    ["just now", now - 5 * 1000],
    ["45 s", now - 45 * 1000],
    ["3 min", now - 3 * MIN],
    ["2 h", now - 2 * HOUR],
    ["4 d", now - 4 * DAY],
    ["9 d", now - 9 * DAY],
    ["a past year", (() => {
      const d = new Date(now);
      d.setFullYear(d.getFullYear() - 1);
      return d.getTime();
    })()],
  ];
  const formatterRows = samples.map(([label, ms]) =>
    h("li", {},
      h("span", { class: "kit-formatters__name" }, label),
      time(new Date(ms).toISOString()),
      h("span", { class: "kit-formatters__exact" }, exactTime(new Date(ms).toISOString()))));

  view.replaceChildren(
    page,
    group("Buttons",
      h("div", { class: "kit-grid" },
        button("Default", { onClick: () => announce("Default pressed.") }),
        button("Primary", { kind: "primary", onClick: () => announce("Primary pressed.") }),
        button("Quiet", { kind: "quiet", onClick: () => announce("Quiet pressed.") }),
        button("+", { kind: "square", ariaLabel: "Zoom in" }),
        button("−", { kind: "square", ariaLabel: "Zoom out" }),
        button("Off", { disabled: true, reason: "There is nothing to add yet." }),
        linkButton("/app/entities", "Link as a button"),
        linkButton("/app/entities?type=project", "Primary link", { kind: "primary" }),
      )),
    group("Fields",
      h("div", { class: "kit-columns" },
        field({
          label: "Name",
          control: h("input", { id: "kit-name", value: "", autocomplete: "off" }),
          help: "The label above ties to the input by its id.",
        }),
        field({
          label: "Type",
          inline: true,
          control: h("select", { id: "kit-type" },
            h("option", { value: "" }, "All types"),
            h("option", { value: "project" }, "project"),
            h("option", { value: "tool" }, "tool")),
          help: "Inline puts the label left of the control.",
        }),
        checkbox({ id: "kit-check", label: "Remember on this device", help: "Unchecked, it is forgotten when this tab closes." }),
        formError("A field is wrong, and this is what to do about it.")),
      ),
    group("Segmented control",
      recent,
      segmentValue),
    group("Tabs",
      tabSet.list,
      tabSet.panel),
    group("Stamps",
      h("div", { class: "kit-grid" },
        stamp("forgotten"),
        stamp("as of"),
        typeStamp("project"),
        typeStamps(["project", "library", "tool"]))),
    group("Table",
      tableHost,
      h("p", { class: "kit-caption" }, "An empty table:"),
      emptyTable),
    group("Fact table", facts.el),
    group("Fact list",
      h("div", { class: "kit-columns" },
        h("div", {}, h("p", { class: "kit-caption" }, "Under an entity: relation object"), underEntity),
        h("div", {}, h("p", { class: "kit-caption" }, "Linked from: subject relation"), linkedFrom),
        h("div", {}, h("p", { class: "kit-caption" }, "Latest: time subject relation object"), latest))),
    group("Notices",
      h("div", { class: "kit-stack" },
        notice({
          stamp: "as of",
          lead: "As of.",
          text: "Facts written later are hidden.",
          action: button("Back to now", { kind: "quiet", onClick: () => announce("Back to now pressed.") }),
        }),
        notice({
          lead: "Needs you.",
          text: "2 near-duplicate names in the brain.",
          action: linkButton("/app/dream", "Dream"),
        }),
        notice({ text: "A notice with no stamp, no lead and no action." }))),
    group("Error box",
      errorBox(new Error("The server answered 500."), { retry: () => announce("Trying again.") })),
    group("Figures",
      h("div", { class: "kit-figures" },
        figure({ label: "Entities", value: "43", href: "/app/entities" }),
        figure({ label: "Facts", value: "128", href: "/app/log" }),
        figure({ label: "Types", value: "9", href: "/app/entities" }),
        figure({ label: "Links", value: "76" }))),
    group("Meter",
      meter({ value: 1820, max: 2000, text: `${num(1820)} of ${num(2000)} characters` })),
    group("Copy blocks",
      h("div", { class: "kit-stack" },
        copyBlock({ label: "Terminal", text: copyText }),
        copyBlock({ label: "~/.profile", text: profileText, wrap: true }))),
    group("Secret box", secretHost),
    group("Dialog", openDialog),
    group("Empty state",
      emptyState("Nothing remembered yet.", linkButton("/app/add", "Add facts", { kind: "primary" }))),
    group("Legend",
      legend([["entity", "Entity"], ["type", "Type"], ["is", "is link"], ["link", "other link"]])),
    group("Glyphs",
      h("div", { class: "kit-glyphs" },
        ["graph", "entities", "search", "log", "dream", "prime", "settings"].map((name) =>
          h("span", { class: "kit-glyph" }, glyph(name), h("span", {}, name))))),
    group("Formatters",
      h("ul", { class: "kit-formatters" }, formatterRows),
      h("p", { class: "kit-value" },
        count(1, "fact", "facts"), " · ", count(2, "fact", "facts"), " · ", num(3000)),
    ),
  );
}
