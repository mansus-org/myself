package neo4j

import (
	"io"
	"os"
	"testing"
	"time"

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
		if err := st.commit([]statement{{
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
