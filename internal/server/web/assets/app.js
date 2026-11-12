// The myself web app: sign-in, the shell and the router over /app.
import { api, auth, takeLoginCode, redeemLogin, signOut } from "./api.js";
import { h, put, link, errorText } from "./dom.js";
import { pageHeader, errorBox, emptyState } from "./ui.js";
import { glyph } from "./bitmap.js";
import { entitiesPage } from "./screens/entities.js";
import { entityPage } from "./screens/entity.js";
import { searchPage } from "./screens/search.js";
import { logPage } from "./screens/log.js";
import { addPage } from "./screens/add.js";
import { dreamPage } from "./screens/dream.js";
import { primePage } from "./screens/prime.js";
import { tokensPage } from "./screens/settings.js";
import { graphPage } from "./screens/graph.js";
import { kitPage } from "./screens/kit.js";
import { renderSignIn } from "./screens/signin.js";

const view = document.getElementById("view");
const nav = document.getElementById("nav");
const menuNav = document.getElementById("menu-nav");
const menu = document.getElementById("menu");
const menuBtn = document.querySelector(".menu-btn");
const addSlot = document.getElementById("add-slot");
const menuAdd = document.getElementById("menu-add");
const sidebarFoot = document.getElementById("sidebar-foot");
const menuFoot = document.getElementById("menu-foot");

const NAV_MAIN = [
  { path: "/app/", id: "graph", label: "Graph", glyph: "graph" },
  { path: "/app/entities", id: "entities", label: "Entities", glyph: "entities" },
  { path: "/app/search", id: "search", label: "Search", glyph: "search" },
  { path: "/app/log", id: "log", label: "Log", glyph: "log" },
  { path: "/app/dream", id: "dream", label: "Dream", glyph: "dream" },
  { path: "/app/prime", id: "prime", label: "Prime", glyph: "prime" },
];
const SETTINGS = { path: "/app/settings", id: "settings", label: "Settings", glyph: "settings" };

const ROUTES = {
  "/app/": { id: "graph", title: "Graph", render: graphPage },
  "/app/entities": { id: "entities", title: "Entities", render: entitiesPage },
  "/app/search": { id: "search", title: "Search", render: searchPage },
  "/app/log": { id: "log", title: "Log", render: logPage },
  "/app/dream": { id: "dream", title: "Dream", render: dreamPage },
  "/app/prime": { id: "prime", title: "Prime", render: primePage },
  "/app/add": { id: "add", title: "Add facts", render: addPage },
  "/app/settings": { id: "settings", title: "Settings", render: tokensPage },
  "/app/kit": { id: "kit", title: "Kit", render: kitPage },
};

let cleanup = null;

function cleanPath(pathname) {
  if (pathname === "/app" || pathname === "/app/") return "/app/";
  return pathname.replace(/\/+$/, "");
}

function applyRedirect() {
  const path = cleanPath(location.pathname);
  const search = location.search;
  if (location.pathname === "/app") {
    history.replaceState(null, "", "/app/" + search);
    return true;
  }
  if (path === "/app/graph") {
    history.replaceState(null, "", "/app/" + search);
    return true;
  }
  if (path === "/app/tokens") {
    const q = new URLSearchParams(search);
    q.set("tab", "tokens");
    const s = q.toString();
    history.replaceState(null, "", "/app/settings" + (s ? "?" + s : ""));
    return true;
  }
  const oldEntity = path.match(/^\/app\/e\/(.+)$/);
  if (oldEntity) {
    history.replaceState(null, "", "/app/entities/" + oldEntity[1] + search);
    return true;
  }
  return false;
}

function route(pathname) {
  const path = cleanPath(pathname);
  const m = path.match(/^\/app\/entities\/(.+)$/);
  if (m) {
    let name;
    try {
      name = decodeURIComponent(m[1]);
    } catch {
      name = m[1];
    }
    return { id: "entity", title: name, render: (ctx) => entityPage(ctx, name) };
  }
  return ROUTES[path] || null;
}

export function navigate(url, { replace = false } = {}) {
  if (replace) history.replaceState(null, "", url);
  else history.pushState(null, "", url);
  render();
}

function context() {
  const ctx = {
    query: new URLSearchParams(location.search),
    navigate,
    setCleanup: (fn) => { cleanup = fn; },
    setQuery(params) {
      for (const [key, value] of Object.entries(params || {})) {
        if (value == null || value === "") ctx.query.delete(key);
        else ctx.query.set(key, String(value));
      }
      const q = ctx.query.toString();
      history.replaceState(null, "", location.pathname + (q ? "?" + q : ""));
      ctx.query = new URLSearchParams(q);
    },
  };
  return ctx;
}

async function render() {
  if (cleanup) {
    cleanup();
    cleanup = null;
  }
  closeMenu();
  if (!auth.identity) return;
  applyRedirect();
  const r = route(location.pathname);
  markNav();
  view.className = "";
  view.replaceChildren();
  if (!r) {
    view.classList.add("s-notfound");
    document.title = "Not found · myself";
    view.replaceChildren(
      pageHeader({ title: "Not here" }),
      emptyState("This page doesn't exist, or your token can't open it.", link("/app/", "Graph")),
    );
    return;
  }
  view.classList.add("s-" + r.id);
  document.title = r.title + " · myself";
  try {
    await r.render(context());
  } catch (err) {
    if (err.status === 401) return;
    view.replaceChildren(pageHeader({ title: r.title }), errorBox(err, { retry: render }));
  }
}

function navItem(item) {
  return h("li", {},
    h("a", {
      class: "nav__item",
      href: item.path,
      "data-link": true,
      "data-nav": item.path,
    }, glyph(item.glyph), h("span", { class: "nav__label" }, item.label)));
}

function addButton() {
  return h("a", {
    class: "btn btn--primary sidebar__add-link",
    href: "/app/add",
    "data-link": true,
    "data-nav": "/app/add",
  }, "Add facts");
}

function foot() {
  const id = auth.identity;
  return [
    h("ul", { class: "nav__list nav__list--foot" }, navItem(SETTINGS)),
    h("div", { class: "who" },
      h("span", { class: "who__name" }, id.name),
      h("span", { class: "stamp who__scope" }, id.admin ? "admin" : id.scope)),
    h("button", { type: "button", class: "btn btn--quiet signout", onclick: signOut }, "Sign out"),
  ];
}

function buildChrome() {
  const main = h("ul", { class: "nav__list" }, NAV_MAIN.map(navItem));
  nav.replaceChildren(main);
  menuNav.replaceChildren(h("ul", { class: "nav__list" }, NAV_MAIN.map(navItem)));
  const canWrite = auth.canWrite;
  addSlot.replaceChildren(canWrite ? addButton() : "");
  menuAdd.replaceChildren(canWrite ? addButton() : "");
  sidebarFoot.replaceChildren(...foot());
  menuFoot.replaceChildren(...foot());
  menuBtn.hidden = false;
  document.body.classList.remove("signed-out");
  markNav();
}

function markNav() {
  const path = cleanPath(location.pathname);
  let active = null;
  if (path === "/app/") active = "/app/";
  else if (path === "/app/entities" || path.startsWith("/app/entities/")) active = "/app/entities";
  else if (path === "/app/add") active = "/app/add";
  else if (ROUTES[path] && path !== "/app/kit") active = path;
  for (const a of document.querySelectorAll("a[data-nav]")) {
    if (a.getAttribute("data-nav") === active) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
}

function closeMenu() {
  menuBtn.setAttribute("aria-expanded", "false");
  menu.hidden = true;
  document.body.classList.remove("menu-open");
}

menuBtn.addEventListener("click", () => {
  const open = menuBtn.getAttribute("aria-expanded") !== "true";
  menuBtn.setAttribute("aria-expanded", String(open));
  menu.hidden = !open;
  document.body.classList.toggle("menu-open", open);
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && !menu.hidden) {
    closeMenu();
    menuBtn.focus();
  }
});

// In-app links: plain clicks only, so Ctrl/Cmd-click still opens a tab.
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-link]");
  if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  e.preventDefault();
  closeMenu();
  if (a.getAttribute("href") === location.pathname + location.search) {
    return;
  }
  navigate(a.getAttribute("href"));
  view.focus({ preventScroll: true });
  window.scrollTo(0, 0);
});
window.addEventListener("popstate", render);

auth.onUnauthorized = () => {
  auth.clear();
  signIn("The server refused this token: it is invalid or was revoked. Paste another one.");
};

function signedOutChrome() {
  if (cleanup) {
    cleanup();
    cleanup = null;
  }
  menuBtn.hidden = true;
  closeMenu();
  document.body.classList.add("signed-out");
}

function signIn(message) {
  signedOutChrome();
  view.className = "s-signin";
  document.title = "Sign in · myself";
  renderSignIn(view, message, () => {
    buildChrome();
    render();
  });
}

async function boot() {
  // A link from `myself graph --web`: sign in with its one-time code, land on the app.
  const code = takeLoginCode();
  if (code) {
    try {
      auth.save(await redeemLogin(code), false);
      history.replaceState(null, "", "/app/");
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
    view.className = "s-signin";
    const retry = h("button", { type: "button", class: "btn" }, "Try again");
    retry.addEventListener("click", boot);
    put(view, h("h1", { class: "page-title" }, "myself"), errorBox(err, { retry: boot }));
    return;
  }
  buildChrome();
  render();
}

boot();
