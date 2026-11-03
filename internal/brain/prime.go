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

// primeHeader opens the index part of a prime output (SPEC section 5).
const primeHeader = "Also in the brain, not loaded (run `myself describe <name>` before working on any of these, or before saying you don't know about one):"

// primeMore is the index trailer when not every index line fits.
func primeMore(k int) string {
	return fmt.Sprintf("  … and %d more (myself ls, myself search <text>)", k)
}

// primeText renders two parts, cut to budget characters. Part A is full
// blocks rendered exactly as renderPrimeEntity prints them: `self` first (if
// it exists), then the entities directly linked to it, breadth-first, and
// nothing else. Part B is an index of the rest, one name-only line per entity
// with at least one live outgoing record, plus any Part A entity cut before
// its first line was printed, in live record count order (descending, then
// name) under primeHeader, with a primeMore trailer when the lines are cut.
// Part A uses at most budget*3/5 characters when Part B has at least one
// entry, otherwise the whole budget; Part B uses what is left. Every cut is
// at a line boundary, and the total never exceeds budget.
func primeText(g *graph.Graph, budget int) (text string, truncated bool) {
	names := g.Entities()
	count := make(map[string]int, len(names))
	for _, n := range names {
		count[n] = len(g.Out(n)) + len(g.In(n))
	}

	// Part A names: `self`, then its one-hop neighbours, BFS order. An empty
	// list means `self` does not exist.
	var partA []string
	inA := make(map[string]bool, 1)
	for _, s := range g.BFS("self", 1) {
		partA = append(partA, s.Name)
		inA[s.Name] = true
	}

	// Index candidates: every entity not in Part A with at least one live
	// outgoing record. Pure type nodes (only incoming `is`) have no outgoing
	// records and stay out.
	var candidates []string
	for _, n := range names {
		if !inA[n] && len(g.Out(n)) > 0 {
			candidates = append(candidates, n)
		}
	}

	// renderA writes the Part A blocks within ab characters, stopping at the
	// first line that does not fit. missing holds the entities that got no
	// line at all; cut reports any cut.
	renderA := func(ab int) (b *strings.Builder, used int, missing []string, cut bool) {
		b = new(strings.Builder)
		for _, n := range partA {
			if cut {
				missing = append(missing, n)
				continue
			}
			printed := false
			for _, line := range renderPrimeEntity(n, g.Out(n), g.In(n)) {
				size := utf8.RuneCountInString(line) + 1
				if used+size > ab {
					cut = true
					break
				}
				fmt.Fprintln(b, line)
				used += size
				printed = true
			}
			if !printed {
				missing = append(missing, n)
			}
		}
		return b, used, missing, cut
	}

	// When nothing is to be indexed, Part A may take the whole budget. If a
	// cut entity then moves to the index, re-render Part A with the 3/5 cap.
	b, used, missing, cut := renderA(budget)
	if len(candidates) > 0 || len(missing) > 0 {
		b, used, missing, cut = renderA(budget * 3 / 5)
	}
	truncated = cut

	// Part B entries: the candidates plus the Part A entities that got no
	// line at all, in live record count order (descending, then name).
	type entry struct {
		name string
		n    int
	}
	entries := make([]entry, 0, len(candidates)+len(missing))
	for _, n := range candidates {
		entries = append(entries, entry{n, count[n]})
	}
	for _, n := range missing {
		entries = append(entries, entry{n, count[n]})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].n != entries[j].n {
			return entries[i].n > entries[j].n
		}
		return entries[i].name < entries[j].name
	})
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = primeIndexLine(e.name, g.Out(e.name), e.n)
	}

	// Fit the index into the budget Part A left. With p of the n lines
	// rendered, the exact final size is the header, the p lines, and the
	// trailer naming n-p when p < n; take the largest p that fits so the
	// total never exceeds the budget.
	if len(lines) > 0 {
		headerSize := utf8.RuneCountInString(primeHeader) + 1
		sizes := make([]int, len(lines))
		for i, l := range lines {
			sizes[i] = utf8.RuneCountInString(l) + 1
		}
		fit := func(p int) int {
			total := headerSize
			for i := 0; i < p; i++ {
				total += sizes[i]
			}
			if p < len(lines) {
				total += utf8.RuneCountInString(primeMore(len(lines)-p)) + 1
			}
			return total
		}
		p := -1
		for cand := len(lines); cand >= 0; cand-- {
			if fit(cand) <= budget-used {
				p = cand
				break
			}
		}
		if p < 0 {
			truncated = true
		} else {
			fmt.Fprintln(b, primeHeader)
			for i := 0; i < p; i++ {
				fmt.Fprintln(b, lines[i])
			}
			if p < len(lines) {
				fmt.Fprintln(b, primeMore(len(lines)-p))
				truncated = true
			}
		}
	}
	return b.String(), truncated
}

// primeIndexLine is one index line: `  <name> (<types>; <N> facts)`, with the
// types (the objects of the entity's live outgoing `is` records, sorted,
// comma-separated) omitted when there are none, and `1 fact` at N=1.
func primeIndexLine(name string, out []model.Record, n int) string {
	seen := make(map[string]bool)
	var types []string
	for _, r := range out {
		if r.Relation == "is" && !seen[r.Object] {
			seen[r.Object] = true
			types = append(types, r.Object)
		}
	}
	sort.Strings(types)
	facts := fmt.Sprintf("%d facts", n)
	if n == 1 {
		facts = "1 fact"
	}
	if len(types) > 0 {
		return "  " + name + " (" + strings.Join(types, ", ") + "; " + facts + ")"
	}
	return "  " + name + " (" + facts + ")"
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
