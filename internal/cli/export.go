package cli

import (
	"fmt"
	"io"

	"github.com/mansus-org/myself/internal/export"
	"github.com/spf13/cobra"
)

type exportResult struct {
	Dir      string   `json:"dir"`
	Entities int      `json:"entities"`
	Files    []string `json:"files"`
}

// runExport writes the notes for the live state of cfg.File into dir.
func runExport(cmd *cobra.Command, cfg *Config, dir string) (exportResult, error) {
	_, g, err := loadGraph(cmd, cfg, nil)
	if err != nil {
		return exportResult{}, err
	}
	files, err := export.Write(dir, g)
	if err != nil {
		return exportResult{}, StoreErr(err)
	}
	return exportResult{dir, len(files), files}, nil
}

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
			res, err := runExport(cmd, cfg, dir)
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
