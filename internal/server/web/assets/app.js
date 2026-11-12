// The myself web app: sign-in, chrome and a small history router over /app.
import { api, auth, takeLoginCode, redeemLogin } from "./api.js";
import { h, put, state, errorText } from "./dom.js";
import { entitiesPage } from "./screens/entities.js";
import { entityPage } from "./screens/entity.js";
import { searchPage } from "./screens/search.js";
import { logPage } from "./screens/log.js";
import { addPage } from "./screens/add.js";
import { dreamPage } from "./screens/dream.js";
import { primePage } from "./screens/prime.js";
import { tokensPage } from "./screens/settings.js";
import { graphPage } from "./screens/graph.js";
import { renderSignIn } from "./screens/signin.js";

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
  renderSignIn(view, message, () => { buildChrome(); render(); });
}

async function boot() {
  // A link from `myself graph --web`: sign in with its one-time code, land on the graph.
  const code = takeLoginCode();
  if (code) {
    try {
      auth.save(await redeemLogin(code), false);
      history.replaceState(null, "", "/app/graph");
    } catch (err) {
      return signIn(err.status === 401
        ? "That login link was already used or has expired. Run the command again for a new one, or paste a token."
        : errorText(err));
    }
  }
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
