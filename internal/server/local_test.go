package server

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newLocalEnv is an admin server that also has a local login key, the way
// `myself serve` and `myself graph --web` run.
func newLocalEnv(t *testing.T) (*env, string) {
	t.Helper()
	key, err := NewLocalKey()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	lb := &syncBuffer{}
	srv, err := New(Config{File: file, AdminHash: HashToken(adminToken), LocalKey: key, Log: lb})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{t, srv, ts, file, lb}, key
}

// The local key mints login codes and opens nothing else: it is not a token,
// and only the user who can read the file next to the store has it.
func TestLocalKeyMintsCodesAndNothingElse(t *testing.T) {
	e, key := newLocalEnv(t)

	var lc loginCodeReply
	e.ok("POST", "/api/v1/login-codes", key, "", 201, &lc)
	if !strings.HasPrefix(lc.Code, loginCodePrefix) || lc.Expires == "" {
		t.Fatalf("login code reply %+v", lc)
	}
	var sr sessionReply
	e.ok("POST", "/api/v1/login-codes/redeem", "", `{"code":"`+lc.Code+`"}`, 200, &sr)
	if sr.Name != LocalKeyName || !sr.Admin {
		t.Fatalf("a login made with the local key is %+v", sr.Identity)
	}
	e.ok("GET", "/api/v1/graph", sr.Token, "", 200, nil)

	// The key is not a bearer token for anything else, not even a read.
	e.ok("GET", "/api/v1/graph", key, "", 401, nil)
	e.ok("POST", "/api/v1/remember", key, `{"subject":"a","relation":"is","object":"b"}`, 401, nil)
	// A wrong key is not a key, and no key at all is no key.
	e.ok("POST", "/api/v1/login-codes", key+"0", "", 401, nil)
	e.ok("POST", "/api/v1/login-codes", "", "", 401, nil)

	// The key only works from loopback.
	req := httptest.NewRequest("POST", "/api/v1/login-codes", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.RemoteAddr = "203.0.113.9:52000"
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("the key from another address: %d, body %s", rec.Code, rec.Body)
	}
	// The log names the caller and never carries the key.
	if log := e.log.String(); !strings.Contains(log, "local") || strings.Contains(log, key) {
		t.Fatalf("request log %q", log)
	}
}

// The key of one server is not the key of another, and a key without an API
// gets the same 503 as every other caller.
func TestLocalKeyOff(t *testing.T) {
	_, key := newLocalEnv(t)
	newAdminEnv(t).ok("POST", "/api/v1/login-codes", key, "", 401, nil)

	local, err := NewLocalKey()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{File: filepath.Join(t.TempDir(), "brain.jsonl"), LocalKey: local})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/login-codes", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("the key while the API is off: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/login-codes", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+local)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("the key while the API is off: %d", resp.StatusCode)
	}
}

func TestLocalInfoFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "brain.jsonl")
	if _, err := ReadLocalInfo(file, 8080); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("before: %v", err)
	}
	remove, err := WriteLocalInfo(LocalInfo{Store: file, Addr: "127.0.0.1:8080", PID: 42, Key: "myslk_x"})
	if err != nil {
		t.Fatal(err)
	}
	path := LocalInfoPath(file, 8080)
	if path != filepath.Join(dir, ".myself-server-8080.json") {
		t.Errorf("path %s", path)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode of the record: %v %v", st, err)
	}
	got, err := ReadLocalInfo(file, 8080)
	if err != nil {
		t.Fatal(err)
	}
	if got.Store != file || got.Addr != "127.0.0.1:8080" || got.PID != 42 || got.Key != "myslk_x" || got.Started == "" {
		t.Fatalf("read back %+v", got)
	}
	// A relative path to the same store finds the same record.
	t.Chdir(dir)
	if got, err := ReadLocalInfo("brain.jsonl", 8080); err != nil || got.Key != "myslk_x" {
		t.Fatalf("relative: %+v %v", got, err)
	}
	// Another port is another server, and no record of it here.
	if _, err := ReadLocalInfo(file, 8081); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("other port: %v", err)
	}
	// A server that starts overwrites the record of one that died.
	if _, err := WriteLocalInfo(LocalInfo{Store: file, Addr: ":8080", Key: "myslk_y"}); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLocalInfo(file, 8080); err != nil || got.Key != "myslk_y" {
		t.Fatalf("after a restart: %+v %v", got, err)
	}
	remove()
	if _, err := ReadLocalInfo(file, 8080); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("after remove: %v", err)
	}
}
