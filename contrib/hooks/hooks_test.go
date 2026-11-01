// Package hooks_test runs the Claude Code hook scripts in this directory
// against a freshly built myself binary and a temp store.
package hooks_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/server"
)

var binary string

func TestMain(m *testing.M) {
	if os.Getenv("MYSELF_URL") != "" {
		os.Unsetenv("MYSELF_URL")
		os.Unsetenv("MYSELF_TOKEN")
		fmt.Fprintln(os.Stderr, "tests: MYSELF_URL is set in the environment; ignoring it (tests never talk to a real server)")
	}
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

const nudge = "Before finishing: is anything from this session worth `myself remember`-ing (durable facts, decisions, corrections)? If so, remember it. Then end with your final answer to the user again, in full, so it stays the last message; do not mention this check."

// env is one simulated agent host: its own TMPDIR (hook markers) and store.
type env struct {
	t     *testing.T
	tmp   string
	store string
	bin   string
	jq    string
	extra []string // appended to the environment, after MYSELF_FILE
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
	cmd.Env = cleanEnv("MYSELF_FILE=" + e.store)
	if err := cmd.Run(); err != nil {
		e.t.Skipf("myself %s is not implemented in this build (myself %s --help: %v); skipping", name, name, err)
	}
}

func (e *env) environ() []string {
	return append(cleanEnv(
		"TMPDIR="+e.tmp,
		"MYSELF_FILE="+e.store,
		"MYSELF_BIN="+e.bin,
		"MYSELF_HOOKS_JQ="+e.jq,
	), e.extra...)
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

// startInput is SessionStart hook JSON for a new session, pretty-printed like
// Claude Code may send it, so the sed fallback has to cope with newlines.
func startInput(sid string) string { return sessionStartInput(sid, "startup") }

// resumeInput is SessionStart hook JSON for `claude --resume`.
func resumeInput(sid string) string { return sessionStartInput(sid, "resume") }

func sessionStartInput(sid, source string) string {
	return fmt.Sprintf(`{
  "session_id": %q,
  "transcript_path": "/nonexistent/transcript.jsonl",
  "cwd": "/",
  "hook_event_name": "SessionStart",
  "source": %q
}`, sid, source)
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

// transcript writes a transcript file shaped like Claude Code's: a request
// line listing the tool definitions (which mention every tool name), some
// filler lines, then one assistant tool_use line per entry of tools.
func transcript(t *testing.T, tools ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"type":"api-request","tools":[{"name":"Bash","description":"x"},{"name":"Edit","description":"x"},{"name":"Write","description":"x"},{"name":"Read","description":"x"}]}` + "\n")
	b.WriteString(strings.Repeat(`{"type":"attachment"}`+"\n", 40))
	for i, name := range tools {
		input := "{}"
		if n, cmd, ok := strings.Cut(name, "="); ok {
			name, input = n, fmt.Sprintf(`{"command":%q}`, cmd)
		}
		fmt.Fprintf(&b, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_%02d","name":%q,"input":%s}]}}`+"\n", i, name, input)
	}
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
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
		t.Errorf("a second startup overwrote the start marker: %q", raw)
	}
}

func TestMyselfMissing(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.bin = filepath.Join(t.TempDir(), "no-such-myself")
		if out := e.hook("session-start.sh", startInput("s1")); out != "" {
			t.Errorf("session-start: want no output, got %q", out)
		}
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Write"), "/", false)); out != "" {
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
		tr := transcript(t, "Write")

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
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Write"), "/", true)); out != "" {
			t.Errorf("stop_hook_active: want no output, got %q", out)
		}
	})
}

func TestStopSilentWhenRemembered(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		e.myself("remember", "relay", "uses", "bun")
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Write"), "/", false)); out != "" {
			t.Errorf("facts written this session: want no output, got %q", out)
		}
	})
}

func TestStopSilentWhenIdle(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Read", "Bash", "Bash"), t.TempDir(), false)); out != "" {
			t.Errorf("read-only transcript, no git changes: want no output, got %q", out)
		}
	})
}

func TestStopSilentWithoutStartMarker(t *testing.T) {
	e := newEnv(t)
	e.requireCommand("log")
	if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Write"), "/", false)); out != "" {
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
	if out := e.hook("stop-nudge.sh", stopInput("../escape", transcript(t, "Write"), "/", false)); out != "" {
		t.Errorf("unsafe session id: want no output, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(e.tmp, "escape.start")); err == nil {
		t.Error("unsafe session id wrote a marker outside myself-hooks")
	}
}

// expectNudge fails unless out is the block JSON with the nudge reason.
func expectNudge(t *testing.T, what, out string) {
	t.Helper()
	var got struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%s: want block JSON, got %q: %v", what, out, err)
	}
	if got.Decision != "block" || got.Reason != nudge {
		t.Errorf("%s: got %+v", what, got)
	}
}

func TestStopSilentWhenReadOnly(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		// No tool uses at all, though the tool definitions name Write and Edit.
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t), "/", false)); out != "" {
			t.Errorf("zero-tool transcript: want no output, got %q", out)
		}
		if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Read", "Grep", "Skill"), "/", false)); out != "" {
			t.Errorf("read-only transcript: want no output, got %q", out)
		}
	})
}

func TestStopNudgesOnEditTools(t *testing.T) {
	for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
		t.Run(tool, func(t *testing.T) {
			e := newEnv(t)
			e.requireCommand("log")
			e.hook("session-start.sh", startInput("s1"))
			expectNudge(t, tool, e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Read", tool), "/", false)))
		})
	}
}

func TestStopNudgesOnBashUses(t *testing.T) {
	e := newEnv(t)
	e.requireCommand("log")
	e.hook("session-start.sh", startInput("s1"))
	if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Bash", "Bash"), "/", false)); out != "" {
		t.Errorf("2 Bash uses: want no output, got %q", out)
	}
	expectNudge(t, "3 Bash uses", e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Bash", "Bash", "Bash"), "/", false)))
}

func TestResumeAllowsAnotherNudge(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		tr := transcript(t, "Write")
		expectNudge(t, "first stop", e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)))
		if out := e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)); out != "" {
			t.Errorf("second stop: want no output, got %q", out)
		}

		// A fact written before the resume must not silence the resumed session.
		e.myself("remember", "relay", "is", "project")
		time.Sleep(1100 * time.Millisecond) // start marker has second precision
		e.hook("session-start.sh", resumeInput("s1"))
		expectNudge(t, "stop after resume", e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)))
		if out := e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)); out != "" {
			t.Errorf("second stop after resume: want no output, got %q", out)
		}
	})
}

func TestStopIgnoresMyselfBash(t *testing.T) {
	parsers(t, func(t *testing.T, e *env) {
		e.requireCommand("log")
		e.hook("session-start.sh", startInput("s1"))
		tr := transcript(t, "Bash=myself describe x", "Bash=myself log -n 3", "Bash="+e.bin+" search y", "Bash=myself path a b")
		if out := e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)); out != "" {
			t.Errorf("myself-only Bash: want no output, got %q", out)
		}
		tr = transcript(t, "Bash=ls", "Bash=myself log", "Bash=git status", "Bash=go test ./...")
		expectNudge(t, "3 non-myself Bash", e.hook("stop-nudge.sh", stopInput("s1", tr, "/", false)))
	})
}

func TestStopNudgeOff(t *testing.T) {
	e := newEnv(t)
	e.requireCommand("log")
	e.hook("session-start.sh", startInput("s1"))
	t.Setenv("MYSELF_NUDGE", "off")
	if out := e.hook("stop-nudge.sh", stopInput("s1", transcript(t, "Write"), "/", false)); out != "" {
		t.Errorf("MYSELF_NUDGE=off: want no output, got %q", out)
	}
}

// With MYSELF_URL and MYSELF_TOKEN in the environment both hooks run against
// the server: prime comes from it, and a fact remembered through it counts
// as written this session.
func TestRemoteMode(t *testing.T) {
	const token = "hooks-test-admin"
	e := newEnv(t)
	e.requireCommand("whoami")
	srv, err := server.New(server.Config{File: filepath.Join(t.TempDir(), "remote.jsonl"), AdminHash: server.HashToken(token)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	e.extra = []string{"MYSELF_URL=" + ts.URL, "MYSELF_TOKEN=" + token}
	e.store = filepath.Join(t.TempDir(), "unused.jsonl") // must not be touched

	e.myself("remember", "relay", "is", "project")
	time.Sleep(1100 * time.Millisecond) // start marker has second precision
	out := e.hook("session-start.sh", startInput("r1"))
	if !strings.Contains(out, "relay\n  is project\n") {
		t.Errorf("prime from the server missing:\n%s", out)
	}
	tr := transcript(t, "Write")
	var got struct{ Decision string }
	if err := json.Unmarshal([]byte(e.hook("stop-nudge.sh", stopInput("r1", tr, "/", false))), &got); err != nil || got.Decision != "block" {
		t.Errorf("want a nudge with nothing remembered, got %+v %v", got, err)
	}
	e.hook("session-start.sh", resumeInput("r1"))
	time.Sleep(1100 * time.Millisecond) // start marker has second precision
	e.myself("remember", "relay", "uses", "bun")
	if out := e.hook("stop-nudge.sh", stopInput("r1", tr, "/", false)); out != "" {
		t.Errorf("fact remembered through the server this session: want no output, got %q", out)
	}
	if _, err := os.Stat(e.store); err == nil {
		t.Error("the local store was written in remote mode")
	}

	// An unreachable server never fails or stalls the session.
	ts.Close()
	if out := e.hook("session-start.sh", startInput("r2")); out != "" {
		t.Errorf("server down: want no output, got %q", out)
	}
}

// cleanEnv returns os.Environ() without the variables that select a brain, so
// a host using a shared brain never leaks into tests; plus extra (KEY=VALUE).
func cleanEnv(extra ...string) []string {
	var env []string
	for _, e := range os.Environ() {
		keep := true
		for _, v := range []string{"MYSELF_URL", "MYSELF_TOKEN", "MYSELF_TIMEOUT", "MYSELF_FILE"} {
			if strings.HasPrefix(e, v+"=") {
				keep = false
			}
		}
		if keep {
			env = append(env, e)
		}
	}
	return append(env, extra...)
}
