package cli_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitMergeRecipe runs the SPEC section 4 merge recipe on two diverged
// logs and checks myself reads the result without warnings.
func TestGitMergeRecipe(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	file := filepath.Join(repo, "brain.jsonl")
	env := cleanEnv(
		"MYSELF_FILE="+file,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	sh := func(allowFail bool, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = repo, env
		out, err := cmd.CombinedOutput()
		if err != nil && !allowFail {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	my := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir, cmd.Env = repo, env
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("myself %v: %v\n%s", args, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("myself %v wrote to stderr: %s", args, stderr.String())
		}
		return string(out)
	}

	sh(false, "git", "init", "-q", "-b", "main")
	my("remember", "relay", "is", "project")
	sh(false, "git", "add", "brain.jsonl")
	sh(false, "git", "commit", "-q", "-m", "base")

	sh(false, "git", "checkout", "-q", "-b", "side")
	my("remember", "relay", "uses", "bun")
	sh(false, "git", "commit", "-q", "-am", "side")

	sh(false, "git", "checkout", "-q", "main")
	my("remember", "project", "is", "software")
	sh(false, "git", "commit", "-q", "-am", "main")

	if out := sh(true, "git", "merge", "side"); !strings.Contains(out, "CONFLICT") {
		t.Fatalf("expected a merge conflict, got:\n%s", out)
	}
	// The recipe from SPEC section 4, verbatim.
	sh(false, "sh", "-c", `git checkout --merge brain.jsonl
{ git show :2:brain.jsonl; git show :3:brain.jsonl; } | sort -u > brain.jsonl
git add brain.jsonl`)
	sh(false, "git", "commit", "-q", "-m", "merge")

	log := my("log")
	for _, want := range []string{" relay is project\n", " relay uses bun\n", " project is software\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	if n := strings.Count(log, "\n"); n != 3 {
		t.Errorf("log has %d lines, want 3:\n%s", n, log)
	}
	// sort -u on uuid-first lines is a uuid sort.
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i-1] > lines[i] {
			t.Errorf("log not in uuid order:\n%s", log)
		}
	}
	if got := my("describe", "relay", "--depth", "2"); !strings.Contains(got, "uses bun") || !strings.Contains(got, "software") {
		t.Errorf("describe misses a side:\n%s", got)
	}
}
