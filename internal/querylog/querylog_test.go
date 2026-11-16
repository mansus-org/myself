package querylog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "queries.jsonl")
}

func add(t *testing.T, l *Log, verb, text string, hits int) {
	t.Helper()
	if err := l.Add(Entry{Verb: verb, Text: text, Hits: hits}); err != nil {
		t.Fatalf("add %s: %v", text, err)
	}
}

func TestAddThenReadNewestFirst(t *testing.T) {
	l := Open(testPath(t), nil)
	add(t, l, "search", "bun", 1)
	add(t, l, "describe", "relay", 3)
	add(t, l, "recall", "what does relay run on", 2)

	res, err := l.Read("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(res.Queries) != 3 {
		t.Fatalf("three questions: %+v", res)
	}
	if got := []string{res.Queries[0].Verb, res.Queries[1].Verb, res.Queries[2].Verb}; got[0] != "recall" || got[2] != "search" {
		t.Fatalf("newest first: %v", got)
	}
	for _, e := range res.Queries {
		if _, err := time.Parse(time.RFC3339, e.Time); err != nil {
			t.Fatalf("the clock fills in the time: %q: %v", e.Time, err)
		}
	}
}

func TestReadFiltersByVerbAndCount(t *testing.T) {
	l := Open(testPath(t), nil)
	add(t, l, "search", "one", 1)
	add(t, l, "search", "two", 1)
	add(t, l, "describe", "relay", 2)

	res, err := l.Read("search", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verb != "search" || len(res.Queries) != 2 || res.Total != 3 {
		t.Fatalf("filtered by verb, total before the filter: %+v", res)
	}
	if res.Queries[0].Text != "two" {
		t.Fatalf("newest first within the filter: %+v", res.Queries)
	}
	res, err = l.Read("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Queries) != 1 || res.Queries[0].Verb != "describe" {
		t.Fatalf("-n 1 keeps the newest: %+v", res.Queries)
	}
	res, err = l.Read("recall", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Queries) != 0 || res.Total != 3 {
		t.Fatalf("a verb nobody used is an empty answer, not an error: %+v", res)
	}
}

// The log is a convenience, so it is bounded: the file keeps the last Keep
// entries and the oldest go, without a second process or a cron job.
func TestTheFileStaysBounded(t *testing.T) {
	path := testPath(t)
	l := Open(path, nil)
	for i := 0; i <= grow; i++ {
		add(t, l, "search", "q", 0)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
	}
	f.Close()
	if lines != Keep {
		t.Fatalf("after %d queries the file holds %d, want %d", grow+1, lines, Keep)
	}
	// A new process reads the same log and keeps trimming at the same size.
	again := Open(path, nil)
	res, err := again.Read("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != Keep || len(res.Queries) != 1 {
		t.Fatalf("a restart reads the whole log: %+v", res)
	}
	add(t, again, "search", "newest", 0)
	res, err = again.Read("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Queries[0].Text != "newest" {
		t.Fatalf("the newest is still the newest: %+v", res.Queries)
	}
}

// A line that does not parse is dropped, not fatal: the entries are the cheap
// part of the system, and a log that cannot be read is no log at all.
func TestABrokenLineIsDropped(t *testing.T) {
	path := testPath(t)
	l := Open(path, nil)
	add(t, l, "search", "one", 1)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("not json at all\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	add(t, l, "search", "two", 1)

	res, err := l.Read("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("the broken line is not a query: %+v", res)
	}
}

// A log that is not there is not a fault of the caller's: recording nothing and
// reading empty is the answer, not an error.
func TestNoLogIsNotAnError(t *testing.T) {
	off := Open("", nil)
	if err := off.Add(Entry{Verb: "search", Text: "x"}); err != nil {
		t.Fatalf("a log that is off: %v", err)
	}
	res, err := off.Read("", 5)
	if err != nil || res.Total != 0 || len(res.Queries) != 0 {
		t.Fatalf("a log that is off reads empty: %+v %v", res, err)
	}
	if off.Path() != "" {
		t.Fatalf("Path: %q", off.Path())
	}
	var none *Log
	if err := none.Add(Entry{Verb: "search"}); err != nil {
		t.Fatalf("a nil log: %v", err)
	}
	if res, err := none.Read("", 5); err != nil || res.Total != 0 {
		t.Fatalf("a nil log reads empty: %+v %v", res, err)
	}
}

func TestEntryLine(t *testing.T) {
	e := Entry{
		Time:  "2026-01-01T00:00:00Z",
		Verb:  "recall",
		Text:  "what does relay run on",
		Hits:  2,
		Facts: []string{"relay uses bun", "bun is runtime"},
		Took:  812,
		Model: "nomic",
	}
	want := `2026-01-01T00:00:00Z recall "what does relay run on" 2 facts in 812ms with nomic (relay uses bun; bun is runtime)`
	if got := e.Line(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	e.Hits, e.Facts, e.Model = 1, nil, ""
	if got := e.Line(); !strings.Contains(got, " 1 fact ") {
		t.Fatalf("one fact is not facts: %q", got)
	}
	e.At = "2026-01-01"
	e.Error = "no vectors here"
	got := e.Line()
	if !strings.Contains(got, "as of 2026-01-01") || !strings.Contains(got, "error: no vectors here") {
		t.Fatalf("a failed query says when it was asked and why it failed: %q", got)
	}
	if strings.Contains(got, "1 fact in") {
		t.Fatalf("a failure has no count: %q", got)
	}
}

func TestCheckVerb(t *testing.T) {
	for _, ok := range []string{"", "search", "recall", "describe"} {
		if err := CheckVerb(ok); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	err := CheckVerb("dream")
	if err == nil || !strings.Contains(err.Error(), strings.Join(Verbs, ", ")) {
		t.Fatalf("a bad verb is refused by name: %v", err)
	}
}

func TestPathNextToTheBrain(t *testing.T) {
	if got := Path("/data/brain.jsonl"); got != filepath.Join("/data", "queries.jsonl") {
		t.Fatalf("got %q", got)
	}
}

// A Neo4j-backed serve has no store file, so nothing has made the directory the
// log was given. Making the question fail because a folder is missing would be
// absurd.
func TestAddMakesTheDirectoryItNeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not", "yet", "queries.jsonl")
	l := Open(path, nil)
	if err := l.Add(Entry{Verb: "search", Text: "bun", Hits: 1}); err != nil {
		t.Fatal(err)
	}
	res, err := l.Read("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("one question kept: %+v", res)
	}
}

// The file is JSONL: one entry per line, so a reader can be a shell pipeline.
func TestTheFileIsOneEntryPerLine(t *testing.T) {
	path := testPath(t)
	l := Open(path, nil)
	if err := l.Add(Entry{Time: "2026-01-01T00:00:00Z", Verb: "search", Text: `a "quoted" text`, Hits: 1}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("one query is one line: %q", string(b))
	}
	var e Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Text != `a "quoted" text` {
		t.Fatalf("text survives: %q", e.Text)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("what people asked is not for everyone: %v", err)
	}
}
