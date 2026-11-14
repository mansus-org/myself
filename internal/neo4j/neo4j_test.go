package neo4j

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/model"
)

// fake is a stand-in for the Neo4j HTTP transaction endpoint. It records every
// request body and replies with whatever the test queued, so the store is tested
// on what it actually sends — the same shape the real endpoint documents.
type fake struct {
	t        *testing.T
	requests []commitBody
	replies  []string
	auth     [2]string
	path     string
}

func newFake(t *testing.T) (*fake, *Store) {
	t.Helper()
	f := &fake{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var cb commitBody
		if err := json.Unmarshal(body, &cb); err != nil {
			t.Fatalf("store sent unreadable JSON: %v", err)
		}
		f.requests = append(f.requests, cb)
		f.path = r.URL.Path
		u, p, ok := r.BasicAuth()
		if !ok {
			t.Fatal("store sent no basic auth")
		}
		f.auth = [2]string{u, p}
		reply := `{"results":[],"errors":[]}`
		if len(f.replies) > 0 {
			reply = f.replies[0]
			f.replies = f.replies[1:]
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	st := New(Config{Base: srv.URL, User: "neo4j", Password: "secret", Database: "neo4j"}, io.Discard)
	return f, st
}

func rowsReply(cols string, rows ...string) string {
	data := make([]string, 0, len(rows))
	for _, r := range rows {
		data = append(data, `{"row":[`+r+`]}`)
	}
	return `{"results":[{"columns":[` + cols + `],"data":[` + strings.Join(data, ",") + `]}],"errors":[]}`
}

func TestFromEnv(t *testing.T) {
	if _, ok, err := FromEnv(func(string) string { return "" }); ok || err != nil {
		t.Fatalf("unset means the file store: ok=%v err=%v", ok, err)
	}
	cfg, ok, err := FromEnv(func(k string) string {
		if k == EnvURL {
			return "http://neo4j.neo4j.svc.cluster.local:7474/"
		}
		if k == EnvPassword {
			return "pw"
		}
		return ""
	})
	if !ok || err != nil {
		t.Fatalf("expected the backend selected: %v", err)
	}
	if cfg.Base != "http://neo4j.neo4j.svc.cluster.local:7474" {
		t.Fatalf("trailing slash not trimmed: %q", cfg.Base)
	}
	if cfg.User != DefaultUser || cfg.Database != DefaultDatabase || cfg.Timeout != DefaultTimeout {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	// A URL without a password is a misconfiguration, not "use the file": the
	// silent version of that writes beliefs somewhere nobody is looking.
	if _, _, err := FromEnv(func(k string) string {
		if k == EnvURL {
			return "http://x:7474"
		}
		return ""
	}); err == nil || !strings.Contains(err.Error(), EnvPassword) {
		t.Fatalf("missing password: want an error naming %s, got %v", EnvPassword, err)
	}
	if _, _, err := FromEnv(func(k string) string {
		if k == EnvURL {
			return "neo4j://x:7687"
		}
		return "p"
	}); err == nil {
		t.Fatal("a bolt URL must be refused: this store speaks HTTP")
	}
	if _, _, err := FromEnv(func(k string) string {
		switch k {
		case EnvURL:
			return "http://x:7474"
		case EnvPassword:
			return "p"
		case EnvTimeout:
			return "soon"
		}
		return ""
	}); err == nil {
		t.Fatal("a bad timeout must be refused")
	}
}

func TestLoadReadsEveryFieldInUUIDOrder(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{rowsReply(
		`"uuid","subject","relation","object","time"`,
		`"01a1","relay","uses","bun","2026-11-01T00:00:00Z"`,
		`"01a2","relay","is","project","2026-11-02T00:00:00Z"`,
	)}
	snap, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	recs := snap.Records
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	want := model.Record{UUID: "01a1", Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-11-01T00:00:00Z"}
	if recs[0] != want {
		t.Fatalf("record 0 = %+v, want %+v", recs[0], want)
	}
	if f.path != "/db/neo4j/tx/commit" {
		t.Fatalf("endpoint = %s", f.path)
	}
	if f.auth != [2]string{"neo4j", "secret"} {
		t.Fatalf("auth = %v", f.auth)
	}
	if !strings.Contains(f.requests[0].Statements[0].Statement, "ORDER BY r.uuid") {
		t.Fatal("Load must ask for uuid order: brain requires uuid-sorted records")
	}
}

func TestAppendSendsTheWholeBatchAsOneStatement(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{`{"results":[{"columns":[],"data":[]}],"errors":[]}`}
	recs := []model.Record{
		{UUID: "01a1", Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-11-01T00:00:00Z"},
		{UUID: "01a2", Subject: "relay", Relation: "is", Object: "project", Time: "2026-11-02T00:00:00Z"},
	}
	if err := st.Append(recs...); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 || len(f.requests[0].Statements) != 1 {
		t.Fatalf("a batch must be one statement in one transaction: %+v", f.requests)
	}
	stmt := f.requests[0].Statements[0]
	if !strings.HasPrefix(stmt.Statement, "UNWIND $rows") {
		t.Fatalf("append must be a parameterized UNWIND, got %q", stmt.Statement)
	}
	if !strings.Contains(stmt.Statement, "MERGE (r:Record {uuid: row.uuid})") {
		t.Fatal("append must MERGE on uuid, so a retry and an import are idempotent")
	}
	rows, _ := stmt.Parameters["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows in params, got %v", stmt.Parameters["rows"])
	}
	// No value is ever spliced into the query text.
	if strings.Contains(stmt.Statement, "bun") {
		t.Fatal("a value leaked into the statement")
	}
	if err := st.Append(); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 {
		t.Fatal("an empty batch must not reach the database")
	}
}

func TestErrorsSurfaceWithCodeAndMessage(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{`{"results":[],"errors":[{"code":"Neo.ClientError.Security.Unauthorized","message":"Invalid authentication credentials"}]}`}
	if _, err := st.Load(); err == nil ||
		!strings.Contains(err.Error(), "Neo.ClientError.Security.Unauthorized") {
		t.Fatalf("want the Neo4j code in the error, got %v", err)
	}
}

func TestNonJSONFailureStillReportsTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "Basic realm=\"Neo4j\"\n")
	}))
	t.Cleanup(srv.Close)
	st := New(Config{Base: srv.URL, User: "u", Password: "p", Database: "neo4j"}, io.Discard)
	if _, err := st.Load(); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("want HTTP 401 in the error, got %v", err)
	}
}

func TestEnsureSchemaIsIdempotentStatements(t *testing.T) {
	f, st := newFake(t)
	if err := st.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	if len(f.requests[0].Statements) != len(schema) {
		t.Fatalf("want %d statements, got %d", len(schema), len(f.requests[0].Statements))
	}
	for _, s := range f.requests[0].Statements {
		if !strings.Contains(s.Statement, "IF NOT EXISTS") {
			t.Fatalf("schema must be safe to re-apply: %q", s.Statement)
		}
	}
}

func TestStringNeverLeaksThePassword(t *testing.T) {
	st := New(Config{Base: "http://h:7474", User: "neo4j", Password: "hunter2", Database: "neo4j"}, nil)
	if s := st.String(); strings.Contains(s, "hunter2") || !strings.Contains(s, "neo4j@http://h:7474") {
		t.Fatalf("String() = %q", s)
	}
}
