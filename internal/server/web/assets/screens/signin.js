// Sign-in: the token form, its message and its submit.
import { api, auth } from "../api.js";
import { h, errorText } from "../dom.js";
import { mBitmap } from "../bitmap.js";

export function renderSignIn(view, message, onSignedIn) {
  const input = h("input", {
    id: "token", name: "token", type: "password", required: true, autocomplete: "off",
    spellcheck: "false", autocapitalize: "off", "aria-describedby": "token-help",
  });
  const remember = h("input", { id: "remember", type: "checkbox" });
  const err = h("p", { class: "form-error", role: "alert", hidden: !message }, message || "");
  const submit = h("button", { type: "submit", class: "btn btn--ink" }, "Sign in");
  const form = h("form", { class: "signin__form", novalidate: true },
    h("label", { for: "token", class: "label" }, "Token"),
    input,
    h("p", { id: "token-help", class: "help" },
      "The admin token, or an agent token that starts with mys_. Ask whoever runs this server for one."),
    h("label", { class: "check" }, remember,
      h("span", {}, "Remember on this device"),
    ),
    h("p", { class: "help" }, "Unchecked, the token is forgotten when you close this tab. Checked, it stays in this browser until you sign out."),
    err,
    submit,
  );
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const token = input.value.trim();
    if (!token) {
      err.textContent = "Paste a token first.";
      err.hidden = false;
      input.focus();
      return;
    }
    submit.disabled = true;
    submit.textContent = "Checking…";
    try {
      const id = await api("whoami", { token });
      auth.save(token, remember.checked);
      auth.identity = id;
      onSignedIn();
    } catch (ex) {
      err.textContent = ex.status === 401 ? "That token was refused: it is invalid or was revoked." : errorText(ex);
      err.hidden = false;
      submit.disabled = false;
      submit.textContent = "Sign in";
      input.focus();
    }
  });
  view.replaceChildren(
    h("section", { class: "signin" },
      h("div", { class: "signin__m" }, mBitmap()),
      h("div", { class: "signin__body" },
        h("h1", { class: "title signin__title" }, "myself"),
        h("p", { class: "statement" }, "Browse, search and fix what your agents remember."),
        form,
      ),
    ),
  );
  input.focus();
}
