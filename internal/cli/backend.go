package cli

import (
	"errors"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/client"
	"github.com/mansus-org/myself/internal/model"
	"github.com/spf13/cobra"
)

// backend is where a command's work happens: the local store, or a server.
// Both return the brain result structs, which the commands render, so the
// output cannot differ between the modes.
type backend interface {
	Remember(facts []brain.Fact, replace bool) (brain.RememberResult, error)
	Forget(a brain.ForgetArgs) (brain.ForgetResult, error)
	Describe(entity string, depth int, at *string) (brain.DescribeResult, error)
	Ls(typ, at *string) (brain.LsResult, error)
	Relations() (brain.RelationsResult, error)
	Search(text string, at *string) (brain.SearchResult, error)
	Path(a, b string, maxDepth int) (brain.PathResult, error)
	Log(since *string, n *int) (brain.RecordsResult, error)
	History(subject string, relation *string) (brain.RecordsResult, error)
	Prime(budget int) (brain.PrimeResult, error)
	Dream() (brain.DreamResult, error)
	ExportObsidian(dir string) (brain.ExportResult, error)
}

// backendFor picks the backend of a command from cfg.
func backendFor(cmd *cobra.Command, cfg *Config) backend {
	if cfg.Remote() {
		return remoteBackend{newClient(cmd, cfg)}
	}
	return localBackend{storeFor(cmd, cfg)}
}

func newClient(cmd *cobra.Command, cfg *Config) *client.Client {
	stderr := cmd.ErrOrStderr()
	return &client.Client{
		URL:     cfg.URL,
		Token:   cfg.Token,
		Timeout: cfg.Timeout,
		Warn:    func(msg string) { Warnf(stderr, "%s", msg) },
	}
}

type localBackend struct{ st brain.Store }

func (b localBackend) Remember(facts []brain.Fact, replace bool) (brain.RememberResult, error) {
	return brain.Remember(b.st, facts, replace, time.Now())
}
func (b localBackend) Forget(a brain.ForgetArgs) (brain.ForgetResult, error) {
	return brain.Forget(b.st, a, time.Now())
}
func (b localBackend) Describe(entity string, depth int, at *string) (brain.DescribeResult, error) {
	return brain.Describe(b.st, entity, depth, at)
}
func (b localBackend) Ls(typ, at *string) (brain.LsResult, error) { return brain.Ls(b.st, typ, at) }
func (b localBackend) Relations() (brain.RelationsResult, error)  { return brain.Relations(), nil }
func (b localBackend) Search(text string, at *string) (brain.SearchResult, error) {
	return brain.Search(b.st, text, at)
}
func (b localBackend) Path(a, c string, maxDepth int) (brain.PathResult, error) {
	return brain.Path(b.st, a, c, maxDepth)
}
func (b localBackend) Log(since *string, n *int) (brain.RecordsResult, error) {
	return brain.Log(b.st, since, n)
}
func (b localBackend) History(subject string, relation *string) (brain.RecordsResult, error) {
	return brain.History(b.st, subject, relation)
}
func (b localBackend) Prime(budget int) (brain.PrimeResult, error) { return brain.Prime(b.st, budget) }
func (b localBackend) Dream() (brain.DreamResult, error)           { return brain.Dream(b.st) }
func (b localBackend) ExportObsidian(dir string) (brain.ExportResult, error) {
	return brain.ExportObsidian(b.st, dir)
}

// remoteBackend embeds the client for the query and write calls; export is
// built locally from the downloaded log.
type remoteBackend struct{ *client.Client }

// ExportObsidian fetches the log and writes the notes with the same code as
// the local export.
func (b remoteBackend) ExportObsidian(dir string) (brain.ExportResult, error) {
	return brain.ExportObsidian(downloadStore{b.Client}, dir)
}

// downloadStore is a read-only brain.Store over the server's export.jsonl.
type downloadStore struct{ c *client.Client }

func (s downloadStore) Load() (*brain.Snapshot, error) {
	recs, err := s.c.Records()
	if err != nil {
		return nil, err
	}
	return brain.NewSnapshot(recs), nil
}
func (s downloadStore) Append(...model.Record) error { return errors.New("the download is read-only") }
func (s downloadStore) Warn(string)                  {}
