// One e-ink flash when the page first paints. Off under prefers-reduced-motion.
(() => {
  if (window.Bitmap.reduceMotion) return;
  const veil = document.querySelector(".flash");
  if (!veil) return;
  veil.hidden = false;
  requestAnimationFrame(() => requestAnimationFrame(() => { veil.hidden = true; }));
  setTimeout(() => veil.remove(), 400);
})();

// Copy buttons. Uses the Clipboard API; without it (or when it refuses) the text is selected so Ctrl+C works.
(() => {
  const live = document.createElement("p");
  live.className = "sr";
  live.setAttribute("role", "status");
  document.body.append(live);

  const textOf = (btn, code) => {
    if (btn.dataset.copy !== "commands") return code.textContent;
    // The example mixes commands, comments and output: copy only the commands.
    return code.textContent.split("\n").filter((l) => l.startsWith("$ ")).map((l) => l.slice(2)).join("\n");
  };
  const select = (code) => {
    const r = document.createRange();
    r.selectNodeContents(code);
    const s = getSelection();
    s.removeAllRanges();
    s.addRange(r);
  };
  const flash = (btn, state, label, say) => {
    btn.textContent = label;
    btn.dataset.state = state;
    live.textContent = say;
    clearTimeout(btn._t);
    btn._t = setTimeout(() => { btn.textContent = "Copy"; delete btn.dataset.state; live.textContent = ""; }, 1500);
  };

  document.querySelectorAll(".copy").forEach((btn) => {
    const code = btn.closest(".code").querySelector("code");
    btn.addEventListener("click", async () => {
      const text = textOf(btn, code);
      try {
        await navigator.clipboard.writeText(text);
        flash(btn, "ok", "Copied", "Copied to clipboard");
      } catch {
        select(code);
        flash(btn, "select", "Selected", "Selected. Press Ctrl+C to copy.");
      }
    });
  });
})();
