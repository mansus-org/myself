package cli

import (
	"github.com/mansus-org/myself/internal/gitsync"
	"github.com/spf13/cobra"
)

func newMergeDriverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge-driver <base> <ours> <theirs>",
		Short: "Git merge driver: union of both logs, sorted by uuid (written to <ours>)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitsync.MergeFiles(args[0], args[1], args[2]); err != nil {
				return &ExitError{Code: ExitUsage, Err: err}
			}
			return nil
		},
	}
}
