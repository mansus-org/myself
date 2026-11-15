package brain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/model"
)

// words is the whole vocabulary of the test embedder: a fact's vector counts how
// often each word appears in it, so similarity is a fact a test can compute by
// hand and no model is in the room.
var words = []string{"relay", "bun", "project", "garden", "go", "server", "self", "agent", "deploys", "nightly"}

type fakeEmbedder struct{ model string }

func (e fakeEmbedder) Embed(ctx context.Context, texts ...string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v := make([]float32, len(words))
		for i, w := range words {
			v[i] = float32(strings.Count(strings.ToLower(t), w))
		}
		out = append(out, v)
	}
	return out, nil
}

func (e fakeEmbedder) Model() string {
	if e.model == "" {
		return "test-model"
	}
	return e.model
}

func (e fakeEmbedder) Dimensions() int { return len(words) }

// vecStore is a brain.Store that also keeps a vector projection: the seam a
// backend has to answer, held to the same rules as the Neo4j one.
type vecStore struct {
	recs    []model.Record
	vectors map[string][]float32
	from    map[string]string // uuid → the model that made its vector
	dims    int
	ensured int
	asked   []int // the k of every search, so a test can see the net was wide
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
			Time:     fmt.Sprintf("2026-01-%02dT00:00:00Z", i+1),
		})
	}
	return &vecStore{recs: recs, vectors: map[string][]float32{}, from: map[string]string{}}
}

func (s *vecStore) Load() (*Snapshot, error) { return NewSnapshot(s.recs), nil }
func (s *vecStore) Append(recs ...model.Record) error {
	s.recs = append(s.recs, recs...)
	return nil
}
func (s *vecStore) Warn(string) {}

func (s *vecStore) EnsureVectorIndex(ctx context.Context, dims int) error {
	s.dims = dims
	s.ensured++
	return nil
}

func (s *vecStore) SearchVectors(ctx context.Context, vector []float32, k int) ([]ScoredRecord, error) {
	s.asked = append(s.asked, k)
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
		hits = append(hits, hit{r, cosine(vector, v)})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > k {
		hits = hits[:k]
	}
	out := make([]ScoredRecord, 0, len(hits))
	for _, h := range hits {
		out = append(out, ScoredRecord{Record: h.rec, Score: h.score})
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

func cosine(a, b []float32) float64 {
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

// indexEverything is what `myself index` does to a store: every live fact gets
// its vector, which is how a test gets to the state recall needs.
func indexEverything(t *testing.T, s *vecStore) {
	t.Helper()
	res, err := Index(s, fakeEmbedder{}, false)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if res.Embedded == 0 {
		t.Fatalf("nothing was indexed: %+v", res)
	}
}

func ptr[T any](v T) *T { return &v }

func TestRecallAnswersWithTheClosestLiveFacts(t *testing.T) {
	s := newVecStore(
		"relay is project",
		"relay uses bun",
		"self is agent",
		"relay deploys nightly",
	)
	indexEverything(t, s)
	res, err := Recall(s, fakeEmbedder{}, "bun", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "test-model" {
		t.Fatalf("the answer must say which model gave it: %+v", res)
	}
	if len(res.Matches) == 0 {
		t.Fatal("no matches")
	}
	if res.Matches[0].Record.Object != "bun" {
		t.Fatalf("the bun fact should answer first, got %+v", res.Matches[0])
	}
	if res.Matches[0].Score <= 0 {
		t.Fatalf("the closest fact scores %f", res.Matches[0].Score)
	}
	// Scores come back best first: an agent has to be able to tell a strong hit
	// from a stretch.
	for i := 1; i < len(res.Matches); i++ {
		if res.Matches[i-1].Score < res.Matches[i].Score {
			t.Fatalf("not sorted by score: %+v", res.Matches)
		}
	}
}

func TestRecallNeverReturnsAForgottenFact(t *testing.T) {
	s := newVecStore("relay uses bun", "relay is project")
	indexEverything(t, s)
	forgotten := s.recs[0].UUID
	if err := s.Append(model.NewForget(forgotten, "changed my mind", forgetTime)); err != nil {
		t.Fatal(err)
	}
	res, err := Recall(s, fakeEmbedder{}, "bun", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Matches {
		if m.UUID == forgotten {
			t.Fatalf("a forgotten fact is not a fact: %+v", res.Matches)
		}
	}
}

func TestRecallCutsToNAndToTheFloor(t *testing.T) {
	s := newVecStore("relay is project", "relay uses bun", "relay deploys nightly", "self is agent")
	indexEverything(t, s)
	res, err := Recall(s, fakeEmbedder{}, "relay", nil, ptr(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 2 {
		t.Fatalf("-n 2 gave %d: %+v", len(res.Matches), res.Matches)
	}
	res, err = Recall(s, fakeEmbedder{}, "relay", nil, nil, ptr(0.9))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Matches {
		if m.Score < 0.9 {
			t.Fatalf("--min 0.9 let %f through: %+v", m.Score, res.Matches)
		}
	}
}

func TestRecallAsOfADateDropsLaterFacts(t *testing.T) {
	s := newVecStore("relay is project", "relay uses bun")
	indexEverything(t, s)
	// The bun fact is the second one, dated 2026-01-02: as of January 1st only
	// the project fact was live, even though the bun fact is the closer answer.
	res, err := Recall(s, fakeEmbedder{}, "bun relay", ptr("2026-01-01T00:00:00Z"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 || res.Matches[0].Object != "project" {
		t.Fatalf("--at did not hold the brain to that date: %+v", res.Matches)
	}
	if last := s.asked[len(s.asked)-1]; last <= 1 {
		t.Fatalf("--at drops candidates, so the net must be wider than the answer: asked for %d", last)
	}
	if res.At == nil || *res.At != "2026-01-01T00:00:00Z" {
		t.Fatalf("the answer must say as of when: %+v", res.At)
	}
}

func TestRecallRefusesABadCountOrFloor(t *testing.T) {
	s := newVecStore("relay is project")
	indexEverything(t, s)
	if _, err := Recall(s, fakeEmbedder{}, "relay", nil, ptr(0), nil); !isUsage(err) {
		t.Fatalf("-n 0: %v", err)
	}
	if _, err := Recall(s, fakeEmbedder{}, "relay", nil, nil, ptr(2.0)); !isUsage(err) {
		t.Fatalf("--min 2: %v", err)
	}
	if _, err := Recall(s, fakeEmbedder{}, "relay", ptr("not a time"), nil, nil); !isUsage(err) {
		t.Fatalf("--at: %v", err)
	}
}

// plainStore is a brain with no vector projection, which is what the file is.
type plainStore struct{ recs []model.Record }

func (s plainStore) Load() (*Snapshot, error)     { return NewSnapshot(s.recs), nil }
func (s plainStore) Append(...model.Record) error { return nil }
func (s plainStore) Warn(string)                  {}

func TestRecallAndIndexSayWhatIsMissing(t *testing.T) {
	s := newVecStore("relay is project")
	if _, err := Recall(plainStore{}, fakeEmbedder{}, "relay", nil, nil, nil); !isUsage(err) {
		t.Fatalf("a brain with no vectors: %v", err)
	}
	if _, err := Index(plainStore{}, fakeEmbedder{}, false); !isUsage(err) {
		t.Fatalf("a brain with no vectors: %v", err)
	}
	if _, err := Recall(s, nil, "relay", nil, nil, nil); !isUsage(err) {
		t.Fatalf("no embedding endpoint: %v", err)
	}
	if err := EnsureVectors(context.Background(), s, nil); err == nil {
		t.Fatal("no embedding endpoint should be an error")
	}
}

func TestIndexGivesEveryLiveFactAVector(t *testing.T) {
	s := newVecStore("relay is project", "relay uses bun", "self is agent")
	res, err := Index(s, fakeEmbedder{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 3 || res.Skipped != 0 {
		t.Fatalf("want 3 indexed, got %+v", res)
	}
	if res.Model != "test-model" {
		t.Fatalf("the answer must name the model: %+v", res)
	}
	if s.dims != len(words) || s.ensured != 1 {
		t.Fatalf("the index must be created, sized by the vectors: dims=%d ensured=%d", s.dims, s.ensured)
	}
	for _, r := range s.recs {
		if s.from[r.UUID] != "test-model" {
			t.Fatalf("%s has no vector from the model: %v", r.UUID, s.from)
		}
	}
	// The second run is the one an operator actually runs: nothing to do, and
	// nothing paid for twice.
	res, err = Index(s, fakeEmbedder{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 0 || res.Skipped != 3 {
		t.Fatalf("a second index must do nothing: %+v", res)
	}
	if !strings.Contains(strings.Join(res.Lines(), "\n"), "already had a vector") {
		t.Fatalf("the line should say so: %v", res.Lines())
	}
}

func TestIndexRebuildsOnlyWhatIsAskedFor(t *testing.T) {
	s := newVecStore("relay is project", "relay uses bun")
	indexEverything(t, s)
	// A model change: the old vectors are not comparable, so every fact needs one.
	s.from[s.recs[0].UUID] = "other-model"
	res, err := Index(s, fakeEmbedder{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 1 || res.Skipped != 1 {
		t.Fatalf("a stale vector must be redone: %+v", res)
	}
	res, err = Index(s, fakeEmbedder{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 2 || res.Skipped != 0 {
		t.Fatalf("--rebuild re-embeds every live fact: %+v", res)
	}
}

func TestIndexLeavesForgottenFactsAlone(t *testing.T) {
	s := newVecStore("relay uses bun", "relay is project")
	if err := s.Append(model.NewForget(s.recs[0].UUID, "", forgetTime)); err != nil {
		t.Fatal(err)
	}
	res, err := Index(s, fakeEmbedder{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 1 {
		t.Fatalf("a forgotten fact is never recalled, so it is never embedded: %+v", res)
	}
	if _, ok := s.vectors[s.recs[0].UUID]; ok {
		t.Fatal("the forgotten fact got a vector")
	}
}

func TestIndexReportsAnUnreachableModel(t *testing.T) {
	s := newVecStore("relay is project")
	// The embedder is reached through the store's own failure: a fake that
	// refuses is the same shape as a refused connection.
	broken := &refusingStore{vecStore: s}
	_, err := Index(broken, fakeEmbedder{}, false)
	if err == nil {
		t.Fatal("an endpoint that cannot answer must fail the command")
	}
	if Code(err) != ExitStore {
		t.Fatalf("an unreachable endpoint is a store failure, exit %d: %v", Code(err), err)
	}
}

// refusingStore is a vector store whose writes fail, which is how a broken
// endpoint shows up when the vectors are already in hand.
type refusingStore struct{ *vecStore }

func (s *refusingStore) SetEmbeddings(ctx context.Context, vectors map[string][]float32, model string) error {
	return errors.New("the endpoint went away mid-run")
}

func TestEnsureVectorsSizesTheIndexFromTheModel(t *testing.T) {
	s := newVecStore()
	if err := EnsureVectors(context.Background(), s, fakeEmbedder{}); err != nil {
		t.Fatal(err)
	}
	if s.dims != len(words) || s.ensured != 1 {
		t.Fatalf("dims=%d ensured=%d", s.dims, s.ensured)
	}
}

func TestRecallLinesCarryTheScoreFirst(t *testing.T) {
	res := RecallResult{Matches: []ScoredRecord{{
		Record: model.Record{UUID: "01a1", Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-01-01T00:00:00Z"},
		Score:  0.867,
	}}}
	got := strings.Join(res.Lines(), "\n")
	want := "0.87 01a1 2026-01-01T00:00:00Z relay uses bun"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScoredRecordJSONIsARecordPlusAScore(t *testing.T) {
	raw, err := model.Marshal(ScoredRecord{
		Record: model.Record{UUID: "01a1", Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-01-01T00:00:00Z"},
		Score:  0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"uuid":"01a1"`, `"subject":"relay"`, `"score":0.5`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("%s missing from %s", want, raw)
		}
	}
}

func isUsage(err error) bool {
	return Code(err) == ExitUsage
}

// forgetTime is the timestamp of a forget built by hand in these tests.
var forgetTime = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
