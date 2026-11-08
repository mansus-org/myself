package brain

import (
	"fmt"
	"regexp"
	"strconv"
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

func primeTestMore(k int) string {
	return fmt.Sprintf("  … and %d more (myself ls, myself search <text>)\n", k)
}

func TestPrimeIndexOrderAndFormat(t *testing.T) {
	text, truncated := primeText(primeGraph(primeFixture...), 4000)
	if truncated {
		t.Error("truncated with a large budget")
	}
	want := "self\n  is agent\n  related felipe\n" +
		"agent\n  <- is: self\n" +
		"felipe\n  is person\n  has a long note\n  <- self related\n" +
		primeTestHeader +
		"  casa-mimosa (place, project; 3 facts)\n" +
		"  relay (project; 3 facts)\n" +
		"  bun (tool; 1 fact)\n" +
		"  scratch (1 fact)\n"
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
	for _, typ := range []string{"person (", "agent (", "project (", "place (", "tool ("} {
		if strings.Contains(text, typ) {
			t.Errorf("type node indexed: %q", typ)
		}
	}
}

func TestPrimeIndexOrderByFacts(t *testing.T) {
	// heavy has more live records than light, so it is listed first even
	// though its name sorts later.
	g := primeGraph(
		"light is project", "light has x",
		"heavy is project", "heavy has 1", "heavy has 2", "heavy has 3", "heavy uses x",
		"bare has x",
	)
	text, _ := primeText(g, 4000)
	want := primeTestHeader +
		"  heavy (project; 5 facts)\n" +
		"  light (project; 2 facts)\n" +
		"  bare (1 fact)\n"
	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}
}

func TestPrimeIndexMore(t *testing.T) {
	g := primeGraph(primeFixture...)
	all := []string{
		"  casa-mimosa (place, project; 3 facts)",
		"  relay (project; 3 facts)",
		"  bun (tool; 1 fact)",
		"  scratch (1 fact)",
	}
	trailer := regexp.MustCompile(`(?m)^  … and (\d+) more \(myself ls, myself search <text>\)$`)
	checked := 0
	for budget := 150; budget <= 500; budget++ {
		text, truncated := primeText(g, budget)
		m := trailer.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		checked++
		if !truncated {
			t.Errorf("budget %d: trailer but not truncated", budget)
		}
		k, _ := strconv.Atoi(m[1])
		i := strings.Index(text, primeTestHeader)
		if i < 0 {
			t.Fatalf("budget %d: no header:\n%s", budget, text)
		}
		shown := 0
		for _, l := range strings.Split(text[i+len(primeTestHeader):], "\n") {
			if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "  …") {
				shown++
			}
		}
		if shown+k != len(all) {
			t.Errorf("budget %d: %d listed, trailer says %d, want %d total:\n%s", budget, shown, k, len(all), text)
		}
		if want := strings.Join(all[:shown], "\n"); shown > 0 && !strings.Contains(text, want) {
			t.Errorf("budget %d: the topics listed are not the first %d:\n%s", budget, shown, text)
		}
		if n := utf8.RuneCountInString(text); n > budget {
			t.Errorf("budget %d: len %d", budget, n)
		}
	}
	if checked == 0 {
		t.Error("no budget in range produced a trailer")
	}
}

// TestPrimeTrimsLeastUsedFirst checks that a tight budget drops the topics with
// the fewest facts, and only those.
func TestPrimeTrimsLeastUsedFirst(t *testing.T) {
	var triples []string
	for i := 0; i < 20; i++ {
		triples = append(triples, fmt.Sprintf("big-%02d is project", i))
		for j := 0; j < 10; j++ {
			triples = append(triples, fmt.Sprintf("big-%02d has fact %d", i, j))
		}
	}
	for i := 0; i < 20; i++ {
		triples = append(triples, fmt.Sprintf("small-%02d has a lone fact", i))
	}
	text, truncated := primeText(primeGraph(triples...), 1000)
	if !truncated {
		t.Error("not truncated")
	}
	if n := utf8.RuneCountInString(text); n > 1000 {
		t.Errorf("len %d > 1000", n)
	}
	var bigs, smalls int
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "  big-"):
			bigs++
		case strings.HasPrefix(l, "  small-"):
			smalls++
		}
	}
	if bigs != 20 {
		t.Errorf("%d of 20 heavy topics listed:\n%s", bigs, text)
	}
	if smalls == 20 {
		t.Error("nothing was trimmed")
	}
	want := fmt.Sprintf("  … and %d more (myself ls, myself search <text>)", 20-smalls)
	if !strings.Contains(text, want) {
		t.Errorf("trailer should count the %d trimmed light topics:\n%s", 20-smalls, text)
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

// TestPrimeCutEntityMovesToIndex sweeps budgets on a brain whose Part A cannot
// fit: an entity cut before its first line must be listed in the index, not
// dropped.
func TestPrimeCutEntityMovesToIndex(t *testing.T) {
	triples := append([]string{}, primeFixture...)
	triples = append(triples, "self does "+strings.Repeat("x", 300))
	for i := 2; i <= 6; i++ {
		triples = append(triples, fmt.Sprintf("felipe has note %d", i))
	}
	g := primeGraph(triples...)
	checked := 0
	for budget := 200; budget <= 2000; budget++ {
		text, _ := primeText(g, budget)
		if strings.Contains(text, "\nfelipe\n") {
			continue
		}
		i := strings.Index(text, primeTestHeader)
		if i < 0 {
			continue
		}
		lines := 0
		for _, l := range strings.Split(text[i+len(primeTestHeader):], "\n") {
			if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "  \u2026 and") {
				lines++
			}
		}
		if lines == 0 {
			continue
		}
		checked++
		if !strings.Contains(text, "  felipe (person; 8 facts)") {
			t.Errorf("budget %d: felipe cut but not indexed:\n%s", budget, text)
			break
		}
	}
	if checked == 0 {
		t.Error("no budget cut a Part A entity with an index left")
	}
}

func TestPrimeNoSelf(t *testing.T) {
	g := primeGraph("relay is project", "relay has a server", "bun is tool")
	text, truncated := primeText(g, 4000)
	want := primeTestHeader + "  relay (project; 2 facts)\n  bun (tool; 1 fact)\n"
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

// TestPrimeEveryEntityAccountedFor sweeps budgets on a bigger brain: every
// entity is either printed as a block, listed in the index, or the output
// reports itself truncated.
func TestPrimeEveryEntityAccountedFor(t *testing.T) {
	var triples []string
	for i := 0; i < 40; i++ {
		triples = append(triples, fmt.Sprintf("topic-%02d is project", i))
		for j := 0; j < i%5; j++ {
			triples = append(triples, fmt.Sprintf("topic-%02d has fact %d", i, j))
		}
	}
	g := primeGraph(triples...)
	for budget := 1; budget <= 3000; budget += 7 {
		text, truncated := primeText(g, budget)
		if n := utf8.RuneCountInString(text); n > budget {
			t.Fatalf("budget %d: len %d", budget, n)
		}
		if truncated {
			continue
		}
		for i := 0; i < 40; i++ {
			name := fmt.Sprintf("topic-%02d", i)
			if !strings.Contains(text, name) {
				t.Fatalf("budget %d: %s missing and not truncated", budget, name)
			}
		}
	}
}
