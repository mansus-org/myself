// Dream: subjects that are probably the same thing.
import { api } from "../api.js";
import { h, put, entityLink, state } from "../dom.js";

const view = () => document.getElementById("view");

export async function dreamPage() {
  const v = view();
  put(v, h("h1", { class: "title" }, "Dream"),
    h("p", { class: "lead" }, "Subjects that are probably the same thing: equal once dashes and a trailing s are removed."));
  const res = await api("dream");
  if (!res.near_duplicates.length) {
    put(v, state("No near-duplicates. Every subject is distinct."));
    return;
  }
  put(v, 
    h("p", { class: "count" }, res.near_duplicates.length === 1 ? "1 group" : `${res.near_duplicates.length} groups`),
    h("ul", { class: "dream" }, res.near_duplicates.map((g) => h("li", { class: "dream__group" },
      g.map((name, i) => [i ? h("span", { class: "dream__eq", "aria-hidden": "true" }, "≈") : null, i ? h("span", { class: "sr" }, " and ") : null, entityLink(name)])))),
    h("p", { class: "help" }, "To merge, remember the facts under one name and forget the others from their entity pages."),
  );
}
