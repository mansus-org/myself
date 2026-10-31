package export

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
)

func rec(id, s, rel, o string) model.Record {
	return model.Record{UUID: id, Subject: s, Relation: rel, Object: o, Time: "2026-10-29T16:00:00Z"}
}

func sample() *graph.Graph {
	return graph.Build([]model.Record{
		rec("1", "relay", "is", "project"),
		rec("2", "relay", "uses", "bun for the backend"),
		rec("3", "project", "is", "software"),
	})
}

func TestNote(t *testing.T) {
	g := sample()
	want := map[string]string{
		"project":  "# project\n\n- is [[software]]\n\n## Linked from\n- [[relay]] is\n",
		"relay":    "# relay\n\n- is [[project]]\n- uses bun for the backend\n",
		"software": "# software\n\n## Linked from\n- [[project]] is\n",
	}
	for name, w := range want {
		if got := Note(g, name); got != w {
			t.Errorf("%s:\n got %q\nwant %q", name, got, w)
		}
	}
}

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "vault")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "relay.md")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Write(dir, sample())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"project.md", "relay.md", "software.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
	b, _ := os.ReadFile(stale)
	if string(b) != Note(sample(), "relay") {
		t.Errorf("relay.md not overwritten: %q", b)
	}
}

func TestWriteCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x", "y")
	if _, err := Write(dir, sample()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project.md")); err != nil {
		t.Error(err)
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"relay":   "relay.md",
		"a/b":     "a_b.md",
		"../evil": "__/evil.md",
		".hidden": "_hidden.md",
	} {
		got := FileName(in)
		if strings.Contains(got, "/") || strings.HasPrefix(got, ".") {
			t.Errorf("FileName(%q) = %q is unsafe", in, got)
		}
		if in != "../evil" && got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}
