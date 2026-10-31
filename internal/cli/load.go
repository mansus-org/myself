package cli

import (
	"io"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

// Commands follow one pattern: parse args and flags (usage errors, exit 1),
// load the records with loadRecords (store errors, exit 3), compute, then
// print once with emit. Nothing is written to stdout before the last step,
// so with --json stdout stays empty on error.

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

// set reports whether --at was given.
func (a *atFlag) set() bool { return a != nil && a.cmd.Flags().Changed("at") }

// time parses --at. ok is false when the flag was not given. A bad value is a
// usage error.
func (a *atFlag) time() (t time.Time, ok bool, err error) {
	if !a.set() {
		return time.Time{}, false, nil
	}
	t, err = store.ParseAt(a.raw)
	if err != nil {
		return time.Time{}, false, Usagef("--at: %w", err)
	}
	return t, true, nil
}

// JSON returns the --at string as given, or nil without the flag: the "at"
// field of `describe --json`.
func (a *atFlag) JSON() *string {
	if !a.set() {
		return nil
	}
	s := a.raw
	return &s
}

// loadRecords reads cfg.File, uuid-sorted, forgets included. Corrupt lines
// are reported as warnings on the command's stderr. If at is non-nil and
// --at was given, only records with time <= TIME are kept. Pass a nil at for
// commands without the flag.
func loadRecords(cmd *cobra.Command, cfg *Config, at *atFlag) ([]model.Record, error) {
	t, ok, err := at.time()
	if err != nil {
		return nil, err
	}
	recs, err := store.ReadFile(cfg.File, cmd.ErrOrStderr())
	if err != nil {
		return nil, StoreErr(err)
	}
	if ok {
		recs = store.AsOf(recs, t)
	}
	return recs, nil
}

// appendRecords writes recs in one store.Append call. Write and lock errors
// exit 3.
func appendRecords(cfg *Config, recs ...model.Record) error {
	if err := store.Append(cfg.File, recs...); err != nil {
		return StoreErr(err)
	}
	return nil
}

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
