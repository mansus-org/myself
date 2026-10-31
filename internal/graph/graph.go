// Package graph is the in-memory index behind describe, ls and path
// (SPEC sections 1, 5 and 8). Build it once from the live records, then query
// it; a Graph is immutable and safe for concurrent reads.
//
// Every name passed to a Graph method is normalized with model.NormalizeName,
// so "Relay" finds "relay" and "me" finds "self". Methods that return records
// return them in uuid order when the input was in uuid order (store.Read
// sorts, and store.Live and store.AsOf preserve it).
package graph

import (
	"sort"

	"github.com/mansus-org/myself/internal/model"
)

// Graph indexes live records by entity. Entities are dense ints internally.
type Graph struct {
	recs  []model.Record
	ids   map[string]int // entity name -> id
	names []string       // id -> entity name

	subj []int // per record: subject entity id
	obj  []int // per record: object entity id, or -1 if the object is not an entity

	out [][]int // per entity: indices of records with that subject
	in  [][]int // per entity: indices of records whose object is that entity

	isOut [][]int // per entity: object ids of its `is` records
	isIn  [][]int // per entity: subject ids of `is` records pointing at it
}

// Step is one entity reached by BFS and its distance from the start.
type Step struct {
	Name  string
	Depth int
}

// Build indexes recs, which should be the live records (store.Live output).
// Forget records and records with an empty normalized subject are ignored.
// Records are expected in uuid order; if they are not they are sorted.
func Build(recs []model.Record) *Graph {
	g := &Graph{ids: make(map[string]int, len(recs))}

	g.recs = make([]model.Record, 0, len(recs))
	for _, r := range recs {
		if !r.IsForget() {
			g.recs = append(g.recs, r)
		}
	}
	if !sort.SliceIsSorted(g.recs, func(i, j int) bool { return g.recs[i].UUID < g.recs[j].UUID }) {
		sort.SliceStable(g.recs, func(i, j int) bool { return g.recs[i].UUID < g.recs[j].UUID })
	}

	// Pass 1: entities, and the normalized object of each record.
	g.subj = make([]int, len(g.recs))
	objName := make([]string, len(g.recs))
	for i, r := range g.recs {
		g.subj[i] = -1
		s := model.NormalizeName(r.Subject)
		if s == "" {
			continue
		}
		g.subj[i] = g.intern(s)
		objName[i] = model.NormalizeName(r.Object)
		if r.Relation == "is" && objName[i] != "" {
			g.intern(objName[i])
		}
	}

	// Pass 2: edges.
	n := len(g.names)
	g.out = make([][]int, n)
	g.in = make([][]int, n)
	g.isOut = make([][]int, n)
	g.isIn = make([][]int, n)
	g.obj = make([]int, len(g.recs))
	for i, r := range g.recs {
		g.obj[i] = -1
		s := g.subj[i]
		if s < 0 {
			continue
		}
		g.out[s] = append(g.out[s], i)
		if o, ok := g.ids[objName[i]]; ok && objName[i] != "" {
			g.obj[i] = o
			g.in[o] = append(g.in[o], i)
			if r.Relation == "is" {
				g.isOut[s] = append(g.isOut[s], o)
				g.isIn[o] = append(g.isIn[o], s)
			}
		}
	}
	return g
}

func (g *Graph) intern(name string) int {
	if id, ok := g.ids[name]; ok {
		return id
	}
	id := len(g.names)
	g.ids[name] = id
	g.names = append(g.names, name)
	return id
}

func (g *Graph) lookup(name string) (int, bool) {
	id, ok := g.ids[model.NormalizeName(name)]
	return id, ok
}

// Has reports whether name is an entity.
func (g *Graph) Has(name string) bool {
	_, ok := g.lookup(name)
	return ok
}

// Entities returns every entity name, sorted.
func (g *Graph) Entities() []string {
	out := append([]string(nil), g.names...)
	sort.Strings(out)
	return out
}

// Out returns the records whose subject is name, in uuid order. It includes
// records whose object is free text. Unknown names give nil.
func (g *Graph) Out(name string) []model.Record {
	id, ok := g.lookup(name)
	if !ok {
		return nil
	}
	return g.pick(g.out[id])
}

// In returns the records whose normalized object is the entity name, in uuid
// order. Unknown names give nil.
func (g *Graph) In(name string) []model.Record {
	id, ok := g.lookup(name)
	if !ok {
		return nil
	}
	return g.pick(g.in[id])
}

func (g *Graph) pick(idx []int) []model.Record {
	if len(idx) == 0 {
		return nil
	}
	out := make([]model.Record, len(idx))
	for i, ri := range idx {
		out[i] = g.recs[ri]
	}
	return out
}

// edge is one traversable link from the current entity: the record and the
// entity on its other end.
type edge struct{ rec, to int }

// edges returns every edge touching id in either direction, ordered by
// neighbour name then record uuid, so traversals are deterministic.
func (g *Graph) edges(id int) []edge {
	var es []edge
	for _, ri := range g.out[id] {
		if o := g.obj[ri]; o >= 0 {
			es = append(es, edge{ri, o})
		}
	}
	for _, ri := range g.in[id] {
		es = append(es, edge{ri, g.subj[ri]})
	}
	sort.Slice(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.to != b.to {
			return g.names[a.to] < g.names[b.to]
		}
		return a.rec < b.rec // records are uuid-ordered
	})
	return es
}

// BFS walks edges in both directions from name up to depth hops and returns
// every entity reached, start included at depth 0, in breadth-first order
// with ties broken by name. A negative depth is treated as 0. Unknown names
// give nil.
func (g *Graph) BFS(name string, depth int) []Step {
	start, ok := g.lookup(name)
	if !ok {
		return nil
	}
	seen := map[int]bool{start: true}
	steps := []Step{{g.names[start], 0}}
	level := []int{start}
	for d := 1; d <= depth && len(level) > 0; d++ {
		var next []int
		for _, id := range level {
			for _, e := range g.edges(id) {
				if !seen[e.to] {
					seen[e.to] = true
					next = append(next, e.to)
				}
			}
		}
		sort.Slice(next, func(i, j int) bool { return g.names[next[i]] < g.names[next[j]] })
		for _, id := range next {
			steps = append(steps, Step{g.names[id], d})
		}
		level = next
	}
	return steps
}

// ShortestPath returns a shortest chain of records linking a to b, walking
// edges in either direction, using at most maxDepth records. Ties are broken
// by neighbour name, then record uuid. The path is empty with ok true when a
// and b are the same entity; ok is false if either is unknown or no chain
// within maxDepth exists.
func (g *Graph) ShortestPath(a, b string, maxDepth int) (path []model.Record, ok bool) {
	from, ok1 := g.lookup(a)
	to, ok2 := g.lookup(b)
	if !ok1 || !ok2 {
		return nil, false
	}
	if from == to {
		return nil, true
	}
	parent := map[int]edge{} // entity -> edge used to reach it (to = previous entity)
	seen := map[int]bool{from: true}
	level := []int{from}
	for d := 1; d <= maxDepth && len(level) > 0; d++ {
		var next []int
		for _, id := range level {
			for _, e := range g.edges(id) {
				if seen[e.to] {
					continue
				}
				seen[e.to] = true
				parent[e.to] = edge{e.rec, id}
				if e.to == to {
					for cur := to; cur != from; cur = parent[cur].to {
						path = append(path, g.recs[parent[cur].rec])
					}
					for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
						path[i], path[j] = path[j], path[i]
					}
					return path, true
				}
				next = append(next, e.to)
			}
		}
		level = next
	}
	return nil, false
}

// SubtypesOf returns every entity that reaches typ through a chain of live
// `is` records, sorted. It is transitive and cycle-safe, and never contains
// typ itself. Unknown names give nil.
func (g *Graph) SubtypesOf(typ string) []string {
	root, ok := g.lookup(typ)
	if !ok {
		return nil
	}
	seen := map[int]bool{root: true}
	stack := []int{root}
	var out []string
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, s := range g.isIn[id] {
			if !seen[s] {
				seen[s] = true
				out = append(out, g.names[s])
				stack = append(stack, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Types returns every entity that is the object of a live `is` record,
// sorted.
func (g *Graph) Types() []string {
	var out []string
	for id, subs := range g.isIn {
		if len(subs) > 0 {
			out = append(out, g.names[id])
		}
	}
	sort.Strings(out)
	return out
}
