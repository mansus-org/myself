package cli

import (
	"fmt"
	"runtime/debug"

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

const (
	defaultVersion   = "dev"
	defaultCommit    = "none"
	defaultBuildDate = "unknown"
)

// readBuildInfo is the build-info source for `version`; tests pass their own.
var readBuildInfo = debug.ReadBuildInfo

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// resolveVersion returns the ldflags values, filling any still at their
// defaults from the module build info (set by `go install pkg@version`).
func resolveVersion(read func() (*debug.BuildInfo, bool)) versionInfo {
	v := versionInfo{Version, Commit, BuildDate}
	if v.Version != defaultVersion && v.Commit != defaultCommit && v.Date != defaultBuildDate {
		return v
	}
	bi, ok := read()
	if !ok || bi == nil {
		return v
	}
	if v.Version == defaultVersion && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		v.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision" && v.Commit == defaultCommit && s.Value != "":
			v.Commit = s.Value
			if len(v.Commit) > 7 {
				v.Commit = v.Commit[:7]
			}
		case s.Key == "vcs.time" && v.Date == defaultBuildDate && s.Value != "":
			v.Date = s.Value
		}
	}
	return v
}

func newVersionCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			v := resolveVersion(readBuildInfo)
			if cfg.JSON {
				return printJSON(out, v)
			}
			_, err := fmt.Fprintf(out, "myself %s (commit %s, built %s)\n", v.Version, v.Commit, v.Date)
			return err
		},
	}
}
