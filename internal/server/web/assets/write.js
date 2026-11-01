// Writing: the Add facts page and the forget dialog. Both need a write token.
import { api, auth, dropGraph } from "./api.js";
import { h, put, dialog, entityLink, state, errorText } from "./dom.js";

// forgetRecord asks for a reason and forgets rec. It resolves true once
// the forget record is written, false if the reader cancelled.
export async function forgetRecord(rec) {
  const result = await dialog("Forget this fact?", (close) => {
    const reason = h("textarea", { id: "forget-reason", rows: 3, maxlength: 2000 });
    const err = h("p", { class: "form-error", role: "alert", hidden: true });
    const ok = h("button", { type: "submit", class: "btn btn--ink" }, "Forget");
    const form = h("form", { class: "stack", method: "dialog" },
      h("p", { class: "dialog__fact" }, h("strong", {}, rec.subject), " ", h("span", { class: "rel" }, rec.relation), " ", rec.object),
      h("p", { class: "help" }, "Nothing is erased: a forget record is appended and this fact moves to the history, struck through."),
      h("label", { for: "forget-reason", class: "label" }, "Reason (optional)"),
      reason,
      err,
      h("div", { class: "row" },
        ok,
        h("button", { type: "button", class: "btn", onclick: () => close(false) }, "Cancel"),
      ),
    );
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      ok.disabled = true;
      ok.textContent = "Forgetting…";
      try {
        await api("forget", { method: "POST", body: { uuid: rec.uuid, reason: reason.value.trim() } });
        dropGraph();
        close(true);
      } catch (ex) {
        err.textContent = errorText(ex);
        err.hidden = false;
        ok.disabled = false;
        ok.textContent = "Forget";
      }
    });
    queueMicrotask(() => reason.focus());
    return form;
  });
  return result === true;
}

// parseBulk splits "subject relation object..." lines like `remember -`:
// blank lines skipped, one matching pair of quotes stripped from the object.
// It returns the facts and the line number of each, or the first error.
export function parseBulk(text) {
  const facts = [];
  const lines = [];
  const rows = text.split("\n");
  for (let i = 0; i < rows.length; i++) {
    const words = rows[i].trim().split(/\s+/).filter(Boolean);
    if (!words.length) continue;
    if (words.length < 2) return { error: `line ${i + 1}: want "subject relation object..."` };
    let object = words.slice(2).join(" ");
    const q = object[0];
    if (object.length >= 2 && (q === "'" || q === '"') && object.at(-1) === q) object = object.slice(1, -1);
    facts.push({ subject: words[0], relation: words[1], object });
    lines.push(i + 1);
  }
  if (!facts.length) return { error: "Write at least one line: subject relation object." };
  return { facts, lines };
}

export async function addPage() {
  const view = document.getElementById("view");
  if (!auth.canWrite) {
    put(view, h("h1", { class: "title" }, "Add facts"), state("This token is read-only. Sign in with a write token to add facts."));
    return;
  }
  const { relations } = await api("relations");
  const out = h("div", { class: "result", "aria-live": "polite" });

  async function send(facts, replace, lines, button) {
    const label = button.textContent;
    button.disabled = true;
    button.textContent = "Saving…";
    out.replaceChildren();
    try {
      const res = await api("remember", { method: "POST", body: { facts, replace } });
      dropGraph();
      out.replaceChildren(resultView(res));
      return true;
    } catch (ex) {
      let msg = errorText(ex);
      // The server numbers facts; the reader thinks in lines.
      if (lines) msg = msg.replace(/^fact (\d+): /, (_, n) => `line ${lines[n - 1]}: `);
      out.replaceChildren(state(msg, "error"));
      return false;
    } finally {
      button.disabled = false;
      button.textContent = label;
    }
  }

  // One fact.
  const subject = h("input", { id: "f-subject", name: "subject", required: true, autocomplete: "off", spellcheck: "false" });
  const relation = h("select", { id: "f-relation", name: "relation" },
    relations.filter((r) => r !== "are").map((r) => h("option", { value: r }, r)));
  const object = h("input", { id: "f-object", name: "object", required: true, autocomplete: "off" });
  const replace1 = h("input", { type: "checkbox", id: "f-replace" });
  const save1 = h("button", { type: "submit", class: "btn btn--ink" }, "Remember");
  const one = h("form", { class: "fact-form", novalidate: true },
    h("div", { class: "field" }, h("label", { for: "f-subject", class: "label" }, "Subject"), subject),
    h("div", { class: "field field--rel" }, h("label", { for: "f-relation", class: "label" }, "Relation"), relation),
    h("div", { class: "field field--obj" }, h("label", { for: "f-object", class: "label" }, "Object"), object),
    h("label", { class: "check" }, replace1, h("span", {}, "Replace: forget the live facts with this subject and relation first")),
    h("div", { class: "row" }, save1),
  );
  one.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!subject.value.trim() || !object.value.trim()) {
      out.replaceChildren(state("Fill in the subject and the object.", "error"));
      (subject.value.trim() ? object : subject).focus();
      return;
    }
    const fact = { subject: subject.value.trim(), relation: relation.value, object: object.value.trim() };
    if (await send([fact], replace1.checked, null, save1)) {
      object.value = "";
      object.focus();
    }
  });

  // Many facts.
  const bulk = h("textarea", { id: "f-bulk", rows: 8, spellcheck: "false", "aria-describedby": "bulk-help" });
  const replaceN = h("input", { type: "checkbox", id: "f-replace-bulk" });
  const saveN = h("button", { type: "submit", class: "btn btn--ink" }, "Remember all");
  const many = h("form", { class: "stack", novalidate: true },
    h("label", { for: "f-bulk", class: "label" }, "One fact per line"),
    bulk,
    h("p", { id: "bulk-help", class: "help" },
      "Like ", h("code", {}, "myself remember -"), ": the first word is the subject, the second the relation (",
      relations.join(", "), "), the rest is the object. All lines are saved, or none."),
    h("label", { class: "check" }, replaceN, h("span", {}, "Replace, for every line")),
    h("div", { class: "row" }, saveN),
  );
  many.addEventListener("submit", async (e) => {
    e.preventDefault();
    const parsed = parseBulk(bulk.value);
    if (parsed.error) {
      out.replaceChildren(state(parsed.error, "error"));
      bulk.focus();
      return;
    }
    if (await send(parsed.facts, replaceN.checked, parsed.lines, saveN)) bulk.value = "";
  });

  put(view, 
    h("h1", { class: "title" }, "Add facts"),
    h("div", { class: "split" },
      h("section", { class: "panel", "aria-labelledby": "one-h" }, h("h2", { id: "one-h", class: "h2" }, "One fact"), one),
      h("section", { class: "panel", "aria-labelledby": "many-h" }, h("h2", { id: "many-h", class: "h2" }, "Many at once"), many),
    ),
    out,
  );
  subject.focus();
}

function resultView(res) {
  const n = res.remembered.length;
  return h("div", { class: "result__box", role: "status" },
    h("p", { class: "result__head" }, n === 1 ? "Remembered 1 fact." : `Remembered ${n} facts.`),
    h("ul", { class: "facts" }, res.remembered.map((r) =>
      h("li", { class: "fact" }, entityLink(r.subject), " ", h("span", { class: "rel" }, r.relation), " ", h("span", { class: "obj" }, r.object)))),
    res.forgotten.length ? h("p", { class: "help" },
      res.forgotten.length === 1 ? "Replace forgot 1 older fact." : `Replace forgot ${res.forgotten.length} older facts.`) : null,
    (res.warnings || []).map((w) => h("p", { class: "warning" }, h("span", { class: "stamp" }, "Warning"), " ", w)),
  );
}
