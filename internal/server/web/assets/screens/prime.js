// Prime: the brief an agent reads at the start of a session.
import { api, enc } from "../api.js";
import { h, put, state, copyButton } from "../dom.js";

const view = () => document.getElementById("view");

export async function primePage(ctx) {
  const v = view();
  const budget = Math.max(1, parseInt(ctx.query.get("budget") || "2000", 10) || 2000);
  const input = h("input", { id: "budget", type: "number", min: 1, step: 100, value: budget, inputmode: "numeric" });
  const form = h("form", { class: "tools tools--row" },
    h("div", { class: "field" }, h("label", { for: "budget", class: "label" }, "Budget (characters)"), input),
    h("button", { type: "submit", class: "btn" }, "Show"),
  );
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    ctx.navigate("/app/prime?budget=" + enc(input.value || "2000"), { replace: true });
  });
  put(v, h("h1", { class: "title" }, "Prime"),
    h("p", { class: "lead" }, "The brief an agent reads at the start of a session (", h("code", {}, "myself prime"), ")."),
    form);
  const res = await api("prime", { query: { budget } });
  const pre = h("pre", { class: "prime", tabindex: "0" }, res.text || "");
  if (!res.text) {
    put(v, state("The brain is empty, so the brief is empty too."));
    return;
  }
  put(v, 
    h("div", { class: "row row--between" },
      h("p", { class: "count" }, `${res.text.length} of ${res.budget} characters` + (res.truncated ? ", cut at a line to fit." : ".")),
      copyButton("Copy", () => res.text, pre)),
    pre,
  );
}
