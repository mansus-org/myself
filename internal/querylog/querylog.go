// Package querylog keeps what a server was asked: the questions that came in
// over the API, and what came back (SPEC section 10).
//
// It is not the brain. A query is not a fact about the world — it is what someone
// asked this server — so it lives in its own bounded file next to the brain, never
// in the log, and it is never searched, recalled, exported or primed. That is also
// why it is bounded: an account of questions grows without ever becoming useful,
// so the file keeps the last few hundred and drops the oldest.
package querylog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Verbs is what the server records, in the order a reader lists them.
var Verbs = []string{"search", "recall", "describe"}

const (
	// Keep is how many entries the file holds after a write trims it.
	Keep = 500

	// grow is how far past Keep the file gets before a write trims it. Trimming
	// on every append would rewrite the file for nothing; this makes it once per
	// Keep queries.
	grow = Keep * 2

	// MaxFacts caps how many result facts one entry names. The point is "what
	// came back", not a copy of the answer: the answer is one query away.
	MaxFacts = 5

	// DefaultN is how many entries a reader gets when it asks for none.
	DefaultN = 50

	// MaxN caps what a reader may ask for, so one request cannot read the whole
	// file into a response nobody reads.
	MaxN = 1000
)

// Entry is one query and what it got. Time is RFC 3339 UTC.
type Entry struct {
	Time  string   `json:"time"`
	Verb  string   `json:"verb"`
	Text  string   `json:"text"`
	Hits  int      `json:"hits"`
	Facts []string `json:"facts,omitempty"`
	Took  int64    `json:"ms,omitempty"`
	Who   string   `json:"who,omitempty"`
	Model string   `json:"model,omitempty"`
	At    string   `json:"at,omitempty"`
	// Error is why the query did not answer. A failed query is worth seeing: it
	// is usually a brain without vectors, or a word nobody has written yet.
	Error string `json:"error,omitempty"`
}

// Line is the entry on one line, oldest-first reading order.
func (e Entry) Line() string {
	line := fmt.Sprintf("%s %s %q", e.Time, e.Verb, e.Text)
	if e.At != "" {
		line += " as of " + e.At
	}
	if e.Error != "" {
		return line + " error: " + e.Error
	}
	line += fmt.Sprintf(" %d %s", e.Hits, pluralFacts(e.Hits))
	if e.Took > 0 {
		line += fmt.Sprintf(" in %dms", e.Took)
	}
	if e.Model != "" {
		line += " with " + e.Model
	}
	if len(e.Facts) > 0 {
		line += " (" + strings.Join(e.Facts, "; ") + ")"
	}
	return line
}

// Result is `queries --json`: the entries, newest first, and how many the log
// holds in all.
type Result struct {
	Queries []Entry `json:"queries"`
	Total   int     `json:"total"`
	Verb    string  `json:"verb,omitempty"`
}

// Lines is one line per query, newest first.
func (r Result) Lines() []string {
	out := make([]string, len(r.Queries))
	for i, e := range r.Queries {
		out[i] = e.Line()
	}
	return out
}

func pluralFacts(n int) string {
	if n == 1 {
		return "fact"
	}
	return "facts"
}

// Log is the file. A nil *Log, or one opened with an empty path, is a log that
// records nothing and reads empty: the queries are a convenience and no query
// should fail because of them.
type Log struct {
	path string
	now  func() time.Time

	mu    sync.Mutex
	lines int
}

// Open prepares the log at path ("" turns recording off). It counts what is
// already there so a restart keeps trimming at the same size.
func Open(path string, now func() time.Time) *Log {
	if now == nil {
		now = time.Now
	}
	l := &Log{path: path, now: now}
	if path != "" {
		l.lines = countLines(path)
	}
	return l
}

// Path is where the log lives, or "" when nothing is recorded.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Add appends one entry. An entry with no Time gets the log's clock.
func (l *Log) Add(e Entry) error {
	if l == nil || l.path == "" {
		return nil
	}
	if e.Time == "" {
		e.Time = l.now().UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// The directory is not always there: a Neo4j-backed serve has no store file,
	// so nothing created one next to the path it was given.
	if dir := filepath.Dir(l.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	l.lines++
	if l.lines > grow {
		return l.trimLocked()
	}
	return nil
}

// Read returns the last n entries — newest first, only verb when verb is not
// "" — and how many the log holds before the filter.
func (l *Log) Read(verb string, n int) (Result, error) {
	res := Result{Queries: []Entry{}, Verb: verb}
	if l == nil || l.path == "" {
		return res, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := read(l.path)
	if err != nil {
		return res, err
	}
	res.Total = len(all)
	if verb != "" {
		var kept []Entry
		for _, e := range all {
			if e.Verb == verb {
				kept = append(kept, e)
			}
		}
		all = kept
	}
	if n <= 0 {
		n = DefaultN
	}
	for i := len(all) - 1; i >= 0 && len(res.Queries) < n; i-- {
		res.Queries = append(res.Queries, all[i])
	}
	return res, nil
}

// trimLocked rewrites the file with the last Keep entries. A line that does not
// parse is dropped rather than kept: a log that cannot be read is no log at all,
// and the entries are the cheap part.
func (l *Log) trimLocked() error {
	all, err := read(l.path)
	if err != nil {
		return err
	}
	if len(all) <= Keep {
		l.lines = len(all)
		return nil
	}
	all = all[len(all)-Keep:]
	tmp := l.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, e := range all {
		if err := enc.Encode(e); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, l.path); err != nil {
		os.Remove(tmp)
		return err
	}
	l.lines = len(all)
	return nil
}

// read parses the file in the order it was written, oldest first. A missing file
// is an empty log, not an error.
func read(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		n++
	}
	return n
}

// CheckVerb reports whether verb is one this log keeps, or "" for all of them.
// The message is here so the CLI and the server refuse the same word the same way.
func CheckVerb(verb string) error {
	if verb == "" {
		return nil
	}
	for _, v := range Verbs {
		if v == verb {
			return nil
		}
	}
	return fmt.Errorf("unknown verb %q: use one of %s", verb, strings.Join(Verbs, ", "))
}

// Fact renders one result fact the way the log names it: the triple, no uuid and
// no time, because the entry already says when the query was asked.
func Fact(subject, relation, object string) string {
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", subject, relation, object))
}

// Path returns the path of the query log next to a brain file, which is where the
// server keeps its side files (tokens.json lives there too).
func Path(brainFile string) string {
	return filepath.Join(filepath.Dir(brainFile), "queries.jsonl")
}
