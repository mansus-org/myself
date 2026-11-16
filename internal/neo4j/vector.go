package neo4j

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/embed"
	"github.com/mansus-org/myself/internal/model"
)

const (
	// VectorIndexName is the name of the vector index over :Record(embedding):
	// one embedding per live fact, kept on the record node itself, in Neo4j's own
	// HNSW vector index (5.11 or newer, Community included), queried with
	// db.index.vector.queryNodes. One name, one model: two models' vectors in one
	// index answers nothing.
	//
	// The vectors are a projection and never the source of truth. The five fact
	// properties stay the whole record; `embedding` and `embeddingModel` are
	// derived, rebuildable from the log with `myself index`, and allowed to be
	// missing or out of date. Nothing here computes a vector — internal/embed
	// does — so the model stays a configuration choice.
	VectorIndexName = "myself_embedding"

	// vectorBatch is how many vectors one write carries. A 1536-dimension
	// vector is a few kilobytes of JSON, so a batch of this size stays a small
	// request even on a shared HTTP/2 connection.
	vectorBatch = 64

	// indexPoll is how often EnsureVectorIndex asks whether the index is
	// searchable yet. A new index is POPULATING for a moment, and a search
	// against a POPULATING index is an error the database refuses to answer.
	indexPoll = 100 * time.Millisecond

	// indexWaitFloor is the shortest wait for a new index to come up, whatever
	// the request timeout says: populating an index of a whole brain outlasts a
	// polite per-request timeout.
	indexWaitFloor = 30 * time.Second
)

// Errors a caller can act on. They are wrapped, so the database's own words stay
// in the message.
var (
	// ErrNoVectorIndex means nothing has been embedded yet.
	ErrNoVectorIndex = errors.New("neo4j: the brain has no vector index yet")
	// ErrIndexPopulating means the index exists but is still being built.
	ErrIndexPopulating = errors.New("neo4j: the vector index is still being built")
)

// UseEmbedder turns the vector projection on: every fact written through Append
// is embedded first, and recall can search. Call it once at startup, before the
// store is used; it is not meant to change underneath a running server. A nil
// embedder is the same as never calling it.
func (s *Store) UseEmbedder(e embed.Embedder) { s.embedder = e }

// Embedder returns the embedder in use, or nil when recall is off.
func (s *Store) Embedder() embed.Embedder { return s.embedder }

// vectorIndexStatement is the CREATE for one index size. The dimensions are
// spliced, not parameterized: a schema statement's OPTIONS map is not equally
// generous about parameters across versions, and dims is a validated integer
// this package computes, never text from a caller's data.
func vectorIndexStatement(dims int) string {
	return fmt.Sprintf("CREATE VECTOR INDEX `%s` IF NOT EXISTS FOR (r:Record) ON (r.embedding) "+
		"OPTIONS { indexConfig: { `vector.dimensions`: %d, `vector.similarity_function`: 'cosine' } }",
		VectorIndexName, dims)
}

// EnsureVectorIndex creates the vector index sized for dims and waits until it
// can be searched. It is idempotent, so it is safe on every start and against a
// database an operator prepared by hand.
//
// The wait is what turns "the index exists" into "a recall can run": a fresh
// index is POPULATING for a moment, and the database refuses to search it then.
// If the state cannot be read at all, the index is left as created and the
// failure is a warning — the CREATE worked, which is the part that matters, and
// refusing to start over an unreadable status would be the worse failure.
func (s *Store) EnsureVectorIndex(ctx context.Context, dims int) error {
	if dims < 1 || dims > 65536 {
		return fmt.Errorf("%s: %d is not a vector length", s, dims)
	}
	if err := s.commit(ctx, toStatements([]string{vectorIndexStatement(dims)}, nil)); err != nil {
		return err
	}
	deadline := time.Now().Add(s.indexWait)
	for {
		state, percent, err := s.vectorIndexState(ctx)
		if err != nil {
			s.Warn(fmt.Sprintf("cannot read the state of the vector index: %v", err))
			return nil
		}
		switch state {
		case "ONLINE":
			return nil
		case "FAILED":
			return fmt.Errorf("%s: the vector index %s failed to build", s, VectorIndexName)
		case "":
			return fmt.Errorf("%s: the vector index %s is gone after being created: %w", s, VectorIndexName, ErrNoVectorIndex)
		}
		if !time.Now().Add(indexPoll).Before(deadline) {
			return fmt.Errorf("%s: the vector index %s is still %s (%.0f%%): %w",
				s, VectorIndexName, state, percent, ErrIndexPopulating)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(indexPoll):
		}
	}
}

const showVectorIndexes = "SHOW VECTOR INDEXES YIELD name, state, populationPercent"

// vectorIndexState reports the state of the vector index, and how far its
// population has got. A missing index is reported as "" rather than an error:
// "there is no index" is an answer, not a failure.
func (s *Store) vectorIndexState(ctx context.Context) (state string, percent float64, err error) {
	rows, err := s.rows(ctx, showVectorIndexes, nil)
	if err != nil {
		return "", 0, err
	}
	for _, row := range rows {
		if str(row["name"]) == VectorIndexName {
			return str(row["state"]), num(row["populationPercent"]), nil
		}
	}
	return "", 0, nil
}

const searchVectorStatement = "CALL db.index.vector.queryNodes($index, $k, $vector) YIELD node, score " +
	"RETURN node.uuid AS uuid, node.subject AS subject, node.relation AS relation, " +
	"node.object AS object, node.time AS time, score ORDER BY score DESC"

// SearchVectors returns the k records whose vectors are closest to vector,
// best first, with the score the index gave them. Forgotten facts are still
// there: whether a fact is live is the brain's question, not the index's.
func (s *Store) SearchVectors(ctx context.Context, vector []float32, k int) ([]brain.ScoredRecord, error) {
	if k < 1 {
		return nil, fmt.Errorf("%s: k must be at least 1", s)
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("%s: an empty vector asks nothing", s)
	}
	rows, err := s.rows(ctx, searchVectorStatement, map[string]any{
		"index": VectorIndexName, "k": k, "vector": vector,
	})
	if err != nil {
		return nil, vectorSearchError(err)
	}
	out := make([]brain.ScoredRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, brain.ScoredRecord{
			Record: model.Record{
				UUID:     str(row["uuid"]),
				Subject:  str(row["subject"]),
				Relation: str(row["relation"]),
				Object:   str(row["object"]),
				Time:     str(row["time"]),
			},
			Score: num(row["score"]),
		})
	}
	return out, nil
}

// vectorSearchError turns the two failures that are the caller's to explain —
// nothing indexed yet, and still indexing — into errors they can name.
func vectorSearchError(err error) error {
	var ne *neo4jError
	if !errors.As(err, &ne) {
		return err
	}
	switch {
	case strings.Contains(ne.Message, "no such index") || strings.Contains(ne.Code, "IndexNotFound"):
		return fmt.Errorf("%v: %w", ne, ErrNoVectorIndex)
	case strings.Contains(ne.Message, "population state"):
		return fmt.Errorf("%v: %w", ne, ErrIndexPopulating)
	default:
		return err
	}
}

const missingEmbeddingsStatement = "MATCH (r:Record) WHERE r.relation <> 'forget' AND " +
	"(r.embedding IS NULL OR coalesce(r.embeddingModel, '') <> $model) RETURN r.uuid AS uuid"

// MissingEmbeddings returns the uuids of the records that carry no vector from
// model — none at all, or one made by a different model, which is not
// comparable with this one's. Forgotten records are reported too; brain.Index
// decides what is worth embedding, because only it knows what is live.
func (s *Store) MissingEmbeddings(ctx context.Context, model string) (map[string]bool, error) {
	rows, err := s.rows(ctx, missingEmbeddingsStatement, map[string]any{"model": model})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[str(row["uuid"])] = true
	}
	return out, nil
}

const setEmbeddingStatement = "UNWIND $rows AS row MATCH (r:Record {uuid: row.uuid}) " +
	"SET r.embedding = row.embedding, r.embeddingModel = row.model"

// SetEmbeddings attaches vectors to records by uuid, one transaction per batch.
// A uuid with no record is ignored: the fact was never written, or this is a
// rebuild racing a forget, and neither is a reason to lose the other vectors.
func (s *Store) SetEmbeddings(ctx context.Context, vectors map[string][]float32, model string) error {
	if len(vectors) == 0 {
		return nil
	}
	uuids := make([]string, 0, len(vectors))
	for u := range vectors {
		uuids = append(uuids, u)
	}
	// Sorted so a batch is the same set of rows every time, which makes a
	// repeated run and a captured request comparable.
	sort.Strings(uuids)
	for start := 0; start < len(uuids); start += vectorBatch {
		end := min(start+vectorBatch, len(uuids))
		rows := make([]map[string]any, 0, end-start)
		for _, u := range uuids[start:end] {
			rows = append(rows, map[string]any{"uuid": u, "embedding": vectors[u], "model": model})
		}
		if err := s.commit(ctx, []statement{{Statement: setEmbeddingStatement, Parameters: map[string]any{"rows": rows}}}); err != nil {
			return err
		}
	}
	return nil
}

// embedRows fills the embedding of every fact row, best-effort. A fact whose
// vector cannot be had is still written: the vector is a projection, and losing
// what the agent just said because an endpoint is down would be the worse
// failure by far. `myself index` fills the gap afterwards.
//
// rows is filled in place and is index-aligned with recs.
func (s *Store) embedRows(ctx context.Context, recs []model.Record, rows []map[string]any) {
	if s.embedder == nil {
		return
	}
	facts := make([]int, 0, len(recs))
	texts := make([]string, 0, len(recs))
	for i, r := range recs {
		if r.IsForget() {
			continue // a retraction says nothing a recall should answer with
		}
		facts = append(facts, i)
		texts = append(texts, embed.Text(r.Subject, r.Relation, r.Object))
	}
	if len(texts) == 0 {
		return
	}
	vecs, err := s.embedder.Embed(ctx, texts...)
	if err != nil {
		s.Warn(fmt.Sprintf("not embedded: %v (myself index will catch up)", err))
		return
	}
	if len(vecs) != len(texts) {
		s.Warn(fmt.Sprintf("not embedded: %d vectors for %d facts (myself index will catch up)", len(vecs), len(texts)))
		return
	}
	for i, idx := range facts {
		rows[idx]["embedding"] = vecs[i]
		rows[idx]["embeddingModel"] = s.embedder.Model()
	}
}

// isDimensionError reports a write the index refused because the vector did not
// match its size — the signature of a model change.
func isDimensionError(err error) bool {
	var ne *neo4jError
	if !errors.As(err, &ne) {
		return false
	}
	return strings.Contains(ne.Message, "dimension")
}

// appendWithoutVectors drops the derived properties from rows, leaving the five
// fact properties that are the record.
func (s *Store) appendWithoutVectors(rows []map[string]any) {
	for _, row := range rows {
		delete(row, "embedding")
		delete(row, "embeddingModel")
	}
}
