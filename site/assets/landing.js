// One e-ink flash when the page first paints. Off under prefers-reduced-motion.
(() => {
  if (window.Bitmap.reduceMotion) return;
  const veil = document.querySelector(".flash");
  if (!veil) return;
  veil.hidden = false;
  requestAnimationFrame(() => requestAnimationFrame(() => { veil.hidden = true; }));
  setTimeout(() => veil.remove(), 400);
})();
