package cli

import (
	"sort"
	"strings"

	"github.com/mansus-org/myself/internal/model"
)

// renderEntity returns the lines of one entity as `describe` prints it
// (SPEC section 9): the name flush left, then `<relation> <object>` for each
// outgoing record and `<- <subject> <relation>` for each incoming one,
// indented two spaces. Shared by describe and prime.
func renderEntity(name string, out, in []model.Record) []string {
	lines := []string{name}
	for _, r := range out {
		lines = append(lines, "  "+r.Relation+" "+r.Object)
	}
	for _, r := range in {
		lines = append(lines, "  <- "+r.Subject+" "+r.Relation)
	}
	return lines
}

// renderPrimeEntity is renderEntity with the incoming `is` records folded
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
