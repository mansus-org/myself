package cli_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/server"
)

// remoteEnv is a server over a temp store plus a way to run the binary
// against it.
type remoteEnv struct {
	t     *testing.T
	url   string
	store string
}

func newRemote(t *testing.T) *remoteEnv {
	t.Helper()
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	srv, err := server.New(server.Config{File: file, AdminHash: server.HashToken(testAdminToken)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &remoteEnv{t, ts.URL, file}
}

// myself runs the binary with a clean environment plus extra (KEY=VALUE).
func (r *remoteEnv) myself(stdin string, extra []string, args ...string) (stdout, stderr string, code int) {
	r.t.Helper()
	return runBinary(r.t, stdin, extra, args...)
}

func runBinary(t *testing.T, stdin string, extra []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Dir = t.TempDir()
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = cleanEnv(extra...)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func (r *remoteEnv) env(token string) []string {
	return []string{"MYSELF_URL=" + r.url, "MYSELF_TOKEN=" + token}
}

func (r *remoteEnv) createToken(scope string) string {
	r.t.Helper()
	out, errs, code := r.myself("", r.env(testAdminToken), "remember", "x", "is", "y")
	if code != 0 {
		r.t.Fatalf("seed: %s %s", out, errs)
	}
	req, _ := http.NewRequest("POST", r.url+"/api/v1/tokens", strings.NewReader(`{"name":"t-`+scope+`","scope":"`+scope+`"}`))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	s := b.String()
	i := strings.Index(s, `"token":"`)
	if i < 0 {
		r.t.Fatalf("no token in %s", s)
	}
	s = s[i+len(`"token":"`):]
	return s[:strings.Index(s, `"`)]
}

func TestRemoteAuthFailures(t *testing.T) {
	r := newRemote(t)
	read := r.createToken("read")
	cases := []struct {
		name  string
		token string
		args  []string
		want  string
	}{
		{"missing", "", []string{"describe", "x"}, "error: missing token: send the header Authorization: Bearer <token>\n"},
		{"invalid", "mys_nope", []string{"ls"}, "error: invalid or revoked token\n"},
		{"read-only write", read, []string{"remember", "a", "is", "b"}, "error: token t-read is read-only; this endpoint needs a write token\n"},
		{"read-only forget", read, []string{"forget", "a", "is"}, "error: token t-read is read-only; this endpoint needs a write token\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := r.myself("", r.env(tc.token), append([]string{"--json"}, tc.args...)...)
			if out != "" || code != 3 || errs != tc.want {
				t.Errorf("stdout %q stderr %q exit %d; want stderr %q exit 3", out, errs, code, tc.want)
			}
			if tc.token != "" && strings.Contains(errs, tc.token) {
				t.Errorf("token leaked: %q", errs)
			}
		})
	}
	// A read token reads.
	if out, _, code := r.myself("", r.env(read), "describe", "x"); code != 0 || out != "x\n  is y\n" {
		t.Errorf("read token: %q %d", out, code)
	}
}

func TestRemoteDisabledServer(t *testing.T) {
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	srv, _ := server.New(server.Config{File: file})
	ts := httptest.NewServer(srv)
	defer ts.Close()
	out, errs, code := runBinary(t, "", []string{"MYSELF_URL=" + ts.URL, "MYSELF_TOKEN=x"}, "ls")
	if out != "" || code != 3 || !strings.HasPrefix(errs, "error: the API is disabled") {
		t.Errorf("%q %q %d", out, errs, code)
	}
}

func TestRemoteUnreachable(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	out, errs, code := runBinary(t, "", []string{"MYSELF_URL=" + url, "MYSELF_TOKEN=secret-token"}, "ls")
	if out != "" || code != 3 || !strings.HasPrefix(errs, "error: cannot reach "+url+": ") || strings.Contains(errs, "secret-token") {
		t.Errorf("%q %q %d", out, errs, code)
	}
}

func TestRemoteTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer slow.Close()
	start := time.Now()
	_, errs, code := runBinary(t, "", []string{"MYSELF_URL=" + slow.URL, "MYSELF_TIMEOUT=0.2"}, "ls")
	if code != 3 || !strings.HasPrefix(errs, "error: cannot reach "+slow.URL+": ") || time.Since(start) > 1500*time.Millisecond {
		t.Errorf("%q %d after %v", errs, code, time.Since(start))
	}
}

func TestRemoteNotMyself(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>hi</html>"))
	}))
	defer ts.Close()
	_, errs, code := runBinary(t, "", []string{"MYSELF_URL=" + ts.URL}, "ls")
	if code != 3 || !strings.Contains(errs, "not a myself server") {
		t.Errorf("%q %d", errs, code)
	}
}

func TestRemoteFlagsAndPrecedence(t *testing.T) {
	r := newRemote(t)
	other := httptest.NewServer(http.NotFoundHandler())
	other.Close()

	// --url beats MYSELF_URL; --token beats MYSELF_TOKEN.
	out, errs, code := runBinary(t, "", []string{"MYSELF_URL=" + other.URL, "MYSELF_TOKEN=bad"}, "--url", r.url, "--token", testAdminToken, "whoami")
	if code != 0 || out != "name admin\nscope write\nadmin true\n" || errs != "" {
		t.Errorf("flags: %q %q %d", out, errs, code)
	}
	out, _, _ = runBinary(t, "", r.env(testAdminToken), "whoami", "--json")
	if out != `{"name":"admin","scope":"write","admin":true}`+"\n" {
		t.Errorf("whoami json: %q", out)
	}

	// --file with a URL is a usage error; MYSELF_FILE alone is not.
	for _, extra := range [][]string{r.env(testAdminToken), nil} {
		args := []string{"--file", filepath.Join(t.TempDir(), "b.jsonl"), "ls"}
		if extra == nil {
			args = append([]string{"--url", r.url}, args...)
		}
		out, errs, code = runBinary(t, "", extra, args...)
		if out != "" || code != 1 || !strings.HasPrefix(errs, "error: --file works on a local store") {
			t.Errorf("--file conflict: %q %q %d", out, errs, code)
		}
	}
	if _, _, code = runBinary(t, "", append(r.env(testAdminToken), "MYSELF_FILE=/dev/null/x"), "ls"); code != 0 {
		t.Errorf("MYSELF_FILE with a URL: exit %d", code)
	}

	// Bad URL and bad timeout are usage errors; version and serve ignore them.
	if _, errs, code = runBinary(t, "", []string{"MYSELF_URL=ftp://x"}, "ls"); code != 1 || !strings.HasPrefix(errs, "error: invalid server URL") {
		t.Errorf("bad url: %q %d", errs, code)
	}
	if _, errs, code = runBinary(t, "", append(r.env(""), "MYSELF_TIMEOUT=soon"), "ls"); code != 1 || !strings.Contains(errs, "MYSELF_TIMEOUT") {
		t.Errorf("bad timeout: %q %d", errs, code)
	}
	if _, _, code = runBinary(t, "", []string{"MYSELF_URL=ftp://x"}, "version"); code != 0 {
		t.Errorf("version with bad url: %d", code)
	}
	if _, errs, code = runBinary(t, "", nil, "--token", "x", "ls"); code != 1 || !strings.Contains(errs, "--token needs a server URL") {
		t.Errorf("token without url: %q %d", errs, code)
	}
}

func TestRemoteSyncRefused(t *testing.T) {
	r := newRemote(t)
	out, errs, code := r.myself("", r.env("tok"), "sync")
	want := "error: sync works on a local store; with MYSELF_URL set, back up with: curl -H \"Authorization: Bearer $MYSELF_TOKEN\" " + r.url + "/api/v1/export.jsonl > brain.jsonl\n"
	if out != "" || code != 1 || errs != want {
		t.Errorf("%q %q %d", out, errs, code)
	}
}

func TestWhoamiLocal(t *testing.T) {
	out, errs, code := runBinary(t, "", []string{"MYSELF_FILE=/tmp/x/brain.jsonl"}, "whoami")
	if out != "local store /tmp/x/brain.jsonl\n" || errs != "" || code != 0 {
		t.Errorf("%q %q %d", out, errs, code)
	}
	out, _, _ = runBinary(t, "", []string{"MYSELF_FILE=/tmp/x/brain.jsonl"}, "whoami", "--json")
	if out != `{"store":"/tmp/x/brain.jsonl"}`+"\n" {
		t.Errorf("json: %q", out)
	}
}

func TestRemoteEntityWithSlash(t *testing.T) {
	r := newRemote(t)
	env := r.env(testAdminToken)
	if _, errs, code := r.myself("", env, "remember", "a/b", "is", "thing"); code != 0 {
		t.Fatal(errs)
	}
	out, errs, code := r.myself("", env, "describe", "a/b")
	if code != 0 || out != "a/b\n  is thing\n" {
		t.Errorf("%q %q %d", out, errs, code)
	}
	if out, _, code = r.myself("", env, "history", "a/b"); code != 0 || !strings.Contains(out, "a/b is thing") {
		t.Errorf("history %q %d", out, code)
	}
}
