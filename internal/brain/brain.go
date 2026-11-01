// Package brain computes every command of SPEC section 5 against a Store and
// returns the --json result structs of section 6. It does no parsing of
// command lines or HTTP requests and no text rendering beyond prime's text:
// the CLI and the server both call it and render or encode the results.
//
// Errors are *ExitError values carrying the exit code. Their messages are
// what the CLI prints after "error: ".
package brain

import (
	"sync"
	"time"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

// Store is where the records come from and go to. The CLI reads the file on
// every Load; the server caches. Errors from Load and Append are reported
// with exit code 3.
type Store interface {
	// Load returns every record, uuid-sorted, forgets included.
	Load() (*Snapshot, error)
	// Append writes recs in one store.Append call.
	Append(recs ...model.Record) error
	// Warn reports a warning (the text after "warning: ").
	Warn(msg string)
}

// Snapshot is a loaded log. The live records and the graph are built on
// first use and cached, so a server can share one Snapshot between requests.
// A Snapshot is safe for concurrent use.
type Snapshot struct {
	Records []model.Record // uuid-sorted, forgets included

	once  sync.Once
	live  []model.Record
	graph *graph.Graph
}

// NewSnapshot wraps uuid-sorted records.
func NewSnapshot(recs []model.Record) *Snapshot {
	return &Snapshot{Records: recs}
}

func (s *Snapshot) build() {
	s.once.Do(func() {
		s.live = store.Live(s.Records)
		s.graph = graph.Build(s.live)
	})
}

// Live returns the records that are not forgotten, forgets excluded.
func (s *Snapshot) Live() []model.Record { s.build(); return s.live }

// Graph returns the graph of the live records.
func (s *Snapshot) Graph() *graph.Graph { s.build(); return s.graph }

// AsOf returns a new Snapshot of the records with time <= t.
func (s *Snapshot) AsOf(t time.Time) *Snapshot {
	return NewSnapshot(store.AsOf(s.Records, t))
}

// load calls st.Load, mapping errors to exit 3.
func load(st Store) (*Snapshot, error) {
	snap, err := st.Load()
	if err != nil {
		return nil, StoreErr(err)
	}
	return snap, nil
}

// ParseAt parses an --at value; a bad one is a usage error naming flag
// ("--at", "--since").
func ParseAt(flag, raw string) (time.Time, error) {
	t, err := store.ParseAt(raw)
	if err != nil {
		return time.Time{}, Usagef("%s: %w", flag, err)
	}
	return t, nil
}

// view parses at (nil means now), loads the store and returns the snapshot
// as of that time. The --at value is parsed before the store is read, so a
// bad value is reported even when the store is unreadable.
func view(st Store, at *string) (*Snapshot, error) {
	var t time.Time
	if at != nil {
		var err error
		if t, err = ParseAt("--at", *at); err != nil {
			return nil, err
		}
	}
	snap, err := load(st)
	if err != nil {
		return nil, err
	}
	if at != nil {
		snap = snap.AsOf(t)
	}
	return snap, nil
}

// nonNil keeps empty record lists as [] in JSON.
func nonNil(recs []model.Record) []model.Record {
	if recs == nil {
		return []model.Record{}
	}
	return recs
}
