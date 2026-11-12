// Shared UI components: type stamps and a fact's object.
import { enc } from "./api.js";
import { h, link, entityHref, normalizeName } from "./dom.js";

export function typeStamps(types) {
  return types.length ? h("span", { class: "types" }, types.map((t) => h("a", {
    class: "stamp", href: "/app/?type=" + enc(t), "data-link": true, title: "Every " + t,
  }, t))) : null;
}

// object renders a record's object, linked when it names an entity.
export function object(rec, names) {
  const n = normalizeName(rec.object);
  return names.has(n) ? link(entityHref(n), rec.object) : rec.object;
}
