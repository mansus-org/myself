package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const line1 = `{"uuid":"0192d7c4-0001-7000-8000-000000000001","subject":"relay","relation":"is","object":"project","time":"2026-10-29T16:00:00Z"}`
const line2 = `{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun","time":"2026-10-29T16:00:05Z"}`

func TestReadValid(t *testing.T) {
	recs, warns, err := Read(strings.NewReader(line1 + "\n" + line2 + "\n"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if len(recs) != 2 || recs[0].Subject != "relay" || recs[1].Relation != "uses" {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestReadTrailingPartialLineIgnored(t *testing.T) {
	recs, warns, err := Read(strings.NewReader(line1 + "\n" + line2[:40]))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if len(recs) != 1 {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestReadCorruptLineWarns(t *testing.T) {
	in := line1 + "\nnot json\n{\"uuid\":\"\"}\n" + line2 + "\n"
	recs, warns, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("recs=%+v", recs)
	}
	want := []string{"line 2: corrupt, skipped", "line 3: corrupt, skipped"}
	if !reflect.DeepEqual(warns, want) {
		t.Fatalf("warns=%v want %v", warns, want)
	}
}

func TestReadFileMissingIsEmpty(t *testing.T) {
	recs, warns, err := ReadFile(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil || len(recs) != 0 || len(warns) != 0 {
		t.Fatalf("recs=%v warns=%v err=%v", recs, warns, err)
	}
}

func TestAppendRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "brain.jsonl")
	a := Record{"0192d7c4-0001-7000-8000-000000000001", "relay", "is", "project", "2026-10-29T16:00:00Z"}
	b := Record{"0192d7c4-0002-7000-8000-000000000002", "relay", "uses", "bun", "2026-10-29T16:00:05Z"}
	if err := Append(path, a); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, b); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != line1+"\n"+line2+"\n" {
		t.Fatalf("raw=%q", raw)
	}
	recs, warns, err := ReadFile(path)
	if err != nil || len(warns) != 0 || !reflect.DeepEqual(recs, []Record{a, b}) {
		t.Fatalf("recs=%v warns=%v err=%v", recs, warns, err)
	}
}
