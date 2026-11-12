// The landing page's small behaviour: the e-ink flash, the menu, the current
// section in the sidebar, and the Copy buttons. All content is in the HTML.
(() => {
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // One e-ink flash when the page first paints. Off under prefers-reduced-motion.
  const veil = document.querySelector(".flash");
  if (veil) {
    if (reduce) {
      veil.remove();
    } else {
      veil.hidden = false;
      setTimeout(() => veil.remove(), 400);
    }
  }

  // The top bar's Menu, under 940 px. Escape closes it and puts focus back.
  const menuBtn = document.querySelector(".menu-btn");
  const menu = document.getElementById("menu");
  if (menuBtn && menu) {
    menuBtn.hidden = false;
    const close = () => {
      menuBtn.setAttribute("aria-expanded", "false");
      menu.hidden = true;
    };
    menuBtn.addEventListener("click", () => {
      const open = menuBtn.getAttribute("aria-expanded") !== "true";
      menuBtn.setAttribute("aria-expanded", String(open));
      menu.hidden = !open;
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && !menu.hidden) {
        close();
        menuBtn.focus();
      }
    });
    menu.addEventListener("click", (e) => {
      if (e.target.closest("a")) close();
    });
  }

  // Mark the section the reader is in, in both copies of the nav.
  const links = [...document.querySelectorAll('a.nav__item[href^="#"]')];
  const sections = links
    .map((a) => document.getElementById(a.getAttribute("href").slice(1)))
    .filter(Boolean);
  let ticking = false;
  const mark = () => {
    ticking = false;
    let current = sections[0];
    for (const section of sections) {
      if (section.getBoundingClientRect().top <= 120) current = section;
    }
    for (const a of links) {
      if (a.getAttribute("href") === "#" + current.id) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    }
  };
  if (sections.length) {
    mark();
    window.addEventListener("scroll", () => {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(mark);
    }, { passive: true });
  }

  // Copy buttons. A terminal block copies only the lines after `$`, never
  // output or comments. Without the Clipboard API (or when it refuses), the
  // text is selected so Ctrl+C works.
  const live = document.createElement("p");
  live.className = "sr";
  live.setAttribute("role", "status");
  document.body.append(live);

  const textOf = (block, code) => {
    if (block.dataset.copy !== "commands") return code.textContent;
    return code.textContent
      .split("\n")
      .filter((line) => line.startsWith("$ "))
      .map((line) => line.slice(2))
      .join("\n");
  };
  const select = (code) => {
    const range = document.createRange();
    range.selectNodeContents(code);
    const selection = getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
  };
  const flash = (btn, state, label, say) => {
    btn.textContent = label;
    btn.dataset.state = state;
    live.textContent = say;
    clearTimeout(btn._t);
    btn._t = setTimeout(() => {
      btn.textContent = "Copy";
      delete btn.dataset.state;
      live.textContent = "";
    }, 1500);
  };

  document.querySelectorAll(".copy-block .copy").forEach((btn) => {
    const block = btn.closest(".copy-block");
    const code = block.querySelector("code");
    btn.hidden = false;
    btn.addEventListener("click", async () => {
      const text = textOf(block, code);
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
