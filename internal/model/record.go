// Package model holds the core types and rules of SPEC sections 1 to 3: the
// five-field Record, the closed relation enum, name normalization and UUIDv7
// generation. Every command builds on it.
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TimeLayout is the RFC 3339 UTC form used for Record.Time.
const TimeLayout = "2006-01-02T15:04:05Z"

// Record is one line of the event log. Field order is the on-disk key order
// (SPEC section 4), so encoding/json emits uuid first.
type Record struct {
	UUID     string `json:"uuid"`
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
	Time     string `json:"time"`
}

// FormatTime renders t as an RFC 3339 UTC timestamp with second precision.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// NewRecord validates and builds a record for `remember`: the subject is
// normalized, the relation parsed (`are` stored as `is`), the object trimmed
// but otherwise verbatim. Errors carry no "error: " prefix and are usage
// errors (exit 1).
func NewRecord(subject, relation, object string, now time.Time) (Record, error) {
	rel, err := ParseRelation(relation)
	if err != nil {
		return Record{}, err
	}
	subj := NormalizeName(subject)
	if subj == "" {
		return Record{}, errors.New("empty subject. give a name as the subject")
	}
	obj := strings.TrimSpace(object)
	if obj == "" {
		return Record{}, errors.New("empty object. give the object as free text")
	}
	return Record{
		UUID:     newUUIDAt(now),
		Subject:  subj,
		Relation: rel,
		Object:   obj,
		Time:     FormatTime(now),
	}, nil
}

// NewForget builds a retraction of target. The reason may be empty.
func NewForget(target, reason string, now time.Time) Record {
	return Record{
		UUID:     newUUIDAt(now),
		Subject:  target,
		Relation: RelationForget,
		Object:   strings.TrimSpace(reason),
		Time:     FormatTime(now),
	}
}

// IsForget reports whether r is a retraction.
func (r Record) IsForget() bool { return r.Relation == RelationForget }

// Marshal is json.Marshal without HTML escaping: `<`, `>` and `&` stay
// literal in the log and in --json output (SPEC sections 4 and 6), where
// "<" would only cost tokens and break the canonical line form.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
