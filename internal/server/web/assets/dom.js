// Small DOM helpers. Text always goes in as text nodes, never as HTML:
// every name and object comes from agents and is untrusted.

export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k in el && typeof v !== "string") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const c of children) {
    if (c == null || c === false) continue;
    if (Array.isArray(c)) append(el, c);
    else el.append(c instanceof Node ? c : String(c));
  }
}

// link is an in-app link; the router intercepts clicks on [data-link].
export function link(href, ...children) {
  return h("a", { href, "data-link": true }, ...children);
}

export function entityHref(name) {
  return "/app/e/" + encodeURIComponent(name);
}

export function entityLink(name) {
  return link(entityHref(name), name);
}

// normalizeName mirrors model.NormalizeName (SPEC section 3), to tell
// whether an object names an entity.
export function normalizeName(s) {
  s = s.trim().toLowerCase().split(/\s+/u).filter(Boolean).join("-");
  s = s.replace(/^[\p{P}\p{S}]+|[\p{P}\p{S}]+$/gu, "");
  return s === "i" || s === "me" || s === "myself" ? "self" : s;
}

const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

// time renders an RFC 3339 time as a <time> in the reader's locale.
export function time(t) {
  const d = new Date(t);
  return h("time", { datetime: t, title: t }, isNaN(d) ? t : dateFmt.format(d));
}

// state is a one-line message for loading, empty and error views.
export function state(text, kind) {
  return h("p", { class: "state" + (kind ? " state--" + kind : ""), role: kind === "error" ? "alert" : null }, text);
}

// copyButton copies text() and says so for 1.5 s; without the Clipboard
// API it selects target instead, so the reader can copy by hand.
export function copyButton(label, text, target) {
  const btn = h("button", { type: "button", class: "btn" }, label);
  btn.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(text());
      btn.textContent = "Copied";
      setTimeout(() => (btn.textContent = label), 1500);
    } catch {
      if (target) {
        const range = document.createRange();
        range.selectNodeContents(target);
        const sel = getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
      }
      btn.textContent = "Press Ctrl+C";
      setTimeout(() => (btn.textContent = label), 2500);
    }
  });
  return btn;
}

// dialog opens a modal <dialog> built from body(close) and resolves with
// the value passed to close (undefined on Escape or Cancel). Focus goes
// back where it was.
export function dialog(title, body) {
  return new Promise((resolve) => {
    const back = document.activeElement;
    let value;
    const d = h("dialog", { class: "dialog", "aria-labelledby": "dialog-title" });
    const close = (v) => {
      value = v;
      d.close();
    };
    d.append(h("h2", { id: "dialog-title", class: "dialog__title" }, title), body(close));
    d.addEventListener("close", () => {
      d.remove();
      if (back && back.isConnected) back.focus();
      resolve(value);
    });
    document.body.append(d);
    d.showModal();
  });
}

// errorText is the message of an API error, or a generic network one.
export function errorText(err) {
  return err && err.message ? err.message : "Something went wrong talking to the server.";
}

// put appends children like h does: arrays flattened, null and false skipped.
export function put(el, ...children) {
  append(el, children);
  return el;
}
