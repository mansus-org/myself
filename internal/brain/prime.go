package brain

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
)

// DefaultBudget is the default of prime --budget.
const DefaultBudget = 2000

// PrimeResult is `prime --json`.
type PrimeResult struct {
	Budget    int    `json:"budget"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text"`
}

// Prime renders the summary of the brain cut to budget characters.
func Prime(st Store, budget int) (PrimeResult, error) {
	if budget < 1 {
		return PrimeResult{}, Usagef("--budget must be at least 1")
	}
	snap, err := load(st)
	if err != nil {
		return PrimeResult{}, err
	}
	text, truncated := primeText(snap.Graph(), budget)
	return PrimeResult{budget, truncated, text}, nil
}

// primeText renders `self` first, then the entities directly linked to it,
// then the rest by live record count (descending, then name), stopping before
// the first line that would push the text past budget characters.
func primeText(g *graph.Graph, budget int) (text string, truncated bool) {
	names := g.Entities()
	count := make(map[string]int, len(names))
	for _, n := range names {
		count[n] = len(g.Out(n)) + len(g.In(n))
	}
	sort.SliceStable(names, func(i, j int) bool { return count[names[i]] > count[names[j]] })

	order := make([]string, 0, len(names))
	placed := make(map[string]bool, len(names))
	for _, s := range g.BFS("self", 1) {
		order = append(order, s.Name)
		placed[s.Name] = true
	}
	for _, n := range names {
		if !placed[n] {
			order = append(order, n)
		}
	}

	var b strings.Builder
	used := 0
	for _, n := range order {
		for _, line := range renderPrimeEntity(n, g.Out(n), g.In(n)) {
			size := utf8.RuneCountInString(line) + 1
			if used+size > budget {
				return b.String(), true
			}
			fmt.Fprintln(&b, line)
			used += size
		}
	}
	return b.String(), false
}

// RenderEntity returns the lines of one entity as `describe` prints it
// (SPEC section 9): the name flush left, then `<relation> <object>` for each
// outgoing record and `<- <subject> <relation>` for each incoming one,
// indented two spaces.
func RenderEntity(name string, out, in []model.Record) []string {
	lines := []string{name}
	for _, r := range out {
		lines = append(lines, "  "+r.Relation+" "+r.Object)
	}
	for _, r := range in {
		lines = append(lines, "  <- "+r.Subject+" "+r.Relation)
	}
	return lines
}

// renderPrimeEntity is RenderEntity with the incoming `is` records folded
// into one `  <- is: a, b, c` line (subjects sorted), since a type with many
// members would otherwise spend a line on each.
func renderPrimeEntity(name string, out, in []model.Record) []string {
	lines := []string{name}
	for _, r := range out {
		lines = append(lines, "  "+r.Relation+" "+r.Object)
	}
	var isSubjects []string
	for _, r := range in {
		if r.Relation == "is" {
			isSubjects = append(isSubjects, r.Subject)
			continue
		}
		lines = append(lines, "  <- "+r.Subject+" "+r.Relation)
	}
	if len(isSubjects) > 0 {
		sort.Strings(isSubjects)
		lines = append(lines, "  <- is: "+strings.Join(isSubjects, ", "))
	}
	return lines
}
