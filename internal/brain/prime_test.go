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

func TestPrimeIndexOrderAndFormat(t *testing.T) {
	text, truncated := primeText(primeGraph(primeFixture...), 4000)
	if truncated {
		t.Error("truncated with a large budget")
	}
	want := "self\n  is agent\n  related felipe\n" +
		"agent\n  <- is: self\n" +
		"felipe\n  is person\n  has a long note\n  <- self related\n" +
		"Also in the brain, not loaded (run `myself describe <name>` before working on any of these, or before saying you don't know about one):\n" +
		"  casa-mimosa (place, project; 3 facts)\n" +
		"  relay (project; 3 facts)\n" +
		"  bun (tool; 1 fact)\n" +
		"  scratch (1 fact)\n"
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
	for _, typ := range []string{"  project (", "  person (", "  place (", "  tool ("} {
		if strings.Contains(text, typ) {
			t.Errorf("type node indexed: %q", typ)
		}
	}
}

func TestPrimeIndexMore(t *testing.T) {
	g := primeGraph(primeFixture...)
	full, _ := primeText(g, 4000)
	// Leave room for one index line and the trailer: a "… and K more" line
	// naming the other three.
	j := strings.Index(full, "  relay (")
	budget := utf8.RuneCountInString(full[:j]) + utf8.RuneCountInString(primeMore(3)) + 1
	text, truncated := primeText(g, budget)
	if !truncated {
		t.Error("not truncated")
	}
	if !strings.HasSuffix(text, "  casa-mimosa (place, project; 3 facts)\n  … and 3 more (myself ls, myself search <text>)\n") {
		t.Errorf("missing trailer:\n%s", text)
	}
	if strings.Contains(text, "scratch") || strings.Contains(text, "relay (") {
		t.Errorf("cut lines present:\n%s", text)
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
	// Part A capped at 3/5 when an index follows: self does not fit in 40
	// at 3/5 (24), so the first lines are cut and the header cannot fit.
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
	if !strings.Contains(text, "  felipe (person; 5 facts)\n") && !strings.Contains(text, "\nfelipe\n") {
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
		"  relay (project; 2 facts)\n  bun (tool; 1 fact)\n"
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
