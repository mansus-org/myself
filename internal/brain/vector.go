package brain

import (
	"context"
	"errors"
	"fmt"

	"github.com/mansus-org/myself/internal/embed"
	"github.com/mansus-org/myself/internal/model"
)

// ScoredRecord is a record plus the similarity score of the recall that found
// it. It embeds Record, so `--json` shows the five fields of a record and a
// sixth, `score`, and a consumer that only knows records still works.
type ScoredRecord struct {
	model.Record
	Score float64 `json:"score"`
}

// VectorStore is the part of a backend that keeps a vector projection of the log:
// an embedding of every fact, and an index over them. The Neo4j backend has one
// (internal/neo4j); the file backend does not, and answers recall with a usage
// error rather than a worse answer.
//
// A projection is never the source of truth. Every vector here is derived from the
// log and can be rebuilt from it with `myself index`, so a missing or stale vector
// costs a recall a hit, never a fact. That is also why recall is its own verb
// instead of a mode of `search`: `search` answers from the log alone and will
// answer the same way forever, while recall answers with a model, and an agent has
// to be able to tell the two apart.
type VectorStore interface {
	// EnsureVectorIndex creates the index over the embeddings, sized for dims,
	// and waits until it can be searched.
	EnsureVectorIndex(ctx context.Context, dims int) error
	// SearchVectors returns the k most similar records to vector, best first,
	// forgotten ones included: liveness is decided here, not in the backend.
	SearchVectors(ctx context.Context, vector []float32, k int) ([]ScoredRecord, error)
	// MissingEmbeddings returns the uuids of the records with no vector from
	// model — none at all, or one made by a different model.
	MissingEmbeddings(ctx context.Context, model string) (map[string]bool, error)
	// SetEmbeddings attaches vectors to records by uuid, in one transaction.
	SetEmbeddings(ctx context.Context, vectors map[string][]float32, model string) error
}

// DefaultRecallN is how many facts recall returns when -n is not given.
const DefaultRecallN = 10

// recallCandidates is how many neighbours recall asks the index for before it
// drops the forgotten ones and cuts to -n. The index knows every fact it was
// ever given, so a brain with many forgets needs a wider net than the answer.
const recallCandidates = 40

// indexBatch is how many facts are embedded and written per step of `index`.
const indexBatch = 64

// RecallResult is `recall --json`.
type RecallResult struct {
	Matches []ScoredRecord `json:"matches"`
	// Model names the embedding model that answered. Two models disagree about
	// what two sentences mean, so an agent reading an answer has to know which
	// one gave it.
	Model string  `json:"model"`
	At    *string `json:"at"`
}

// Recall returns the live facts closest in meaning to text, best first, at most
// n of them (DefaultRecallN when nil) and none scoring below min (0 when nil).
// at is the --at value, nil for now.
//
// st must also be a VectorStore and em must be set; a caller that has neither
// says so in its own words (the CLI names the variables, the server does not
// need to), so these two checks are a safety net, not the message a user reads.
func Recall(st Store, em embed.Embedder, text string, at *string, n *int, min *float64) (RecallResult, error) {
	vs, err := vectorPart(st, em)
	if err != nil {
		return RecallResult{}, err
	}
	if n != nil && *n < 1 {
		return RecallResult{}, Usagef("-n must be at least 1")
	}
	if min != nil && (*min < -1 || *min > 1) {
		return RecallResult{}, Usagef("--min wants a score between -1 and 1")
	}
	limit := DefaultRecallN
	if n != nil {
		limit = *n
	}
	floor := 0.0
	if min != nil {
		floor = *min
	}
	// The snapshot is read for the same reason on the file and in the database:
	// a vector says what a fact meant when it was written, not whether it is
	// still live, and not what was live in April.
	snap, err := view(st, at)
	if err != nil {
		return RecallResult{}, err
	}
	live := make(map[string]bool, len(snap.Live()))
	for _, r := range snap.Live() {
		live[r.UUID] = true
	}
	ctx := context.Background()
	vecs, err := em.Embed(ctx, text)
	if err != nil {
		return RecallResult{}, StoreErr(err)
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 {
		return RecallResult{}, StoreErr(fmt.Errorf("embed: no vector for the question"))
	}
	hits, err := vs.SearchVectors(ctx, vecs[0], max(limit*4, recallCandidates))
	if err != nil {
		return RecallResult{}, storeErr(err)
	}
	res := RecallResult{Model: em.Model(), At: at, Matches: []ScoredRecord{}}
	for _, h := range hits {
		if len(res.Matches) >= limit {
			break
		}
		if !live[h.UUID] || h.Score < floor {
			continue
		}
		res.Matches = append(res.Matches, h)
	}
	return res, nil
}

// IndexResult is `index --json`.
type IndexResult struct {
	// Embedded counts the facts given a new vector.
	Embedded int `json:"embedded"`
	// Skipped counts the live facts that already had a vector from this model.
	Skipped int    `json:"skipped"`
	Model   string `json:"model"`
}

// Index gives every live fact a vector from em, creating the index when the
// backend does not have one yet. Facts that already carry a vector from this
// model are left alone — embedding a brain costs time and, on a hosted model,
// money — unless rebuild is true, which re-embeds every live fact.
//
// Forgotten facts are not embedded: recall never returns them, and the log keeps
// them anyway, so a rebuild after a model change re-embeds what is live.
func Index(st Store, em embed.Embedder, rebuild bool) (IndexResult, error) {
	vs, err := vectorPart(st, em)
	if err != nil {
		return IndexResult{}, err
	}
	snap, err := load(st)
	if err != nil {
		return IndexResult{}, err
	}
	live := snap.Live()
	ctx := context.Background()
	need := map[string]bool{}
	if !rebuild {
		if need, err = vs.MissingEmbeddings(ctx, em.Model()); err != nil {
			return IndexResult{}, storeErr(err)
		}
	}
	var want []model.Record
	for _, r := range live {
		if rebuild || need[r.UUID] {
			want = append(want, r)
		}
	}
	res := IndexResult{Model: em.Model(), Skipped: len(live) - len(want)}
	indexed := false
	for start := 0; start < len(want); start += indexBatch {
		batch := want[start:min(start+indexBatch, len(want))]
		texts := make([]string, len(batch))
		for i, r := range batch {
			texts[i] = embed.Text(r.Subject, r.Relation, r.Object)
		}
		vecs, err := em.Embed(ctx, texts...)
		if err != nil {
			return res, StoreErr(err)
		}
		if len(vecs) != len(batch) {
			return res, StoreErr(fmt.Errorf("embed: %d vectors for %d facts", len(vecs), len(batch)))
		}
		// The index is sized by the vectors, so it can only be created once the
		// model has answered once. Creating it before the first write is what
		// makes a fresh brain work with one command.
		if !indexed {
			dims := em.Dimensions()
			if dims == 0 {
				dims = len(vecs[0])
			}
			if dims > 0 {
				if err := vs.EnsureVectorIndex(ctx, dims); err != nil {
					return res, storeErr(err)
				}
			}
			indexed = true
		}
		vectors := make(map[string][]float32, len(batch))
		for i, r := range batch {
			vectors[r.UUID] = vecs[i]
		}
		if err := vs.SetEmbeddings(ctx, vectors, em.Model()); err != nil {
			return res, storeErr(err)
		}
		res.Embedded += len(batch)
	}
	return res, nil
}

// EnsureVectors makes sure the backend can answer recall with em: it asks the
// model one throwaway question to learn the vector length, then creates the
// index. `serve` calls it at startup so a brain that cannot recall says so
// there, where an operator is listening, rather than on the first recall.
func EnsureVectors(ctx context.Context, st Store, em embed.Embedder) error {
	vs, err := vectorPart(st, em)
	if err != nil {
		return err
	}
	dims := em.Dimensions()
	if dims == 0 {
		vecs, err := em.Embed(ctx, "myself")
		if err != nil {
			return err
		}
		if len(vecs) == 0 {
			return fmt.Errorf("embed: no vector for the probe")
		}
		dims = len(vecs[0])
	}
	return vs.EnsureVectorIndex(ctx, dims)
}

// storeErr wraps a backend failure as exit 3, but leaves an error that already
// carries a code alone: "this brain keeps no vectors" is a usage error even when
// the backend is the one that says it.
func storeErr(err error) error {
	var ee *ExitError
	if errors.As(err, &ee) {
		return err
	}
	return StoreErr(err)
}

// vectorPart checks the two things recall and index need: a backend that keeps
// vectors, and a model to read with.
//
// These messages are the only ones of their kind: the CLI and the server print
// the same words, because the same command must not answer differently depending
// on how the brain is reached. They name the variables an operator sets; a test
// in internal/cli checks they still name the ones this program reads.
func vectorPart(st Store, em embed.Embedder) (VectorStore, error) {
	vs, ok := st.(VectorStore)
	if !ok {
		return nil, Usagef("recall and index need a brain that keeps vectors: only the Neo4j backend does (set MYSELF_NEO4J_URL, then run myself index)")
	}
	if em == nil {
		return nil, Usagef("recall and index need an embedding endpoint: set MYSELF_EMBED_URL to an OpenAI-compatible /embeddings URL and MYSELF_EMBED_MODEL to the model")
	}
	return vs, nil
}

// Lines is one score and log line per match, best first.
func (r RecallResult) Lines() []string {
	lines := make([]string, len(r.Matches))
	for i, m := range r.Matches {
		lines[i] = fmt.Sprintf("%.2f %s", m.Score, RecordLine(m.Record))
	}
	return lines
}

// Lines is one line: what was indexed, and what did not need it.
func (r IndexResult) Lines() []string {
	line := fmt.Sprintf("indexed %d %s with %s", r.Embedded, pluralFacts(r.Embedded), r.Model)
	if r.Skipped > 0 {
		line += fmt.Sprintf(" (%d already had a vector)", r.Skipped)
	}
	return []string{line}
}

func pluralFacts(n int) string {
	if n == 1 {
		return "fact"
	}
	return "facts"
}
