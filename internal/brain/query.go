package brain

import (
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/model"
)

// DescribedEntity is one entity of `describe --json`.
type DescribedEntity struct {
	Name  string         `json:"name"`
	Depth int            `json:"depth"`
	Out   []model.Record `json:"out"`
	In    []model.Record `json:"in"`
}

// DescribeResult is `describe --json`.
type DescribeResult struct {
	Entity   string            `json:"entity"`
	At       *string           `json:"at"`
	Entities []DescribedEntity `json:"entities"`
}

// Describe returns entity and every entity within depth hops. at is the
// --at value, nil for now.
func Describe(st Store, entity string, depth int, at *string) (DescribeResult, error) {
	if depth < 0 {
		return DescribeResult{}, Usagef("--depth must not be negative")
	}
	snap, err := view(st, at)
	if err != nil {
		return DescribeResult{}, err
	}
	g := snap.Graph()
	if !g.Has(entity) {
		return DescribeResult{}, NotFoundf("unknown entity %q", entity)
	}
	res := DescribeResult{Entity: model.NormalizeName(entity), At: at, Entities: []DescribedEntity{}}
	for _, s := range g.BFS(entity, depth) {
		res.Entities = append(res.Entities, DescribedEntity{
			Name: s.Name, Depth: s.Depth, Out: nonNil(g.Out(s.Name)), In: nonNil(g.In(s.Name)),
		})
	}
	return res, nil
}

// LsResult is `ls --json`.
type LsResult struct {
	Type     *string  `json:"type"`
	Entities []string `json:"entities"`
}

// Ls lists the subtypes of typ, or every type when typ is nil.
func Ls(st Store, typ *string, at *string) (LsResult, error) {
	snap, err := view(st, at)
	if err != nil {
		return LsResult{}, err
	}
	g := snap.Graph()
	res := LsResult{}
	if typ == nil {
		res.Entities = g.Types()
	} else {
		if !g.Has(*typ) {
			return LsResult{}, NotFoundf("unknown type %q", *typ)
		}
		t := model.NormalizeName(*typ)
		res.Type = &t
		res.Entities = g.SubtypesOf(*typ)
	}
	if res.Entities == nil {
		res.Entities = []string{}
	}
	return res, nil
}

// RelationsResult is `relations --json`.
type RelationsResult struct {
	Relations []string `json:"relations"`
}

// Relations lists the relation enum.
func Relations() RelationsResult { return RelationsResult{model.Relations()} }

// SearchResult is `search --json`.
type SearchResult struct {
	Matches []model.Record `json:"matches"`
}

// Search returns the live records whose subject, relation or object
// contains text, case-insensitively.
func Search(st Store, text string, at *string) (SearchResult, error) {
	snap, err := view(st, at)
	if err != nil {
		return SearchResult{}, err
	}
	needle := strings.ToLower(text)
	matches := []model.Record{}
	for _, r := range snap.Live() {
		if strings.Contains(strings.ToLower(r.Subject), needle) ||
			strings.Contains(strings.ToLower(r.Relation), needle) ||
			strings.Contains(strings.ToLower(r.Object), needle) {
			matches = append(matches, r)
		}
	}
	return SearchResult{matches}, nil
}

// DefaultMaxDepth is the default of path --max-depth.
const DefaultMaxDepth = 6

// PathResult is `path --json`.
type PathResult struct {
	From string         `json:"from"`
	To   string         `json:"to"`
	Path []model.Record `json:"path"`
}

// Path returns the shortest chain of live records from a to b.
func Path(st Store, a, b string, maxDepth int) (PathResult, error) {
	if maxDepth < 1 {
		return PathResult{}, Usagef("--max-depth must be at least 1")
	}
	snap, err := load(st)
	if err != nil {
		return PathResult{}, err
	}
	g := snap.Graph()
	for _, n := range []string{a, b} {
		if !g.Has(n) {
			return PathResult{}, NotFoundf("unknown entity %q", n)
		}
	}
	p, ok := g.ShortestPath(a, b, maxDepth)
	if !ok {
		return PathResult{}, NotFoundf("no path from %q to %q within %d hops", a, b, maxDepth)
	}
	return PathResult{From: model.NormalizeName(a), To: model.NormalizeName(b), Path: nonNil(p)}, nil
}

// RecordsResult is `log --json` and `history --json`.
type RecordsResult struct {
	Records []model.Record `json:"records"`
}

// Log returns every record in uuid order, from since (nil for all) and the
// last n (nil for all).
func Log(st Store, since *string, n *int) (RecordsResult, error) {
	var from time.Time
	if since != nil {
		var err error
		if from, err = ParseAt("--since", *since); err != nil {
			return RecordsResult{}, err
		}
	}
	if n != nil && *n < 1 {
		return RecordsResult{}, Usagef("-n must be at least 1")
	}
	snap, err := load(st)
	if err != nil {
		return RecordsResult{}, err
	}
	recs := snap.Records
	if since != nil {
		kept := recs[:0:0]
		for _, r := range recs {
			if t, err := time.Parse(time.RFC3339, r.Time); err == nil && !t.Before(from) {
				kept = append(kept, r)
			}
		}
		recs = kept
	}
	if n != nil && len(recs) > *n {
		recs = recs[len(recs)-*n:]
	}
	return RecordsResult{nonNil(recs)}, nil
}

// History returns every record for subject (and relation when non-nil),
// forgotten or not, with the forgets pointing at them.
func History(st Store, subject string, relation *string) (RecordsResult, error) {
	subj := model.NormalizeName(subject)
	if subj == "" {
		return RecordsResult{}, Usagef("empty subject. give a name")
	}
	rel := ""
	if relation != nil {
		var err error
		if rel, err = model.ParseRelation(*relation); err != nil {
			return RecordsResult{}, Usagef("%w", err)
		}
	}
	snap, err := load(st)
	if err != nil {
		return RecordsResult{}, err
	}
	targets := map[string]bool{}
	known := false
	for _, r := range snap.Records {
		if r.IsForget() || model.NormalizeName(r.Subject) != subj {
			continue
		}
		known = true
		if rel == "" || r.Relation == rel {
			targets[r.UUID] = true
		}
	}
	if !known {
		return RecordsResult{}, NotFoundf("unknown subject %q", subject)
	}
	out := []model.Record{}
	for _, r := range snap.Records {
		if targets[r.UUID] || (r.IsForget() && targets[r.Subject]) {
			out = append(out, r)
		}
	}
	return RecordsResult{out}, nil
}
