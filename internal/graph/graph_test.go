package graph

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

func rec(n int, subj, rel, obj string) model.Record {
	return model.Record{
		UUID:     fmt.Sprintf("0192d7c4-%04x-7000-8000-%012x", n, n),
		Subject:  subj,
		Relation: rel,
		Object:   obj,
		Time:     "2026-10-29T16:00:00Z",
	}
}

// specGraph is the SPEC section 9 example after the failed record.
func specGraph() *Graph {
	return Build([]model.Record{
		rec(1, "relay", "is", "project"),
		rec(2, "relay", "uses", "bun for the backend, biome for linting, deployed on k3s"),
		rec(3, "project", "is", "software"),
	})
}

func uuids(rs []model.Record) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.UUID[9:13])
	}
	return out
}

func names(steps []Step) []string {
	var out []string
	for _, s := range steps {
		out = append(out, fmt.Sprintf("%s:%d", s.Name, s.Depth))
	}
	return out
}

func eq(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSpecExample(t *testing.T) {
	g := specGraph()
	eq(t, g.Entities(), []string{"project", "relay", "software"})
	eq(t, uuids(g.Out("relay")), []string{"0001", "0002"})
	eq(t, uuids(g.In("project")), []string{"0001"})
	eq(t, uuids(g.In("software")), []string{"0003"})
	eq(t, g.In("relay"), []model.Record(nil))
	eq(t, names(g.BFS("relay", 0)), []string{"relay:0"})
	eq(t, names(g.BFS("relay", 1)), []string{"relay:0", "project:1"})
	eq(t, names(g.BFS("relay", 2)), []string{"relay:0", "project:1", "software:2"})
	eq(t, names(g.BFS("software", 5)), []string{"software:0", "project:1", "relay:2"})
	eq(t, g.SubtypesOf("software"), []string{"project", "relay"})
	eq(t, g.SubtypesOf("project"), []string{"relay"})
	eq(t, g.SubtypesOf("relay"), []string(nil))
	eq(t, g.Types(), []string{"project", "software"})
	p, ok := g.ShortestPath("relay", "software", 6)
	if !ok {
		t.Fatal("no path")
	}
	eq(t, uuids(p), []string{"0001", "0003"})
	p, ok = g.ShortestPath("software", "relay", 6)
	if !ok {
		t.Fatal("no reverse path")
	}
	eq(t, uuids(p), []string{"0003", "0001"})
}

func TestNamesAreNormalized(t *testing.T) {
	g := Build([]model.Record{
		rec(1, "self", "prefers", "terse output"),
		rec(2, "relay-app", "is", "Project"),
	})
	if !g.Has("Relay App") || !g.Has("ME") || !g.Has("project") || g.Has("nope") {
		t.Error("Has")
	}
	eq(t, uuids(g.Out("me")), []string{"0001"})
	eq(t, g.SubtypesOf(" PROJECT. "), []string{"relay-app"})
}

func TestLiteralObjectMakesNoEdge(t *testing.T) {
	g := Build([]model.Record{rec(1, "relay", "uses", "bun for the backend")})
	eq(t, g.Entities(), []string{"relay"})
	eq(t, names(g.BFS("relay", 3)), []string{"relay:0"})
}

func TestObjectOfNonIsIsNotAnEntityButLinksToOne(t *testing.T) {
	g := Build([]model.Record{
		rec(1, "relay", "uses", "Bun"),
		rec(2, "bun", "is", "runtime"),
	})
	eq(t, g.Entities(), []string{"bun", "relay", "runtime"})
	eq(t, uuids(g.In("bun")), []string{"0001"})
	eq(t, g.SubtypesOf("runtime"), []string{"bun"}) // uses is not an is edge
	p, ok := g.ShortestPath("relay", "runtime", 6)
	eq(t, ok, true)
	eq(t, uuids(p), []string{"0001", "0002"})
}

func TestForgetRecordsIgnored(t *testing.T) {
	f := model.NewForget("x", "", time.Now())
	g := Build([]model.Record{f})
	eq(t, g.Entities(), []string(nil))
}

func TestCycles(t *testing.T) {
	g := Build([]model.Record{
		rec(1, "a", "is", "b"),
		rec(2, "b", "is", "c"),
		rec(3, "c", "is", "a"),
		rec(4, "d", "related", "d"),
	})
	eq(t, g.SubtypesOf("a"), []string{"b", "c"})
	eq(t, g.SubtypesOf("b"), []string{"a", "c"})
	eq(t, names(g.BFS("a", 10)), []string{"a:0", "b:1", "c:1"})
	eq(t, names(g.BFS("d", 3)), []string{"d:0"})
	eq(t, g.Types(), []string{"a", "b", "c"})
	_, ok := g.ShortestPath("a", "d", 6)
	eq(t, ok, false)
}

func TestPathCases(t *testing.T) {
	g := Build([]model.Record{
		rec(1, "a", "is", "b"),
		rec(2, "b", "is", "c"),
		rec(3, "c", "is", "d"),
		rec(4, "x", "is", "y"),
	})
	p, ok := g.ShortestPath("a", "a", 6)
	eq(t, ok, true)
	eq(t, len(p), 0)
	_, ok = g.ShortestPath("a", "d", 2)
	eq(t, ok, false) // needs 3 hops
	p, ok = g.ShortestPath("a", "d", 3)
	eq(t, ok, true)
	eq(t, len(p), 3)
	_, ok = g.ShortestPath("a", "y", 6)
	eq(t, ok, false)
	_, ok = g.ShortestPath("a", "ghost", 6)
	eq(t, ok, false)
}

func TestPathTieBreakDeterministic(t *testing.T) {
	// a -> m1 -> z and a -> m2 -> z: pick the lower neighbour name.
	recs := []model.Record{
		rec(1, "a", "related", "m2"),
		rec(2, "m2", "related", "z"),
		rec(3, "a", "related", "m1"),
		rec(4, "m1", "related", "z"),
		rec(5, "z", "has", "a literal fact"), // z must be a subject to be an entity
	}
	g := Build(recs)
	p, ok := g.ShortestPath("a", "z", 6)
	eq(t, ok, true)
	eq(t, uuids(p), []string{"0003", "0004"})

	// parallel records between the same pair: lowest uuid wins
	g = Build([]model.Record{
		rec(1, "a", "related", "b"),
		rec(2, "a", "uses", "b"),
		rec(3, "b", "has", "c"),
		rec(4, "c", "has", "b"), // c must be a subject to be an entity
	})
	p, _ = g.ShortestPath("a", "c", 6)
	eq(t, uuids(p), []string{"0001", "0003"})
}

func TestBFSOrderTiesByName(t *testing.T) {
	g := Build([]model.Record{
		rec(1, "hub", "related", "zed"),
		rec(2, "hub", "related", "alpha"),
		rec(3, "mid", "related", "hub"),
		rec(4, "alpha", "related", "leaf"),
		rec(5, "zed", "related", "alpha"),
		rec(6, "leaf", "related", "alpha"),
	})
	eq(t, names(g.BFS("hub", 2)), []string{"hub:0", "alpha:1", "mid:1", "zed:1", "leaf:2"})
	eq(t, g.BFS("ghost", 2), []Step(nil))
	eq(t, names(g.BFS("hub", -1)), []string{"hub:0"})
}

// genRecords builds n records over about n/5 entities: a chain of `is`
// records, `uses` links between entities, and free-text facts.
func genRecords(n int) []byte {
	var b bytes.Buffer
	ents := n/5 + 1
	for i := 0; i < n; i++ {
		var r model.Record
		switch i % 5 {
		case 0:
			r = rec(i, fmt.Sprintf("entity-%d", i%ents), "is", fmt.Sprintf("Entity %d", (i*7+1)%ents))
		case 1, 2:
			r = rec(i, fmt.Sprintf("entity-%d", (i*3)%ents), "uses", fmt.Sprintf("entity-%d", (i*11)%ents))
		default:
			r = rec(i, fmt.Sprintf("entity-%d", i%ents), "has", fmt.Sprintf("a free text fact number %d about the thing, with some detail", i))
		}
		fmt.Fprintf(&b, `{"uuid":%q,"subject":%q,"relation":%q,"object":%q,"time":%q}`+"\n",
			r.UUID, r.Subject, r.Relation, r.Object, r.Time)
	}
	return b.Bytes()
}

func loadAndIndex(data []byte) (*Graph, error) {
	recs, err := store.Read(bytes.NewReader(data), nil)
	if err != nil {
		return nil, err
	}
	return Build(store.Live(recs)), nil
}

func BenchmarkBuild50k(b *testing.B) {
	data := genRecords(50000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loadAndIndex(data); err != nil {
			b.Fatal(err)
		}
	}
}

// TestLoadIndex50k enforces the SPEC section 8 target: read, filter live and
// index 50k records in under 100 ms. CI runners get 300 ms of headroom. The
// check is skipped with -short and under the race detector.
func TestLoadIndex50k(t *testing.T) {
	data := genRecords(50000)
	var g *Graph
	best := time.Duration(1<<63 - 1)
	for i := 0; i < 3; i++ { // best of three smooths scheduler noise
		start := time.Now()
		var err error
		if g, err = loadAndIndex(data); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}
	if len(g.recs) != 50000 || len(g.Entities()) < 10000 {
		t.Fatalf("unexpected index: %d records, %d entities", len(g.recs), len(g.Entities()))
	}
	limit := 100 * time.Millisecond
	if os.Getenv("CI") == "true" {
		limit = 300 * time.Millisecond
	}
	t.Logf("load+index 50k records: %v (limit %v)", best, limit)
	if testing.Short() || raceEnabled {
		t.Skip("timing assertion skipped (short mode or race detector)")
	}
	if best > limit {
		t.Errorf("load+index took %v, want < %v", best, limit)
	}
}
