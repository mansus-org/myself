package neo4j

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
)

// The compile-time proof that the backend answers the seam brain.Recall and
// brain.Index ask for.
var _ brain.VectorStore = (*Store)(nil)

// stubEmbedder is a deterministic stand-in for internal/embed: the vectors are
// fingerprints of the text, so a test can see which fact each one came from
// without a model in the room.
type stubEmbedder struct {
	dims  int
	model string
	err   error
	calls int
	texts []string
}

func (e *stubEmbedder) Embed(ctx context.Context, texts ...string) ([][]float32, error) {
	e.calls++
	e.texts = append(e.texts, texts...)
	if e.err != nil {
		return nil, e.err
	}
	if e.dims == 0 {
		e.dims = 4
	}
	out := make([][]float32, 0, len(texts))
	for i, t := range texts {
		v := make([]float32, e.dims)
		v[0] = float32(len(t))
		v[e.dims-1] = float32(i)
		out = append(out, v)
	}
	return out, nil
}

func (e *stubEmbedder) Model() string {
	if e.model == "" {
		return "stub-model"
	}
	return e.model
}

func (e *stubEmbedder) Dimensions() int { return e.dims }

// rowsParam reads the rows one request sent.
func rowsParam(t *testing.T, f *fake, request int) []map[string]any {
	t.Helper()
	stmts := f.requests[request].Statements
	raw, err := json.Marshal(stmts[0].Parameters["rows"])
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func fact(uuid, subject, relation, object string) model.Record {
	return model.Record{UUID: uuid, Subject: subject, Relation: relation, Object: object, Time: "2026-11-01T00:00:00Z"}
}

func TestAppendCarriesAVectorPerFact(t *testing.T) {
	f, st := newFake(t)
	st.UseEmbedder(&stubEmbedder{})
	recs := []model.Record{
		fact("01a1", "relay", "uses", "bun"),
		fact("01a2", "relay", "is", "project"),
		{UUID: "01a3", Subject: "01a1", Relation: "forget", Object: "changed my mind", Time: "2026-11-02T00:00:00Z"},
	}
	if err := st.Append(recs...); err != nil {
		t.Fatal(err)
	}
	rows := rowsParam(t, f, 0)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	for _, i := range []int{0, 1} {
		if rows[i]["embedding"] == nil {
			t.Fatalf("fact %d was written without a vector: %v", i, rows[i])
		}
		if rows[i]["embeddingModel"] != "stub-model" {
			t.Fatalf("the model must travel with its vectors: %v", rows[i])
		}
	}
	// A retraction is not a fact: embedding "01a1 forget changed my mind" would
	// let a recall answer with a fact nobody believes any more.
	if rows[2]["embedding"] != nil || rows[2]["embeddingModel"] != nil {
		t.Fatalf("a forget must not be embedded: %v", rows[2])
	}
	if got := strings.Join(st.embedder.(*stubEmbedder).texts, "|"); got != "relay uses bun|relay is project" {
		t.Fatalf("the sentence of the fact is what gets embedded, got %q", got)
	}
}

func TestAppendWritesTheFactWhenTheEmbedderIsDown(t *testing.T) {
	f, st := newFake(t)
	st.UseEmbedder(&stubEmbedder{err: errors.New("connection refused")})
	if err := st.Append(fact("01a1", "relay", "uses", "bun")); err != nil {
		t.Fatalf("an unreachable model must not lose a fact: %v", err)
	}
	rows := rowsParam(t, f, 0)
	if rows[0]["embedding"] != nil {
		t.Fatalf("no vector was had, so none should be sent: %v", rows[0])
	}
	if !strings.Contains(f.warn.String(), "not embedded") {
		t.Fatalf("the user must be told the projection is behind: %q", f.warn.String())
	}
}

func TestAppendRetriesWithoutVectorsWhenTheIndexRefusesThem(t *testing.T) {
	f, st := newFake(t)
	st.UseEmbedder(&stubEmbedder{dims: 384})
	f.replies = []string{
		`{"results":[],"errors":[{"code":"Neo.ClientError.Statement.ArgumentError","message":"Node property embedding has 384 dimensions, but the index expects 1536"}]}`,
		`{"results":[],"errors":[]}`,
	}
	if err := st.Append(fact("01a1", "relay", "uses", "bun")); err != nil {
		t.Fatalf("a fact is never lost to a model change: %v", err)
	}
	if len(f.requests) != 2 {
		t.Fatalf("want a second attempt without vectors, got %d requests", len(f.requests))
	}
	for _, row := range rowsParam(t, f, 1) {
		if row["embedding"] != nil || row["embeddingModel"] != nil {
			t.Fatalf("the retry must drop the derived properties: %v", row)
		}
		if row["uuid"] != "01a1" {
			t.Fatalf("the retry must write the same fact: %v", row)
		}
	}
	warn := f.warn.String()
	if !strings.Contains(warn, "index --rebuild") {
		t.Fatalf("the warning must say how to fix it: %q", warn)
	}
}

func TestEnsureVectorIndexSizesTheIndexAndWaitsForIt(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{
		`{"results":[],"errors":[]}`,
		rowsReply(`"name","state","populationPercent"`, fmt.Sprintf("%q,%q,%s", VectorIndexName, "POPULATING", "40.0")),
		rowsReply(`"name","state","populationPercent"`, fmt.Sprintf("%q,%q,%s", VectorIndexName, "ONLINE", "100.0")),
	}
	if err := st.EnsureVectorIndex(context.Background(), 1536); err != nil {
		t.Fatal(err)
	}
	create := f.requests[0].Statements[0].Statement
	if !strings.Contains(create, "CREATE VECTOR INDEX") || !strings.Contains(create, "IF NOT EXISTS") {
		t.Fatalf("creating the index must be safe to repeat: %q", create)
	}
	if !strings.Contains(create, "1536") || !strings.Contains(create, "cosine") {
		t.Fatalf("the index must be sized and cosine: %q", create)
	}
	if !strings.Contains(create, VectorIndexName) || strings.Contains(create, "`1536`") {
		t.Fatalf("the index name must be the one recall asks for: %q", create)
	}
	if len(f.requests) != 3 {
		t.Fatalf("want a create and two state checks, got %d requests", len(f.requests))
	}
	if !strings.Contains(f.requests[1].Statements[0].Statement, "SHOW VECTOR INDEXES") {
		t.Fatal("the wait must ask the database for the index state")
	}
}

func TestEnsureVectorIndexRefusesABadLength(t *testing.T) {
	_, st := newFake(t)
	for _, dims := range []int{0, -1, 1 << 20} {
		if err := st.EnsureVectorIndex(context.Background(), dims); err == nil {
			t.Errorf("%d dimensions: want an error", dims)
		}
	}
}

func TestEnsureVectorIndexGivesUpOnStillBuilding(t *testing.T) {
	f, st := newFake(t)
	st.indexWait = 150 * time.Millisecond
	f.replies = []string{
		`{"results":[],"errors":[]}`,
		rowsReply(`"name","state","populationPercent"`, fmt.Sprintf("%q,%q,%s", VectorIndexName, "POPULATING", "12.0")),
	}
	for i := 0; i < 50; i++ {
		f.replies = append(f.replies, rowsReply(`"name","state","populationPercent"`, fmt.Sprintf("%q,%q,%s", VectorIndexName, "POPULATING", "12.0")))
	}
	err := st.EnsureVectorIndex(context.Background(), 8)
	if !errors.Is(err, ErrIndexPopulating) {
		t.Fatalf("want ErrIndexPopulating, got %v", err)
	}
	if !strings.Contains(err.Error(), "12") {
		t.Fatalf("the message should say how far it got: %v", err)
	}
}

func TestEnsureVectorIndexStartsAnywayWhenTheStateIsUnreadable(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{
		`{"results":[],"errors":[]}`,
		`{"results":[],"errors":[{"code":"Neo.ClientError.Procedure.ProcedureNotFound","message":"There is no procedure 'show.vector.indexes'"}]}`,
	}
	if err := st.EnsureVectorIndex(context.Background(), 8); err != nil {
		t.Fatalf("the CREATE worked; an unreadable status is a warning, not a refusal to start: %v", err)
	}
	if !strings.Contains(f.warn.String(), "state of the vector index") {
		t.Fatalf("want a warning about the state: %q", f.warn.String())
	}
}

func TestSearchVectorsAsksTheIndexByNameAndKeepsScores(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{rowsReply(`"uuid","subject","relation","object","time","score"`,
		`"01a1","relay","uses","bun for the backend","2026-11-01T00:00:00Z",0.91`,
		`"01a2","relay","is","project","2026-11-02T00:00:00Z",0.44`,
	)}
	got, err := st.SearchVectors(context.Background(), []float32{1, 2, 3}, 5)
	if err != nil {
		t.Fatal(err)
	}
	stmt := f.requests[0].Statements[0]
	if !strings.Contains(stmt.Statement, "db.index.vector.queryNodes($index, $k, $vector)") {
		t.Fatalf("the index, k and vector must be parameters: %q", stmt.Statement)
	}
	if stmt.Parameters["index"] != VectorIndexName {
		t.Fatalf("searched the wrong index: %v", stmt.Parameters["index"])
	}
	if stmt.Parameters["k"].(float64) != 5 {
		t.Fatalf("k not sent: %v", stmt.Parameters["k"])
	}
	if len(got) != 2 {
		t.Fatalf("want 2 hits, got %d", len(got))
	}
	if got[0].Record.UUID != "01a1" || got[0].Record.Object != "bun for the backend" || got[0].Score != 0.91 {
		t.Fatalf("hit 0 = %+v", got[0])
	}
	if got[1].Score != 0.44 {
		t.Fatalf("hit 1 = %+v", got[1])
	}
}

func TestSearchVectorsNamesTheFailuresACanActOn(t *testing.T) {
	t.Run("nothing indexed yet", func(t *testing.T) {
		f, st := newFake(t)
		f.replies = []string{`{"results":[],"errors":[{"code":"Neo.ClientError.Schema.IndexNotFound","message":"There is no such index 'myself_embedding'"}]}`}
		_, err := st.SearchVectors(context.Background(), []float32{1}, 3)
		if !errors.Is(err, ErrNoVectorIndex) {
			t.Fatalf("want ErrNoVectorIndex, got %v", err)
		}
	})
	t.Run("still building", func(t *testing.T) {
		f, st := newFake(t)
		f.replies = []string{`{"results":[],"errors":[{"code":"Neo.ClientError.Statement.ExecutionFailed","message":"Could not perform vector search while index is in population state POPULATING"}]}`}
		_, err := st.SearchVectors(context.Background(), []float32{1}, 3)
		if !errors.Is(err, ErrIndexPopulating) {
			t.Fatalf("want ErrIndexPopulating, got %v", err)
		}
	})
	t.Run("anything else keeps the database's words", func(t *testing.T) {
		f, st := newFake(t)
		f.replies = []string{`{"results":[],"errors":[{"code":"Neo.DatabaseError.General.UnknownError","message":"boom"}]}`}
		_, err := st.SearchVectors(context.Background(), []float32{1}, 3)
		if err == nil || strings.Contains(err.Error(), "neo4j: the brain has no") || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestSearchVectorsRefusesAnEmptyQuestion(t *testing.T) {
	_, st := newFake(t)
	if _, err := st.SearchVectors(context.Background(), []float32{1}, 0); err == nil {
		t.Fatal("k below 1 asks nothing")
	}
	if _, err := st.SearchVectors(context.Background(), nil, 3); err == nil {
		t.Fatal("an empty vector asks nothing")
	}
}

func TestMissingEmbeddingsListsTheUUIDsThatNeedOne(t *testing.T) {
	f, st := newFake(t)
	f.replies = []string{rowsReply(`"uuid"`, `"01a1"`, `"01a2"`)}
	got, err := st.MissingEmbeddings(context.Background(), "stub-model")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["01a1"] || !got["01a2"] {
		t.Fatalf("got %v", got)
	}
	stmt := f.requests[0].Statements[0]
	if !strings.Contains(stmt.Statement, "r.relation <> 'forget'") {
		t.Fatalf("a forget never needs a vector: %q", stmt.Statement)
	}
	if !strings.Contains(stmt.Statement, "embedding IS NULL") || !strings.Contains(stmt.Statement, "embeddingModel") {
		t.Fatalf("a vector from another model counts as missing: %q", stmt.Statement)
	}
	if stmt.Parameters["model"] != "stub-model" {
		t.Fatalf("the model must be a parameter: %v", stmt.Parameters)
	}
}

func TestSetEmbeddingsWritesInBatchesOfOneTransactionEach(t *testing.T) {
	f, st := newFake(t)
	vectors := map[string][]float32{}
	for i := 0; i < 150; i++ {
		vectors[fmt.Sprintf("01%02d", i)] = []float32{float32(i)}
	}
	if err := st.SetEmbeddings(context.Background(), vectors, "stub-model"); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 3 {
		t.Fatalf("150 vectors: want 3 batches of %d, got %d", vectorBatch, len(f.requests))
	}
	stmt := f.requests[0].Statements[0]
	if !strings.HasPrefix(stmt.Statement, "UNWIND $rows AS row MATCH (r:Record {uuid: row.uuid})") {
		t.Fatalf("vectors must be matched by uuid and parameterized: %q", stmt.Statement)
	}
	if strings.Contains(stmt.Statement, "0101") {
		t.Fatal("a uuid leaked into the statement text")
	}
	if err := st.SetEmbeddings(context.Background(), nil, "stub-model"); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 3 {
		t.Fatal("no vectors must not reach the database")
	}
}

func TestStoreSatisfiesBrainStoreToo(t *testing.T) {
	// Recall and index load the log through the same store, so both seams have to
	// be on one object.
	var st any = New(Config{Base: "http://h", User: "u", Password: "p", Database: "neo4j"}, nil)
	if _, ok := st.(brain.VectorStore); !ok {
		t.Fatal("the backend must answer brain.VectorStore")
	}
}
