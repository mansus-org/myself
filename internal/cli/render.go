package cli

import (
	"github.com/mansus-org/myself/internal/graph"
)

// renderEntity returns the lines of an entity as `describe` prints it at
// depth 0 (SPEC section 9): the name flush left, then `<relation> <object>`
// for each outgoing record and `<- <subject> <relation>` for each incoming
// one, indented two spaces.
func renderEntity(g *graph.Graph, name string) []string {
	lines := []string{name}
	for _, r := range g.Out(name) {
		lines = append(lines, "  "+r.Relation+" "+r.Object)
	}
	for _, r := range g.In(name) {
		lines = append(lines, "  <- "+r.Subject+" "+r.Relation)
	}
	return lines
}
