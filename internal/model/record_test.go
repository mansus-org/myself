package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)

func TestRecordJSONKeyOrder(t *testing.T) {
	r := Record{
		UUID:     "0192d7c4-0002-7000-8000-000000000002",
		Subject:  "relay",
		Relation: "uses",
		Object:   "bun for the backend, biome for linting",
		Time:     "2026-10-29T16:00:05Z",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun for the backend, biome for linting","time":"2026-10-29T16:00:05Z"}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

func TestFormatTime(t *testing.T) {
	loc := time.FixedZone("x", 2*3600)
	tests := []struct {
		in   time.Time
		want string
	}{
		{t0, "2026-10-29T16:00:00Z"},
		{time.Date(2026, 10, 29, 18, 0, 0, 0, loc), "2026-10-29T16:00:00Z"},
		{t0.Add(123456789 * time.Nanosecond), "2026-10-29T16:00:00Z"},
	}
	for _, tt := range tests {
		if got := FormatTime(tt.in); got != tt.want {
			t.Errorf("FormatTime(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewRecord(t *testing.T) {
	tests := []struct {
		name              string
		subj, rel, obj    string
		wantSubj, wantRel string
		wantObj           string
		wantErr           string
	}{
		{name: "basic", subj: "relay", rel: "is", obj: "project", wantSubj: "relay", wantRel: "is", wantObj: "project"},
		{name: "are stored as is", subj: "project", rel: "are", obj: "software", wantSubj: "project", wantRel: "is", wantObj: "software"},
		{name: "subject normalized", subj: "Relay App", rel: "USES", obj: "bun", wantSubj: "relay-app", wantRel: "uses", wantObj: "bun"},
		{name: "self subject", subj: "me", rel: "prefers", obj: "terse output", wantSubj: "self", wantRel: "prefers", wantObj: "terse output"},
		{name: "object verbatim but trimmed", subj: "relay", rel: "uses", obj: "  Bun for the Backend.  ", wantSubj: "relay", wantRel: "uses", wantObj: "Bun for the Backend."},
		{name: "object not self-resolved", subj: "relay", rel: "related", obj: "Me", wantSubj: "relay", wantRel: "related", wantObj: "Me"},
		{name: "unknown relation", subj: "relay", rel: "built-with", obj: "bun", wantErr: `unknown relation "built-with". use one of: is, are, has, uses, does, prefers, related`},
		{name: "forget reserved", subj: "relay", rel: "forget", obj: "x", wantErr: `relation "forget" is reserved, use the forget command. use one of: is, are, has, uses, does, prefers, related`},
		{name: "empty object", subj: "relay", rel: "is", obj: "   ", wantErr: "empty object"},
		{name: "empty subject", subj: " ?! ", rel: "is", obj: "x", wantErr: "empty subject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRecord(tt.subj, tt.rel, tt.obj, t0)
			if tt.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want prefix %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Subject != tt.wantSubj || r.Relation != tt.wantRel || r.Object != tt.wantObj {
				t.Errorf("got %+v", r)
			}
			if r.Time != "2026-10-29T16:00:00Z" {
				t.Errorf("time = %q", r.Time)
			}
			if !IsUUID(r.UUID) || r.UUID[14] != '7' {
				t.Errorf("uuid = %q", r.UUID)
			}
			if r.IsForget() {
				t.Error("IsForget on a normal record")
			}
		})
	}
}

func TestNewRecordKeepsBulkOrder(t *testing.T) {
	var prev string
	for i := 0; i < 10000; i++ {
		r, err := NewRecord("relay", "has", "thing", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r.UUID <= prev {
			t.Fatalf("record %d uuid %s not after %s", i, r.UUID, prev)
		}
		prev = r.UUID
	}
}

func TestNewForget(t *testing.T) {
	target := "0192d7c4-0002-7000-8000-000000000002"
	tests := []struct{ reason, want string }{
		{"moved off bun", "moved off bun"},
		{"  padded  ", "padded"},
		{"", ""},
	}
	for _, tt := range tests {
		r := NewForget(target, tt.reason, t0)
		if r.Subject != target || r.Relation != "forget" || r.Object != tt.want || !r.IsForget() {
			t.Errorf("NewForget(%q) = %+v", tt.reason, r)
		}
		if !IsUUID(r.UUID) || r.Time != "2026-10-29T16:00:00Z" {
			t.Errorf("uuid/time wrong: %+v", r)
		}
	}
}
