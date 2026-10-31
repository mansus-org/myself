package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
	"github.com/spf13/cobra"
)

type rememberResult struct {
	Remembered []model.Record `json:"remembered"`
	Forgotten  []model.Record `json:"forgotten"`
}

// fact is one validated "subject relation object" to remember.
type fact struct{ subject, relation, object string }

func newRememberCmd(cfg *Config) *cobra.Command {
	var replace bool
	cmd := &cobra.Command{
		Use:   "remember <subject> <relation> <object...> | -",
		Short: "Write a fact, or one per stdin line with -",
		Long: "Write a fact. Everything after the relation is the object. With - read one\n" +
			"\"subject relation object...\" per stdin line; nothing is written if any line is bad.\n" +
			"--replace first forgets every live record with the same subject and relation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			var facts []fact
			switch {
			case len(args) == 1 && args[0] == "-":
				var err error
				if facts, err = readBulk(cmd.InOrStdin(), now); err != nil {
					return err
				}
			case len(args) >= 2:
				f := fact{args[0], args[1], joinObject(args[2:])}
				if _, err := f.record(now); err != nil {
					return Usagef("%w", err)
				}
				facts = []fact{f}
			default:
				return Usagef("remember needs <subject> <relation> <object...>, or - to read stdin")
			}

			for _, f := range facts {
				if rel, _ := model.ParseRelation(f.relation); (rel == "is" || rel == "are") && len(strings.Fields(f.object)) > 3 {
					Warnf(cmd.ErrOrStderr(), "\"is\" object looks like a sentence; use a short type name and put details in has or does")
				}
			}

			res := rememberResult{Remembered: []model.Record{}, Forgotten: []model.Record{}}
			if replace && len(facts) > 0 {
				existing, err := loadRecords(cmd, cfg, nil)
				if err != nil {
					return err
				}
				res.Forgotten = replaced(store.Live(existing), facts, now)
			}
			// Built after the forgets so the new records sort after them.
			for _, f := range facts {
				rec, _ := f.record(now) // validated above
				res.Remembered = append(res.Remembered, rec)
			}
			if err := appendRecords(cfg, append(res.Forgotten, res.Remembered...)...); err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, f := range res.Forgotten {
					if _, err := fmt.Fprintf(w, "forgot %s\n", f.Subject); err != nil {
						return err
					}
				}
				for _, r := range res.Remembered {
					if _, err := fmt.Fprintf(w, "remembered %s\n", r.UUID); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "forget live records with the same subject and relation first")
	return cmd
}

func (f fact) record(now time.Time) (model.Record, error) {
	return model.NewRecord(f.subject, f.relation, f.object, now)
}

// readBulk parses `remember -` input: one "subject relation object..." per
// line, blank lines skipped. Every line is validated before anything is
// written; the first bad one is reported with its line number.
func readBulk(r io.Reader, now time.Time) ([]fact, error) {
	var facts []fact
	br := bufio.NewReader(r)
	for n := 1; ; n++ {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, StoreErr(fmt.Errorf("reading stdin: %w", err))
		}
		if words := strings.Fields(line); len(words) > 0 {
			if len(words) < 2 {
				return nil, Usagef("line %d: want \"subject relation object...\"", n)
			}
			f := fact{words[0], words[1], unquote(joinObject(words[2:]))}
			if _, ferr := f.record(now); ferr != nil {
				return nil, Usagef("line %d: %w", n, ferr)
			}
			facts = append(facts, f)
		}
		if err == io.EOF {
			return facts, nil
		}
	}
}

// replaced returns one forget record per live record sharing a subject and
// relation with one of facts, in uuid order. Only records already in the log
// are replaced, not other lines of the same bulk write.
func replaced(live []model.Record, facts []fact, now time.Time) []model.Record {
	type key struct{ subject, relation string }
	keys := make(map[key]bool, len(facts))
	for _, f := range facts {
		rel, _ := model.ParseRelation(f.relation) // validated by the caller
		keys[key{model.NormalizeName(f.subject), rel}] = true
	}
	forgets := []model.Record{}
	for _, r := range live {
		if keys[key{r.Subject, r.Relation}] {
			forgets = append(forgets, model.NewForget(r.UUID, "", now))
		}
	}
	return forgets
}

// unquote strips one matching pair of surrounding '...' or "..." from a bulk
// object, which the shell would have stripped in single-argument mode.
func unquote(s string) string {
	if n := len(s); n >= 2 && (s[0] == '\'' || s[0] == '"') && s[n-1] == s[0] {
		return s[1 : n-1]
	}
	return s
}
