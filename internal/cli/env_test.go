package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// isolatedVars are the variables that point myself at a brain, or at what reads
// it. A host that uses a shared brain sets them in its environment; tests must
// never inherit them, or a golden file would depend on the machine running them.
var isolatedVars = []string{
	"MYSELF_URL", "MYSELF_TOKEN", "MYSELF_TIMEOUT", "MYSELF_FILE",
	"MYSELF_NEO4J_URL", "MYSELF_NEO4J_USER", "MYSELF_NEO4J_PASSWORD", "MYSELF_NEO4J_DATABASE", "MYSELF_NEO4J_TIMEOUT",
	"MYSELF_EMBED_URL", "MYSELF_EMBED_MODEL", "MYSELF_EMBED_API_KEY", "MYSELF_EMBED_DIMENSIONS", "MYSELF_EMBED_TIMEOUT",
}

// cleanEnv returns os.Environ() without the brain-selecting variables, plus extra (KEY=VALUE).
func cleanEnv(extra ...string) []string {
	var env []string
	for _, e := range os.Environ() {
		drop := false
		for _, v := range isolatedVars {
			if strings.HasPrefix(e, v+"=") {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, e)
		}
	}
	return append(env, extra...)
}

// unsetRemoteEnv is the TestMain safety net: a leaked MYSELF_URL would send
// test traffic to a real server.
func unsetRemoteEnv() {
	if os.Getenv("MYSELF_URL") != "" {
		os.Unsetenv("MYSELF_URL")
		os.Unsetenv("MYSELF_TOKEN")
		os.Stderr.WriteString("tests: MYSELF_URL is set in the environment; ignoring it (tests never talk to a real server)\n")
	}
}

// TestCleanEnvIgnoresHostRemote sets MYSELF_URL to a server that fails on any
// request, then runs the binary with the golden-suite env and expects silence.
func TestCleanEnvIgnoresHostRemote(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("unexpected request to host MYSELF_URL: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	t.Setenv("MYSELF_URL", ts.URL)
	t.Setenv("MYSELF_TOKEN", "host-token")
	t.Setenv("MYSELF_TIMEOUT", "1s")

	for _, e := range cleanEnv() {
		if strings.HasPrefix(e, "MYSELF_URL=") || strings.HasPrefix(e, "MYSELF_TOKEN=") {
			t.Fatalf("cleanEnv leaked %s", e)
		}
	}
	file := t.TempDir() + "/brain.jsonl"
	cmd := exec.Command(binary, "remember", "isolation-check", "is", "test")
	cmd.Env = cleanEnv("MYSELF_FILE=" + file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remember: %v\n%s", err, out)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("host server got %d requests, want 0", n)
	}
}
