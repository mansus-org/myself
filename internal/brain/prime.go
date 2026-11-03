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

// primeOther is the index group of entities with no `is` record.
const primeOther = "other"

// primeMinLine is the least a type line of the index may be capped at.
const primeMinLine = 60

// primeMoreTypes is the index trailer when not every type line fits.
func primeMoreTypes(k int) string {
	return fmt.Sprintf("  … and %d more types (myself ls)", k)
}

// primeText renders two parts, cut to budget characters. Part A is full
// blocks rendered exactly as renderPrimeEntity prints them: `self` first (if
// it exists), then the entities directly linked to it, breadth-first, and
// nothing else. Part B is an index of the rest under primeHeader: the
// entities with at least one live outgoing record, plus any Part A entity cut
// before its first line was printed, grouped by type, one line per type,
// `  <type> (<N>): <name>, <name>, ...`. An entity with several types is
// listed under each; one with none goes last under `other`. Types are ordered
// by the summed live record count of their members (descending, then name),
// names within a type by their own count (descending, then name). A line that
// does not fit, or is over the line cap, loses names from its end and
// finishes with `, … +K`; types after a line cut by the budget are replaced
// by a primeMoreTypes trailer. Part A uses at most
// budget*3/4 characters when Part B has at least one entry, otherwise the
// whole budget; Part B uses what is left. Every cut is at a line boundary,
// and the total never exceeds budget.
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
	// cut entity then moves to the index, re-render Part A with the 3/4 cap.
	b, used, missing, cut := renderA(budget)
	if len(candidates) > 0 || len(missing) > 0 {
		b, used, missing, cut = renderA(budget * 3 / 4)
	}
	truncated = cut

	// Part B entries: the candidates plus the Part A entities that got no
	// line at all.
	entries := append(append([]string{}, candidates...), missing...)
	sort.Slice(entries, func(i, j int) bool {
		if count[entries[i]] != count[entries[j]] {
			return count[entries[i]] > count[entries[j]]
		}
		return entries[i] < entries[j]
	})
	members := make(map[string][]string)
	sum := make(map[string]int)
	for _, e := range entries {
		ts := primeTypes(g.Out(e))
		if len(ts) == 0 {
			ts = []string{primeOther}
		}
		for _, t := range ts {
			members[t] = append(members[t], e)
			sum[t] += count[e]
		}
	}
	types := make([]string, 0, len(members))
	for t := range members {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool {
		a, c := types[i], types[j]
		if (a == primeOther) != (c == primeOther) {
			return c == primeOther
		}
		if sum[a] != sum[c] {
			return sum[a] > sum[c]
		}
		return a < c
	})

	// Fit the index into the budget Part A left. Each type line is capped at
	// max(primeMinLine, B/5) characters, B being the room left for Part B, so
	// one big type cannot take the whole index. Room is reserved for the
	// primeMoreTypes trailer whenever types remain after a line.
	if len(types) > 0 {
		size := func(s string) int { return utf8.RuneCountInString(s) + 1 }
		room := budget - used
		lineCap := max(primeMinLine, room/5)
		avail := room - size(primeHeader)
		n := len(types)
		var lines []string
		total := 0
		for i, t := range types {
			names := members[t]
			reserve := 0
			if i < n-1 {
				reserve = size(primeMoreTypes(n - i - 1))
			}
			kcap := len(names)
			for kcap > 1 && utf8.RuneCountInString(primeTypeLine(t, names, kcap)) > lineCap {
				kcap--
			}
			placed, budgetCut := false, false
			for k := kcap; k >= 1; k-- {
				line := primeTypeLine(t, names, k)
				if total+size(line)+reserve > avail {
					continue
				}
				lines = append(lines, line)
				total += size(line)
				placed, budgetCut = true, k < kcap
				if k < len(names) {
					truncated = true
				}
				break
			}
			if !placed || budgetCut {
				break
			}
		}
		rest := func() int { return n - len(lines) }
		if rest() > 0 {
			truncated = true
			for len(lines) > 0 && total+size(primeMoreTypes(rest())) > avail {
				total -= size(lines[len(lines)-1])
				lines = lines[:len(lines)-1]
			}
		}
		if rest() == 0 || total+size(primeMoreTypes(rest())) <= avail {
			fmt.Fprintln(b, primeHeader)
			for _, l := range lines {
				fmt.Fprintln(b, l)
			}
			if rest() > 0 {
				fmt.Fprintln(b, primeMoreTypes(rest()))
			}
		} else {
			truncated = true
		}
	}
	return b.String(), truncated
}

// primeTypeLine is one index line, `  <type> (<N>): <names>` with N the
// number of members, showing the first k names and, when k < N, `, … +K` for
// the K left out.
func primeTypeLine(typ string, names []string, k int) string {
	s := fmt.Sprintf("  %s (%d): %s", typ, len(names), strings.Join(names[:k], ", "))
	if k < len(names) {
		s += fmt.Sprintf(", … +%d", len(names)-k)
	}
	return s
}

// primeTypes returns the objects of the live outgoing `is` records, sorted
// and without duplicates.
func primeTypes(out []model.Record) []string {
	seen := make(map[string]bool)
	var types []string
	for _, r := range out {
		if r.Relation == "is" && !seen[r.Object] {
			seen[r.Object] = true
			types = append(types, r.Object)
		}
	}
	sort.Strings(types)
	return types
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
