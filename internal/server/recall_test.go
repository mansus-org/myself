package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/embed"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/querylog"
)

// wordEmbedder is a deterministic stand-in for an embedding endpoint: a vector
// counts the words of the text, so a test can say which fact is closest without
// a model in the room.
type wordEmbedder struct{ model string }

var wordList = []string{"relay", "bun", "go", "project", "deploys", "nightly", "garden"}

func (e wordEmbedder) Embed(ctx context.Context, texts ...string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v := make([]float32, len(wordList))
		for i, w := range wordList {
			v[i] = float32(strings.Count(strings.ToLower(t), w))
		}
		out = append(out, v)
	}
	return out, nil
}

func (e wordEmbedder) Model() string {
	if e.model == "" {
		return "test-model"
	}
	return e.model
}

func (e wordEmbedder) Dimensions() int { return len(wordList) }

// vecStore is a backend that keeps a vector projection, which is what the Neo4j
// one is: the server's recall and index paths are covered without a database.
type vecStore struct {
	memStore
	vectors map[string][]float32
	from    map[string]string
	dims    int
}

func newVecStore(triples ...string) *vecStore {
	recs := make([]model.Record, 0, len(triples))
	for i, t := range triples {
		f := strings.SplitN(t, " ", 3)
		recs = append(recs, model.Record{
			UUID:     fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
			Subject:  f[0],
			Relation: f[1],
			Object:   f[2],
			Time:     "2026-01-01T00:00:00Z",
		})
	}
	return &vecStore{memStore: memStore{recs: recs}, vectors: map[string][]float32{}, from: map[string]string{}}
}

func (s *vecStore) EnsureVectorIndex(ctx context.Context, dims int) error {
	s.dims = dims
	return nil
}

func (s *vecStore) SearchVectors(ctx context.Context, vector []float32, k int) ([]brain.ScoredRecord, error) {
	type hit struct {
		rec   model.Record
		score float64
	}
	var hits []hit
	for _, r := range s.recs {
		v, ok := s.vectors[r.UUID]
		if !ok {
			continue
		}
		hits = append(hits, hit{r, cosineOf(vector, v)})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > k {
		hits = hits[:k]
	}
	out := make([]brain.ScoredRecord, 0, len(hits))
	for _, h := range hits {
		out = append(out, brain.ScoredRecord{Record: h.rec, Score: h.score})
	}
	return out, nil
}

func (s *vecStore) MissingEmbeddings(ctx context.Context, model string) (map[string]bool, error) {
	need := map[string]bool{}
	for _, r := range s.recs {
		if r.IsForget() {
			continue
		}
		if _, ok := s.vectors[r.UUID]; !ok || s.from[r.UUID] != model {
			need[r.UUID] = true
		}
	}
	return need, nil
}

func (s *vecStore) SetEmbeddings(ctx context.Context, vectors map[string][]float32, model string) error {
	for uuid, v := range vectors {
		s.vectors[uuid] = v
		s.from[uuid] = model
	}
	return nil
}

func cosineOf(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// vecEnv is a server whose brain is a vector backend with an embedding endpoint.
func vecEnv(t *testing.T, store brain.Store, em embed.Embedder) *httptest.Server {
	ts, _ := vecEnvLog(t, store, em)
	return ts
}

// vecEnvLog is vecEnv with the query log it records to, for the tests that read
// what was asked.
func vecEnvLog(t *testing.T, store brain.Store, em embed.Embedder) (*httptest.Server, *querylog.Log) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "brain.jsonl")
	q := querylog.Open(querylog.Path(file), nil)
	srv, err := New(Config{
		File:      file,
		Store:     store,
		Embedder:  em,
		AdminHash: HashToken(adminToken),
		Log:       &syncBuffer{},
		QueryLog:  q,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, q
}

// doAuth sends one request with a bearer token and returns the status and body.
func doAuth(t *testing.T, ts *httptest.Server, method, path, token string, body ...string) (int, string) {
	t.Helper()
	var rd io.Reader
	if len(body) > 0 {
		rd = strings.NewReader(body[0])
	}
	req, err := http.NewRequest(method, ts.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

func TestRecallEndpoint(t *testing.T) {
	vs := newVecStore("relay uses bun", "relay is project", "relay deploys nightly")
	ts := vecEnv(t, vs, wordEmbedder{})
	if _, err := brain.Index(vs, wordEmbedder{}, false); err != nil {
		t.Fatal(err)
	}
	code, body := doAuth(t, ts, "GET", "/api/v1/recall?q=what+does+relay+run+on", adminToken)
	if code != 200 {
		t.Fatalf("recall: %d %s", code, body)
	}
	var got struct {
		Matches []struct {
			UUID     string  `json:"uuid"`
			Subject  string  `json:"subject"`
			Relation string  `json:"relation"`
			Object   string  `json:"object"`
			Time     string  `json:"time"`
			Score    float64 `json:"score"`
		} `json:"matches"`
		Model string  `json:"model"`
		At    *string `json:"at"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("recall reply %s: %v", body, err)
	}
	if got.Model != "test-model" {
		t.Fatalf("the reply must name the model: %s", body)
	}
	if len(got.Matches) == 0 || got.Matches[0].Object != "bun" {
		t.Fatalf("the bun fact should answer first: %s", body)
	}
	if got.Matches[0].Score <= 0 || got.Matches[0].UUID == "" {
		t.Fatalf("a match is a record plus its score: %s", body)
	}

	// -n and --min reach the brain through the query string.
	code, body = doAuth(t, ts, "GET", "/api/v1/recall?q=relay&n=1", adminToken)
	if code != 200 {
		t.Fatalf("recall n: %d %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || len(got.Matches) != 1 {
		t.Fatalf("recall n=1: %d %s (%v)", code, body, err)
	}
	code, body = doAuth(t, ts, "GET", "/api/v1/recall?q=relay&min=0.99", adminToken)
	if code != 200 {
		t.Fatalf("recall min: %d %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	for _, m := range got.Matches {
		if m.Score < 0.99 {
			t.Fatalf("min did not reach the brain: %s", body)
		}
	}

	// The graph around a match comes back with it, and graph=0 leaves it out:
	// the same command must not answer differently because it came over HTTP.
	type ctxFact struct {
		UUID string `json:"uuid"`
		Hop  int    `json:"hop"`
	}
	var withGraph struct {
		Matches []struct {
			UUID    string    `json:"uuid"`
			Context []ctxFact `json:"context"`
		} `json:"matches"`
		Graph int `json:"graph"`
	}
	code, body = doAuth(t, ts, "GET", "/api/v1/recall?q=relay+uses+bun&n=1", adminToken)
	if code != 200 {
		t.Fatalf("recall with the graph: %d %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &withGraph); err != nil {
		t.Fatal(err)
	}
	if withGraph.Graph != brain.DefaultRecallGraph || len(withGraph.Matches) != 1 || len(withGraph.Matches[0].Context) == 0 {
		t.Fatalf("the match should have come with what it is about: %s", body)
	}
	for _, c := range withGraph.Matches[0].Context {
		if c.Hop != 1 || c.UUID == withGraph.Matches[0].UUID {
			t.Fatalf("context is reached, not matched: %s", body)
		}
	}
	code, body = doAuth(t, ts, "GET", "/api/v1/recall?q=relay+uses+bun&n=1&graph=0", adminToken)
	if code != 200 {
		t.Fatalf("recall graph=0: %d %s", code, body)
	}
	var alone struct {
		Matches []struct {
			Context []ctxFact `json:"context"`
		} `json:"matches"`
		Graph int `json:"graph"`
	}
	if err := json.Unmarshal([]byte(body), &alone); err != nil {
		t.Fatal(err)
	}
	if alone.Graph != 0 || len(alone.Matches[0].Context) != 0 {
		t.Fatalf("graph=0 asked for the matches alone: %s", body)
	}
}

// The MCP tool is the same command with the same choices, so it carries the graph
// the same way: an agent gets what a match is about without a second call.
func TestRecallToolCarriesTheGraph(t *testing.T) {
	vs := newVecStore("relay uses bun", "relay is project", "bun is runtime")
	ts := vecEnv(t, vs, wordEmbedder{})
	if _, err := brain.Index(vs, wordEmbedder{}, false); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ts: ts}
	cs := e.session(adminToken)
	text, v, isErr := call(t, cs, "recall", map[string]any{"q": "relay uses bun", "n": 1})
	if isErr {
		t.Fatalf("recall: %q", text)
	}
	if !strings.Contains(text, "bun is runtime") {
		t.Fatalf("the graph should say what bun is: %q", text)
	}
	if v["graph"] == nil {
		t.Fatalf("the answer must say how wide the graph went: %v", v)
	}
	text, _, isErr = call(t, cs, "recall", map[string]any{"q": "relay uses bun", "n": 1, "graph": 0})
	if isErr || strings.Contains(text, "bun is runtime") {
		t.Fatalf("graph 0 asked for the matches alone: %q", text)
	}
}

func TestRecallEndpointRefusesABadRequest(t *testing.T) {
	vs := newVecStore("relay uses bun")
	ts := vecEnv(t, vs, wordEmbedder{})
	for _, c := range []struct {
		path string
		want string
	}{
		{"/api/v1/recall", "recall needs <text>"},
		{"/api/v1/recall?q=x&n=zero", "n"},
		{"/api/v1/recall?q=x&min=heavy", "min"},
		{"/api/v1/recall?q=x&min=9", "--min"},
		{"/api/v1/recall?q=x&graph=many", "graph"},
	} {
		code, body := doAuth(t, ts, "GET", c.path, adminToken)
		if code != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s, want 400 with %q", c.path, code, body, c.want)
		}
	}
}

// A brain without a vector projection says so. An empty match list would read as
// "nothing in the brain is close to this", which is a different answer and wrong.
func TestRecallOnAFileBrainSaysSo(t *testing.T) {
	e := newAdminEnv(t)
	code, body := e.do("GET", "/api/v1/recall?q=relay", adminToken, "")
	if code != http.StatusBadRequest || !strings.Contains(body, "vectors") {
		t.Fatalf("recall on the file: %d %s", code, body)
	}
	code, body = e.do("POST", "/api/v1/index", adminToken, `{}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "vectors") {
		t.Fatalf("index on the file: %d %s", code, body)
	}
}

// A server with no embedding endpoint refuses recall rather than guessing.
func TestRecallWithoutAnEndpointSaysSo(t *testing.T) {
	vs := newVecStore("relay uses bun")
	ts := vecEnv(t, vs, nil)
	code, body := doAuth(t, ts, "GET", "/api/v1/recall?q=relay", adminToken)
	if code != http.StatusBadRequest || !strings.Contains(body, "embedding") {
		t.Fatalf("recall without an endpoint: %d %s", code, body)
	}
}

func TestRecallScopeAndIndexWrite(t *testing.T) {
	vs := newVecStore("relay uses bun")
	ts := vecEnv(t, vs, wordEmbedder{})
	// Make a read-only token with the admin one.
	code, body := doAuth(t, ts, "POST", "/api/v1/tokens", adminToken, `{"name":"laptop","scope":"read"}`)
	if code != http.StatusCreated {
		t.Fatalf("create token: %d %s", code, body)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &tok); err != nil || tok.Token == "" {
		t.Fatalf("token reply: %s (%v)", body, err)
	}
	if code, body := doAuth(t, ts, "GET", "/api/v1/recall?q=relay", tok.Token); code != 200 {
		t.Fatalf("a read token may recall: %d %s", code, body)
	}
	if code, body := doAuth(t, ts, "POST", "/api/v1/index", tok.Token, `{}`); code != http.StatusForbidden {
		t.Fatalf("index is a write: %d %s", code, body)
	}
}

func TestIndexEndpoint(t *testing.T) {
	vs := newVecStore("relay uses bun", "relay is project")
	ts := vecEnv(t, vs, wordEmbedder{})
	code, body := doAuth(t, ts, "POST", "/api/v1/index", adminToken, `{}`)
	if code != 200 {
		t.Fatalf("index: %d %s", code, body)
	}
	var got struct {
		Embedded int    `json:"embedded"`
		Skipped  int    `json:"skipped"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("index reply %s: %v", body, err)
	}
	if got.Embedded != 2 || got.Model != "test-model" {
		t.Fatalf("index: %+v", got)
	}
	// The second call is the one an operator runs on a schedule: nothing to do.
	code, body = doAuth(t, ts, "POST", "/api/v1/index", adminToken, `{}`)
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Embedded != 0 || got.Skipped != 2 {
		t.Fatalf("a second index must do nothing: %d %s (%v)", code, body, err)
	}
	// rebuild re-embeds everything.
	code, body = doAuth(t, ts, "POST", "/api/v1/index", adminToken, `{"rebuild":true}`)
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Embedded != 2 || got.Skipped != 0 {
		t.Fatalf("rebuild: %d %s (%v)", code, body, err)
	}
	// An unknown field is a usage error, as everywhere else in the API.
	if code, body := doAuth(t, ts, "POST", "/api/v1/index", adminToken, `{"nope":1}`); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d %s", code, body)
	}
}
