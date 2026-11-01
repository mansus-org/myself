// Token admin: list, create (plaintext shown once), revoke. Admin token only.
import { api, auth } from "./api.js";
import { h, put, time, state, dialog, copyButton, errorText } from "./dom.js";

export async function tokensPage() {
  const v = document.getElementById("view");
  put(v, h("h1", { class: "title" }, "Tokens"));
  if (!auth.isAdmin) {
    put(v, state("Only the admin token can manage tokens."));
    return;
  }
  const table = h("div", { class: "tokens" });
  const created = h("div", { class: "created" });

  async function load() {
    const list = await api("tokens");
    if (!list.length) {
      table.replaceChildren(state("No agent tokens yet. Create one below for each agent or harness that should reach this brain."));
      return;
    }
    table.replaceChildren(h("table", { class: "table" },
      h("caption", { class: "sr" }, "Agent tokens, oldest first"),
      h("thead", {}, h("tr", {},
        h("th", { scope: "col" }, "Name"), h("th", { scope: "col" }, "Scope"),
        h("th", { scope: "col" }, "Created"), h("th", { scope: "col" }, "Last used"),
        h("th", { scope: "col" }, h("span", { class: "sr" }, "Actions")))),
      h("tbody", {}, list.map((t) => h("tr", {},
        h("th", { scope: "row", "data-label": "Name" }, t.name),
        h("td", { "data-label": "Scope" }, h("span", { class: "stamp stamp--small" }, t.scope)),
        h("td", { "data-label": "Created" }, time(t.created)),
        h("td", { "data-label": "Last used" }, t.last_used ? time(t.last_used) : "Never"),
        h("td", {}, h("button", {
          type: "button", class: "btn btn--small", "aria-label": `Revoke ${t.name}`,
          onclick: () => revoke(t),
        }, "Revoke")),
      ))),
    ));
  }

  async function revoke(t) {
    const ok = await dialog(`Revoke ${t.name}?`, (close) => {
      const err = h("p", { class: "form-error", role: "alert", hidden: true });
      const yes = h("button", { type: "button", class: "btn btn--ink" }, "Revoke");
      const no = h("button", { type: "button", class: "btn", onclick: () => close(false) }, "Keep it");
      yes.addEventListener("click", async () => {
        yes.disabled = true;
        try {
          await api("tokens/" + encodeURIComponent(t.id), { method: "DELETE" });
          close(true);
        } catch (ex) {
          err.textContent = errorText(ex);
          err.hidden = false;
          yes.disabled = false;
        }
      });
      queueMicrotask(() => no.focus());
      return h("div", { class: "stack" },
        h("p", {}, `Agents using ${t.name} get 401 on their next call. This can't be undone; you can create a new token instead.`),
        err, h("div", { class: "row" }, yes, no));
    });
    if (ok) {
      created.replaceChildren(state(`Revoked ${t.name}.`));
      await load();
    }
  }

  const name = h("input", { id: "t-name", required: true, autocomplete: "off", spellcheck: "false", maxlength: 100 });
  const read = h("input", { type: "radio", name: "scope", value: "read", id: "t-read", checked: true });
  const write = h("input", { type: "radio", name: "scope", value: "write", id: "t-write" });
  const submit = h("button", { type: "submit", class: "btn btn--ink" }, "Create token");
  const err = h("p", { class: "form-error", role: "alert", hidden: true });
  const form = h("form", { class: "stack token-form", novalidate: true },
    h("div", { class: "field" },
      h("label", { for: "t-name", class: "label" }, "Name"),
      name,
      h("p", { class: "help" }, "Who will use it, for example claude-runner. It shows in the server log.")),
    h("fieldset", { class: "fieldset" },
      h("legend", { class: "label" }, "Scope"),
      h("label", { class: "check" }, read, h("span", {}, h("strong", {}, "read"), ": describe, search, log and the rest")),
      h("label", { class: "check" }, write, h("span", {}, h("strong", {}, "write"), ": also remember and forget"))),
    err,
    h("div", { class: "row" }, submit),
  );
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    err.hidden = true;
    if (!name.value.trim()) {
      err.textContent = "Give the token a name.";
      err.hidden = false;
      name.focus();
      return;
    }
    submit.disabled = true;
    try {
      const t = await api("tokens", { method: "POST", body: { name: name.value.trim(), scope: write.checked ? "write" : "read" } });
      name.value = "";
      showCreated(t);
      await load();
    } catch (ex) {
      err.textContent = errorText(ex);
      err.hidden = false;
    } finally {
      submit.disabled = false;
    }
  });

  function showCreated(t) {
    const code = h("code", { class: "secret", tabindex: "0" }, t.token);
    const done = h("button", { type: "button", class: "btn", onclick: () => {
      created.replaceChildren();
      name.focus();
    } }, "Done, I saved it");
    created.replaceChildren(h("section", { class: "secret-box", role: "status", "aria-labelledby": "secret-h" },
      h("h3", { id: "secret-h", class: "secret-box__title" }, `Token for ${t.name} (${t.scope})`),
      h("p", {}, h("strong", {}, "Copy it now. You won't see this token again: "), "the server keeps only its hash."),
      code,
      h("div", { class: "row" }, copyButton("Copy token", () => t.token, code), done),
    ));
    code.focus();
  }

  put(v, 
    h("section", { class: "section" }, h("h2", { class: "h2" }, "Agent tokens"), table),
    h("section", { class: "section", "aria-labelledby": "new-h" }, h("h2", { id: "new-h", class: "h2" }, "New token"), created, form),
  );
  table.append(state("Loading tokens…"));
  await load();
}
