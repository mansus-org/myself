package cli

import (
	"io"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/spf13/cobra"
)

func emitRecords(cmd *cobra.Command, cfg *Config, res brain.RecordsResult) error {
	return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
}

func newLogCmd(cfg *Config) *cobra.Command {
	var since string
	var n int
	cmd := &cobra.Command{
		Use:   "log [--since TIME] [-n N]",
		Short: "List every record, forgets included, in uuid order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var sinceP *string
			var nP *int
			if cmd.Flags().Changed("since") {
				sinceP = &since
			}
			if cmd.Flags().Changed("n") {
				nP = &n
			}
			res, err := backendFor(cmd, cfg).Log(sinceP, nP)
			if err != nil {
				return err
			}
			return emitRecords(cmd, cfg, res)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only records at or after TIME (RFC 3339 or YYYY-MM-DD)")
	cmd.Flags().IntVarP(&n, "n", "n", 0, "keep only the last N records")
	return cmd
}

func newHistoryCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "history <subject> [<relation>]",
		Short: "List every record for a subject, forgotten or not, with its forgets",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var rel *string
			if len(args) == 2 {
				rel = &args[1]
			}
			res, err := backendFor(cmd, cfg).History(args[0], rel)
			if err != nil {
				return err
			}
			return emitRecords(cmd, cfg, res)
		},
	}
}
