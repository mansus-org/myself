package cli

import (
	"io"
	"time"

	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

type recordsResult struct {
	Records []model.Record `json:"records"`
}

func emitRecords(cmd *cobra.Command, cfg *Config, recs []model.Record) error {
	return emit(cmd, cfg, recordsResult{nonNil(recs)}, func(w io.Writer) error {
		lines := make([]string, len(recs))
		for i, r := range recs {
			lines[i] = recordLine(r)
		}
		return writeLines(w, lines)
	})
}

func newLogCmd(cfg *Config) *cobra.Command {
	var since string
	var n int
	cmd := &cobra.Command{
		Use:   "log [--since TIME] [-n N]",
		Short: "List every record, forgets included, in uuid order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var from time.Time
			if cmd.Flags().Changed("since") {
				t, err := store.ParseAt(since)
				if err != nil {
					return Usagef("--since: %w", err)
				}
				from = t
			}
			if cmd.Flags().Changed("n") && n < 1 {
				return Usagef("-n must be at least 1")
			}
			recs, err := loadRecords(cmd, cfg, nil)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("since") {
				kept := recs[:0:0]
				for _, r := range recs {
					if t, err := time.Parse(time.RFC3339, r.Time); err == nil && !t.Before(from) {
						kept = append(kept, r)
					}
				}
				recs = kept
			}
			if cmd.Flags().Changed("n") && len(recs) > n {
				recs = recs[len(recs)-n:]
			}
			return emitRecords(cmd, cfg, recs)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only records at or after TIME (RFC 3339 or YYYY-MM-DD)")
	cmd.Flags().IntVarP(&n, "n", "n", 0, "keep only the last N records")
	return cmd
}

func newHistoryCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "history <subject> [<relation>]",
		Short: "List every record for a subject, forgotten or not, with its forgets",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			subject := model.NormalizeName(args[0])
			if subject == "" {
				return Usagef("empty subject. give a name")
			}
			relation := ""
			if len(args) == 2 {
				rel, err := model.ParseRelation(args[1])
				if err != nil {
					return Usagef("%w", err)
				}
				relation = rel
			}
			recs, err := loadRecords(cmd, cfg, nil)
			if err != nil {
				return err
			}
			targets := map[string]bool{}
			known := false
			for _, r := range recs {
				if r.IsForget() || model.NormalizeName(r.Subject) != subject {
					continue
				}
				known = true
				if relation == "" || r.Relation == relation {
					targets[r.UUID] = true
				}
			}
			if !known {
				return NotFoundf("unknown subject %q", args[0])
			}
			out := []model.Record{}
			for _, r := range recs {
				if targets[r.UUID] || (r.IsForget() && targets[r.Subject]) {
					out = append(out, r)
				}
			}
			return emitRecords(cmd, cfg, out)
		},
	}
}
