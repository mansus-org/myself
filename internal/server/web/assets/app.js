// The myself web app: sign-in, chrome and a small history router over /app.
import { api, auth } from "./api.js";
import { h, put, state, errorText } from "./dom.js";
import { mBitmap } from "./bitmap.js";
import { entitiesPage, entityPage, searchPage, logPage, primePage, dreamPage } from "./pages.js";
import { addPage } from "./write.js";
import { tokensPage } from "./tokens.js";
import { graphPage } from "./graph.js";

const view = document.getElementById("view");
const nav = document.getElementById("nav");
const menuBtn = document.querySelector(".menu-btn");
const who = document.querySelector(".who");

// Pages in nav order. show decides whether the token may see the page.
const PAGES = [
  { path: "/app/", label: "Entities", render: entitiesPage },
  { path: "/app/graph", label: "Graph", render: graphPage, wide: true },
  { path: "/app/search", label: "Search", render: searchPage },
  { path: "/app/log", label: "Log", render: logPage },
  { path: "/app/add", label: "Add facts", render: addPage, show: () => auth.canWrite },
  { path: "/app/dream", label: "Dream", render: dreamPage },
  { path: "/app/prime", label: "Prime", render: primePage },
  { path: "/app/tokens", label: "Tokens", render: tokensPage, show: () => auth.isAdmin },
];

let cleanup = null;

function route(pathname) {
  const m = pathname.match(/^\/app\/e\/(.+)$/);
  if (m) {
    let name;
    try {
      name = decodeURIComponent(m[1]);
    } catch {
      name = m[1];
    }
    return { render: (ctx) => entityPage(ctx, name), wide: false, title: name };
  }
  const p = pathname === "/app" ? "/app/" : pathname.replace(/\/+$/, "") || "/app/";
  const page = PAGES.find((pg) => pg.path === p || pg.path === p + "/");
  if (!page || (page.show && !page.show())) return null;
  return { ...page, title: page.label };
}

export function navigate(url, { replace = false } = {}) {
  if (replace) history.replaceState(null, "", url);
  else history.pushState(null, "", url);
  render();
}

async function render() {
  if (cleanup) {
    cleanup();
    cleanup = null;
  }
  closeMenu();
  if (!auth.identity) return;
  const r = route(location.pathname);
  markNav();
  document.body.classList.toggle("is-wide", !!(r && r.wide));
  view.replaceChildren();
  if (!r) {
    document.title = "Not found · myself";
    put(view, h("h1", { class: "title" }, "Not here"), state("This page doesn't exist, or your token can't open it."));
    return;
  }
  document.title = r.title + " · myself";
  const ctx = { query: new URLSearchParams(location.search), navigate, setCleanup: (fn) => (cleanup = fn) };
  try {
    await r.render(ctx);
  } catch (err) {
    if (err.status === 401) return;
    view.replaceChildren(h("h1", { class: "title" }, r.title), state(errorText(err), "error"));
  }
}

function buildChrome() {
  const id = auth.identity;
  nav.replaceChildren(
    h("ul", { class: "nav__list" },
      PAGES.filter((p) => !p.show || p.show()).map((p) =>
        h("li", {}, h("a", { href: p.path, "data-link": true, class: "nav__link" }, p.label)))),
  );
  put(who.querySelector(".who__name").replaceChildren() || who.querySelector(".who__name"),
    h("span", { class: "who__label" }, "Signed in as "),
    h("strong", {}, id.name),
    " ",
    id.admin ? null : h("span", { class: "stamp stamp--small", title: "Token scope" }, id.scope),
  );
  nav.hidden = false;
  who.hidden = false;
  menuBtn.hidden = false;
}

function markNav() {
  const p = location.pathname.replace(/\/+$/, "") || "/app";
  for (const a of nav.querySelectorAll("a")) {
    const ap = a.getAttribute("href").replace(/\/+$/, "");
    const on = ap === p || (ap === "/app" && p.startsWith("/app/e/"));
    if (on) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
}

function closeMenu() {
  menuBtn.setAttribute("aria-expanded", "false");
  document.body.classList.remove("menu-open");
}

menuBtn.addEventListener("click", () => {
  const open = menuBtn.getAttribute("aria-expanded") !== "true";
  menuBtn.setAttribute("aria-expanded", String(open));
  document.body.classList.toggle("menu-open", open);
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && document.body.classList.contains("menu-open")) {
    closeMenu();
    menuBtn.focus();
  }
});

// In-app links: plain clicks only, so Ctrl/Cmd-click still opens a tab.
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-link]");
  if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  e.preventDefault();
  if (a.getAttribute("href") === location.pathname + location.search) {
    closeMenu();
    return;
  }
  navigate(a.getAttribute("href"));
  view.focus({ preventScroll: true });
  window.scrollTo(0, 0);
});
window.addEventListener("popstate", render);

document.querySelector(".signout").addEventListener("click", () => {
  auth.clear();
  signIn();
});

auth.onUnauthorized = () => {
  auth.clear();
  signIn("The server refused this token: it is invalid or was revoked. Paste another one.");
};

function signedOutChrome() {
  if (cleanup) {
    cleanup();
    cleanup = null;
  }
  nav.hidden = true;
  who.hidden = true;
  menuBtn.hidden = true;
  closeMenu();
  document.body.classList.remove("is-wide");
}

function signIn(message) {
  signedOutChrome();
  document.title = "Sign in · myself";
  const input = h("input", {
    id: "token", name: "token", type: "password", required: true, autocomplete: "off",
    spellcheck: "false", autocapitalize: "off", "aria-describedby": "token-help",
  });
  const remember = h("input", { id: "remember", type: "checkbox" });
  const err = h("p", { class: "form-error", role: "alert", hidden: !message }, message || "");
  const submit = h("button", { type: "submit", class: "btn btn--ink" }, "Sign in");
  const form = h("form", { class: "signin__form", novalidate: true },
    h("label", { for: "token", class: "label" }, "Token"),
    input,
    h("p", { id: "token-help", class: "help" },
      "The admin token, or an agent token that starts with mys_. Ask whoever runs this server for one."),
    h("label", { class: "check" }, remember,
      h("span", {}, "Remember on this device"),
    ),
    h("p", { class: "help" }, "Unchecked, the token is forgotten when you close this tab. Checked, it stays in this browser until you sign out."),
    err,
    submit,
  );
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const token = input.value.trim();
    if (!token) {
      err.textContent = "Paste a token first.";
      err.hidden = false;
      input.focus();
      return;
    }
    submit.disabled = true;
    submit.textContent = "Checking…";
    try {
      const id = await api("whoami", { token });
      auth.save(token, remember.checked);
      auth.identity = id;
      buildChrome();
      render();
    } catch (ex) {
      err.textContent = ex.status === 401 ? "That token was refused: it is invalid or was revoked." : errorText(ex);
      err.hidden = false;
      submit.disabled = false;
      submit.textContent = "Sign in";
      input.focus();
    }
  });
  view.replaceChildren(
    h("section", { class: "signin" },
      h("div", { class: "signin__m" }, mBitmap()),
      h("div", { class: "signin__body" },
        h("h1", { class: "title signin__title" }, "myself"),
        h("p", { class: "statement" }, "Browse, search and fix what your agents remember."),
        form,
      ),
    ),
  );
  input.focus();
}

async function boot() {
  if (!auth.token) return signIn();
  try {
    auth.identity = await api("whoami");
  } catch (err) {
    if (err.status === 401) return;
    signedOutChrome();
    const retry = h("button", { type: "button", class: "btn" }, "Try again");
    retry.addEventListener("click", boot);
    view.replaceChildren(h("h1", { class: "title" }, "myself"), state(errorText(err), "error"), retry);
    return;
  }
  buildChrome();
  render();
}

boot();
