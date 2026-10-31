package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/spf13/cobra"
)

const defaultBudget = 2000

type primeResult struct {
	Budget    int    `json:"budget"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text"`
}

func newPrimeCmd(cfg *Config) *cobra.Command {
	var budget int
	cmd := &cobra.Command{
		Use:   "prime [--budget CHARS]",
		Short: "Compact summary of the brain for an agent context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if budget < 1 {
				return Usagef("--budget must be at least 1")
			}
			_, g, err := loadGraph(cmd, cfg, nil)
			if err != nil {
				return err
			}
			text, truncated := primeText(g, budget)
			return emit(cmd, cfg, primeResult{budget, truncated, text}, func(w io.Writer) error {
				_, err := io.WriteString(w, text)
				return err
			})
		},
	}
	cmd.Flags().IntVar(&budget, "budget", defaultBudget, "maximum characters of output")
	return cmd
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
