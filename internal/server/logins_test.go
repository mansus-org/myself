package server

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// newClockEnv is an admin env whose server clock the test controls.
func newClockEnv(t *testing.T) (*env, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)}
	file := filepath.Join(t.TempDir(), "brain.jsonl")
	srv, err := New(Config{File: file, AdminHash: HashToken(adminToken), Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{t, srv, ts, file, &syncBuffer{}}, clk
}

func (e *env) loginCode(token string) string {
	e.t.Helper()
	var lc loginCodeReply
	e.ok("POST", "/api/v1/login-codes", token, "", 201, &lc)
	if !strings.HasPrefix(lc.Code, loginCodePrefix) || lc.Expires == "" {
		e.t.Fatalf("login code reply %+v", lc)
	}
	return lc.Code
}

func (e *env) redeemBody(code string) string { return `{"code":"` + code + `"}` }

func TestLoginCodeSingleUse(t *testing.T) {
	e, _ := newClockEnv(t)
	code := e.loginCode(adminToken)
	var sr sessionReply
	e.ok("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(code), 200, &sr)
	if !strings.HasPrefix(sr.Token, sessionPrefix) {
		t.Fatalf("session token %q", sr.Token)
	}
	e.fails("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(code), 401, 3, "invalid, used or expired login code")
}

func TestLoginCodeExpires(t *testing.T) {
	e, clk := newClockEnv(t)
	code := e.loginCode(adminToken)
	clk.advance(LoginCodeTTL)
	e.fails("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(code), 401, 3, "invalid, used or expired login code")
	e.fails("POST", "/api/v1/login-codes/redeem", "", e.redeemBody("mysl_nope"), 401, 3, "")
}

func TestLoginSessionKeepsScope(t *testing.T) {
	e, _ := newClockEnv(t)
	_, readTok := e.token("reader", ScopeRead)

	var sr sessionReply
	e.ok("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(e.loginCode(adminToken)), 200, &sr)
	var id Identity
	e.ok("GET", "/api/v1/whoami", sr.Token, "", 200, &id)
	if id != (Identity{Name: AdminName, Scope: ScopeWrite, Admin: true}) || sr.Identity != id {
		t.Fatalf("admin session is %+v / %+v", id, sr.Identity)
	}
	e.ok("GET", "/api/v1/graph", sr.Token, "", 200, nil)
	e.ok("GET", "/api/v1/tokens", sr.Token, "", 200, nil)

	e.ok("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(e.loginCode(readTok)), 200, &sr)
	e.ok("GET", "/api/v1/whoami", sr.Token, "", 200, &id)
	if id != (Identity{Name: "reader", Scope: ScopeRead}) {
		t.Fatalf("read session is %+v", id)
	}
	e.ok("GET", "/api/v1/graph", sr.Token, "", 200, nil)
	e.fails("POST", "/api/v1/remember", sr.Token, seed, 403, 3, "")
	e.fails("GET", "/api/v1/tokens", sr.Token, "", 403, 3, "")
}

func TestLoginSessionExpires(t *testing.T) {
	e, clk := newClockEnv(t)
	var sr sessionReply
	e.ok("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(e.loginCode(adminToken)), 200, &sr)
	clk.advance(SessionTTL - time.Second)
	e.ok("GET", "/api/v1/whoami", sr.Token, "", 200, nil)
	clk.advance(time.Second)
	e.fails("GET", "/api/v1/whoami", sr.Token, "", 401, 3, "invalid or expired session")
}

func TestLoginCodeNeedsToken(t *testing.T) {
	e, _ := newClockEnv(t)
	e.fails("POST", "/api/v1/login-codes", "", "", 401, 3, "")
	e.fails("POST", "/api/v1/login-codes", "mys_wrong", "", 401, 3, "")
	// A code is not a bearer token, and a session can be traded for a new code.
	code := e.loginCode(adminToken)
	e.fails("GET", "/api/v1/whoami", code, "", 401, 3, "")
	var sr sessionReply
	e.ok("POST", "/api/v1/login-codes/redeem", "", e.redeemBody(code), 200, &sr)
	e.loginCode(sr.Token)
}

func TestLoginDisabledWithoutAdminHash(t *testing.T) {
	e := newEnv(t, "")
	e.fails("POST", "/api/v1/login-codes", adminToken, "", 503, 3, "")
	e.fails("POST", "/api/v1/login-codes/redeem", "", `{"code":"x"}`, 503, 3, "")
}

func TestLoginCodesAreBounded(t *testing.T) {
	e, _ := newClockEnv(t)
	for i := 0; i < maxPending; i++ {
		if _, _, err := e.srv.NewLoginCode(Identity{Name: AdminName}); err != nil {
			t.Fatal(err)
		}
	}
	e.fails("POST", "/api/v1/login-codes", adminToken, "", 429, 3, "")
}
