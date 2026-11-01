package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mansus-org/myself/internal/store"
)

const adminToken = "test-admin-token"

type env struct {
	t    *testing.T
	srv  *Server
	ts   *httptest.Server
	file string
	log  *syncBuffer
}

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newEnv(t *testing.T, adminHash string) *env {
	t.Helper()
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	lb := &syncBuffer{}
	site := fstest.MapFS{"index.html": {Data: []byte("<h1>myself</h1>")}}
	srv, err := New(Config{File: file, Site: site, Skill: []byte("---\nname: myself\n---\n"), AdminHash: adminHash, Log: lb})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{t, srv, ts, file, lb}
}

func newAdminEnv(t *testing.T) *env { return newEnv(t, HashToken(adminToken)) }

// do sends a request and returns the status and body.
func (e *env) do(method, path, token, body string) (int, string) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// ok sends a request that must answer want, and decodes the body into v.
func (e *env) ok(method, path, token, body string, want int, v any) string {
	e.t.Helper()
	status, got := e.do(method, path, token, body)
	if status != want {
		e.t.Fatalf("%s %s: status %d, want %d; body %s", method, path, status, want, got)
	}
	if v != nil {
		if err := json.Unmarshal([]byte(got), v); err != nil {
			e.t.Fatalf("%s %s: %v; body %s", method, path, err, got)
		}
	}
	return got
}

type apiError struct {
	Error    string `json:"error"`
	ExitCode int    `json:"exit_code"`
}

func (e *env) fails(method, path, token, body string, status, exit int, msg string) {
	e.t.Helper()
	var ae apiError
	e.ok(method, path, token, body, status, &ae)
	if ae.ExitCode != exit || (msg != "" && ae.Error != msg) {
		e.t.Fatalf("%s %s: got %+v, want exit %d error %q", method, path, ae, exit, msg)
	}
}

func (e *env) token(name, scope string) (string, string) {
	e.t.Helper()
	var ct CreatedToken
	e.ok("POST", "/api/v1/tokens", adminToken, fmt.Sprintf(`{"name":%q,"scope":%q}`, name, scope), 201, &ct)
	if !strings.HasPrefix(ct.Token, TokenPrefix) || len(ct.Token) != len(TokenPrefix)+64 {
		e.t.Fatalf("token format: %q", ct.Token)
	}
	return ct.ID, ct.Token
}

const seed = `{"facts":[
 {"subject":"relay","relation":"is","object":"project"},
 {"subject":"relay","relation":"uses","object":"bun for the backend"},
 {"subject":"project","relation":"are","object":"software"}]}`

func TestSiteAndHealth(t *testing.T) {
	e := newAdminEnv(t)
	if got := e.ok("GET", "/healthz", "", "", 200, nil); got != "ok\n" {
		t.Fatalf("healthz %q", got)
	}
	if got := e.ok("GET", "/", "", "", 200, nil); !strings.Contains(got, "<h1>myself</h1>") {
		t.Fatalf("landing %q", got)
	}
	if st, _ := e.do("POST", "/", "", "x"); st != 405 {
		t.Fatalf("POST /: %d", st)
	}
}

func TestDisabledWithoutAdminHash(t *testing.T) {
	e := newEnv(t, "")
	msg := "the API is disabled: MYSELF_ADMIN_TOKEN_SHA256 is not set on the server"
	for _, p := range []string{"/api/v1/whoami", "/api/v1/log", "/api/v1/nope"} {
		e.fails("GET", p, adminToken, "", 503, 3, msg)
	}
	e.fails("POST", "/api/v1/remember", "", seed, 503, 3, msg)
	e.ok("GET", "/healthz", "", "", 200, nil)
	e.ok("GET", "/", "", "", 200, nil)
}

func TestAuth(t *testing.T) {
	e := newAdminEnv(t)
	e.fails("GET", "/api/v1/log", "", "", 401, 3, "missing token: send the header Authorization: Bearer <token>")
	e.fails("GET", "/api/v1/log", "mys_nope", "", 401, 3, "invalid or revoked token")

	var who Identity
	e.ok("GET", "/api/v1/whoami", adminToken, "", 200, &who)
	if who != (Identity{"admin", "write", true}) {
		t.Fatalf("admin whoami %+v", who)
	}

	readID, read := e.token("reader", "read")
	_, write := e.token("writer", "write")
	e.ok("GET", "/api/v1/whoami", read, "", 200, &who)
	if who != (Identity{"reader", "read", false}) {
		t.Fatalf("reader whoami %+v", who)
	}
	e.ok("GET", "/api/v1/log", read, "", 200, nil)
	e.fails("POST", "/api/v1/remember", read, seed, 403, 3, "token reader is read-only; this endpoint needs a write token")
	e.fails("POST", "/api/v1/forget", read, `{"subject":"relay","relation":"is"}`, 403, 3, "")
	e.ok("POST", "/api/v1/remember", write, seed, 200, nil)

	// Admin-only endpoints refuse agent tokens, even write ones.
	for _, tok := range []string{read, write} {
		e.fails("GET", "/api/v1/tokens", tok, "", 403, 3, "this endpoint needs the admin token")
		e.fails("POST", "/api/v1/tokens", tok, `{"name":"x","scope":"read"}`, 403, 3, "")
		e.fails("DELETE", "/api/v1/tokens/"+readID, tok, "", 403, 3, "")
		e.fails("POST", "/api/v1/import", tok, "", 403, 3, "")
	}

	// Revoked tokens are rejected.
	e.ok("DELETE", "/api/v1/tokens/"+readID, adminToken, "", 200, nil)
	e.fails("GET", "/api/v1/log", read, "", 401, 3, "invalid or revoked token")
	e.fails("DELETE", "/api/v1/tokens/"+readID, adminToken, "", 404, 2, fmt.Sprintf("unknown token %q", readID))

	// One log line per request, with the token name and never a token.
	log := e.log.String()
	for _, secret := range []string{adminToken, read, write} {
		if strings.Contains(log, secret) {
			t.Fatal("a token leaked into the request log")
		}
	}
	if !strings.Contains(log, "GET /api/v1/whoami 200 ") || !strings.Contains(log, " reader\n") || !strings.Contains(log, "GET /api/v1/log 401 ") {
		t.Fatalf("request log:\n%s", log)
	}
}

func TestTokens(t *testing.T) {
	e := newAdminEnv(t)
	e.fails("POST", "/api/v1/tokens", adminToken, `{"name":"x","scope":"admin"}`, 400, 1, `scope must be "read" or "write"`)
	e.fails("POST", "/api/v1/tokens", adminToken, `{"name":" ","scope":"read"}`, 400, 1, "")
	e.fails("POST", "/api/v1/tokens", adminToken, `{"name":"x","scope":"read","extra":1}`, 400, 1, "")

	id, tok := e.token("bot", "write")
	var list []TokenInfo
	e.ok("GET", "/api/v1/tokens", adminToken, "", 200, &list)
	if len(list) != 1 || list[0].ID != id || list[0].Name != "bot" || list[0].Scope != "write" || list[0].LastUsed != nil {
		t.Fatalf("list %+v", list)
	}
	e.ok("GET", "/api/v1/whoami", tok, "", 200, nil)
	e.ok("GET", "/api/v1/tokens", adminToken, "", 200, &list)
	if list[0].LastUsed == nil {
		t.Fatal("last_used not set")
	}

	// Only the hash is stored, in a 0600 file, and it survives a restart.
	path := filepath.Join(filepath.Dir(e.file), "tokens.json")
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("tokens.json: %v %v", fi, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), tok) || !strings.Contains(string(raw), HashToken(tok)) {
		t.Fatalf("tokens.json holds the plaintext or no hash:\n%s", raw)
	}
	srv2, err := New(Config{File: e.file, AdminHash: HashToken(adminToken)})
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(srv2)
	defer ts2.Close()
	req, _ := http.NewRequest("GET", ts2.URL+"/api/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("token after restart: %v %v", resp.StatusCode, err)
	}
	resp.Body.Close()
}

func TestLastUsedThrottled(t *testing.T) {
	dir := t.TempDir()
	tk, err := OpenTokens(filepath.Join(dir, "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	_, plain, err := tk.Create("bot", "read", t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{t0, "2026-11-01T12:00:00Z"},
		{t0.Add(30 * time.Second), "2026-11-01T12:00:00Z"},
		{t0.Add(61 * time.Second), "2026-11-01T12:01:01Z"},
	} {
		info, ok, err := tk.Check(plain, c.at)
		if !ok || err != nil || info.LastUsed == nil || *info.LastUsed != c.want {
			t.Fatalf("at %v: %+v %v %v", c.at, info, ok, err)
		}
	}
	if _, ok, _ := tk.Check("mys_other", t0); ok {
		t.Fatal("unknown token accepted")
	}
}

type record struct {
	UUID     string `json:"uuid"`
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
	Time     string `json:"time"`
}

func TestRememberAndQueries(t *testing.T) {
	e := newAdminEnv(t)
	var rem struct {
		Remembered []record  `json:"remembered"`
		Forgotten  []record  `json:"forgotten"`
		Warnings   *[]string `json:"warnings"`
	}
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, &rem)
	if len(rem.Remembered) != 3 || len(rem.Forgotten) != 0 || rem.Warnings != nil || rem.Remembered[2].Relation != "is" {
		t.Fatalf("remember %+v", rem)
	}

	// Validation: same messages as the CLI, nothing written on error.
	e.fails("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"relay","relation":"built-with","object":"bun"}]}`, 400, 1,
		`unknown relation "built-with". use one of: is, are, has, uses, does, prefers, related`)
	e.fails("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"a","relation":"is","object":"b"},{"subject":"a","relation":"is","object":" "}]}`, 400, 1,
		"fact 2: empty object. give the object as free text")
	e.fails("POST", "/api/v1/remember", adminToken, `{"facts":[]}`, 400, 1, "remember needs at least one fact")
	e.fails("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"a","relation":"is","object":"b","by":"x"}]}`, 400, 1, "")
	e.fails("POST", "/api/v1/remember", adminToken, `not json`, 400, 1, "")

	// The "is" sentence warning.
	got := e.ok("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"bun","relation":"is","object":"a fast all in one toolkit"}]}`, 200, nil)
	if !strings.HasSuffix(got, `"forgotten":[],"warnings":["\"is\" object looks like a sentence; use a short type name and put details in has or does"]}`+"\n") {
		t.Fatalf("warning: %s", got)
	}

	// --replace.
	e.ok("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"relay","relation":"uses","object":"go"}],"replace":true}`, 200, &rem)
	if len(rem.Forgotten) != 1 || rem.Forgotten[0].Relation != "forget" {
		t.Fatalf("replace %+v", rem)
	}

	var desc struct {
		Entity   string  `json:"entity"`
		At       *string `json:"at"`
		Entities []struct {
			Name  string   `json:"name"`
			Depth int      `json:"depth"`
			Out   []record `json:"out"`
			In    []record `json:"in"`
		} `json:"entities"`
	}
	e.ok("GET", "/api/v1/describe/Relay?depth=1", adminToken, "", 200, &desc)
	if desc.Entity != "relay" || desc.At != nil || len(desc.Entities) != 2 || desc.Entities[1].Name != "project" || len(desc.Entities[0].Out) != 2 {
		t.Fatalf("describe %+v", desc)
	}
	e.ok("GET", "/api/v1/describe/relay?at=2000-01-01", adminToken, "", 404, nil)
	e.ok("GET", "/api/v1/describe/relay?at=2999-01-01", adminToken, "", 200, &desc)
	if desc.At == nil || *desc.At != "2999-01-01" {
		t.Fatalf("describe at %+v", desc.At)
	}
	e.fails("GET", "/api/v1/describe/nope", adminToken, "", 404, 2, `unknown entity "nope"`)
	e.fails("GET", "/api/v1/describe/relay?depth=-1", adminToken, "", 400, 1, "--depth must not be negative")
	e.fails("GET", "/api/v1/describe/relay?depth=x", adminToken, "", 400, 1, "")
	e.fails("GET", "/api/v1/describe/relay?at=yesterday", adminToken, "", 400, 1, `--at: bad time "yesterday": use RFC 3339 or YYYY-MM-DD`)

	var ls struct {
		Type     *string  `json:"type"`
		Entities []string `json:"entities"`
	}
	e.ok("GET", "/api/v1/ls?type=software", adminToken, "", 200, &ls)
	if ls.Type == nil || *ls.Type != "software" || strings.Join(ls.Entities, ",") != "project,relay" {
		t.Fatalf("ls %+v", ls)
	}
	e.ok("GET", "/api/v1/ls", adminToken, "", 200, &ls)
	if ls.Type != nil || len(ls.Entities) == 0 {
		t.Fatalf("ls types %+v", ls)
	}
	e.fails("GET", "/api/v1/ls?type=nope", adminToken, "", 404, 2, `unknown type "nope"`)

	if got := e.ok("GET", "/api/v1/relations", adminToken, "", 200, nil); got != `{"relations":["is","are","has","uses","does","prefers","related"]}`+"\n" {
		t.Fatalf("relations %s", got)
	}

	var search struct {
		Matches []record `json:"matches"`
	}
	e.ok("GET", "/api/v1/search?q=GO", adminToken, "", 200, &search)
	if len(search.Matches) != 1 || search.Matches[0].Object != "go" {
		t.Fatalf("search %+v", search)
	}
	e.ok("GET", "/api/v1/search?q=RELAY&n=1", adminToken, "", 200, &search)
	if len(search.Matches) != 1 || search.Matches[0].Object != "project" {
		t.Fatalf("search -n %+v", search)
	}
	e.fails("GET", "/api/v1/search?q=RELAY&n=0", adminToken, "", 400, 1, "-n must be at least 1")
	e.fails("GET", "/api/v1/search", adminToken, "", 400, 1, "search needs <text>")

	var path struct {
		From string   `json:"from"`
		To   string   `json:"to"`
		Path []record `json:"path"`
	}
	e.ok("GET", "/api/v1/path?from=relay&to=software", adminToken, "", 200, &path)
	if path.From != "relay" || path.To != "software" || len(path.Path) != 2 {
		t.Fatalf("path %+v", path)
	}
	e.fails("GET", "/api/v1/path?from=relay&to=software&max_depth=1", adminToken, "", 404, 2, `no path from "relay" to "software" within 1 hops`)
	e.fails("GET", "/api/v1/path?from=relay", adminToken, "", 400, 1, "path needs <a> and <b>")
	e.fails("GET", "/api/v1/path?from=relay&to=software&max_depth=0", adminToken, "", 400, 1, "--max-depth must be at least 1")

	var recs struct {
		Records []record `json:"records"`
	}
	e.ok("GET", "/api/v1/log", adminToken, "", 200, &recs)
	if len(recs.Records) != 6 {
		t.Fatalf("log %d records", len(recs.Records))
	}
	e.ok("GET", "/api/v1/log?n=2", adminToken, "", 200, &recs)
	if len(recs.Records) != 2 || recs.Records[1].Object != "go" {
		t.Fatalf("log -n %+v", recs)
	}
	e.ok("GET", "/api/v1/log?since=2999-01-01", adminToken, "", 200, &recs)
	if len(recs.Records) != 0 {
		t.Fatalf("log since %+v", recs)
	}
	e.fails("GET", "/api/v1/log?n=0", adminToken, "", 400, 1, "-n must be at least 1")
	e.fails("GET", "/api/v1/log?since=x", adminToken, "", 400, 1, `--since: bad time "x": use RFC 3339 or YYYY-MM-DD`)

	e.ok("GET", "/api/v1/history/relay?relation=uses", adminToken, "", 200, &recs)
	if len(recs.Records) != 3 || recs.Records[1].Relation != "forget" {
		t.Fatalf("history %+v", recs)
	}
	e.fails("GET", "/api/v1/history/nope", adminToken, "", 404, 2, `unknown subject "nope"`)
	e.fails("GET", "/api/v1/history/relay?relation=x", adminToken, "", 400, 1, "")

	var prime struct {
		Budget    int    `json:"budget"`
		Truncated bool   `json:"truncated"`
		Text      string `json:"text"`
	}
	e.ok("GET", "/api/v1/prime", adminToken, "", 200, &prime)
	if prime.Budget != 2000 || prime.Truncated || !strings.Contains(prime.Text, "relay\n  is project\n") {
		t.Fatalf("prime %+v", prime)
	}
	e.ok("GET", "/api/v1/prime?budget=10", adminToken, "", 200, &prime)
	if !prime.Truncated {
		t.Fatalf("prime budget %+v", prime)
	}
	e.fails("GET", "/api/v1/prime?budget=0", adminToken, "", 400, 1, "--budget must be at least 1")

	e.ok("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"relays","relation":"is","object":"project"}]}`, 200, nil)
	if got := e.ok("GET", "/api/v1/dream", adminToken, "", 200, nil); got != `{"near_duplicates":[["relay","relays"]]}`+"\n" {
		t.Fatalf("dream %s", got)
	}

	e.fails("GET", "/api/v1/nope", adminToken, "", 404, 2, "no endpoint GET /api/v1/nope")
}

func TestForget(t *testing.T) {
	e := newAdminEnv(t)
	var rem struct {
		Remembered []record `json:"remembered"`
	}
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, &rem)

	var fg struct {
		Forgotten []record `json:"forgotten"`
	}
	e.ok("POST", "/api/v1/forget", adminToken, fmt.Sprintf(`{"uuid":%q,"reason":"wrong"}`, rem.Remembered[0].UUID), 200, &fg)
	if len(fg.Forgotten) != 1 || fg.Forgotten[0].Subject != rem.Remembered[0].UUID || fg.Forgotten[0].Object != "wrong" {
		t.Fatalf("forget uuid %+v", fg)
	}
	e.fails("POST", "/api/v1/forget", adminToken, fmt.Sprintf(`{"uuid":%q}`, rem.Remembered[0].UUID), 404, 2,
		"no live record matches uuid "+rem.Remembered[0].UUID)
	e.fails("POST", "/api/v1/forget", adminToken, `{"subject":"relay","relation":"uses","object":"go"}`, 404, 2,
		`no live record matches "relay uses go"`)
	e.ok("POST", "/api/v1/forget", adminToken, `{"subject":"relay","relation":"uses","object":"Bun for the backend"}`, 200, &fg)
	e.ok("POST", "/api/v1/forget", adminToken, `{"subject":"project","relation":"is"}`, 200, &fg)
	e.fails("POST", "/api/v1/forget", adminToken, `{}`, 400, 1, "forget needs <uuid>, or <subject> <relation> [<object...>]")
	e.fails("POST", "/api/v1/forget", adminToken, `{"uuid":"nope"}`, 400, 1, `"nope" is not a uuid`)
	e.fails("POST", "/api/v1/forget", adminToken, `{"subject":"relay","relation":"forget"}`, 400, 1, "")
	e.fails("POST", "/api/v1/forget", adminToken, fmt.Sprintf(`{"uuid":%q,"subject":"relay"}`, rem.Remembered[0].UUID), 400, 1, "")

	var recs struct {
		Records []record `json:"records"`
	}
	e.ok("GET", "/api/v1/log", adminToken, "", 200, &recs)
	if len(recs.Records) != 6 {
		t.Fatalf("log has %d records, want 6", len(recs.Records))
	}
}

func TestGraph(t *testing.T) {
	e := newAdminEnv(t)
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	var g struct {
		Nodes []struct {
			Name    string   `json:"name"`
			Types   []string `json:"types"`
			Records int      `json:"records"`
		} `json:"nodes"`
		Edges []struct {
			UUID, From, To, Relation string
		} `json:"edges"`
	}
	e.ok("GET", "/api/v1/graph", adminToken, "", 200, &g)
	got := fmt.Sprint(g)
	want := "{[{project [software] 2} {relay [project] 2} {software [] 1}]"
	if !strings.HasPrefix(got, want) || len(g.Edges) != 2 || g.Edges[0].From != "relay" || g.Edges[0].To != "project" || g.Edges[1].To != "software" {
		t.Fatalf("graph %s", got)
	}
	e.ok("GET", "/api/v1/graph?at=2000-01-01", adminToken, "", 200, &g)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("graph at %+v", g)
	}
	if body := e.ok("GET", "/api/v1/graph?at=2000-01-01", adminToken, "", 200, nil); body != `{"nodes":[],"edges":[]}`+"\n" {
		t.Fatalf("empty graph %s", body)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	a := newAdminEnv(t)
	a.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	a.ok("POST", "/api/v1/forget", adminToken, `{"subject":"relay","relation":"uses"}`, 200, nil)
	export := a.ok("GET", "/api/v1/export.jsonl", adminToken, "", 200, nil)
	file, _ := os.ReadFile(a.file)
	if export != string(file) || strings.Count(export, "\n") != 4 {
		t.Fatalf("export differs from the file:\n%s\n---\n%s", export, file)
	}

	b := newAdminEnv(t)
	b.ok("POST", "/api/v1/remember", adminToken, `{"facts":[{"subject":"bun","relation":"is","object":"tool"}]}`, 200, nil)
	var res ImportResult
	b.ok("POST", "/api/v1/import", adminToken, export, 200, &res)
	if res != (ImportResult{4, 0}) {
		t.Fatalf("import %+v", res)
	}
	// Importing again, or overlapping, skips by uuid. No trailing newline is fine.
	b.ok("POST", "/api/v1/import", adminToken, strings.TrimSuffix(export, "\n"), 200, &res)
	if res != (ImportResult{0, 4}) {
		t.Fatalf("re-import %+v", res)
	}
	var recs struct {
		Records []record `json:"records"`
	}
	b.ok("GET", "/api/v1/log", adminToken, "", 200, &recs)
	if len(recs.Records) != 5 {
		t.Fatalf("log after import: %d records", len(recs.Records))
	}
	var desc struct {
		Entities []struct {
			Out []record `json:"out"`
		} `json:"entities"`
	}
	b.ok("GET", "/api/v1/describe/relay", adminToken, "", 200, &desc)
	if len(desc.Entities[0].Out) != 1 {
		t.Fatalf("forget not imported: %+v", desc)
	}

	// Back up b and restore into a: a's records are skipped, bun is new.
	a.ok("POST", "/api/v1/import", adminToken, b.ok("GET", "/api/v1/export.jsonl", adminToken, "", 200, nil), 200, &res)
	if res != (ImportResult{1, 4}) {
		t.Fatalf("union %+v", res)
	}

	// Bad lines reject the whole import.
	before, _ := os.ReadFile(b.file)
	good := `{"uuid":"01a19418-81d8-7344-b9f2-f3a54bd13f7d","subject":"x","relation":"is","object":"y","time":"2026-10-31T22:33:16Z"}`
	for body, msg := range map[string]string{
		good + "\nnot json\n":                                         "line 2: not a record",
		strings.Replace(good, `"is"`, `"built-with"`, 1):              `line 1: relation "built-with" is not one of: is, are, has, uses, does, prefers, related`,
		strings.Replace(good, `"is"`, `"are"`, 1):                     `line 1: relation "are" is not one of: is, are, has, uses, does, prefers, related`,
		strings.Replace(good, "2026-10-31T22:33:16Z", "yesterday", 1): `line 1: time "yesterday" is not RFC 3339`,
		strings.Replace(good, "01a19418", "zz", 1):                    `line 1: uuid "zz-81d8-7344-b9f2-f3a54bd13f7d" is not a uuid`,
		strings.Replace(good, `"y"`, `""`, 1):                         "line 1: empty object",
		`{"uuid":"01a19418-81d8-7344-b9f2-f3a54bd13f7e","subject":"x","relation":"forget","object":"","time":"2026-10-31T22:33:16Z"}`: `line 1: forget subject "x" is not a uuid`,
	} {
		b.fails("POST", "/api/v1/import", adminToken, body, 400, 1, msg)
	}
	after, _ := os.ReadFile(b.file)
	if string(before) != string(after) {
		t.Fatal("a rejected import wrote to the store")
	}
}

func TestCorruptLineWarnings(t *testing.T) {
	e := newAdminEnv(t)
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	f, err := os.OpenFile(e.file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("garbage\n")
	f.Close()
	got := e.ok("GET", "/api/v1/describe/relay", adminToken, "", 200, nil)
	if !strings.HasSuffix(got, `,"warnings":["line 4: corrupt, skipped"]}`+"\n") {
		t.Fatalf("describe with a corrupt line: %s", got)
	}
}

// The cache notices writes by other processes (the CLI on the same file).
func TestCacheReloadsOnExternalWrite(t *testing.T) {
	e := newAdminEnv(t)
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	e.ok("GET", "/api/v1/describe/relay", adminToken, "", 200, nil)
	e.fails("GET", "/api/v1/describe/bun", adminToken, "", 404, 2, "")
	line := `{"uuid":"01a19418-81d8-7344-b9f2-f3a54bd13f7d","subject":"bun","relation":"is","object":"tool","time":"2026-10-31T22:33:16Z"}`
	f, _ := os.OpenFile(e.file, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line + "\n")
	f.Close()
	e.ok("GET", "/api/v1/describe/bun", adminToken, "", 200, nil)
}

func TestConcurrentWrites(t *testing.T) {
	e := newAdminEnv(t)
	_, tok := e.token("writer", "write")
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan string, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf(`{"facts":[{"subject":"e%d","relation":"is","object":"thing"},{"subject":"counter","relation":"has","object":"v%d"}],"replace":true}`, i, i)
			if st, got := e.do("POST", "/api/v1/remember", tok, body); st != 200 {
				errs <- got
			}
			if st, got := e.do("GET", "/api/v1/ls?type=thing", tok, ""); st != 200 {
				errs <- got
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
	recs, err := store.ReadFile(e.file, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ls struct {
		Entities []string `json:"entities"`
	}
	e.ok("GET", "/api/v1/ls?type=thing", tok, "", 200, &ls)
	if len(ls.Entities) != n {
		t.Fatalf("%d entities, want %d", len(ls.Entities), n)
	}
	// Writes are serialized, so --replace leaves exactly one live counter.
	var desc struct {
		Entities []struct {
			Out []record `json:"out"`
		} `json:"entities"`
	}
	e.ok("GET", "/api/v1/describe/counter", tok, "", 200, &desc)
	if len(desc.Entities[0].Out) != 1 {
		t.Fatalf("counter has %d live values", len(desc.Entities[0].Out))
	}
	// 2 facts per request plus one forget for every request after the first.
	if want := 2*n + n - 1; len(recs) != want {
		t.Fatalf("%d records in the file, want %d", len(recs), want)
	}
}

func TestSkill(t *testing.T) {
	e := newAdminEnv(t)
	st, body := e.do("GET", "/skill.md", "", "")
	if st != 200 || !strings.HasPrefix(body, "---\nname: myself\n") {
		t.Fatalf("skill: %d %q", st, body)
	}
	resp, err := http.Get(e.ts.URL + "/skill.md")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing headers: %v", resp.Header)
	}
	if st, _ := e.do("POST", "/skill.md", "", "x"); st != 405 {
		t.Fatalf("POST /skill.md: %d", st)
	}
}
