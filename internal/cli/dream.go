package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/spf13/cobra"
)

func newDreamCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "dream",
		Short: "Report near-duplicate subjects (read-only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := brain.Dream(storeFor(cmd, cfg))
			if err != nil {
				return err
			}
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
