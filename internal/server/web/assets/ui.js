// Shared UI components. Text always goes in as text nodes, never as HTML:
// every name and object comes from agents and is untrusted.
import { enc } from "./api.js";
import {
  h, link, entityHref, entityLink, normalizeName, time, relTime, num,
  errorText, copyButton, dialog,
} from "./dom.js";

let uid = 0;
const nextID = (prefix) => `${prefix}-${++uid}`;

function textNode(value) {
  return value instanceof Node ? value : String(value == null ? "" : value);
}

function stampValue(value) {
  return value instanceof Node ? value : stamp(value);
}

export function pageHeader({ crumbs, title, actions, stamps, meta, description }) {
  const head = h("header", { class: "page-head" });
  if (crumbs && crumbs.length) {
    const nav = h("nav", { class: "page-head__crumbs", "aria-label": "Breadcrumb" });
    crumbs.forEach((crumb, i) => {
      if (i) nav.append(" / ");
      nav.append(i === crumbs.length - 1
        ? h("span", { "aria-current": "page" }, crumb.label)
        : link(crumb.href, crumb.label));
    });
    head.append(nav);
  }
  const titleRow = h("div", { class: "page-head__title-row" },
    h("h1", { class: "page-title" }, title),
    actions && actions.length ? h("div", { class: "page-head__actions" }, actions) : null,
  );
  head.append(titleRow);
  if (stamps && stamps.length) head.append(h("div", { class: "page-head__stamps" }, stamps));
  if (meta && meta.length) {
    const line = h("p", { class: "page-head__meta" });
    meta.forEach(([label, value], i) => {
      if (i) line.append(" · ");
      line.append(h("strong", {}, label), " ", textNode(value));
    });
    head.append(line);
  }
  if (description != null && description !== "") {
    head.append(h("p", { class: "page-head__desc" }, textNode(description)));
  }
  return head;
}

export function button(label, { kind = "default", ariaLabel, onClick, type = "button", disabled, reason } = {}) {
  const cls = "btn" + (kind && kind !== "default" ? " btn--" + kind : "");
  const btn = h("button", {
    type,
    class: cls,
    "aria-label": ariaLabel,
    disabled: disabled || null,
    onclick: onClick,
  }, label);
  if (!disabled || !reason) return btn;
  return h("span", { class: "btn-note" }, btn, h("span", { class: "btn-note__reason" }, reason));
}

export function linkButton(href, label, { kind = "default" } = {}) {
  const cls = "btn" + (kind && kind !== "default" ? " btn--" + kind : "");
  return h("a", { class: cls, href, "data-link": true }, label);
}

export function field({ label, control, help, inline }) {
  const id = control && control.id ? control.id : nextID("field");
  if (control && !control.id) control.id = id;
  const helpID = help ? nextID("help") : null;
  if (help) control.setAttribute("aria-describedby", helpID);
  return h("div", { class: "field" + (inline ? " field--inline" : "") },
    h("label", { class: "field__label", for: id }, label),
    control,
    help ? h("p", { class: "field__help", id: helpID }, help) : null,
  );
}

export function checkbox({ id = nextID("check"), label, checked, help }) {
  const helpID = help ? nextID("check-help") : null;
  const input = h("input", {
    type: "checkbox",
    id,
    checked: checked || null,
    "aria-describedby": helpID,
  });
  return h("div", { class: "check-field" },
    h("label", { class: "check", for: id }, input, h("span", {}, label)),
    help ? h("p", { class: "field__help", id: helpID }, help) : null,
  );
}

export function formError(text) {
  return h("p", { class: "form-error", role: "alert" }, text);
}

export function segmented({ label, options, value, onChange }) {
  const group = h("div", { class: "segmented", role: "radiogroup", "aria-label": label });
  let current = value;
  const buttons = options.map((option) =>
    h("button", {
      type: "button",
      class: "segmented__option",
      role: "radio",
      "data-value": option.value,
      onclick: () => choose(option.value, true),
    }, option.label));

  function paint() {
    buttons.forEach((btn) => {
      const on = btn.dataset.value === current;
      btn.setAttribute("aria-checked", String(on));
      btn.tabIndex = on ? 0 : -1;
    });
  }

  function choose(next, notify, focus) {
    if (!options.some((o) => o.value === next)) return;
    const changed = current !== next;
    current = next;
    paint();
    if (focus) buttons.find((b) => b.dataset.value === next)?.focus();
    if (notify && changed && onChange) onChange(next);
  }

  group.append(...buttons);
  group.addEventListener("keydown", (e) => {
    const at = buttons.findIndex((b) => b.dataset.value === current);
    let next = null;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (at + 1) % buttons.length;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (at - 1 + buttons.length) % buttons.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = buttons.length - 1;
    if (next == null) return;
    e.preventDefault();
    choose(options[next].value, true, true);
  });
  Object.defineProperty(group, "value", {
    get: () => current,
    set: (next) => choose(next, false, false),
  });
  paint();
  return group;
}

export function tabs({ label, items, selected, onSelect }) {
  const base = nextID("tab");
  const list = h("div", { class: "tabs", role: "tablist", "aria-label": label });
  const panel = h("div", { class: "tabpanel", role: "tabpanel", tabindex: "0" });
  let current = items.some((item) => item.id === selected) ? selected : items[0]?.id;
  const buttons = items.map((item) =>
    h("button", {
      type: "button",
      class: "tab",
      role: "tab",
      id: `${base}-${item.id}`,
      "data-tab": item.id,
      "aria-controls": `${base}-panel`,
      onclick: () => choose(item.id, true),
    },
    h("span", {}, item.label),
    item.count != null ? h("span", { class: "tab__count" }, num(item.count)) : null));

  function paint() {
    buttons.forEach((btn) => {
      const on = btn.dataset.tab === current;
      btn.setAttribute("aria-selected", String(on));
      btn.tabIndex = on ? 0 : -1;
    });
    panel.setAttribute("aria-labelledby", `${base}-${current}`);
  }

  function choose(next, notify) {
    if (!items.some((item) => item.id === next)) return;
    const changed = current !== next;
    current = next;
    paint();
    if (notify && changed && onSelect) onSelect(next);
  }

  panel.id = `${base}-panel`;
  list.append(...buttons);
  list.addEventListener("keydown", (e) => {
    const at = buttons.findIndex((b) => b.dataset.tab === current);
    let next = null;
    if (e.key === "ArrowRight") next = (at + 1) % buttons.length;
    else if (e.key === "ArrowLeft") next = (at - 1 + buttons.length) % buttons.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = buttons.length - 1;
    if (next == null) return;
    e.preventDefault();
    choose(items[next].id, true);
    buttons[next].focus();
  });
  paint();
  return { list, panel };
}

export function stamp(text) {
  return h("span", { class: "stamp" }, text);
}

export function typeStamp(type) {
  return h("a", {
    class: "stamp stamp--type",
    href: "/app/entities?type=" + enc(type),
    "data-link": true,
  }, type);
}

export function typeStamps(types) {
  return types && types.length ? h("span", { class: "type-stamps" }, types.map(typeStamp)) : null;
}

// object renders a record's object, linked when it names an entity.
export function object(rec, names) {
  const n = normalizeName(rec.object);
  return names && names.has(n) ? link(entityHref(n), rec.object) : rec.object;
}

// table draws a real table. A screen that can be empty passes empty: a Node
// shown as the one row when there are no rows.
export function table({ caption, columns, rows, sort, onSort, empty, limit = 100 }) {
  let shown = Math.min(limit, rows.length);
  const wrapper = h("div", { class: "data-table-wrap" });
  const tableEl = h("table", { class: "data-table" });
  if (caption) tableEl.append(h("caption", { class: "sr" }, caption));
  const headRow = h("tr");
  for (const col of columns) {
    const th = h("th", { scope: "col", class: col.numeric ? "numeric" : null });
    if (col.sortable) {
      th.setAttribute("aria-sort", sort && sort.key === col.key
        ? (sort.dir === "asc" ? "ascending" : "descending")
        : "none");
      th.append(h("button", {
        type: "button",
        class: "data-table__sort",
        onclick: () => onSort && onSort(col.key),
      }, col.label));
    } else th.append(col.label);
    headRow.append(th);
  }
  const tbody = h("tbody");
  tableEl.append(h("thead", {}, headRow), tbody);
  const foot = h("div", { class: "data-table-foot" });
  const countLine = h("p", { class: "data-table__count" });
  const more = button("Show 100 more", { onClick: () => {
    shown = Math.min(shown + 100, rows.length);
    paint();
  } });

  function bodyRow(row) {
    const tr = h("tr", { "data-key": row.key == null ? null : row.key });
    let firstLink = null;
    columns.forEach((col) => {
      const value = row.cells ? row.cells[col.key] : null;
      const content = value instanceof Node ? value : String(value == null ? "" : value);
      if (!firstLink && content instanceof Element) firstLink = content.matches("a") ? content : content.querySelector("a");
      tr.append(h("td", {
        "data-label": col.label,
        class: col.numeric ? "numeric" : null,
      }, content));
    });
    if (firstLink) firstLink.classList.add("row-link");
    return tr;
  }

  function paint() {
    tbody.replaceChildren();
    if (!rows.length) {
      tbody.append(h("tr", { class: "data-table-empty" },
        h("td", { colspan: columns.length }, empty)));
    } else {
      tbody.append(...rows.slice(0, shown).map(bodyRow));
    }
    countLine.textContent = rows.length > limit
      ? `${num(shown)} of ${num(rows.length)}`
      : "";
    countLine.hidden = !(rows.length > limit);
    more.hidden = shown >= rows.length;
    foot.replaceChildren(countLine, more);
  }

  wrapper.append(tableEl, foot);
  paint();
  return wrapper;
}

const FACT_COLUMNS = {
  written: { key: "written", label: "Written" },
  subject: { key: "subject", label: "Subject" },
  relation: { key: "relation", label: "Relation" },
  object: { key: "object", label: "Object" },
  actions: { key: "actions", label: "Actions" },
};

export function factTable(facts, { columns, names, canForget, onForget, forgottenBy }) {
  const keys = (columns || ["written", "subject", "relation", "object"]).map((key) =>
    typeof key === "string" ? FACT_COLUMNS[key] : key).filter(Boolean);
  const wrapper = h("div", { class: "fact-table-wrap" });
  const tableEl = h("table", { class: "data-table fact-table" });
  tableEl.append(h("thead", {}, h("tr", {}, keys.map((col) =>
    h("th", {
      scope: "col",
      class: col.key === "actions" ? "fact-table__actions" : null,
    }, col.key === "actions" ? h("span", { class: "sr" }, col.label) : col.label)))));
  const tbody = h("tbody");
  tableEl.append(tbody);
  wrapper.append(tableEl);

  const rows = new Map();

  function content(col, rec, forget) {
    switch (col.key) {
      case "written": return time(rec.time);
      case "subject": return entityLink(rec.subject);
      case "relation": return h("span", { class: "fact-table__relation" }, rec.relation);
      case "object": {
        const value = object(rec, names);
        return value instanceof Node ? value : String(value);
      }
      case "actions":
        return canForget && !forget
          ? button("Forget", {
            ariaLabel: `Forget: ${rec.subject} ${rec.relation} ${rec.object}`,
            onClick: () => onForget && onForget(rec),
          })
          : "";
      default: return "";
    }
  }

  function makeRow(rec) {
    const forget = forgottenBy && forgottenBy.get(rec.uuid);
    const tr = h("tr", { class: "fact-table__row" + (forget ? " is-forgotten" : ""), "data-uuid": rec.uuid });
    let noteCell = null;
    for (const col of keys) {
      const value = content(col, rec, forget);
      const td = h("td", { "data-label": col.label, class: col.key === "actions" ? "fact-table__actions" : null });
      if (forget && col.key !== "actions") {
        td.append(h("del", { class: "fact-table__del" }, value));
        if (!noteCell && col.key === "object") noteCell = td;
      } else td.append(value);
      tr.append(td);
    }
    if (forget) {
      if (!noteCell) noteCell = tr.querySelector("td:not(.fact-table__actions)") || tr.lastElementChild;
      if (noteCell) {
        noteCell.append(
          " ",
          h("span", { class: "stamp" }, "forgotten"),
          h("span", { class: "fact-table__forgot" },
            "Forgotten ", relTime(forget.time), " · ", forget.object || "no reason"),
        );
      }
    }
    return tr;
  }

  function signature(rec) {
    const forget = forgottenBy && forgottenBy.get(rec.uuid);
    return JSON.stringify([rec.subject, rec.relation, rec.object, rec.time,
      forget ? [forget.time, forget.object] : null]);
  }

  function update(next) {
    const keep = new Set();
    next.forEach((rec, i) => {
      keep.add(rec.uuid);
      let row = rows.get(rec.uuid);
      if (!row || row.sig !== signature(rec)) {
        const tr = makeRow(rec);
        if (row) tbody.replaceChild(tr, row.el);
        row = { el: tr, sig: signature(rec) };
        rows.set(rec.uuid, row);
      }
      const at = tbody.children[i];
      if (at !== row.el) tbody.insertBefore(row.el, at || null);
    });
    for (const [uuid, row] of rows) {
      if (!keep.has(uuid)) {
        row.el.remove();
        rows.delete(uuid);
      }
    }
  }

  update(facts || []);
  return { el: wrapper, update };
}

export function factList(items, { names, onSelect } = {}) {
  const known = names || new Set();
  const click = (name) => onSelect && onSelect(name);
  const nameNode = (name) => {
    const n = normalizeName(name);
    return known.has(n)
      ? h("button", { type: "button", class: "fact-list__entity", onclick: () => click(n) }, name)
      : h("span", {}, name);
  };
  return h("ul", { class: "fact-list" }, items.map((item) => {
    const line = [];
    const add = (node) => {
      if (line.length) line.push(" ");
      line.push(node);
    };
    if (item.time != null) add(h("span", { class: "fact-list__time" }, time(item.time)));
    if (item.subject != null) add(nameNode(item.subject));
    if (item.relation != null) add(h("span", { class: "fact-list__relation" }, item.relation));
    if (item.object != null) add(nameNode(item.object));
    return h("li", {
      class: "fact-list__item" + (item.forgotten ? " is-forgotten" : ""),
    },
    h("span", { class: "fact-list__text" }, line),
    item.forgotten ? h("span", { class: "stamp" }, "forgotten") : null);
  }));
}

export function notice({ stamp: mark, lead, text, action, role } = {}) {
  return h("div", { class: "notice", role: role || null },
    mark != null ? stampValue(mark) : null,
    h("p", { class: "notice__text" }, lead != null ? h("strong", {}, lead) : null,
      lead != null && text != null ? " " : null, text != null ? textNode(text) : null),
    action != null ? h("div", { class: "notice__action" }, action) : null,
  );
}

export function errorBox(err, { retry } = {}) {
  return notice({
    text: errorText(err),
    role: "alert",
    action: retry ? button("Try again", { onClick: retry }) : null,
  });
}

export function figure({ label, value, href }) {
  const fig = h(href ? "a" : "div", {
    class: "figure",
    href: href || null,
    "data-link": href ? true : null,
  },
  h("span", { class: "figure__label" }, label),
  h("span", { class: "figure__value" }, textNode(value)));
  return fig;
}

export function meter({ value, max, text }) {
  const fill = h("span", { class: "meter__fill" });
  const track = h("div", {
    class: "meter__track",
    role: "progressbar",
    "aria-valuemin": "0",
    "aria-valuemax": String(max),
    "aria-valuenow": String(value),
    "aria-label": text || `${value} of ${max}`,
  }, fill);
  const fraction = max > 0 ? Math.max(0, Math.min(1, value / max)) : 0;
  fill.style.width = fraction * 100 + "%";
  return h("div", { class: "meter" }, track, h("span", { class: "meter__text" }, text));
}

export function copyBlock({ label, text, wrap }) {
  const pre = h("pre", { class: "copy-block__body" + (wrap ? " copy-block__body--wrap" : ""), tabindex: "0" }, text);
  return h("div", { class: "copy-block" },
    h("div", { class: "copy-block__head" },
      h("span", { class: "copy-block__label" }, label),
      copyButton("Copy", () => text, pre)),
    pre);
}

export function secretBox({ name, scope, token, onDone }) {
  return h("section", { class: "secret-panel", role: "status", "aria-label": `Token for ${name}` },
    copyBlock({ label: `Token for ${name} · ${scope}`, text: token }),
    h("p", { class: "secret-panel__note" }, "This is the only time it is shown."),
    button("Done", { kind: "primary", onClick: onDone }));
}

export function emptyState(text, action) {
  return h("div", { class: "empty-state" },
    h("p", {}, textNode(text)),
    action != null ? action : null);
}

const SWATCHES = {
  entity: "legend__swatch--entity",
  type: "legend__swatch--type",
  is: "legend__swatch--is",
  link: "legend__swatch--link",
};

export function legend(items) {
  return h("ul", { class: "legend-list" }, items.map(([swatch, label]) =>
    h("li", {},
      h("span", { class: "legend__swatch " + (SWATCHES[swatch] || ""), "aria-hidden": "true" }),
      h("span", {}, label))));
}

export function announce(text) {
  const region = document.getElementById("announce");
  if (!region) return;
  region.textContent = "";
  setTimeout(() => { region.textContent = text; }, 0);
}

export { dialog, copyButton };
