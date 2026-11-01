package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newWhoamiCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show which store or server token the commands use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cfg.Remote() {
				res := struct {
					Store string `json:"store"`
				}{cfg.File}
				return emit(cmd, cfg, res, func(w io.Writer) error {
					_, err := fmt.Fprintf(w, "local store %s\n", cfg.File)
					return err
				})
			}
			res, err := newClient(cmd, cfg).Whoami()
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "name %s\nscope %s\nadmin %t\n", res.Name, res.Scope, res.Admin)
				return err
			})
		},
	}
}
