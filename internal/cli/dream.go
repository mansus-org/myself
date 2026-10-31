package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/spf13/cobra"
)

type dreamResult struct {
	NearDuplicates [][]string `json:"near_duplicates"`
}

func newDreamCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "dream",
		Short: "Report near-duplicate subjects (read-only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, g, err := loadGraph(cmd, cfg, nil)
			if err != nil {
				return err
			}
			res := dreamResult{nearDuplicates(g)}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, grp := range res.NearDuplicates {
					if _, err := fmt.Fprintln(w, strings.Join(grp, " ")); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
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
