package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/embed"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/neo4j"
)

// The "recall is not available" messages are written once, in brain, so the CLI
// and the server answer the same command the same way. They name the variables an
// operator has to set; this is the guard that they still name the ones this
// program actually reads, which brain cannot import without a cycle.
func TestRecallErrorsNameTheVariables(t *testing.T) {
	if _, err := brain.Recall(recallNoVectorStore{}, nil, "x", nil, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), neo4j.EnvURL) {
		t.Errorf("the missing backend must name %s: %v", neo4j.EnvURL, err)
	}
	for _, what := range []string{"recall", "index"} {
		var err error
		if what == "recall" {
			_, err = brain.Recall(recallVectorStore{}, nil, "x", nil, nil, nil, nil)
		} else {
			_, err = brain.Index(recallVectorStore{}, nil, false)
		}
		if err == nil || !strings.Contains(err.Error(), embed.EnvURL) || !strings.Contains(err.Error(), embed.EnvModel) {
			t.Errorf("%s without an endpoint must name %s and %s: %v", what, embed.EnvURL, embed.EnvModel, err)
		}
	}
}

// recallNoVectorStore is a brain with a log and no projection, which is what the
// file is.
type recallNoVectorStore struct{}

func (recallNoVectorStore) Load() (*brain.Snapshot, error) { return brain.NewSnapshot(nil), nil }
func (recallNoVectorStore) Append(...model.Record) error   { return nil }
func (recallNoVectorStore) Warn(string)                    {}

// recallVectorStore has the projection, so the second message is reachable.
type recallVectorStore struct{ recallNoVectorStore }

func (recallVectorStore) EnsureVectorIndex(context.Context, int) error { return nil }
func (recallVectorStore) SearchVectors(context.Context, []float32, int) ([]brain.ScoredRecord, error) {
	return nil, nil
}
func (recallVectorStore) MissingEmbeddings(context.Context, string) (map[string]bool, error) {
	return nil, nil
}
func (recallVectorStore) SetEmbeddings(context.Context, map[string][]float32, string) error {
	return nil
}
