package brain

import (
	"sort"
	"strings"

	"github.com/mansus-org/myself/internal/export"
	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
)

// DreamResult is `dream --json`.
type DreamResult struct {
	NearDuplicates [][]string `json:"near_duplicates"`
}

// Dream reports near-duplicate subjects. It writes nothing.
func Dream(st Store) (DreamResult, error) {
	snap, err := load(st)
	if err != nil {
		return DreamResult{}, err
	}
	return DreamResult{nearDuplicates(snap.Graph())}, nil
}

// nearDuplicates groups entities that are equal after removing `-` and a
// trailing `s`. Groups and the names in them are sorted.
func nearDuplicates(g *graph.Graph) [][]string {
	groups := map[string][]string{}
	for _, n := range g.Entities() {
		key := strings.TrimSuffix(strings.ReplaceAll(n, "-", ""), "s")
		groups[key] = append(groups[key], n)
	}
	out := [][]string{}
	for _, grp := range groups {
		if len(grp) > 1 {
			out = append(out, grp) // already sorted: Entities is sorted
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// ExportResult is `export --json`.
type ExportResult struct {
	Dir      string   `json:"dir"`
	Entities int      `json:"entities"`
	Files    []string `json:"files"`
}

// ExportObsidian writes one note per entity of the live state into dir.
func ExportObsidian(st Store, dir string) (ExportResult, error) {
	snap, err := load(st)
	if err != nil {
		return ExportResult{}, err
	}
	files, err := export.Write(dir, snap.Graph())
	if err != nil {
		return ExportResult{}, StoreErr(err)
	}
	return ExportResult{dir, len(files), files}, nil
}

// GraphNode is one entity of GraphView.
type GraphNode struct {
	Name    string   `json:"name"`
	Types   []string `json:"types"`   // objects of its live `is` records that are entities, sorted
	Records int      `json:"records"` // live records with it as subject or linked object
}

// GraphEdge is one live record whose object is an entity.
type GraphEdge struct {
	UUID     string `json:"uuid"`
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

// GraphResult is the body of GET /api/v1/graph.
type GraphResult struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphView returns every entity (sorted by name) and every edge (in uuid
// order) of the live graph, as of at when non-nil.
func GraphView(st Store, at *string) (GraphResult, error) {
	snap, err := view(st, at)
	if err != nil {
		return GraphResult{}, err
	}
	g := snap.Graph()
	res := GraphResult{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	for _, n := range g.Entities() {
		in := g.In(n)
		node := GraphNode{Name: n, Types: []string{}, Records: len(g.Out(n)) + len(in)}
		for _, r := range in {
			res.Edges = append(res.Edges, GraphEdge{r.UUID, model.NormalizeName(r.Subject), n, r.Relation})
		}
		res.Nodes = append(res.Nodes, node)
	}
	types := map[string][]string{}
	for _, e := range res.Edges {
		if e.Relation == "is" {
			types[e.From] = append(types[e.From], e.To)
		}
	}
	for i := range res.Nodes {
		if ts := types[res.Nodes[i].Name]; len(ts) > 0 {
			sort.Strings(ts)
			res.Nodes[i].Types = dedupe(ts)
		}
	}
	sort.Slice(res.Edges, func(i, j int) bool { return res.Edges[i].UUID < res.Edges[j].UUID })
	return res, nil
}

func dedupe(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
