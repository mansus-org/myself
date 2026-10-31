package cli

import (
	"io"
	"strings"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

// Commands follow one pattern: parse args and flags (usage errors, exit 1),
// call the brain function with the command's store, then print once with
// emit. Nothing is written to stdout before the last step, so with --json
// stdout stays empty on error.

// atFlag is the --at TIME flag of the commands that list it in SPEC section 5
// (describe, ls, search). Register it with addAtFlag.
type atFlag struct {
	cmd *cobra.Command
	raw string
}

func addAtFlag(cmd *cobra.Command) *atFlag {
	a := &atFlag{cmd: cmd}
	cmd.Flags().StringVar(&a.raw, "at", "", "evaluate the log as of TIME (RFC 3339 or YYYY-MM-DD)")
	return a
}

// value returns the --at string as given, or nil without the flag.
func (a *atFlag) value() *string {
	if a == nil || !a.cmd.Flags().Changed("at") {
		return nil
	}
	s := a.raw
	return &s
}

// fileStore is the CLI's brain.Store: it reads cfg.File on every Load and
// writes warnings, corrupt lines included, to the command's stderr.
type fileStore struct {
	file string
	warn io.Writer
}

func storeFor(cmd *cobra.Command, cfg *Config) brain.Store {
	return fileStore{cfg.File, cmd.ErrOrStderr()}
}

func (s fileStore) Load() (*brain.Snapshot, error) {
	recs, err := store.ReadFile(s.file, s.warn)
	if err != nil {
		return nil, err
	}
	return brain.NewSnapshot(recs), nil
}

func (s fileStore) Append(recs ...model.Record) error { return store.Append(s.file, recs...) }

func (s fileStore) Warn(msg string) { Warnf(s.warn, "%s", msg) }

// emit prints the command's result: v as one JSON value with --json,
// otherwise the plain text written by text.
func emit(cmd *cobra.Command, cfg *Config, v any, text func(w io.Writer) error) error {
	if cfg.JSON {
		return printJSON(cmd.OutOrStdout(), v)
	}
	return text(cmd.OutOrStdout())
}

// joinObject joins the words after the relation into the object, with
// single spaces (SPEC section 5).
func joinObject(words []string) string {
	return strings.Join(words, " ")
}
