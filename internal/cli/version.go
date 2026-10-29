package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set at build time:
//
//	-ldflags "-X github.com/mansus-org/myself/internal/cli.Version=v0.1.0 ..."
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func newVersionCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if cfg.JSON {
				return printJSON(out, versionInfo{Version, Commit, BuildDate})
			}
			_, err := fmt.Fprintf(out, "myself %s (commit %s, built %s)\n", Version, Commit, BuildDate)
			return err
		},
	}
}
