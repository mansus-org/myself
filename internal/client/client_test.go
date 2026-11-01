package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mansus-org/myself/internal/brain"
)

// flaky closes the first n connections without answering.
func flaky(n int32, h http.Handler) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= n {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		h.ServeHTTP(w, r)
	}))
	return ts, &calls
}

func TestGetRetriesOnceOnConnectionError(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"relations":["is"]}`))
	})
	ts, calls := flaky(1, ok)
	defer ts.Close()
	res, err := (&Client{URL: ts.URL}).Relations()
	if err != nil || len(res.Relations) != 1 || calls.Load() != 2 {
		t.Fatalf("%v %v calls=%d", res, err, calls.Load())
	}

	ts2, calls2 := flaky(2, ok)
	defer ts2.Close()
	_, err = (&Client{URL: ts2.URL}).Relations()
	if brain.Code(err) != brain.ExitStore || !strings.HasPrefix(err.Error(), "cannot reach "+ts2.URL+": ") || calls2.Load() != 2 {
		t.Fatalf("%v calls=%d", err, calls2.Load())
	}
}

func TestPostIsNotRetried(t *testing.T) {
	ts, calls := flaky(1, http.NotFoundHandler())
	defer ts.Close()
	_, err := (&Client{URL: ts.URL}).Remember([]brain.Fact{{Subject: "a", Relation: "is", Object: "b"}}, false)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("%v calls=%d", err, calls.Load())
	}
}

func TestWarningsAndErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"invalid or revoked token","exit_code":3}`))
			return
		}
		if r.URL.Path == "/api/v1/dream" {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"nothing here","exit_code":2}`))
			return
		}
		w.Write([]byte(`{"entity":"x","at":null,"entities":[],"warnings":["line 2: corrupt, skipped"]}`))
	}))
	defer ts.Close()
	var warned []string
	c := &Client{URL: ts.URL, Token: "tok", Warn: func(m string) { warned = append(warned, m) }}
	if _, err := c.Describe("x", 0, nil); err != nil || len(warned) != 1 || warned[0] != "line 2: corrupt, skipped" {
		t.Fatalf("%v %v", err, warned)
	}
	if _, err := c.Dream(); brain.Code(err) != brain.ExitNotFound || err.Error() != "nothing here" {
		t.Fatalf("%v", err)
	}
	_, err := (&Client{URL: ts.URL, Token: "wrong"}).Dream()
	if brain.Code(err) != brain.ExitStore || err.Error() != "invalid or revoked token" || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("%v", err)
	}
}

func TestParseURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://myself.mansus.org/":  "https://myself.mansus.org",
		" http://localhost:8080 ":     "http://localhost:8080",
		"https://h/base//":            "https://h/base",
		"https://user:pw@example.org": "https://example.org",
	} {
		if got, err := ParseURL(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "myself.mansus.org", "ftp://x", "https://", "https://x?a=b"} {
		_, err := ParseURL(bad)
		var ee *brain.ExitError
		if !errors.As(err, &ee) || ee.Code != brain.ExitUsage {
			t.Errorf("%q: %v", bad, err)
		}
	}
	_, err := ParseURL("ftp://user:hunter2@x")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("password leaked: %v", err)
	}
}
