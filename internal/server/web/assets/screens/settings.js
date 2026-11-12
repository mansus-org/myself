// Settings: connecting an agent, the agent tokens and this server's addresses.
import { api, auth, signOut } from "../api.js";
import { h, put, time, dialog, errorText } from "../dom.js";
import {
  pageHeader, button, field, segmented, tabs, stamp, table, copyBlock,
  copyButton, secretBox, errorBox, formError, emptyState, announce,
} from "../ui.js";

const AGENTS = [
  { value: "claude-code", label: "Claude Code", name: "claude-code" },
  { value: "pi", label: "pi", name: "pi" },
  { value: "mcp", label: "Other MCP client", name: "mcp-client" },
];
const SCOPES = [
  { value: "write", label: "write" },
  { value: "read", label: "read" },
];

const defaultName = (agent) => AGENTS.find((a) => a.value === agent)?.name || "claude-code";

function installCommand() {
  return "curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh";
}

function step(line, block) {
  return h("li", {},
    h("p", { class: "s-settings__step" }, line),
    copyBlock(block));
}

function claudeSteps(origin, token) {
  const settings = `{
  "env": {
    "MYSELF_URL": "${origin}",
    "MYSELF_TOKEN": "${token}"
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
}`;
  return h("div", { class: "s-settings__step-list" },
    h("ol", {},
      step("Install myself. Terminal:", { label: "Terminal", text: installCommand() }),
      step("Install the skill. Terminal:", {
        label: "Terminal",
        text: `mkdir -p ~/.claude/skills/myself && curl -fsSL ${origin}/skill.md -o ~/.claude/skills/myself/SKILL.md`,
      }),
      step("Install the hooks. Terminal:", {
        label: "Terminal",
        text: "mkdir -p ~/.myself/hooks && for h in session-start stop-nudge; do curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/contrib/hooks/$h.sh -o ~/.myself/hooks/$h.sh; done && chmod +x ~/.myself/hooks/*.sh",
      }),
      step("Merge into ~/.claude/settings.json.", {
        label: "~/.claude/settings.json",
        text: settings,
      }),
      step("Check. Terminal:", {
        label: "Terminal",
        text: `MYSELF_URL=${origin} MYSELF_TOKEN=${token} myself whoami`,
      }),
    ),
    h("div", { class: "s-settings__mcp" },
      h("p", { class: "s-settings__mcp-lead" }, "Or connect over MCP, with nothing to install:"),
      copyBlock({
        label: "Terminal",
        text: `claude mcp add --transport http --scope user myself ${origin}/mcp --header "Authorization: Bearer ${token}"`,
      }),
    ),
  );
}

function piSteps(origin, token) {
  return h("div", { class: "s-settings__step-list" },
    h("ol", {},
      step("Install myself. Terminal:", { label: "Terminal", text: installCommand() }),
      step("Install the pi package. Terminal:", {
        label: "Terminal",
        text: "pi install git:github.com/mansus-org/myself",
      }),
      step("Point pi at this server.", {
        label: "~/.profile",
        text: `export MYSELF_URL=${origin}\nexport MYSELF_TOKEN=${token}`,
      }),
      step("Check, in a new shell. Terminal:", {
        label: "Terminal",
        text: "myself whoami",
      }),
    ),
  );
}

function mcpSteps(origin, token) {
  return h("div", { class: "s-settings__step-list" },
    h("ol", {},
      step("Add this server to its MCP config.", {
        label: "MCP config",
        text: `{"mcpServers":{"myself":{"type":"http","url":"${origin}/mcp","headers":{"Authorization":"Bearer ${token}"}}}}`,
      }),
      step("Give it the skill.", {
        label: "Prompt",
        text: `Read ${origin}/skill.md and follow it.`,
      }),
    ),
  );
}

function steps(agent, token) {
  const origin = location.origin;
  if (agent === "pi") return piSteps(origin, token);
  if (agent === "mcp") return mcpSteps(origin, token);
  return claudeSteps(origin, token);
}

function address(text) {
  const code = h("code", {}, text);
  return h("span", { class: "s-settings__address" },
    code,
    copyButton("Copy", () => text, code));
}

function tokenTable(list, onRevoke) {
  return table({
    caption: "Agent tokens, oldest first",
    columns: [
      { key: "name", label: "Name" },
      { key: "scope", label: "Scope" },
      { key: "created", label: "Created" },
      { key: "last", label: "Last used" },
      { key: "actions", label: "Actions" },
    ],
    rows: list.map((t) => ({
      key: t.id,
      cells: {
        name: h("strong", {}, t.name),
        scope: stamp(t.scope),
        created: time(t.created),
        last: t.last_used ? time(t.last_used) : "never",
        actions: button("Revoke", { onClick: () => onRevoke(t) }),
      },
    })),
    empty: emptyState("No agent tokens yet."),
  });
}

function newTokenDialog() {
  return dialog("New token", (close) => {
    const name = h("input", {
      type: "text", id: "new-token-name", autocomplete: "off",
      spellcheck: "false", maxlength: "100", required: true,
    });
    const scope = segmented({ label: "Scope", options: SCOPES, value: "write" });
    const errors = h("div");
    const create = button("Create token", { kind: "primary", type: "submit" });
    const form = h("form", { novalidate: true },
      field({ label: "Name", control: name, help: "The agent or harness that will use it." }),
      field({ label: "Scope", control: scope, help: "write also allows remember and forget." }),
      errors,
      h("div", { class: "dialog__actions" },
        button("Cancel", { onClick: () => close(null) }),
        create,
      ),
    );
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      errors.replaceChildren();
      create.disabled = true;
      try {
        const t = await api("tokens", {
          method: "POST",
          body: { name: name.value.trim(), scope: scope.value },
        });
        close(t);
      } catch (err) {
        errors.replaceChildren(formError(errorText(err)));
        create.disabled = false;
      }
    });
    return form;
  });
}

function revokeDialog(t) {
  return dialog(`Revoke ${t.name}?`, (close) => {
    const errors = h("div");
    const revoke = button("Revoke", {
      kind: "primary",
      onClick: async () => {
        errors.replaceChildren();
        revoke.disabled = true;
        try {
          await api("tokens/" + encodeURIComponent(t.id), { method: "DELETE" });
          close(true);
        } catch (err) {
          errors.replaceChildren(formError(errorText(err)));
          revoke.disabled = false;
        }
      },
    });
    return h("div", {},
      h("p", {}, "Agents using it get 401 from their next call."),
      errors,
      h("div", { class: "dialog__actions" },
        button("Cancel", { onClick: () => close(false) }),
        revoke,
      ),
    );
  });
}

export async function tokensPage(ctx) {
  const view = document.getElementById("view");
  const allowed = ["connect", "server"];
  if (auth.isAdmin) allowed.splice(1, 0, "tokens");

  const requestedTab = ctx.query.get("tab");
  const requestedAgent = ctx.query.get("agent");
  let tab = allowed.includes(requestedTab) ? requestedTab : "connect";
  let agent = AGENTS.some((a) => a.value === requestedAgent) ? requestedAgent : "claude-code";

  // Canonical URL: defaults are left out, unknown and forbidden tabs fall back.
  const wantTab = tab === "connect" ? null : tab;
  const wantAgent = agent === "claude-code" ? null : agent;
  if (ctx.query.get("tab") !== wantTab || ctx.query.get("agent") !== wantAgent) {
    ctx.setQuery({ tab: wantTab, agent: wantAgent });
  }

  let scope = "write";
  let nameValue = null;
  let pastedToken = "";
  let createdToken = "";
  let connectSecret = null;
  let tokensSecret = null;
  let tokenSeq = 0;

  const header = pageHeader({ title: "Settings" });
  const tabSet = tabs({
    label: "Settings sections",
    items: allowed.map((id) => ({
      id,
      label: id === "connect" ? "Connect an agent" : id === "tokens" ? "Tokens" : "Server",
    })),
    selected: tab,
    onSelect: (id) => {
      tab = id;
      ctx.setQuery({ tab: id === "connect" ? null : id });
      draw();
    },
  });
  const panel = tabSet.panel;

  function stepToken() {
    return pastedToken || createdToken || "<token>";
  }

  function drawConnect() {
    let nameInput = null;
    let pasteInput = null;
    const stepsHost = h("div");

    function paintSteps() {
      stepsHost.replaceChildren(steps(agent, stepToken()));
    }

    const agentControl = segmented({
      label: "Agent",
      options: AGENTS,
      value: agent,
      onChange: (next) => {
        const oldDefault = defaultName(agent);
        agent = next;
        ctx.setQuery({ agent: agent === "claude-code" ? null : agent });
        if (nameInput) {
          const nextDefault = defaultName(agent);
          if (!nameInput.value.trim() || nameInput.value === oldDefault) {
            nameInput.value = nextDefault;
            nameValue = nextDefault;
          }
        }
        paintSteps();
      },
    });

    const tokenBlock = h("div", { class: "s-settings__block" },
      h("h2", { class: "s-settings__h2" }, "Token"));

    const secretHost = h("div", { class: "s-settings__secret" });
    if (connectSecret) secretHost.append(connectSecret);

    if (auth.isAdmin) {
      nameInput = h("input", {
        type: "text", id: "connect-token-name", autocomplete: "off",
        spellcheck: "false", maxlength: "100", value: nameValue ?? defaultName(agent),
      });
      nameInput.addEventListener("input", () => { nameValue = nameInput.value; });
      const scopeControl = segmented({
        label: "Scope",
        options: SCOPES,
        value: scope,
        onChange: (next) => { scope = next; },
      });
      const errors = h("div");
      const create = button("Create token", { kind: "primary", type: "submit" });

      const form = h("form", { class: "s-settings__create", novalidate: true },
        h("div", { class: "s-settings__fields" },
          field({ label: "Name", control: nameInput }),
          field({ label: "Scope", control: scopeControl }),
        ),
        errors,
        h("div", { class: "s-settings__actions" }, create),
      );
      form.addEventListener("submit", async (e) => {
        e.preventDefault();
        errors.replaceChildren();
        create.disabled = true;
        try {
          const t = await api("tokens", {
            method: "POST",
            body: { name: nameInput.value.trim(), scope: scopeControl.value },
          });
          createdToken = t.token;
          pastedToken = "";
          if (pasteInput) pasteInput.value = "";
          nameValue = nameInput.value;
          connectSecret = secretBox({
            name: t.name,
            scope: t.scope,
            token: t.token,
            onDone: () => {
              connectSecret = null;
              secretHost.replaceChildren();
              nameInput.focus();
            },
          });
          secretHost.replaceChildren(connectSecret);
          announce(`Created token ${t.name}`);
          paintSteps();
        } catch (err) {
          errors.replaceChildren(formError(errorText(err)));
        } finally {
          create.disabled = false;
        }
      });
      tokenBlock.append(form);
    }

    pasteInput = h("input", {
      type: "password", id: "paste-token", autocomplete: "off",
      spellcheck: "false", value: pastedToken,
    });
    pasteInput.addEventListener("input", () => {
      pastedToken = pasteInput.value;
      paintSteps();
    });
    tokenBlock.append(field({ label: "Or paste a token", control: pasteInput }));
    if (auth.isAdmin) tokenBlock.append(secretHost);

    paintSteps();
    return h("div", { class: "s-settings__connect" },
      h("div", { class: "s-settings__block" },
        field({ label: "Agent", control: agentControl }),
      ),
      tokenBlock,
      h("div", { class: "s-settings__block" },
        h("h2", { class: "s-settings__h2" }, "Steps"),
        stepsHost,
      ),
    );
  }

  function drawServer() {
    const id = auth.identity;
    return h("div", { class: "s-settings__server" }, table({
      caption: "This server",
      columns: [{ key: "label", label: "Label" }, { key: "value", label: "Value" }],
      rows: [
        {
          key: "who",
          cells: {
            label: "Signed in as",
            value: h("span", { class: "s-settings__identity" },
              h("strong", {}, id.name),
              stamp(id.admin ? "admin" : id.scope),
              button("Sign out", { kind: "quiet", onClick: signOut }),
            ),
          },
        },
        { key: "api", cells: { label: "API", value: address(`${location.origin}/api/v1`) } },
        { key: "mcp", cells: { label: "MCP", value: address(`${location.origin}/mcp`) } },
        { key: "skill", cells: { label: "Skill", value: address(`${location.origin}/skill.md`) } },
        {
          key: "fonts",
          cells: {
            label: "Fonts",
            value: h("a", { href: "/app/assets/fonts/OFL.txt" }, "Fonts and their licence"),
          },
        },
      ],
    }));
  }

  async function drawTokens() {
    const seq = ++tokenSeq;
    const tokens = h("div", { class: "s-settings__tokens" });
    panel.replaceChildren(tokens);
    panel.setAttribute("aria-busy", "true");
    let list;
    try {
      list = await api("tokens");
    } catch (err) {
      if (seq !== tokenSeq || tab !== "tokens") return;
      panel.removeAttribute("aria-busy");
      if (err.status === 401) return;
      tokens.replaceChildren(errorBox(err, { retry: drawTokens }));
      return;
    }
    if (seq !== tokenSeq || tab !== "tokens") return;
    panel.removeAttribute("aria-busy");

    const newButton = button("New token", { kind: "primary", onClick: async () => {
      const t = await newTokenDialog();
      if (!t) return;
      tokensSecret = secretBox({
        name: t.name,
        scope: t.scope,
        token: t.token,
        onDone: () => {
          tokensSecret = null;
          secretHost.replaceChildren();
          newButton.focus();
        },
      });
      secretHost.replaceChildren(tokensSecret);
      announce(`Created token ${t.name}`);
      await reload();
    } });

    const secretHost = h("div", { class: "s-settings__secret" });
    if (tokensSecret) secretHost.append(tokensSecret);
    const tableHost = h("div", { class: "s-settings__table" });
    const paint = (rows) => tableHost.replaceChildren(tokenTable(rows, revoke));

    async function reload() {
      try {
        paint(await api("tokens"));
      } catch (err) {
        if (err.status === 401) return;
        tableHost.replaceChildren(errorBox(err, { retry: reload }));
      }
    }

    async function revoke(t) {
      const ok = await revokeDialog(t);
      if (!ok) return;
      announce(`Revoked ${t.name}`);
      await reload();
    }

    paint(list);
    tokens.append(
      h("div", { class: "s-settings__toolbar" }, newButton),
      secretHost,
      tableHost,
    );
  }

  function draw() {
    if (tab === "tokens" && auth.isAdmin) drawTokens();
    else if (tab === "server") panel.replaceChildren(drawServer());
    else panel.replaceChildren(drawConnect());
  }

  put(view, header, tabSet.list, panel);
  draw();
}
