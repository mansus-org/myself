package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/querylog"
)

// The query log is what the server was asked, not what the brain knows: the three
// question verbs, who asked, and what came back — including the questions that
// failed, which are usually the interesting ones.
func TestQueryLogRecordsTheQuestions(t *testing.T) {
	e := newAdminEnv(t)
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	e.ok("GET", "/api/v1/search?q=bun", adminToken, "", 200, nil)
	e.ok("GET", "/api/v1/describe/relay", adminToken, "", 200, nil)
	e.ok("GET", "/api/v1/search?q=nowhere", adminToken, "", 200, nil)
	// The file keeps no vectors, so this one fails: a failed query is still a
	// query someone asked.
	e.fails("GET", "/api/v1/recall?q=bun", adminToken, "", http.StatusBadRequest, 1, "")
	// A write is not a question, and is not logged.
	if _, err := e.queries.Read("", 50); err != nil {
		t.Fatal(err)
	}

	res, err := e.queries.Read("", 50)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 {
		t.Fatalf("want 4 queries (remember is not one), got %d: %+v", res.Total, res.Queries)
	}
	first := res.Queries[0]
	if first.Verb != "recall" || first.Error == "" {
		t.Fatalf("newest first, and a failure keeps its reason: %+v", first)
	}
	if first.Who != AdminName {
		t.Fatalf("the log says who asked: %+v", first)
	}
	var verbs []string
	for _, q := range res.Queries {
		verbs = append(verbs, q.Verb)
	}
	if strings.Join(verbs, ",") != "recall,search,describe,search" {
		t.Fatalf("newest first: %v", verbs)
	}
	for _, q := range res.Queries {
		if q.Time == "" || q.Took < 0 {
			t.Fatalf("every entry says when: %+v", q)
		}
	}
	search := res.Queries[1]
	if search.Text != "nowhere" || search.Hits != 0 || len(search.Facts) != 0 {
		t.Fatalf("a search that found nothing is still recorded: %+v", search)
	}
	found := res.Queries[3]
	if found.Text != "bun" || found.Hits != 1 || len(found.Facts) != 1 || found.Facts[0] != "relay uses bun for the backend" {
		t.Fatalf("the answer is named, not copied: %+v", found)
	}
	described := res.Queries[2]
	if described.Verb != "describe" || described.Text != "relay" || described.Hits < 2 {
		t.Fatalf("describe is logged with the entity's facts: %+v", described)
	}
}

// The endpoint is the same answer the CLI prints, newest first, filtered by verb,
// and open to a read token: everything in it is a fact that token could already
// read.
func TestQueriesEndpoint(t *testing.T) {
	e := newAdminEnv(t)
	e.ok("POST", "/api/v1/remember", adminToken, seed, 200, nil)
	e.ok("GET", "/api/v1/search?q=bun", adminToken, "", 200, nil)
	e.ok("GET", "/api/v1/describe/relay", adminToken, "", 200, nil)
	_, reader := e.token("laptop", "read")
	e.ok("GET", "/api/v1/search?q=software", reader, "", 200, nil)

	var got struct {
		Queries []querylog.Entry `json:"queries"`
		Total   int              `json:"total"`
		Verb    string           `json:"verb"`
	}
	e.ok("GET", "/api/v1/queries", reader, "", 200, &got)
	if got.Total != 3 || len(got.Queries) != 3 {
		t.Fatalf("three questions: %+v", got)
	}
	if got.Queries[0].Who != "laptop" {
		t.Fatalf("the newest is the reader's: %+v", got.Queries[0])
	}
	if len(got.Queries[0].Facts) == 0 {
		t.Fatalf("the answer should be named: %+v", got.Queries[0])
	}

	e.ok("GET", "/api/v1/queries?verb=search", reader, "", 200, &got)
	if got.Verb != "search" || len(got.Queries) != 2 || got.Total != 3 {
		t.Fatalf("filtered by verb, total unfiltered: %+v", got)
	}
	e.ok("GET", "/api/v1/queries?n=1", reader, "", 200, &got)
	if len(got.Queries) != 1 || got.Queries[0].Verb != "search" {
		t.Fatalf("-n 1 keeps the newest: %+v", got)
	}
	e.fails("GET", "/api/v1/queries?verb=dream", reader, "", http.StatusBadRequest, 1, "")
	e.fails("GET", "/api/v1/queries?n=zero", reader, "", http.StatusBadRequest, 1, "")
	e.fails("GET", "/api/v1/queries?n=0", reader, "", http.StatusBadRequest, 1, "")
	e.fails("GET", "/api/v1/queries", "", "", 401, 3, "")
}

// Recall is logged with the model that answered, because a score means nothing
// without it.
func TestQueryLogNamesTheModel(t *testing.T) {
	vs := newVecStore("relay uses bun", "relay is project")
	ts, q := vecEnvLog(t, vs, wordEmbedder{})
	if _, err := brain.Index(vs, wordEmbedder{}, false); err != nil {
		t.Fatal(err)
	}
	if code, _ := doAuth(t, ts, "GET", "/api/v1/recall?q=relay+uses+bun&n=1", adminToken); code != 200 {
		t.Fatalf("recall: %d", code)
	}
	res, err := q.Read("recall", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Queries) != 1 || res.Queries[0].Model != "test-model" || res.Queries[0].Hits != 1 {
		t.Fatalf("recall is logged with its model: %+v", res.Queries)
	}
	line := res.Queries[0].Line()
	if !strings.Contains(line, "with test-model") {
		t.Fatalf("the line says which model read it: %q", line)
	}
}

// A server started without a query log answers the endpoint with an empty list
// rather than an error: the log is a convenience, and its absence is not a fault
// of the caller's.
func TestQueriesWithoutALog(t *testing.T) {
	dir := t.TempDir()
	srv, err := New(Config{File: filepath.Join(dir, "brain.jsonl"), AdminHash: HashToken(adminToken), Log: &syncBuffer{}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	code, body := doAuth(t, ts, "GET", "/api/v1/queries", adminToken)
	if code != 200 {
		t.Fatalf("queries: %d %s", code, body)
	}
	var got struct {
		Queries []querylog.Entry `json:"queries"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Total != 0 || len(got.Queries) != 0 {
		t.Fatalf("no log, no queries: %s (%v)", body, err)
	}
}
