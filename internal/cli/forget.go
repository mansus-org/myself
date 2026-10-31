package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

type forgetResult struct {
	Forgotten []model.Record `json:"forgotten"`
}

func newForgetCmd(cfg *Config) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "forget <uuid> | <subject> <relation> [<object...>]",
		Short: "Retract a record by uuid, or every live record with a subject and relation",
		Long: "Retract a record. With one uuid argument, retract that record. Otherwise retract\n" +
			"every live record with that subject and relation, narrowed to those whose object\n" +
			"matches when one is given. Exits 2 if nothing matches.",
		RunE: func(cmd *cobra.Command, args []string) error {
			match, err := forgetMatcher(args)
			if err != nil {
				return err
			}
			recs, err := loadRecords(cmd, cfg, nil)
			if err != nil {
				return err
			}
			now := time.Now()
			res := forgetResult{Forgotten: []model.Record{}}
			for _, r := range store.Live(recs) {
				if match.fn(r) {
					res.Forgotten = append(res.Forgotten, model.NewForget(r.UUID, reason, now))
				}
			}
			if len(res.Forgotten) == 0 {
				return NotFoundf("no live record matches %s", match.desc)
			}
			if err := appendRecords(cfg, res.Forgotten...); err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, f := range res.Forgotten {
					if _, err := fmt.Fprintf(w, "forgot %s\n", f.Subject); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the record is retracted, stored as the forget record's object")
	return cmd
}

type matcher struct {
	fn   func(model.Record) bool
	desc string // for the not-found error
}

// forgetMatcher picks the form of SPEC section 5: `forget <uuid>` when there
// is exactly one argument and it is a uuid, else `<subject> <relation>
// [<object...>]` compared after normalization.
func forgetMatcher(args []string) (matcher, error) {
	if len(args) == 1 && model.IsUUID(args[0]) {
		id := args[0]
		return matcher{func(r model.Record) bool { return r.UUID == id }, "uuid " + id}, nil
	}
	if len(args) < 2 {
		return matcher{}, Usagef("forget needs <uuid>, or <subject> <relation> [<object...>]")
	}
	rel, err := model.ParseRelation(args[1])
	if err != nil {
		return matcher{}, Usagef("%w", err)
	}
	subj := model.NormalizeName(args[0])
	if subj == "" {
		return matcher{}, Usagef("empty subject. give a name as the subject")
	}
	desc := fmt.Sprintf("%q", subj+" "+rel)
	if len(args) == 2 {
		return matcher{func(r model.Record) bool { return r.Subject == subj && r.Relation == rel }, desc}, nil
	}
	obj := joinObject(args[2:])
	norm := model.NormalizeName(obj)
	desc = fmt.Sprintf("%q", subj+" "+rel+" "+obj)
	return matcher{func(r model.Record) bool {
		return r.Subject == subj && r.Relation == rel && model.NormalizeName(r.Object) == norm
	}, desc}, nil
}
