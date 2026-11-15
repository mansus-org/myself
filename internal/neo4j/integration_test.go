package neo4j

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
)

// TestAgainstRealNeo4j is the only test that needs a database. It is skipped
// unless both MYSELF_NEO4J_URL and MYSELF_NEO4J_PASSWORD are set, so `go test
// ./...` stays runnable anywhere, and it is what proves the store against the
// real endpoint rather than a fixture that could drift from it.
//
//	NEO4J_URL=... NEO4J_NEO4J_PASSWORD=... go test ./internal/neo4j -run Real -v
func TestAgainstRealNeo4j(t *testing.T) {
	cfg, ok, err := FromEnv(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skipf("%s and %s not set; skipping the real-database test", EnvURL, EnvPassword)
	}
	st := New(cfg, io.Discard)
	if err := st.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	marker := "myself-it-" + time.Now().UTC().Format("20060102150405")
	uuid := func(n string) string { return "00000000-0000-4000-8000-" + n + marker }
	recs := []model.Record{
		{UUID: uuid("1"), Subject: marker, Relation: "is", Object: "test", Time: "2026-01-01T00:00:00Z"},
		{UUID: uuid("2"), Subject: marker, Relation: "has", Object: "a sentence, not an entity", Time: "2026-01-02T00:00:00Z"},
	}
	cleanup := func() {
		if err := st.commit(context.Background(), []statement{{
			Statement:  "MATCH (r:Record) WHERE r.uuid IN $uuids DELETE r",
			Parameters: map[string]any{"uuids": []string{uuid("1"), uuid("2")}},
		}}); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	t.Cleanup(cleanup)

	if err := st.Append(recs...); err != nil {
		t.Fatal(err)
	}
	// The same batch again must not duplicate: MERGE on uuid is what makes a
	// retried write and `import` idempotent.
	if err := st.Append(recs...); err != nil {
		t.Fatal(err)
	}
	snap, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	var mine []model.Record
	for _, r := range snap.Records {
		if r.Subject == marker {
			mine = append(mine, r)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("want 2 records after a repeated append, got %d", len(mine))
	}
	if mine[0].Object != "test" || mine[1].Object != "a sentence, not an entity" {
		t.Fatalf("round trip lost a field: %+v", mine)
	}
	if !sortedByUUID(snap.Records) {
		t.Fatal("Load must return uuid-sorted records; brain depends on it")
	}
}

func sortedByUUID(recs []model.Record) bool {
	for i := 1; i < len(recs); i++ {
		if recs[i-1].UUID > recs[i].UUID {
			return false
		}
	}
	return true
}

// itVocab is the whole vocabulary of the integration test's embedder: a vector
// counts the words of the fact, so "nearest" means something a test can assert
// without a model. The point of the test is the database's side — that the index
// is created, populated and searched with the Cypher this package sends — not the
// model's.
var itVocab = []string{"relay", "bun", "go", "project", "deploys", "nightly", "garden", "place"}

type itEmbedder struct{}

func (itEmbedder) Embed(ctx context.Context, texts ...string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v := make([]float32, len(itVocab))
		for i, w := range itVocab {
			v[i] = float32(strings.Count(strings.ToLower(t), w))
		}
		out = append(out, v)
	}
	return out, nil
}

func (itEmbedder) Model() string   { return "myself-integration-test" }
func (itEmbedder) Dimensions() int { return len(itVocab) }

// TestAgainstRealNeo4jVectors is the proof that the vector projection works on a
// real database: the index statement, the search procedure, the state check and
// the batched writes are the parts a fake cannot vouch for. It is skipped, like
// TestAgainstRealNeo4j, unless the backend is configured.
func TestAgainstRealNeo4jVectors(t *testing.T) {
	cfg, ok, err := FromEnv(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skipf("%s and %s not set; skipping the real-database test", EnvURL, EnvPassword)
	}
	st := New(cfg, os.Stderr)
	if err := st.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	st.UseEmbedder(itEmbedder{})
	if err := st.EnsureVectorIndex(context.Background(), itEmbedder{}.Dimensions()); err != nil {
		t.Fatal(err)
	}

	marker := time.Now().UTC().Format("myself-vt-20060102150405")
	uuid := func(n string) string { return "00000000-0000-4000-8000-" + n + marker }
	recs := []model.Record{
		{UUID: uuid("1"), Subject: marker, Relation: "uses", Object: "bun", Time: "2026-01-01T00:00:00Z"},
		{UUID: uuid("2"), Subject: marker, Relation: "is", Object: "project", Time: "2026-01-02T00:00:00Z"},
		{UUID: uuid("3"), Subject: marker, Relation: "deploys", Object: "nightly", Time: "2026-01-03T00:00:00Z"},
	}
	t.Cleanup(func() {
		if err := st.commit(context.Background(), []statement{{
			Statement:  "MATCH (r:Record) WHERE r.uuid IN $uuids DELETE r",
			Parameters: map[string]any{"uuids": []string{uuid("1"), uuid("2"), uuid("3")}},
		}}); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	if err := st.Append(recs...); err != nil {
		t.Fatal(err)
	}
	// Every live fact got its vector on the way in, so there is nothing left to do.
	need, err := st.MissingEmbeddings(context.Background(), itEmbedder{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if need[r.UUID] {
			t.Fatalf("%s was written without a vector", r.UUID)
		}
	}
	// A model change is the one thing that makes every vector stale.
	need, err = st.MissingEmbeddings(context.Background(), "another-model")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if !need[r.UUID] {
			t.Fatalf("%s: a vector from another model is not this model's", r.UUID)
		}
	}

	// The search must find the bun fact for a bun question. The index covers the
	// whole database, so the assertion is about the hits carrying this test's
	// marker — nothing else in the brain can have it — and the best of those.
	q, err := itEmbedder{}.Embed(context.Background(), "bun")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := st.SearchVectors(context.Background(), q[0], 100)
	if err != nil {
		t.Fatal(err)
	}
	mine := map[string]bool{uuid("1"): true, uuid("2"): true, uuid("3"): true}
	var first brain.ScoredRecord
	found := false
	for _, h := range hits {
		if mine[h.UUID] && !found {
			first, found = h, true
		}
	}
	if !found {
		t.Fatalf("none of this test's facts came back: %d hits, none of them ours", len(hits))
	}
	if first.UUID != uuid("1") {
		t.Fatalf("the bun fact should answer a bun question first, got %+v", first)
	}
	if first.Score <= 0 {
		t.Fatalf("no score: %+v", first)
	}
}
