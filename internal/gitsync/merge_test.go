package gitsync

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	r1 = `{"uuid":"0192d7c4-0001-7000-8000-000000000001","subject":"a","relation":"is","object":"x","time":"2026-10-29T16:00:00Z"}`
	r2 = `{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"b","relation":"is","object":"x","time":"2026-10-29T16:00:01Z"}`
	r3 = `{"uuid":"0192d7c4-0003-7000-8000-000000000003","subject":"c","relation":"is","object":"x","time":"2026-10-29T16:00:02Z"}`
	r4 = `{"uuid":"0192d7c4-0004-7000-8000-000000000004","subject":"d","relation":"is","object":"x","time":"2026-10-29T16:00:03Z"}`
)

func TestMergeTwoHosts(t *testing.T) {
	base := r1 + "\n"
	ours := r1 + "\n" + r3 + "\n" + r4 + "\n"
	theirs := r1 + "\n" + r2 + "\n" + r3 + "\n" + `{"uuid":"0192d7c4-0005` // partial tail
	want := r1 + "\n" + r2 + "\n" + r3 + "\n" + r4 + "\n"
	if got := string(Merge([]byte(base), []byte(ours), []byte(theirs))); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeKeepsJunkLast(t *testing.T) {
	ours := "garbage\n\n" + r2 + "\n"
	theirs := "{\"x\":1}\n" + r1 + "\ngarbage\n"
	want := r1 + "\n" + r2 + "\ngarbage\n{\"x\":1}\n"
	if got := string(Merge(nil, []byte(ours), []byte(theirs))); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeFilesWritesOurs(t *testing.T) {
	dir := t.TempDir()
	w := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	b, o, th := w("b", ""), w("o", r2+"\n"), w("t", r1+"\n")
	if err := MergeFiles(b, o, th); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(o)
	if string(got) != r1+"\n"+r2+"\n" {
		t.Fatalf("got %q", got)
	}
	if err := MergeFiles(filepath.Join(dir, "missing"), o, th); err == nil {
		t.Fatal("want error for missing base")
	}
}
