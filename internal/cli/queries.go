package cli

import (
	"io"

	"github.com/mansus-org/myself/internal/querylog"
	"github.com/spf13/cobra"
)

// queries is what the server was asked, not what the brain knows: the questions
// that came in over the API and what each one got back. It is a separate command
// from `log` for the same reason the log is a separate file — a record of
// questions is not a record of facts, and mixing them would make `search` answer
// things nobody ever said.
func newQueriesCmd(cfg *Config) *cobra.Command {
	var n int
	var verb string
	cmd := &cobra.Command{
		Use:   "queries [--verb VERB] [-n N]",
		Short: "What the server was asked: search, recall and describe, newest first",
		Long: "Queries is the server's own record of the questions it answered: who asked, what\n" +
			"they asked, how many facts came back and which facts those were. It is kept in\n" +
			"queries.jsonl next to the store, bounded to the last few hundred, and it is not\n" +
			"the brain: nothing here is searched, recalled, exported or primed.\n" +
			"Against a local store it reads what the last `myself serve` recorded.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := querylog.CheckVerb(verb); err != nil {
				return Usagef("%v", err)
			}
			res, err := backendFor(cmd, cfg).Queries(verb, n)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
		},
	}
	cmd.Flags().IntVarP(&n, "n", "n", querylog.DefaultN, "keep only the last N queries")
	cmd.Flags().StringVar(&verb, "verb", "", "only this verb: search, recall or describe")
	return cmd
}
