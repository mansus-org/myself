package cli

import (
	"io"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/spf13/cobra"
)

func newPrimeCmd(cfg *Config) *cobra.Command {
	var budget int
	cmd := &cobra.Command{
		Use:   "prime [--budget CHARS]",
		Short: "Compact summary of the brain for an agent context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := backendFor(cmd, cfg).Prime(budget)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				_, err := io.WriteString(w, res.Text)
				return err
			})
		},
	}
	cmd.Flags().IntVar(&budget, "budget", brain.DefaultBudget, "maximum characters of output")
	return cmd
}
