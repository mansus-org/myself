// Package cli wires the cobra commands, global flags and output helpers.
package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// newRoot builds the command tree. Later tickets add commands here.
func newRoot(stdout, stderr io.Writer) *cobra.Command {
	cfg := &Config{}
	var file string

	root := &cobra.Command{
		Use:           "myself",
		Short:         "Agent-native graph memory",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			f, err := resolveFile(file, cmd.Flags().Changed("file"))
			if err != nil {
				return StoreErr(err)
			}
			cfg.File = f
			return nil
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{ExitUsage, err}
	})
	root.PersistentFlags().StringVar(&file, "file", "", "path to the brain file (default ~/.myself/brain.jsonl, env MYSELF_FILE)")
	root.PersistentFlags().BoolVar(&cfg.JSON, "json", false, "write JSON to stdout")

	root.AddCommand(newVersionCmd(cfg))
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
