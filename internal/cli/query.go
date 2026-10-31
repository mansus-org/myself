package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

// loadGraph loads the records (honoring --at when at is non-nil), keeps the
// live ones and returns them with the graph built from them.
func loadGraph(cmd *cobra.Command, cfg *Config, at *atFlag) ([]model.Record, *graph.Graph, error) {
	recs, err := loadRecords(cmd, cfg, at)
	if err != nil {
		return nil, nil, err
	}
	live := store.Live(recs)
	return live, graph.Build(live), nil
}

// nonNil keeps empty record lists as [] in JSON.
func nonNil(recs []model.Record) []model.Record {
	if recs == nil {
		return []model.Record{}
	}
	return recs
}

func writeLines(w io.Writer, lines []string) error {
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}

func recordLine(r model.Record) string {
	return fmt.Sprintf("%s %s %s %s %s", r.UUID, r.Time, r.Subject, r.Relation, r.Object)
}

type describedEntity struct {
	Name  string         `json:"name"`
	Depth int            `json:"depth"`
	Out   []model.Record `json:"out"`
	In    []model.Record `json:"in"`
}

type describeResult struct {
	Entity   string            `json:"entity"`
	At       *string           `json:"at"`
	Entities []describedEntity `json:"entities"`
}

func newDescribeCmd(cfg *Config) *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:   "describe <entity>",
		Short: "Show the live records of an entity, and of its neighbours with --depth",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return Usagef("describe needs exactly one <entity>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.Flags().IntVar(&depth, "depth", 0, "also show entities reachable within N hops")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if depth < 0 {
			return Usagef("--depth must not be negative")
		}
		_, g, err := loadGraph(cmd, cfg, at)
		if err != nil {
			return err
		}
		if !g.Has(args[0]) {
			return NotFoundf("unknown entity %q", args[0])
		}
		res := describeResult{Entity: model.NormalizeName(args[0]), At: at.JSON(), Entities: []describedEntity{}}
		for _, st := range g.BFS(args[0], depth) {
			res.Entities = append(res.Entities, describedEntity{
				Name: st.Name, Depth: st.Depth, Out: nonNil(g.Out(st.Name)), In: nonNil(g.In(st.Name)),
			})
		}
		return emit(cmd, cfg, res, func(w io.Writer) error {
			var lines []string
			for _, e := range res.Entities {
				lines = append(lines, e.Name)
				for _, r := range e.Out {
					lines = append(lines, fmt.Sprintf("  %s %s", r.Relation, r.Object))
				}
				for _, r := range e.In {
					lines = append(lines, fmt.Sprintf("  <- %s %s", r.Subject, r.Relation))
				}
			}
			return writeLines(w, lines)
		})
	}
	return cmd
}

type lsResult struct {
	Type     *string  `json:"type"`
	Entities []string `json:"entities"`
}

func newLsCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls [<type>]",
		Short: "List the types, or the entities that are of a type",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return Usagef("ls takes at most one <type>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		_, g, err := loadGraph(cmd, cfg, at)
		if err != nil {
			return err
		}
		res := lsResult{}
		if len(args) == 0 {
			res.Entities = g.Types()
		} else {
			if !g.Has(args[0]) {
				return NotFoundf("unknown type %q", args[0])
			}
			t := model.NormalizeName(args[0])
			res.Type = &t
			res.Entities = g.SubtypesOf(args[0])
		}
		if res.Entities == nil {
			res.Entities = []string{}
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Entities) })
	}
	return cmd
}

func newRelationsCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "relations",
		Short: "List the relations you can use",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return Usagef("relations takes no arguments")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			rels := model.Relations()
			return emit(cmd, cfg, struct {
				Relations []string `json:"relations"`
			}{rels}, func(w io.Writer) error { return writeLines(w, rels) })
		},
	}
}

func newSearchCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <text...>",
		Short: "Find live records whose subject, relation or object contains text",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return Usagef("search needs <text>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		live, _, err := loadGraph(cmd, cfg, at)
		if err != nil {
			return err
		}
		needle := strings.ToLower(joinObject(args))
		matches := []model.Record{}
		for _, r := range live {
			if r.IsForget() {
				continue
			}
			if strings.Contains(strings.ToLower(r.Subject), needle) ||
				strings.Contains(strings.ToLower(r.Relation), needle) ||
				strings.Contains(strings.ToLower(r.Object), needle) {
				matches = append(matches, r)
			}
		}
		return emit(cmd, cfg, struct {
			Matches []model.Record `json:"matches"`
		}{matches}, func(w io.Writer) error {
			for _, r := range matches {
				if _, err := fmt.Fprintln(w, recordLine(r)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return cmd
}

type pathResult struct {
	From string         `json:"from"`
	To   string         `json:"to"`
	Path []model.Record `json:"path"`
}

func newPathCmd(cfg *Config) *cobra.Command {
	var maxDepth int
	cmd := &cobra.Command{
		Use:   "path <a> <b>",
		Short: "Show the shortest chain of live records between two entities",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return Usagef("path needs <a> and <b>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxDepth < 1 {
				return Usagef("--max-depth must be at least 1")
			}
			_, g, err := loadGraph(cmd, cfg, nil)
			if err != nil {
				return err
			}
			for _, a := range args {
				if !g.Has(a) {
					return NotFoundf("unknown entity %q", a)
				}
			}
			p, ok := g.ShortestPath(args[0], args[1], maxDepth)
			if !ok {
				return NotFoundf("no path from %q to %q within %d hops", args[0], args[1], maxDepth)
			}
			res := pathResult{From: model.NormalizeName(args[0]), To: model.NormalizeName(args[1]), Path: nonNil(p)}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, r := range res.Path {
					if _, err := fmt.Fprintf(w, "%s %s %s\n", r.Subject, r.Relation, r.Object); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", 6, "longest chain to look for")
	return cmd
}
