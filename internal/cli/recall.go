package cli

import (
	"io"
	"os"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/embed"
	"github.com/spf13/cobra"
)

// recall is `search`'s neighbour, not a mode of it: search answers from the log
// alone and will answer the same way forever, while recall answers with a model.
// Keeping them apart is what lets an agent tell "no fact says this word" from
// "no fact means this".
func newRecallCmd(cfg *Config) *cobra.Command {
	var n int
	var min float64
	var graph int
	cmd := &cobra.Command{
		Use:   "recall <text...> [-n N] [--min SCORE] [--graph N]",
		Short: "Find the live facts closest in meaning to text",
		Long: "Recall answers by meaning instead of by substring: it reads the question with an\n" +
			"embedding model and returns the live facts nearest to it, best first, each with the\n" +
			"facts around it in the graph indented below it — what the things it names are.\n" +
			"It needs the Neo4j backend and an embedding endpoint (see myself index), and\n" +
			"says so when either is missing — it never falls back to a worse answer.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return Usagef("recall needs <text>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var nP *int
		if cmd.Flags().Changed("n") {
			nP = &n
		}
		var minP *float64
		if cmd.Flags().Changed("min") {
			minP = &min
		}
		var graphP *int
		if cmd.Flags().Changed("graph") {
			graphP = &graph
		}
		res, err := backendFor(cmd, cfg).Recall(joinObject(args), at.value(), nP, minP, graphP)
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
	}
	cmd.Flags().IntVarP(&n, "n", "n", brain.DefaultRecallN, "keep only the N closest matches")
	cmd.Flags().Float64Var(&min, "min", 0, "drop a match scoring below this (-1 to 1)")
	cmd.Flags().IntVar(&graph, "graph", brain.DefaultRecallGraph, "bring the facts within this many graph hops of each match (0 for none)")
	return cmd
}

// newIndexCmd is `myself index`: the projection recall reads, built from the log.
func newIndexCmd(cfg *Config) *cobra.Command {
	var rebuild bool
	cmd := &cobra.Command{
		Use:   "index [--rebuild]",
		Short: "Give every live fact a vector, so recall can answer by meaning",
		Long: "Index writes the vector projection: one embedding per live fact, and the index\n" +
			"over them. It is derived from the log and never the other way round, so it can be\n" +
			"run again at any time. Facts that already have a vector from this model are left\n" +
			"alone; --rebuild re-embeds every live fact, which is what a new model needs.\n" +
			"It needs the Neo4j backend and an embedding endpoint.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := backendFor(cmd, cfg).Index(rebuild)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
		},
	}
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "re-embed every live fact, not only the ones without a vector")
	return cmd
}

// embedderFromEnv resolves the embedding endpoint, or nil for "none is
// configured". brain.Recall and brain.Index are the ones that say what is
// missing, in the one message the CLI and the server both give; a bad
// configuration is an error here because a half-read variable is a mistake, not
// a choice.
func embedderFromEnv() (embed.Embedder, error) {
	cfg, ok, err := embed.FromEnv(os.Getenv)
	if err != nil || !ok {
		return nil, err
	}
	return embed.New(cfg), nil
}
