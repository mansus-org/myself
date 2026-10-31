package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type host struct {
	t    *testing.T
	dir  string
	file string
	env  []string
}

func newHost(t *testing.T, dir string) *host {
	h := &host{t: t, dir: dir, file: filepath.Join(dir, "brain.jsonl")}
	h.env = append(os.Environ(),
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MYSELF_FILE="+h.file,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	return h
}

func (h *host) run(name string, args ...string) (string, string, int) {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = h.dir, h.env
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		h.t.Fatal(err)
	}
	return so.String(), se.String(), code
}

func (h *host) must(name string, args ...string) string {
	h.t.Helper()
	so, se, code := h.run(name, args...)
	if code != 0 {
		h.t.Fatalf("%s %v: exit %d\n%s%s", name, args, code, so, se)
	}
	return so
}

func requireGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func TestSyncTwoHosts(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	hosts := []*host{}
	for _, n := range []string{"a", "b"} {
		d := filepath.Join(root, n)
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		h := newHost(t, d)
		if len(hosts) == 0 {
			h.must("git", "init", "-q", "--bare", "-b", "main", remote)
		}
		h.must("git", "init", "-q", "-b", "main")
		h.must("git", "remote", "add", "origin", remote)
		hosts = append(hosts, h)
	}
	a, b := hosts[0], hosts[1]
	a.must(binary, "remember", "relay", "is", "project")
	a.must(binary, "remember", "relay", "uses", "bun")
	b.must(binary, "remember", "bun", "is", "tool")

	out := a.must(binary, "sync")
	if !strings.Contains(out, "committed 2 records") || !strings.Contains(out, "pushed") {
		t.Fatalf("a sync: %q", out)
	}
	vault := filepath.Join(root, "vault")
	out = b.must(binary, "sync", "--export", vault)
	for _, want := range []string{"committed 1 records", "pulled", "exported 4 entities to " + vault, "pushed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("b sync missing %q: %q", want, out)
		}
	}
	// A diverges again while B has pushed: a second round trip.
	a.must(binary, "remember", "project", "is", "thing")
	a.must(binary, "sync")
	b.must(binary, "sync")
	js := a.must(binary, "sync", "--json")
	if !strings.Contains(js, `"committed":0`) || !strings.Contains(js, `"pulled":true`) || !strings.Contains(js, `"exported":null`) {
		t.Fatalf("json: %s", js)
	}

	la, lb := a.must(binary, "log"), b.must(binary, "log")
	if la != lb {
		t.Fatalf("logs differ:\n%s\n---\n%s", la, lb)
	}
	if n := strings.Count(la, "\n"); n != 4 {
		t.Fatalf("want 4 records, got %d:\n%s", n, la)
	}
	if got := a.must("git", "status", "--porcelain", "--", "brain.jsonl"); got != "" {
		t.Fatalf("dirty: %s", got)
	}
}

func TestSyncNoRemote(t *testing.T) {
	requireGit(t)
	h := newHost(t, t.TempDir())
	h.must("git", "init", "-q", "-b", "main")
	h.must(binary, "remember", "a", "is", "b")
	out := h.must(binary, "sync")
	if !strings.Contains(out, "committed 1 records") || !strings.Contains(out, "no remote") {
		t.Fatalf("out: %q", out)
	}
	if out := h.must(binary, "sync"); !strings.Contains(out, "nothing to commit") {
		t.Fatalf("out: %q", out)
	}
	attrs, _ := os.ReadFile(filepath.Join(h.dir, ".gitattributes"))
	if string(attrs) != "brain.jsonl merge=myself\n" {
		t.Fatalf("attrs: %q", attrs)
	}
	if got := strings.TrimSpace(h.must("git", "config", "--local", "merge.myself.driver")); got != "myself merge-driver %O %A %B" {
		t.Fatalf("driver: %q", got)
	}
}

func TestSyncNotARepo(t *testing.T) {
	requireGit(t)
	h := newHost(t, t.TempDir())
	_, se, code := h.run(binary, "sync")
	if code != 1 || !strings.Contains(se, "git init") || !strings.Contains(se, "merge=myself") {
		t.Fatalf("code %d stderr %q", code, se)
	}
}
