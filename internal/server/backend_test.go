package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
)

// memStore stands in for an injected backend (the Neo4j store in production) so
// the server's backend path is covered without a database.
type memStore struct {
	mu    sync.Mutex
	recs  []model.Record
	loads int
}

func (m *memStore) Load() (*brain.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	return brain.NewSnapshot(append([]model.Record(nil), m.recs...)), nil
}

func (m *memStore) Append(recs ...model.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, recs...)
	return nil
}

func (m *memStore) Warn(string) {}

func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.recs)
}

// A backend replaces the file completely: writes land in the backend, reads come
// from it, and the JSONL path is never touched — otherwise a deployment would
// quietly keep two brains and answer from whichever it wrote last.
func TestBackendStoreReplacesTheFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "brain.jsonl")
	m := &memStore{}
	lb := &syncBuffer{}
	srv, err := New(Config{File: file, Store: m, AdminHash: HashToken(adminToken), Log: lb})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	call := func(method, path, body string) (int, string) {
		t.Helper()
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, ts.URL+path, rdr)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw)
	}

	code, body := call("POST", "/api/v1/remember",
		`{"facts":[{"subject":"relay","relation":"uses","object":"bun"}],"replace":false}`)
	if code != 200 {
		t.Fatalf("remember: %d %s", code, body)
	}
	if m.count() != 1 {
		t.Fatalf("the write went to %d records in the backend, want 1", m.count())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the backend is set but the file was created: %v", err)
	}

	code, body = call("GET", "/api/v1/describe/relay?depth=1&json=true", "")
	if code != 200 {
		t.Fatalf("describe: %d %s", code, body)
	}
	if !strings.Contains(body, "bun") {
		t.Fatalf("describe did not read from the backend: %s", body)
	}
	if m.loads == 0 {
		t.Fatal("the backend was never read")
	}

	// A retraction is a record like any other, so forget must reach the backend too.
	var got struct {
		Matches []struct {
			UUID string `json:"uuid"`
		} `json:"matches"`
	}
	code, body = call("GET", "/api/v1/search?q=bun", "")
	if code != 200 {
		t.Fatalf("search: %d %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("search reply %s: %v", body, err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("search found %d records in the backend, want 1", len(got.Matches))
	}
}
