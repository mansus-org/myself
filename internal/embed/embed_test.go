package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fake is a stand-in for an OpenAI-compatible /embeddings endpoint. It records
// every request and replies with whatever the test queues, so the client is
// tested on what it actually sends.
type fake struct {
	t        *testing.T
	requests []embedRequest
	auth     []string
	replies  []string
	status   int
}

func newFake(t *testing.T) (*fake, *Client) {
	t.Helper()
	f := &fake{t: t, status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var req embedRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("client sent unreadable JSON: %v", err)
		}
		f.requests = append(f.requests, req)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		reply := f.vectorReply(len(req.Input))
		if len(f.replies) > 0 {
			reply = f.replies[0]
			f.replies = f.replies[1:]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	cl := New(Config{URL: srv.URL, Model: "test-model"})
	return f, cl
}

// vectorReply answers with one vector per text, each vector a fingerprint of
// the text's position so a test can see whether the order survived.
func (f *fake) vectorReply(n int) string {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"index":%d,"embedding":[%d.5,%d.25]}`, i, i, i)
	}
	b.WriteString(`],"model":"test-model"}`)
	return b.String()
}

func TestFromEnv(t *testing.T) {
	if _, ok, err := FromEnv(func(string) string { return "" }); ok || err != nil {
		t.Fatalf("unset means no embeddings: ok=%v err=%v", ok, err)
	}
	cfg, ok, err := FromEnv(func(k string) string {
		switch k {
		case EnvURL:
			return " https://api.openai.com/v1/embeddings "
		case EnvModel:
			return "text-embedding-3-small"
		}
		return ""
	})
	if !ok || err != nil {
		t.Fatalf("expected the endpoint selected: %v", err)
	}
	if cfg.URL != "https://api.openai.com/v1/embeddings" || cfg.Model != "text-embedding-3-small" {
		t.Fatalf("bad config: %+v", cfg)
	}
	if cfg.Timeout != DefaultTimeout || cfg.Dimensions != 0 {
		t.Fatalf("bad defaults: %+v", cfg)
	}
	// An endpoint with no model is a misconfiguration, not "no embeddings":
	// the silent version asks a model nobody chose what the brain means.
	if _, _, err := FromEnv(func(k string) string {
		if k == EnvURL {
			return "http://x/v1/embeddings"
		}
		return ""
	}); err == nil || !strings.Contains(err.Error(), EnvModel) {
		t.Fatalf("missing model: want an error naming %s, got %v", EnvModel, err)
	}
	for _, bad := range []struct{ key, value string }{
		{EnvURL, "embeddings://x"},
		{EnvDimensions, "wide"},
		{EnvDimensions, "0"},
		{EnvTimeout, "soon"},
	} {
		if _, _, err := FromEnv(func(k string) string {
			if k == bad.key {
				return bad.value
			}
			if k == EnvURL {
				return "http://x/v1/embeddings"
			}
			if k == EnvModel {
				return "m"
			}
			return ""
		}); err == nil || !strings.Contains(err.Error(), bad.key) {
			t.Errorf("%s=%q: want an error naming %s, got %v", bad.key, bad.value, bad.key, err)
		}
	}
	if cfg, _, err := FromEnv(func(k string) string {
		switch k {
		case EnvURL:
			return "http://x/v1/embeddings"
		case EnvModel:
			return "m"
		case EnvDimensions:
			return "8"
		case EnvTimeout:
			return "2.5"
		}
		return ""
	}); err != nil || cfg.Dimensions != 8 || cfg.Timeout.Seconds() != 2.5 {
		t.Fatalf("dimensions and timeout not read: %+v %v", cfg, err)
	}
}

func TestEmbedSendsModelAndKeyAndKeepsOrder(t *testing.T) {
	f, cl := newFake(t)
	cl.cfg.Key = "sk-secret"
	got, err := cl.Embed(context.Background(), "relay is project", "relay uses bun", "bun is runtime")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 {
		t.Fatalf("want one request, got %d", len(f.requests))
	}
	if f.requests[0].Model != "test-model" {
		t.Fatalf("model not sent: %+v", f.requests[0])
	}
	if strings.Join(f.requests[0].Input, "|") != "relay is project|relay uses bun|bun is runtime" {
		t.Fatalf("input not sent as given: %+v", f.requests[0].Input)
	}
	if f.auth[0] != "Bearer sk-secret" {
		t.Fatalf("key not sent as a bearer token: %q", f.auth[0])
	}
	// The fake's vector for text i is [i.5, i.25], so the order is visible.
	want := []string{"[0.5 0.25]", "[1.5 1.25]", "[2.5 2.25]"}
	for i, v := range got {
		if fmt.Sprint(v) != want[i] {
			t.Fatalf("vector %d: got %v, want %s", i, v, want[i])
		}
	}
	if cl.Dimensions() != 2 {
		t.Fatalf("dimensions not learned: %d", cl.Dimensions())
	}
}

func TestEmbedWithoutAKeySendsNoAuthHeader(t *testing.T) {
	f, cl := newFake(t)
	if _, err := cl.Embed(context.Background(), "relay is project"); err != nil {
		t.Fatal(err)
	}
	if f.auth[0] != "" {
		t.Fatalf("an endpoint with no key must get no Authorization header, got %q", f.auth[0])
	}
}

func TestEmbedSplitsALongListIntoRequests(t *testing.T) {
	f, cl := newFake(t)
	texts := make([]string, 150)
	for i := range texts {
		texts[i] = fmt.Sprintf("fact %d", i)
	}
	got, err := cl.Embed(context.Background(), texts...)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 3 {
		t.Fatalf("150 texts: want 3 requests of %d, got %d", maxTexts, len(f.requests))
	}
	if len(got) != 150 {
		t.Fatalf("want 150 vectors, got %d", len(got))
	}
	if got[0][0] != 0.5 || got[64][0] != 0.5 || got[65][0] != 1.5 {
		t.Fatalf("order lost across requests: %v %v %v", got[0], got[64], got[65])
	}
}

func TestEmbedOrdersAReplyThatCameOutOfOrder(t *testing.T) {
	// The API documents `index` as which text a vector belongs to, and not every
	// OpenAI-compatible server answers in the order it was asked. Sorting on it
	// is what keeps Embed's promise of one vector per text, in order — get it
	// wrong and a fact silently gets another fact's vector.
	f, cl := newFake(t)
	f.replies = []string{`{"data":[{"index":2,"embedding":[2,2]},{"index":0,"embedding":[0,0]},{"index":1,"embedding":[1,1]}]}`}
	got, err := cl.Embed(context.Background(), "a is b", "c is d", "e is f")
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range got {
		if v[0] != float32(i) {
			t.Fatalf("vector %d is %v: the reply's index column must decide the order", i, v)
		}
	}
}

func TestEmbedRefusesAReplyThatDoesNotMatch(t *testing.T) {
	t.Run("too few vectors", func(t *testing.T) {
		f, cl := newFake(t)
		f.replies = []string{`{"data":[{"index":0,"embedding":[1,2]}]}`}
		if _, err := cl.Embed(context.Background(), "a is b", "c is d"); err == nil {
			t.Fatal("a reply with fewer vectors than texts must be an error")
		}
	})
	t.Run("empty vector", func(t *testing.T) {
		f, cl := newFake(t)
		f.replies = []string{`{"data":[{"index":0,"embedding":[]}]}`}
		if _, err := cl.Embed(context.Background(), "a is b"); err == nil {
			t.Fatal("an empty vector must be an error")
		}
	})
	t.Run("dimensions change", func(t *testing.T) {
		f, cl := newFake(t)
		if _, err := cl.Embed(context.Background(), "a is b"); err != nil {
			t.Fatal(err)
		}
		f.replies = []string{`{"data":[{"index":0,"embedding":[1,2,3]}]}`}
		if _, err := cl.Embed(context.Background(), "c is d"); err == nil || !strings.Contains(err.Error(), "dimensions") {
			t.Fatalf("an endpoint that changes the vector length must be refused, got %v", err)
		}
	})
	t.Run("empty text", func(t *testing.T) {
		_, cl := newFake(t)
		if _, err := cl.Embed(context.Background(), "a is b", "   "); err == nil {
			t.Fatal("an empty text has no vector and must be refused before the request")
		}
	})
}

func TestEmbedReportsWhatTheEndpointSaid(t *testing.T) {
	t.Run("an api error body", func(t *testing.T) {
		f, cl := newFake(t)
		f.status = http.StatusUnauthorized
		f.replies = []string{`{"error":{"message":"Incorrect API key provided","code":"invalid_api_key"}}`}
		_, err := cl.Embed(context.Background(), "a is b")
		if err == nil || !strings.Contains(err.Error(), "Incorrect API key provided") {
			t.Fatalf("the endpoint's own message must reach the user, got %v", err)
		}
	})
	t.Run("html from a proxy", func(t *testing.T) {
		f, cl := newFake(t)
		f.status = http.StatusBadGateway
		f.replies = []string{"<html>\ngateway\n</html>"}
		_, err := cl.Embed(context.Background(), "a is b")
		if err == nil || !strings.Contains(err.Error(), "502") {
			t.Fatalf("want the status in the error, got %v", err)
		}
	})
}

func TestEmbedHonoursTheContext(t *testing.T) {
	_, cl := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cl.Embed(ctx, "a is b"); err == nil {
		t.Fatal("a cancelled context must stop the request")
	}
}

func TestConfigStringNeverCarriesTheKey(t *testing.T) {
	cfg := Config{URL: "https://user:hunter2@api.example/v1/embeddings", Model: "m", Key: "sk-secret"}
	s := cfg.String()
	if strings.Contains(s, "hunter2") || strings.Contains(s, "sk-secret") {
		t.Fatalf("the key leaked into a log line: %s", s)
	}
	if !strings.Contains(s, "m") || !strings.Contains(s, "api.example") {
		t.Fatalf("the line should say which model and endpoint: %s", s)
	}
}

func TestTextIsTheSentenceOfAFact(t *testing.T) {
	if got := Text("relay", "uses", "bun"); got != "relay uses bun" {
		t.Fatalf("got %q", got)
	}
	if got := Text("relay", "is", "project"); got != "relay is project" {
		t.Fatalf("got %q", got)
	}
}
