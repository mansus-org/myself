package server

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

// cache holds the loaded log. It is reloaded when the file's size or mtime
// changes, or after this process writes.
type cache struct {
	path string

	mu       sync.Mutex
	ok       bool
	size     int64
	mod      time.Time
	snap     *brain.Snapshot
	warnings []string
}

func (c *cache) load() (*brain.Snapshot, []string, error) {
	// Stat before reading: if the file grows in between, the stored size is
	// the older one and the next load reads again.
	size, mod := int64(-1), time.Time{}
	fi, err := os.Stat(c.path)
	switch {
	case err == nil:
		size, mod = fi.Size(), fi.ModTime()
	case !errors.Is(err, fs.ErrNotExist):
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ok && c.size == size && c.mod.Equal(mod) {
		return c.snap, c.warnings, nil
	}
	var warn bytes.Buffer
	recs, err := store.ReadFile(c.path, &warn)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, l := range strings.Split(strings.TrimSpace(warn.String()), "\n") {
		if l != "" {
			warnings = append(warnings, strings.TrimPrefix(l, "warning: "))
		}
	}
	c.ok, c.size, c.mod, c.snap, c.warnings = true, size, mod, brain.NewSnapshot(recs), warnings
	return c.snap, c.warnings, nil
}

func (c *cache) invalidate() {
	c.mu.Lock()
	c.ok = false
	c.mu.Unlock()
}

// RequestStore is the brain.Store of one request. Loads share the server's
// cached snapshot; warnings (corrupt lines, the "is" sentence warning) are
// collected for the response. Take one per request with Server.Store.
type RequestStore struct {
	c        *cache
	warnings []string
}

// Load returns the cached snapshot, reloading it if the file changed.
func (r *RequestStore) Load() (*brain.Snapshot, error) {
	snap, w, err := r.c.load()
	if err != nil {
		return nil, err
	}
	r.warnings = append(r.warnings, w...)
	return snap, nil
}

// Append writes through store.Append (flock) and drops the cache.
func (r *RequestStore) Append(recs ...model.Record) error {
	defer r.c.invalidate()
	return store.Append(r.c.path, recs...)
}

// Warn collects a warning.
func (r *RequestStore) Warn(msg string) { r.warnings = append(r.warnings, msg) }

// Warnings returns the warnings collected so far, deduplicated in order.
func (r *RequestStore) Warnings() []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range r.warnings {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}
