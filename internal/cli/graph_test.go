package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/server"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func clearClientEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvURL, EnvToken, EnvAdminHash, EnvTimeout, EnvFile} {
		t.Setenv(k, "")
	}
}

// loginFrom checks a printed link and returns the one-time code in it.
func loginFrom(t *testing.T, line, base string) string {
	t.Helper()
	line = strings.TrimSpace(line)
	u, err := url.Parse(line)
	if err != nil {
		t.Fatal(err)
	}
	code, ok := strings.CutPrefix(u.Fragment, "login=")
	if !ok || u.Path != "/app/" || u.RawQuery != "" || !strings.HasPrefix(line, base+"/app/#login=mysl_") {
		t.Fatalf("link %q is not %s/app/#login=<code>", line, base)
	}
	return code
}

func redeem(t *testing.T, base, code string) (status int, session string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, err := http.Post(base+"/api/v1/login-codes/redeem", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Token
}

func getGraph(t *testing.T, base, token string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/v1/graph", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestGraphNeedsWeb(t *testing.T) {
	clearClientEnv(t)
	var out, errb bytes.Buffer
	code := Execute([]string{"graph", "--file", filepath.Join(t.TempDir(), "b.jsonl")}, &out, &errb)
	if code != ExitUsage || !strings.Contains(errb.String(), "graph needs --web") || out.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestGraphWebLocalServesAndStops(t *testing.T) {
	clearClientEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "brain.jsonl")
	var out, errb lockedBuf
	root := newRoot(&out, &errb)
	root.SetArgs([]string{"graph", "--web", "--file", file})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	t.Cleanup(cancel)

	var line string
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if s := out.String(); strings.HasSuffix(s, "\n") {
			line = s
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no link printed; stderr %q", errb.String())
		}
	}
	u, _ := url.Parse(strings.TrimSpace(line))
	base := "http://" + u.Host
	if !strings.HasPrefix(u.Host, "127.0.0.1:") {
		t.Fatalf("not bound to loopback: %s", line)
	}
	code := loginFrom(t, line, base)

	if st := getGraph(t, base, code); st != 401 {
		t.Fatalf("a login code worked as a bearer token: %d", st)
	}
	st, session := redeem(t, base, code)
	if st != 200 || session == "" {
		t.Fatalf("redeem: %d", st)
	}
	if st, _ := redeem(t, base, code); st != 401 {
		t.Fatalf("second redeem: %d", st)
	}
	if st := getGraph(t, base, session); st != 200 {
		t.Fatalf("graph with the session: %d", st)
	}

	// Nothing but the one-time code and the session leaks, and no token is written.
	for _, s := range []string{out.String(), errb.String()} {
		if strings.Contains(s, session) || strings.Contains(s, "Bearer") {
			t.Fatalf("secret in output %q", s)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.json")); err == nil {
		t.Fatal("tokens.json was written")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
	if _, err := http.Get(base + "/healthz"); err == nil {
		t.Fatal("still serving after stop")
	}
}

func TestGraphWebLocalAddrInUseByMyself(t *testing.T) {
	clearClientEnv(t)
	srv, err := server.New(server.Config{File: filepath.Join(t.TempDir(), "b.jsonl"), AdminHash: server.HashToken("x")})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	var out, errb bytes.Buffer
	code := Execute([]string{"graph", "--web", "--addr", u.Host, "--file", filepath.Join(t.TempDir(), "b.jsonl")}, &out, &errb)
	if code != ExitUsage || !strings.Contains(errb.String(), "a myself server is already listening on "+u.Host) || out.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestGraphWebRemote(t *testing.T) {
	clearClientEnv(t)
	const admin = "admin-secret"
	srv, err := server.New(server.Config{File: filepath.Join(t.TempDir(), "b.jsonl"), AdminHash: server.HashToken(admin)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	t.Setenv(EnvURL, ts.URL)
	t.Setenv(EnvToken, admin)

	opened := ""
	old := openURL
	openURL = func(u string) error { opened = u; return nil }
	defer func() { openURL = old }()

	var out, errb bytes.Buffer
	if code := Execute([]string{"graph", "--web", "--open"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	code := loginFrom(t, out.String(), ts.URL)
	if opened != strings.TrimSpace(out.String()) {
		t.Fatalf("opened %q", opened)
	}
	if strings.Contains(out.String()+errb.String(), admin) {
		t.Fatal("the token was printed")
	}
	st, session := redeem(t, ts.URL, code)
	if st != 200 || getGraph(t, ts.URL, session) != 200 {
		t.Fatalf("redeem %d", st)
	}

	// Not a TTY and no --open: the link is printed and nothing is opened.
	opened = ""
	out.Reset()
	if code := Execute([]string{"graph", "--web"}, &out, &errb); code != 0 || opened != "" {
		t.Fatalf("exit %d, opened %q", code, opened)
	}
}

func TestGraphWebRemoteErrors(t *testing.T) {
	clearClientEnv(t)
	old := http.NewServeMux()
	old.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"no endpoint POST /api/v1/login-codes","exit_code":2}`))
	})
	ts := httptest.NewServer(old)
	defer ts.Close()
	t.Setenv(EnvURL, ts.URL)

	var out, errb bytes.Buffer
	if code := Execute([]string{"graph", "--web"}, &out, &errb); code != ExitUsage || !strings.Contains(errb.String(), EnvToken) {
		t.Fatalf("no token: exit %d, stderr %q", code, errb.String())
	}
	t.Setenv(EnvToken, "mys_x")
	errb.Reset()
	if code := Execute([]string{"graph", "--web"}, &out, &errb); code != ExitStore || !strings.Contains(errb.String(), "does not support web login links") || out.Len() != 0 {
		t.Fatalf("old server: exit %d, stderr %q", code, errb.String())
	}
}
