// Plays demo.cast (asciinema v2) inside a terminal window. The window already holds the
// finished session as plain HTML, so without JS, or under prefers-reduced-motion, that is
// what people read. The cast only uses colour codes and \r\n, so this is a text player,
// not a terminal emulator: escape sequences are dropped and lines are drawn as the other
// windows draw them (prompt, command, output block).
(() => {
  const root = document.querySelector("[data-cast]");
  if (!root) return;
  const screen = root.querySelector(".win__body");
  const code = screen.querySelector("code");
  const toggle = root.querySelector("[data-cast-toggle]");
  const replay = root.querySelector("[data-cast-replay]");
  const status = root.querySelector("[data-cast-status]");
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)");

  let events = [];
  let i = 0;
  let text = "";
  let clock = 0; // cast time reached, in seconds
  let started = 0; // performance.now() minus clock, while playing
  let timer = 0;
  let playing = false;
  let touched = false; // someone pressed a control: no autoplay after that

  const clean = (s) => s.replace(/\x1b\[[0-9;?]*[A-Za-z]/g, "").replace(/\r/g, "");

  // Same markup as the static windows: <span class="p">$</span> cmd, output lines in one .o block.
  const render = (cursor) => {
    const lines = text.split("\n");
    const frag = document.createDocumentFragment();
    let out = null;
    if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop(); // output ended, next prompt not drawn yet
    lines.forEach((line, n) => {
      if (line.startsWith("$ ")) {
        out = null;
        if (n > 0) frag.append("\n");
        const p = document.createElement("span");
        p.className = "p";
        p.textContent = "$";
        frag.append(p, " " + line.slice(2));
        if (cursor && n === lines.length - 1) {
          const c = document.createElement("span");
          c.className = "cursor";
          frag.append(c);
        }
        return;
      }
      if (out) {
        out.append("\n" + line);
        return;
      }
      if (n > 0) frag.append("\n");
      out = document.createElement("span");
      out.className = "o";
      out.append(line);
      frag.append(out);
    });
    code.replaceChildren(frag);
    screen.scrollTop = screen.scrollHeight;
  };

  const setState = (state) => {
    root.dataset.state = state;
    toggle.textContent = state === "playing" ? "Pause" : "Play";
    toggle.setAttribute("aria-label", state === "playing" ? "Pause the recording" : "Play the recording");
    status.textContent = state === "playing" ? "Playing" : state === "paused" ? "Paused" : "Finished";
  };

  const step = () => {
    clock = (performance.now() - started) / 1000;
    while (i < events.length && events[i][0] <= clock) {
      if (events[i][1] === "o") text += clean(events[i][2]);
      i++;
    }
    render(true);
    if (i >= events.length) {
      playing = false;
      render(false);
      setState("done");
      return;
    }
    timer = setTimeout(step, Math.max(16, (events[i][0] - clock) * 1000));
  };

  const play = () => {
    if (i >= events.length) restart();
    playing = true;
    started = performance.now() - clock * 1000;
    setState("playing");
    step();
  };
  const pause = () => {
    clearTimeout(timer);
    playing = false;
    clock = (performance.now() - started) / 1000;
    setState("paused");
  };
  const restart = () => {
    clearTimeout(timer);
    i = 0;
    text = "";
    clock = 0;
  };

  fetch(root.dataset.cast)
    .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.status))))
    .then((body) => {
      const [, ...rest] = body.trim().split("\n");
      events = rest.map((l) => JSON.parse(l));
      toggle.hidden = false;
      replay.hidden = false;
      toggle.addEventListener("click", () => {
        touched = true;
        playing ? pause() : play();
      });
      replay.addEventListener("click", () => {
        touched = true;
        restart();
        play();
      });
      // Keep the finished session on screen until someone asks for motion.
      if (reduce.matches) {
        setState("done");
        return;
      }
      // Start when the window scrolls into view, once.
      const io = new IntersectionObserver((entries) => {
        if (!entries.some((e) => e.isIntersecting)) return;
        io.disconnect();
        if (touched) return;
        restart();
        play();
      }, { threshold: 0.4 });
      io.observe(root);
    })
    .catch(() => {
      // The static session stays; the controls stay hidden.
    });
})();
