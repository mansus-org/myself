package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mansus-org/myself/internal/model"
	"github.com/spf13/cobra"
)

// loadAt runs a throwaway command with an --at flag, as describe, ls and
// search register it, and returns what loadRecords and atFlag.JSON give.
func loadAt(t *testing.T, file string, args ...string) ([]model.Record, *string, error) {
	t.Helper()
	cfg := &Config{File: file}
	var recs []model.Record
	var at *string
	cmd := &cobra.Command{Use: "x", SilenceErrors: true, SilenceUsage: true}
	a := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		var err error
		recs, err = loadRecords(cmd, cfg, a)
		at = a.JSON()
		return err
	}
	cmd.SetArgs(args)
	return recs, at, cmd.Execute()
}

func TestLoadRecordsAt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	lines := `{"uuid":"0192d7c4-0001-7000-8000-000000000001","subject":"relay","relation":"is","object":"project","time":"2026-10-29T16:00:00Z"}
{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun","time":"2026-10-30T16:00:00Z"}
`
	if err := os.WriteFile(file, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	recs, at, err := loadAt(t, file)
	if err != nil || len(recs) != 2 || at != nil {
		t.Fatalf("no --at: %d records, at %v, err %v", len(recs), at, err)
	}
	recs, at, err = loadAt(t, file, "--at", "2026-10-30")
	if err != nil || len(recs) != 1 || at == nil || *at != "2026-10-30" {
		t.Fatalf("--at day: %d records, at %v, err %v", len(recs), at, err)
	}
	recs, _, err = loadAt(t, file, "--at", "2026-10-30T16:00:00Z")
	if err != nil || len(recs) != 2 {
		t.Fatalf("--at inclusive: %d records, err %v", len(recs), err)
	}

	_, _, err = loadAt(t, file, "--at", "yesterday")
	if exitCode(err) != ExitUsage {
		t.Fatalf("bad --at: exit %d, err %v", exitCode(err), err)
	}
	_, _, err = loadAt(t, filepath.Join(file, "nested"))
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitStore {
		t.Fatalf("unreadable file: err %v", err)
	}
}
