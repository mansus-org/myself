package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bearer adds the Authorization header to every request.
type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) mcpClient(token string) (*mcp.ClientSession, error) {
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	return c.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             e.ts.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearer{token}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
}

func (e *env) session(token string) *mcp.ClientSession {
	e.t.Helper()
	cs, err := e.mcpClient(token)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and returns the text, the structured result and isError.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, map[string]any, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}
	var structured map[string]any
	if res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, &structured); err != nil {
			t.Fatalf("%s: structuredContent %s: %v", name, b, err)
		}
	}
	return text, structured, res.IsError
}

func TestMCPToolsAndInstructions(t *testing.T) {
	e := newAdminEnv(t)
	cs := e.session(adminToken)

	init := cs.InitializeResult()
	if init.ServerInfo.Name != "myself" || !strings.Contains(init.Instructions, "long-term memory") {
		t.Errorf("initialize: %+v", init)
	}
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tl := range tools.Tools {
		got[tl.Name] = true
		if tl.Description == "" {
			t.Errorf("tool %s has no description", tl.Name)
		}
	}
	for _, name := range strings.Fields("remember forget describe ls relations search path log history prime dream") {
		if !got[name] {
			t.Errorf("missing tool %s (have %v)", name, got)
		}
	}
	if len(got) != 11 {
		t.Errorf("%d tools, want 11", len(got))
	}
}

func TestMCPCallEveryTool(t *testing.T) {
	e := newAdminEnv(t)
	cs := e.session(adminToken)

	text, v, isErr := call(t, cs, "remember", map[string]any{"facts": []map[string]any{
		{"subject": "relay", "relation": "is", "object": "project"},
		{"subject": "relay", "relation": "uses", "object": "go"},
		{"subject": "go", "relation": "is", "object": "language"},
	}})
	if isErr || strings.Count(text, "remembered ") != 3 || len(v["remembered"].([]any)) != 3 {
		t.Fatalf("remember: %q %v %v", text, v, isErr)
	}

	// A sentence after "is" warns, in the text and in the structured result.
	text, v, _ = call(t, cs, "remember", map[string]any{"facts": []map[string]any{
		{"subject": "relay", "relation": "is", "object": "a micro social network for friends"},
	}})
	if !strings.Contains(text, "warning: ") || v["warnings"] == nil {
		t.Errorf("sentence warning missing: %q %v", text, v)
	}

	text, v, isErr = call(t, cs, "describe", map[string]any{"entity": "relay"})
	if isErr || !strings.HasPrefix(text, "relay\n") || !strings.Contains(text, "uses go") || v["entity"] != "relay" {
		t.Errorf("describe: %q %v", text, v)
	}
	text, _, _ = call(t, cs, "describe", map[string]any{"entity": "go", "depth": 1})
	if !strings.Contains(text, "language") {
		t.Errorf("describe depth: %q", text)
	}

	text, v, _ = call(t, cs, "ls", nil)
	if !strings.Contains(text, "project") || v["entities"] == nil {
		t.Errorf("ls: %q %v", text, v)
	}
	text, _, _ = call(t, cs, "ls", map[string]any{"type": "project"})
	if strings.TrimSpace(text) != "relay" {
		t.Errorf("ls project: %q", text)
	}

	text, v, _ = call(t, cs, "relations", nil)
	if !strings.Contains(text, "prefers") || v["relations"] == nil {
		t.Errorf("relations: %q %v", text, v)
	}

	text, v, _ = call(t, cs, "search", map[string]any{"q": "LANGUAGE"})
	if !strings.Contains(text, "go is language") || v["matches"] == nil {
		t.Errorf("search: %q %v", text, v)
	}

	text, v, _ = call(t, cs, "path", map[string]any{"from": "relay", "to": "language"})
	if text != "relay uses go\ngo is language" || v["path"] == nil {
		t.Errorf("path: %q %v", text, v)
	}

	text, v, _ = call(t, cs, "log", map[string]any{"n": 2})
	if len(strings.Split(text, "\n")) != 2 || len(v["records"].([]any)) != 2 {
		t.Errorf("log: %q %v", text, v)
	}
	text, _, _ = call(t, cs, "log", map[string]any{"since": "2999-01-01"})
	if text != "(none)" {
		t.Errorf("log since: %q", text)
	}

	text, _, _ = call(t, cs, "history", map[string]any{"subject": "relay", "relation": "uses"})
	if !strings.Contains(text, "relay uses go") {
		t.Errorf("history: %q", text)
	}

	text, v, _ = call(t, cs, "prime", map[string]any{"budget": 500})
	if !strings.Contains(text, "relay") || v["text"] != text {
		t.Errorf("prime: %q %v", text, v)
	}

	_, v, isErr = call(t, cs, "dream", nil)
	if isErr || v["near_duplicates"] == nil {
		t.Errorf("dream: %v", v)
	}

	text, _, isErr = call(t, cs, "remember", map[string]any{
		"facts":   []map[string]any{{"subject": "relay", "relation": "uses", "object": "rust"}},
		"replace": true,
	})
	if isErr || !strings.HasPrefix(text, "forgot ") {
		t.Errorf("remember replace: %q", text)
	}

	text, v, isErr = call(t, cs, "forget", map[string]any{"subject": "relay", "relation": "uses", "object": "rust", "reason": "wrong"})
	if isErr || !strings.HasPrefix(text, "forgot ") || len(v["forgotten"].([]any)) != 1 {
		t.Errorf("forget: %q %v %v", text, v, isErr)
	}
	// The forget shows in the log of the same store the REST API reads.
	if status, body := e.do("GET", "/api/v1/history/relay?relation=uses", adminToken, ""); status != 200 || !strings.Contains(body, "wrong") {
		t.Errorf("REST history: %d %s", status, body)
	}
}

func TestMCPToolErrors(t *testing.T) {
	e := newAdminEnv(t)
	cs := e.session(adminToken)

	for _, c := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"describe", map[string]any{"entity": "nobody"}, "nobody"},
		{"describe", map[string]any{"entity": "x", "at": "yesterday-ish"}, "--at: bad time"},
		{"describe", map[string]any{"entity": "x", "depth": -1}, "--depth must not be negative"},
		{"remember", map[string]any{"facts": []map[string]any{{"subject": "a", "relation": "built-with", "object": "b"}}}, `unknown relation "built-with"`},
		{"remember", map[string]any{"facts": []map[string]any{}}, "remember needs at least one fact"},
		{"forget", map[string]any{}, "forget needs"},
		{"forget", map[string]any{"subject": "ghost", "relation": "has"}, "ghost"},
		{"search", map[string]any{"q": ""}, "search needs <text>"},
		{"path", map[string]any{"from": "a", "to": ""}, "path needs <a> and <b>"},
		{"prime", map[string]any{"budget": -3}, "--budget must be at least 1"},
	} {
		text, _, isErr := call(t, cs, c.tool, c.args)
		if !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s %v: isError=%v text %q, want %q", c.tool, c.args, isErr, text, c.want)
		}
	}
}

func TestMCPAuth(t *testing.T) {
	e := newAdminEnv(t)
	_, reader := e.token("reader", "read")
	_, writer := e.token("writer", "write")

	// No token, a bad token and a missing admin hash never reach the tools.
	for _, tok := range []string{"", "mys_" + strings.Repeat("0", 64)} {
		if _, err := e.mcpClient(tok); err == nil {
			t.Errorf("token %q connected", tok)
		}
		status, body := e.do("POST", "/mcp", tok, `{}`)
		if status != 401 || !strings.Contains(body, `"exit_code":3`) {
			t.Errorf("token %q: %d %s", tok, status, body)
		}
	}
	off := newEnv(t, "")
	if status, _ := off.do("POST", "/mcp", adminToken, `{}`); status != 503 {
		t.Errorf("disabled: status %d, want 503", status)
	}

	// A read token lists everything, reads, and is refused on write tools.
	cs := e.session(reader)
	if text, _, isErr := call(t, cs, "relations", nil); isErr || !strings.Contains(text, "uses") {
		t.Errorf("read token, relations: %q %v", text, isErr)
	}
	if _, _, isErr := call(t, cs, "prime", nil); isErr {
		t.Error("read token cannot prime")
	}
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"remember", map[string]any{"facts": []map[string]any{{"subject": "a", "relation": "is", "object": "b"}}}},
		{"forget", map[string]any{"subject": "a", "relation": "is"}},
	} {
		text, _, isErr := call(t, cs, c.tool, c.args)
		if !isErr || !strings.Contains(text, "token reader is read-only") || !strings.Contains(text, "write token") {
			t.Errorf("read token, %s: isError=%v %q", c.tool, isErr, text)
		}
	}
	if status, body := e.do("GET", "/api/v1/log", adminToken, ""); status != 200 || strings.TrimSpace(body) != `{"records":[]}` {
		t.Errorf("read token wrote: %d %s", status, body)
	}

	// A write token writes, and the read token sees it.
	ws := e.session(writer)
	if _, _, isErr := call(t, ws, "remember", map[string]any{"facts": []map[string]any{{"subject": "relay", "relation": "is", "object": "project"}}}); isErr {
		t.Fatal("write token cannot remember")
	}
	if text, _, _ := call(t, cs, "describe", map[string]any{"entity": "relay"}); !strings.Contains(text, "is project") {
		t.Errorf("read after write: %q", text)
	}
}

func TestMCPResourceAndStateless(t *testing.T) {
	e := newAdminEnv(t)
	cs := e.session(adminToken)
	call(t, cs, "remember", map[string]any{"facts": []map[string]any{{"subject": "self", "relation": "is", "object": "agent"}}})

	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "myself://prime"})
	if err != nil || len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, "self") {
		t.Fatalf("prime resource: %v %+v", err, res)
	}

	// No session id is issued, so a second client (a restarted pod, another
	// replica) works with no state from the first.
	if status, _ := e.do("GET", "/mcp", adminToken, ""); status != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp: status %d, want 405", status)
	}
	cs2 := e.session(adminToken)
	if text, _, _ := call(t, cs2, "describe", map[string]any{"entity": "self"}); !strings.Contains(text, "is agent") {
		t.Errorf("second client: %q", text)
	}
}
