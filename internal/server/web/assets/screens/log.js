// The Log page: the newest records, refreshed while you watch.
import { api, graph } from "../api.js";
import { h, put, entityLink, time, state, errorText } from "../dom.js";
import { object } from "../ui.js";

const view = () => document.getElementById("view");

export async function logPage(ctx) {
  const v = view();
  const sizes = [50, 200, 1000];
  let n = sizes.includes(+ctx.query.get("n")) ? +ctx.query.get("n") : 200;
  let paused = false;
  let timer = null;
  let seen = null;

  const pause = h("button", { type: "button", class: "btn", "aria-pressed": "false" }, "Pause");
  const nSel = h("select", { id: "log-n" }, sizes.map((s) => h("option", { value: s, selected: s === n }, `Last ${s}`)));
  const status = h("p", { class: "count" });
  const news = h("p", { class: "sr", "aria-live": "polite" });
  const list = h("ol", { class: "log" });

  async function load() {
    let res, g;
    try {
      [res, g] = await Promise.all([api("log", { query: { n } }), graph()]);
    } catch (err) {
      if (err.status === 401) return;
      status.textContent = "Refresh failed: " + errorText(err) + " Trying again in 10 s.";
      return;
    }
    const names = new Set(g.nodes.map((x) => x.name));
    const byID = new Map(res.records.map((r) => [r.uuid, r]));
    const recs = res.records.slice().reverse();
    const fresh = seen ? recs.filter((r) => !seen.has(r.uuid)) : [];
    seen = new Set(recs.map((r) => r.uuid));
    if (!recs.length) {
      list.replaceChildren();
      status.textContent = "The log is empty. Records appear here as agents write them.";
      return;
    }
    const isNew = new Set(fresh.map((r) => r.uuid));
    list.replaceChildren(...recs.map((r) => logRow(r, byID, names, isNew.has(r.uuid))));
    const t = new Date().toLocaleTimeString();
    status.textContent = `${recs.length} newest records, newest first. Updated ${t}` + (paused ? ", paused." : ", refreshing every 10 s.");
    if (fresh.length) news.textContent = fresh.length === 1 ? "1 new record." : `${fresh.length} new records.`;
  }

  function schedule() {
    clearInterval(timer);
    timer = paused ? null : setInterval(() => {
      if (!document.hidden) load();
    }, 10000);
  }

  pause.addEventListener("click", () => {
    paused = !paused;
    pause.setAttribute("aria-pressed", String(paused));
    pause.textContent = paused ? "Resume" : "Pause";
    status.textContent = status.textContent.replace(/, (paused|refreshing every 10 s)\.$/, paused ? ", paused." : ", refreshing every 10 s.");
    schedule();
    if (!paused) load();
  });
  nSel.addEventListener("change", () => {
    n = +nSel.value;
    history.replaceState(null, "", "/app/log?n=" + n);
    seen = null;
    load();
  });

  put(v, 
    h("h1", { class: "title" }, "Log"),
    h("p", { class: "lead" }, "What agents write, as they write it."),
    h("div", { class: "tools tools--row" },
      h("div", { class: "field" }, h("label", { for: "log-n", class: "label" }, "Show"), nSel),
      pause),
    status, news, list,
  );
  status.textContent = "Loading the log…";
  await load();
  schedule();
  ctx.setCleanup(() => clearInterval(timer));
}

function logRow(r, byID, names, isNew) {
  const head = [h("span", { class: "meta" }, time(r.time)), isNew ? h("span", { class: "stamp stamp--small" }, "new") : null];
  if (r.relation === "forget") {
    const t = byID.get(r.subject);
    return h("li", { class: "logrow logrow--forget" + (isNew ? " is-new" : "") }, ...head,
      h("span", { class: "logrow__fact" },
        h("span", { class: "rel" }, "forget"), " ",
        t ? h("del", {}, entityLink(t.subject), " ", t.relation, " ", t.object) : h("code", { title: r.subject }, r.subject.slice(0, 13) + "…"),
        r.object ? h("span", { class: "quiet" }, ": " + r.object) : null,
      ));
  }
  return h("li", { class: "logrow" + (isNew ? " is-new" : "") }, ...head,
    h("span", { class: "logrow__fact" }, entityLink(r.subject), " ", h("span", { class: "rel" }, r.relation), " ", object(r, names)));
}
