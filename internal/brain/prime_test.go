package brain

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
)

// primeGraph builds a graph from "subject relation object" triples.
func primeGraph(triples ...string) *graph.Graph {
	recs := make([]model.Record, 0, len(triples))
	for i, t := range triples {
		f := strings.SplitN(t, " ", 3)
		recs = append(recs, model.Record{
			UUID:     fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
			Subject:  f[0],
			Relation: f[1],
			Object:   f[2],
			Time:     "2026-01-01T00:00:00Z",
		})
	}
	return NewSnapshot(recs).Graph()
}

var primeFixture = []string{
	"self is agent",
	"self related felipe",
	"felipe is person",
	"felipe has a long note",
	"relay is project",
	"relay has a Go server",
	"relay has a web client",
	"casa-mimosa is project",
	"casa-mimosa is place",
	"casa-mimosa has a garden",
	"bun is tool",
	"scratch has a lone fact",
}

const primeTestHeader = "Also in the brain, not loaded (run `myself describe <name>` before working on any of these, or before saying you don't know about one):\n"

func TestPrimeIndexOrderAndFormat(t *testing.T) {
	text, truncated := primeText(primeGraph(primeFixture...), 4000)
	if truncated {
		t.Error("truncated with a large budget")
	}
	want := "self\n  is agent\n  related felipe\n" +
		"agent\n  <- is: self\n" +
		"felipe\n  is person\n  has a long note\n  <- self related\n" +
		primeTestHeader +
		"  project (2): casa-mimosa, relay\n" +
		"  place (1): casa-mimosa\n" +
		"  tool (1): bun\n" +
		"  other (1): scratch\n"
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
	for _, typ := range []string{"  person (", "  agent ("} {
		if strings.Contains(text, typ) {
			t.Errorf("type node indexed: %q", typ)
		}
	}
	if strings.Contains(text, "fact") {
		t.Errorf("fact counts in the index:\n%s", text)
	}
}

func TestPrimeIndexOrderByMemberCount(t *testing.T) {
	// big is a type with one heavy member; many has three light ones, but
	// fewer records in total.
	g := primeGraph(
		"a is many", "a has x",
		"b is many", "b has x",
		"c is many", "c has x",
		"h is big", "h has 1", "h has 2", "h has 3", "h has 4", "h has 5", "h has 6", "h has 7",
		"z has x", "y has x", "y has z",
	)
	text, _ := primeText(g, 4000)
	want := primeTestHeader +
		"  big (1): h\n" +
		"  many (3): a, b, c\n" +
		"  other (2): y, z\n"
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
}

func TestPrimeIndexMore(t *testing.T) {
	g := primeGraph(primeFixture...)
	full, _ := primeText(g, 4000)
	// Room for the project line and the trailer for the three other types.
	j := strings.Index(full, "  place (")
	budget := utf8.RuneCountInString(full[:j]) + utf8.RuneCountInString(primeMoreTypes(3)) + 1
	text, truncated := primeText(g, budget)
	if !truncated {
		t.Error("not truncated")
	}
	if !strings.HasSuffix(text, "  project (2): casa-mimosa, relay\n  … and 3 more types (myself ls)\n") {
		t.Errorf("missing trailer:\n%s", text)
	}
	if n := utf8.RuneCountInString(text); n > budget {
		t.Errorf("len %d > budget %d", n, budget)
	}
}

func TestPrimeIndexShortLine(t *testing.T) {
	g := primeGraph(primeFixture...)
	full, _ := primeText(g, 4000)
	j := strings.Index(full, "  project (")
	short := "  project (2): casa-mimosa, … +1"
	budget := utf8.RuneCountInString(full[:j]) + utf8.RuneCountInString(short) + 1 + utf8.RuneCountInString(primeMoreTypes(3)) + 1
	text, truncated := primeText(g, budget)
	if !truncated {
		t.Error("not truncated")
	}
	if !strings.HasSuffix(text, short+"\n  … and 3 more types (myself ls)\n") {
		t.Errorf("missing cut line:\n%s", text)
	}
	if n := utf8.RuneCountInString(text); n > budget {
		t.Errorf("len %d > budget %d", n, budget)
	}
}

func TestPrimeBudgets(t *testing.T) {
	g := primeGraph(primeFixture...)
	for _, budget := range []int{1, 40, 200, 4000} {
		text, _ := primeText(g, budget)
		if n := utf8.RuneCountInString(text); n > budget {
			t.Errorf("budget %d: len %d\n%s", budget, n, text)
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			t.Errorf("budget %d: cut inside a line: %q", budget, text)
		}
	}
	// Part A capped at 3/4 when an index follows: self does not fit in 40
	// at 3/4 (30), so the first lines are cut and the header cannot fit.
	text, truncated := primeText(g, 40)
	if !truncated || strings.Contains(text, "Also in the brain") {
		t.Errorf("budget 40: truncated=%v\n%s", truncated, text)
	}
}

func TestPrimeCutEntityMovesToIndex(t *testing.T) {
	g := primeGraph(primeFixture...)
	text, truncated := primeText(g, 300)
	if !truncated {
		t.Error("not truncated")
	}
	if !strings.Contains(text, "\nfelipe\n") && !strings.Contains(text, "person (1): felipe") {
		t.Errorf("felipe neither printed nor indexed:\n%s", text)
	}
	if n := utf8.RuneCountInString(text); n > 300 {
		t.Errorf("len %d > 300", n)
	}
}

func TestPrimeNoSelf(t *testing.T) {
	g := primeGraph("relay is project", "relay has a server", "bun is tool")
	text, truncated := primeText(g, 4000)
	want := "Also in the brain, not loaded (run `myself describe <name>` before working on any of these, or before saying you don't know about one):\n" +
		"  project (1): relay\n  tool (1): bun\n"
	if text != want || truncated {
		t.Errorf("got %v:\n%s", truncated, text)
	}
	for _, budget := range []int{40, 200} {
		text, _ := primeText(g, budget)
		if n := utf8.RuneCountInString(text); n > budget {
			t.Errorf("budget %d: len %d", budget, n)
		}
	}
}

func TestPrimeOnlySelf(t *testing.T) {
	text, truncated := primeText(primeGraph("self is agent"), 4000)
	if truncated || strings.Contains(text, "Also in the brain") {
		t.Errorf("unexpected index: %v\n%s", truncated, text)
	}
}

func TestPrimeBudgetSweep(t *testing.T) {
	g := primeGraph(primeFixture...)
	for budget := 1; budget <= 400; budget++ {
		text, _ := primeText(g, budget)
		if n := utf8.RuneCountInString(text); n > budget {
			t.Fatalf("budget %d: len %d\n%s", budget, n, text)
		}
	}
}

func TestPrimeLineCap(t *testing.T) {
	var triples []string
	for i := 0; i < 30; i++ {
		triples = append(triples, fmt.Sprintf("item-%02d is big", i), fmt.Sprintf("item-%02d has x", i))
	}
	triples = append(triples, "solo is small", "solo has x")
	text, truncated := primeText(primeGraph(triples...), 1000)
	if !truncated {
		t.Error("not truncated")
	}
	// No self: Part B has the whole budget, so the cap is 1000/5 = 200.
	var big, small string
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "  big ("):
			big = l
		case strings.HasPrefix(l, "  small ("):
			small = l
		}
	}
	if n := utf8.RuneCountInString(big); n > 200 || !strings.Contains(big, ", … +") {
		t.Errorf("big line len %d: %q", n, big)
	}
	if small != "  small (1): solo" {
		t.Errorf("small line missing: %q\n%s", small, text)
	}
}
