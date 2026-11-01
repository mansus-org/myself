// The REST API (SPEC section 10). Every call sends the bearer token; a 401
// signs the reader out through onUnauthorized.

const KEY = "myself.token";

export const auth = {
  token: localStorage.getItem(KEY) || sessionStorage.getItem(KEY) || "",
  identity: null,
  onUnauthorized: () => {},

  // save keeps the token for this tab only, or on this device if remember.
  save(token, remember) {
    this.token = token;
    sessionStorage.removeItem(KEY);
    localStorage.removeItem(KEY);
    (remember ? localStorage : sessionStorage).setItem(KEY, token);
  },

  clear() {
    this.token = "";
    this.identity = null;
    sessionStorage.removeItem(KEY);
    localStorage.removeItem(KEY);
  },

  get canWrite() {
    return !!this.identity && this.identity.scope === "write";
  },
  get isAdmin() {
    return !!this.identity && this.identity.admin;
  },
};

export class APIError extends Error {
  constructor(status, message, exitCode) {
    super(message);
    this.status = status;
    this.exitCode = exitCode;
  }
}

// api calls path (relative to /api/v1) and returns the decoded JSON body.
// query values that are null or undefined are left out.
export async function api(path, { method = "GET", query, body, token = auth.token } = {}) {
  let url = "/api/v1/" + path;
  if (query) {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) if (v != null) q.set(k, v);
    const s = q.toString();
    if (s) url += "?" + s;
  }
  const headers = { Authorization: "Bearer " + token, Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res;
  try {
    res = await fetch(url, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), cache: "no-store" });
  } catch {
    throw new APIError(0, "Can't reach the server. Check your connection and try again.", 0);
  }
  let data = null;
  try {
    data = await res.json();
  } catch {
    // An empty or non-JSON body; handled below by status.
  }
  if (res.ok) return data;
  const msg = data && data.error ? data.error : `The server answered ${res.status}.`;
  const err = new APIError(res.status, msg, data && data.exit_code);
  if (res.status === 401 && token === auth.token) auth.onUnauthorized(err);
  throw err;
}

export const enc = encodeURIComponent;

// The graph is shared by the pages that need entity names and types. It is
// kept for 15 s (agents write all the time), and writes here drop it.
let graphP = null;
let graphAt = 0;

export function graph() {
  if (!graphP || Date.now() - graphAt > 15000) {
    graphAt = Date.now();
    graphP = api("graph").catch((err) => {
      graphP = null;
      throw err;
    });
  }
  return graphP;
}

export function dropGraph() {
  graphP = null;
}
