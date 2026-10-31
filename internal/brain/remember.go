package brain

import (
	"fmt"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/model"
)

// Fact is one "subject relation object" to remember, as given.
type Fact struct {
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// Record validates f and builds its record (SPEC sections 2 and 3).
func (f Fact) Record(now time.Time) (model.Record, error) {
	return model.NewRecord(f.Subject, f.Relation, f.Object, now)
}

// RememberResult is `remember --json`.
type RememberResult struct {
	Remembered []model.Record `json:"remembered"`
	Forgotten  []model.Record `json:"forgotten"`
}

// SentenceWarning is the warning for an `is` object of more than 3 words.
const SentenceWarning = `"is" object looks like a sentence; use a short type name and put details in has or does`

// Remember writes facts in one append, all or nothing. With replace, every
// live record sharing a subject and relation with one of the facts is
// forgotten first. A bad fact is a usage error; with several facts its
// message starts with "fact N: ". No facts write nothing.
func Remember(st Store, facts []Fact, replace bool, now time.Time) (RememberResult, error) {
	for i, f := range facts {
		if _, err := f.Record(now); err != nil {
			if len(facts) == 1 {
				return RememberResult{}, Usagef("%w", err)
			}
			return RememberResult{}, Usagef("fact %d: %w", i+1, err)
		}
	}
	for _, f := range facts {
		if rel, _ := model.ParseRelation(f.Relation); (rel == "is" || rel == "are") && len(strings.Fields(f.Object)) > 3 {
			st.Warn(SentenceWarning)
		}
	}

	res := RememberResult{Remembered: []model.Record{}, Forgotten: []model.Record{}}
	if replace && len(facts) > 0 {
		snap, err := load(st)
		if err != nil {
			return RememberResult{}, err
		}
		res.Forgotten = replaced(snap.Live(), facts, now)
	}
	// Built after the forgets so the new records sort after them.
	for _, f := range facts {
		rec, _ := f.Record(now) // validated above
		res.Remembered = append(res.Remembered, rec)
	}
	if err := appendRecords(st, append(res.Forgotten, res.Remembered...)...); err != nil {
		return RememberResult{}, err
	}
	return res, nil
}

// replaced returns one forget record per live record sharing a subject and
// relation with one of facts, in uuid order. Only records already in the log
// are replaced, not other facts of the same write.
func replaced(live []model.Record, facts []Fact, now time.Time) []model.Record {
	type key struct{ subject, relation string }
	keys := make(map[key]bool, len(facts))
	for _, f := range facts {
		rel, _ := model.ParseRelation(f.Relation) // validated by the caller
		keys[key{model.NormalizeName(f.Subject), rel}] = true
	}
	forgets := []model.Record{}
	for _, r := range live {
		if keys[key{r.Subject, r.Relation}] {
			forgets = append(forgets, model.NewForget(r.UUID, "", now))
		}
	}
	return forgets
}

func appendRecords(st Store, recs ...model.Record) error {
	if len(recs) == 0 {
		return nil
	}
	if err := st.Append(recs...); err != nil {
		return StoreErr(err)
	}
	return nil
}

// ForgetArgs selects what to forget: UUID, or Subject and Relation narrowed
// by Object when it is non-nil (SPEC section 5).
type ForgetArgs struct {
	UUID     string
	Subject  string
	Relation string
	Object   *string
	Reason   string
}

// ForgetResult is `forget --json`.
type ForgetResult struct {
	Forgotten []model.Record `json:"forgotten"`
}

// Forget writes one forget record per matching live record. Nothing
// matching is exit 2.
func Forget(st Store, a ForgetArgs, now time.Time) (ForgetResult, error) {
	match, desc, err := forgetMatcher(a)
	if err != nil {
		return ForgetResult{}, err
	}
	snap, err := load(st)
	if err != nil {
		return ForgetResult{}, err
	}
	res := ForgetResult{Forgotten: []model.Record{}}
	for _, r := range snap.Live() {
		if match(r) {
			res.Forgotten = append(res.Forgotten, model.NewForget(r.UUID, a.Reason, now))
		}
	}
	if len(res.Forgotten) == 0 {
		return ForgetResult{}, NotFoundf("no live record matches %s", desc)
	}
	if err := appendRecords(st, res.Forgotten...); err != nil {
		return ForgetResult{}, err
	}
	return res, nil
}

// forgetMatcher returns the record filter and its description for the
// not-found error. Subject, relation and object are compared normalized.
func forgetMatcher(a ForgetArgs) (func(model.Record) bool, string, error) {
	if a.UUID != "" {
		if a.Subject != "" || a.Relation != "" || a.Object != nil {
			return nil, "", Usagef("forget takes a uuid, or a subject and relation, not both")
		}
		if !model.IsUUID(a.UUID) {
			return nil, "", Usagef("%q is not a uuid", a.UUID)
		}
		id := a.UUID
		return func(r model.Record) bool { return r.UUID == id }, "uuid " + id, nil
	}
	rel, err := model.ParseRelation(a.Relation)
	if err != nil {
		return nil, "", Usagef("%w", err)
	}
	subj := model.NormalizeName(a.Subject)
	if subj == "" {
		return nil, "", Usagef("empty subject. give a name as the subject")
	}
	if a.Object == nil {
		return func(r model.Record) bool { return r.Subject == subj && r.Relation == rel },
			fmt.Sprintf("%q", subj+" "+rel), nil
	}
	obj := *a.Object
	norm := model.NormalizeName(obj)
	return func(r model.Record) bool {
		return r.Subject == subj && r.Relation == rel && model.NormalizeName(r.Object) == norm
	}, fmt.Sprintf("%q", subj+" "+rel+" "+obj), nil
}

// ForgetUsage is the usage error of a forget given neither a uuid nor a
// subject and relation.
const ForgetUsage = "forget needs <uuid>, or <subject> <relation> [<object...>]"
