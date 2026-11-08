package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	// The record that lets another command find this server goes with it.
	if _, err := os.Stat(server.LocalInfoPath(file, port(t, u.Host))); err == nil {
		t.Fatal("the record of the server outlived it")
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
	if code != ExitUsage || !strings.Contains(errb.String(), "is recorded as serving") || out.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

// serveRunning starts `myself serve` in this process on a free loopback port
// and waits until it says it is up. It stops with the test.
func serveRunning(t *testing.T, file string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // the command binds the port itself
	t.Setenv(EnvAdminHash, server.HashToken("serve-token"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var out, errb lockedBuf
	root := newRoot(&out, &errb)
	root.SetArgs([]string{"serve", "--addr", addr, "--file", file})
	go root.ExecuteContext(ctx)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if strings.Contains(errb.String(), "serving "+file) {
			return addr
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve did not start; stderr %q", errb.String())
		}
	}
}

// A myself server that is already up is reused instead of fought over: the link
// comes from that server and signs in there, and the server is left running.
func TestGraphWebReusesRunningServer(t *testing.T) {
	clearClientEnv(t)
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	addr := serveRunning(t, file)

	var out, errb bytes.Buffer
	if code := Execute([]string{"graph", "--web", "--addr", addr, "--file", file}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	base := "http://" + addr
	login := loginFrom(t, out.String(), base)
	if !strings.Contains(errb.String(), "already on "+addr) {
		t.Errorf("stderr does not say the server was reused: %q", errb.String())
	}

	// The code is the running server's: it redeems there and opens the graph.
	st, session := redeem(t, base, login)
	if st != 200 || session == "" {
		t.Fatalf("redeem at the running server: %d", st)
	}
	if st := getGraph(t, base, session); st != 200 {
		t.Fatalf("graph with the session: %d", st)
	}

	// Nothing is printed but the link: not the session, not the login key of
	// the server, which sits in a file next to the store.
	info, err := server.ReadLocalInfo(file, port(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{out.String(), errb.String()} {
		if strings.Contains(s, session) || strings.Contains(s, info.Key) || strings.Contains(s, "Bearer") {
			t.Fatalf("secret in output %q", s)
		}
	}
	// The reused server is still up, and the session still works on it.
	if st := getGraph(t, base, session); st != 200 {
		t.Fatalf("the reused server stopped: %d", st)
	}
}

// A server of another store is not reused: a link into a different brain is
// not what --file asked for.
func TestGraphWebReuseOtherStore(t *testing.T) {
	clearClientEnv(t)
	dir := t.TempDir()
	addr := serveRunning(t, filepath.Join(dir, "a.jsonl"))
	var out, errb bytes.Buffer
	code := Execute([]string{"graph", "--web", "--addr", addr, "--file", filepath.Join(dir, "b.jsonl")}, &out, &errb)
	if code != ExitUsage || out.Len() != 0 || !strings.Contains(errb.String(), "serves "+filepath.Join(dir, "a.jsonl")) {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

// A server started without its admin hash has its API switched off, and the
// local login key is no better off than any other caller there.
func TestGraphWebReuseAPIDisabled(t *testing.T) {
	clearClientEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "b.jsonl")
	key, err := server.NewLocalKey()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(server.Config{File: file, LocalKey: key})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	unrecord, err := server.WriteLocalInfo(server.LocalInfo{Store: file, Addr: u.Host, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	defer unrecord()
	var out, errb bytes.Buffer
	code := Execute([]string{"graph", "--web", "--addr", u.Host, "--file", file}, &out, &errb)
	if code != ExitUsage || out.Len() != 0 || !strings.Contains(errb.String(), "API disabled") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

// Port taken by something that is not myself at all stays the plain error it
// was: the user has to free the port or pick another.
func TestGraphWebPortTakenByOther(t *testing.T) {
	clearClientEnv(t)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not myself\n")
	}))
	defer other.Close()
	var out, errb bytes.Buffer
	u, _ := url.Parse(other.URL)
	code := Execute([]string{"graph", "--web", "--addr", u.Host, "--file", filepath.Join(t.TempDir(), "b.jsonl")}, &out, &errb)
	if code != ExitUsage || !strings.Contains(errb.String(), "not a myself server") || out.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

// port is the port of a host:port address.
func port(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
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

// A connection that never sent a request (a browser's spare preconnect, or an
// HTTP client's extra dial) must not hold up shutdown: net/http only counts
// such a connection as idle after 5 seconds.
func TestRunServerStopsWithUnusedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runServer(ctx, http.NewServeMux(), ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Make sure the server has accepted it before stopping.
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil || time.Since(start) > 2*time.Second {
			t.Fatalf("stop: %v after %v", err, time.Since(start))
		}
	case <-time.After(8 * time.Second):
		t.Fatal("did not stop")
	}
}
