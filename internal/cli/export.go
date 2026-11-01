package cli

import (
	"fmt"
	"io"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/spf13/cobra"
)

func newExportCmd(cfg *Config) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "export --obsidian <dir>",
		Short: "Write one markdown note per entity (live state only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return Usagef("export needs --obsidian <dir>")
			}
			res, err := brain.ExportObsidian(storeFor(cmd, cfg), dir)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "exported %d entities to %s\n", res.Entities, dir)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&dir, "obsidian", "", "directory to write the notes to")
	return cmd
}
