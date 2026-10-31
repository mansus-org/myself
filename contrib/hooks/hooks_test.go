// Package hooks_test runs the Claude Code hook scripts in this directory
// against a freshly built myself binary and a temp store.
package hooks_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "myself-hooks")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "myself")
	build := exec.Command("go", "build", "-o", binary, "github.com/mansus-org/myself/cmd/myself")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const nudge = "Before finishing: anything durable worth `myself remember`-ing from this session? If not, just stop."

// env is one simulated agent host: its own TMPDIR (hook markers) and store.
type env struct {
	t     *testing.T
	tmp   string
	store string
	bin   string
	jq    string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, tmp: tmp, store: filepath.Join(dir, "brain.jsonl"), bin: binary, jq: "jq"}
}

// requireCommand skips the test when the binary lacks a subcommand that is
// still being implemented on another branch.
func (e *env) requireCommand(name string) {
	e.t.Helper()
	cmd := exec.Command(e.bin, name, "--help")
	cmd.Env = append(os.Environ(), "MYSELF_FILE="+e.store)
	if err := cmd.Run(); err != nil {
		e.t.Skipf("myself %s is not implemented in this build (myself %s --help: %v); skipping", name, name, err)
	}
}

func (e *env) environ() []string {
	return append(os.Environ(),
		"TMPDIR="+e.tmp,
		"MYSELF_FILE="+e.store,
		"MYSELF_BIN="+e.bin,
		"MYSELF_HOOKS_JQ="+e.jq,
	)
}

func (e *env) myself(args ...string) {
	e.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = e.environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("myself %v: %v\n%s", args, err, out)
	}
}

// hook runs a script with the given stdin and returns its stdout. Hooks must
// always exit 0 and write nothing to stderr.
func (e *env) hook(script, stdin string) string {
	e.t.Helper()
	cmd := exec.Command("sh", script)
	cmd.Env = e.environ()
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		e.t.Fatalf("%s: %v\nstderr: %s", script, err, stderr.String())
	}
	if stderr.Len() > 0 {
		e.t.Errorf("%s wrote to stderr: %s", script, stderr.String())
	}
	return stdout.String()
}

// startInput is SessionStart hook JSON, pretty-printed like Claude Code may
// send it, so the sed fallback has to cope with newlines.
func startInput(sid string) string {
	return fmt.Sprintf(`{
  "session_id": %q,
  "transcript_path": "/nonexistent/transcript.jsonl",
  "cwd": "/",
  "hook_event_name": "SessionStart",
  "source": "startup"
}`, sid)
}

func stopInput(sid, transcript, cwd string, active bool) string {
	return fmt.Sprintf(`{
  "session_id": %q,
  "transcript_path": %q,
  "cwd": %q,
  "hook_event_name": "Stop",
  "stop_hook_active": %t,
  "last_assistant_message": "done"
}`, sid, transcript, cwd, active)
}

// transcript writes a transcript file with n lines.
func transcript(t *testing.T, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("{\"type\":\"user\"}\n", n)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// parsers runs f once with jq and once with the sed fallback.
func parsers(t *testing.T, f func(t *testing.T, e *env)) {
	for _, jq := range []string{"jq", "no-such-jq"} {
		name := "sed"
		if jq == "jq" {
			name = "jq"
		}
		t.Run(name, func(t *testing.T) {
			if jq == "jq" {
				if _, err := exec.LookPath("jq"); err != nil {
					t.Skip("jq not installed")
				}
			}
			e := newEnv(t)
			e.jq = jq
			f(t, e)
		})
	}
}

func TestSessionStartInjectsPrime(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("prime")
		e.myself("remember", "relay", "is", "project")
		e.myself("remember", "i", "does", "a coding agent that keeps notes in myself")
		out := e.hook("session-start.sh", startInput("s1"))
		for _, want := range []string{"`myself prime`", "relay\n  is project\n", "self\n  does a coding agent that keeps notes in myself\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
		raw, err := os.ReadFile(filepath.Join(e.tmp, "myself-hooks", "s1.start"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw))); err != nil {
			t.Errorf("start marker %q: %v", raw, err)
		}
	})
}

func TestSessionStartEmptyStore(t *testing.T) {
	e := newEnv(t)
	e.requireCommand("prime")
	if out := e.hook("session-start.sh", startInput("s1")); out != "" {
		t.Errorf("empty store: want no output, got %q", out)
	}
}

func TestSessionStartKeepsFirstStartTime(t *testing.T) {
	e := newEnv(t)
	marker := filepath.Join(e.tmp, "myself-hooks", "s1.start")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("2026-01-01T00:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.hook("session-start.sh", startInput("s1"))
	if raw, _ := os.ReadFile(marker); string(raw) != "2026-01-01T00:00:00Z\n" {
		t.Errorf("resume overwrote the start marker: %q", raw)
	}
}

func TestMyselfMissing(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.bin = filepath.Join(t.TempDir(), "no-such-myself")
		if out := e.hook("session-start.sh", startInput("s1")); out != "" {
			t.Errorf("session-start: want no output, got %q", out)
		}
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, 50), "/", false)); out != "" {
			t.Errorf("stop-nudge: want no output, got %q", out)
		}
	})
}

func TestStopNudgesOnce(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.myself("remember", "relay", "is", "project") // before the session: must not count
		time.Sleep(1100 * time.Millisecond)            // start marker has second precision
		e.hook("session-start.sh", startInput("s1"))
		tr := transcript(t, 50)

		out := e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false))
		var got struct{ Decision, Reason string }
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("first stop: want block JSON, got %q: %v", out, err)
		}
		if got.Decision != "block" || got.Reason != nudge {
			t.Errorf("first stop: got %+v", got)
		}
		if out := e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)); out != "" {
			t.Errorf("second stop: want no output, got %q", out)
		}
	})
}

func TestStopSilentWhenActive(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, 50), "/", true)); out != "" {
			t.Errorf("stop_hook_active: want no output, got %q", out)
		}
	})
}

func TestStopSilentWhenRemembered(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		e.myself("remember", "relay", "uses", "bun")
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, 50), "/", false)); out != "" {
			t.Errorf("facts written this session: want no output, got %q", out)
		}
	})
}

func TestStopSilentWhenIdle(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, 3), t.TempDir(), false)); out != "" {
			t.Errorf("short transcript, no git changes: want no output, got %q", out)
		}
	})
}

func TestStopSilentWithoutStartMarker(t *testing.T) {
	e := newEnv(t)
	e.requireCommand("log")
	if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, 50), "/", false)); out != "" {
		t.Errorf("no session-start marker: want no output, got %q", out)
	}
}

func TestStopNudgesOnGitChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := newEnv(t)
	e.requireCommand("log")
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.hook("session-start.sh", startInput("s1"))
	out := e.hook("stop-nudge.sh", stopInput("s1", "", repo, false))
	if !strings.Contains(out, `"decision":"block"`) {
		t.Errorf("git changes: want a nudge, got %q", out)
	}
}

func TestBadSessionID(t *testing.T) {
	e := newEnv(t)
	e.hook("session-start.sh", startInput("../escape"))
	if out := e.hook("stop-nudge.sh", stopInput("../escape", transcript(t, 50), "/", false)); out != "" {
		t.Errorf("unsafe session id: want no output, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(e.tmp, "escape.start")); err == nil {
		t.Error("unsafe session id wrote a marker outside myself-hooks")
	}
}
