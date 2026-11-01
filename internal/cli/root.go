// Package cli wires the cobra commands, global flags and output helpers.
package cli

import (
	"os"
	"strings"

	"io"

	"github.com/mansus-org/myself/internal/client"
	"github.com/spf13/cobra"
)

// newRoot builds the command tree. Later tickets add commands here.
func newRoot(stdout, stderr io.Writer) *cobra.Command {
	cfg := &Config{}
	var file, rawURL, token string

	root := &cobra.Command{
		Use:           "myself",
		Short:         "Agent-native graph memory",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if localOnly[cmd.Name()] {
				f, err := resolveFile(file, cmd.Flags().Changed("file"))
				if err != nil {
					return StoreErr(err)
				}
				cfg.File = f
				return nil
			}
			return resolveMode(cmd, cfg, file, rawURL, token)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: ExitUsage, Err: err}
	})
	root.PersistentFlags().StringVar(&file, "file", "", "path to the brain file (default ~/.myself/brain.jsonl, env MYSELF_FILE)")
	root.PersistentFlags().BoolVar(&cfg.JSON, "json", false, "write JSON to stdout")
	root.PersistentFlags().StringVar(&rawURL, "url", "", "server URL: run against a myself server (env MYSELF_URL)")
	root.PersistentFlags().StringVar(&token, "token", "", "bearer token for the server (env MYSELF_TOKEN; prefer the env, flags show in ps)")

	root.AddCommand(
		newRememberCmd(cfg),
		newForgetCmd(cfg),
		newVersionCmd(cfg),
		newDescribeCmd(cfg),
		newLsCmd(cfg),
		newRelationsCmd(cfg),
		newSearchCmd(cfg),
		newPathCmd(cfg),
		newLogCmd(cfg),
		newHistoryCmd(cfg),
		newPrimeCmd(cfg),
		newDreamCmd(cfg),
		newExportCmd(cfg),
		newSyncCmd(cfg),
		newMergeDriverCmd(),
		newWhoamiCmd(cfg),
		newServeCmd(cfg),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := newRoot(stdout, stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		printError(stderr, err)
	}
	return exitCode(err)
}

// localOnly are the commands that never talk to a server: they ignore
// MYSELF_URL, --url and --token.
var localOnly = map[string]bool{"serve": true, "version": true, "merge-driver": true}

// resolveMode fills cfg for a command that uses the store: the local file, or
// the server when a URL is set (--url > MYSELF_URL). --file together with a
// URL is a usage error rather than one silently winning.
func resolveMode(cmd *cobra.Command, cfg *Config, file, rawURL, token string) error {
	fileSet := cmd.Flags().Changed("file")
	if !cmd.Flags().Changed("url") {
		rawURL = os.Getenv(EnvURL)
	}
	if strings.TrimSpace(rawURL) == "" {
		if cmd.Flags().Changed("token") {
			return Usagef("--token needs a server URL: set --url or %s", EnvURL)
		}
		f, err := resolveFile(file, fileSet)
		if err != nil {
			return StoreErr(err)
		}
		cfg.File = f
		return nil
	}
	if fileSet {
		return Usagef("--file works on a local store and conflicts with the server URL; drop --file, or unset %s and --url", EnvURL)
	}
	u, err := client.ParseURL(rawURL)
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("token") {
		token = os.Getenv(EnvToken)
	}
	timeout, err := parseTimeout(os.Getenv(EnvTimeout))
	if err != nil {
		return err
	}
	cfg.URL, cfg.Token, cfg.Timeout = u, strings.TrimSpace(token), timeout
	return nil
}
