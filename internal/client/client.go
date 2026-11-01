// Package client is the HTTP client of the REST API (SPEC section 10). Each
// method returns the brain result struct of the matching command, or a
// *brain.ExitError carrying the server's message and exit code, so the CLI
// renders remote results exactly like local ones.
//
// The token is only ever sent in the Authorization header. It is not part of
// any error message.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

// DefaultTimeout bounds one request when Client.Timeout is zero.
const DefaultTimeout = 10 * time.Second

// maxResponse caps what the client reads from the server.
const maxResponse = 256 << 20

// Client talks to one myself server.
type Client struct {
	// URL is the server's base URL, e.g. https://myself.mansus.org.
	URL string
	// Token is the bearer token; empty sends no Authorization header.
	Token string
	// Timeout bounds one request; zero means DefaultTimeout.
	Timeout time.Duration
	// Warn receives the "warnings" of a response (the text after "warning: ").
	Warn func(msg string)
	// HTTP is the transport; nil means a plain http.Client.
	HTTP *http.Client
}

// Identity is the caller, as GET /api/v1/whoami returns it.
type Identity struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Admin bool   `json:"admin"`
}

// ParseURL checks and normalizes a server URL: http or https, with a host,
// no query or fragment, no trailing slash.
func ParseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", brain.Usagef("invalid server URL %q: want http://host[:port] or https://host[:port]", redact(raw))
	}
	u.User = nil
	return strings.TrimRight(u.String(), "/"), nil
}

// redact drops any user:password from a URL for messages.
func redact(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		return u.Redacted()
	}
	return raw
}

// Whoami returns the caller's identity.
func (c *Client) Whoami() (Identity, error) {
	var out Identity
	err := c.get("/api/v1/whoami", nil, &out)
	return out, err
}

// Remember posts the facts in one all-or-nothing request.
func (c *Client) Remember(facts []brain.Fact, replace bool) (brain.RememberResult, error) {
	var out brain.RememberResult
	err := c.post("/api/v1/remember", struct {
		Facts   []brain.Fact `json:"facts"`
		Replace bool         `json:"replace"`
	}{facts, replace}, &out)
	return out, err
}

// Forget retracts by uuid, or by subject and relation (narrowed by object).
func (c *Client) Forget(a brain.ForgetArgs) (brain.ForgetResult, error) {
	req := struct {
		UUID     string  `json:"uuid,omitempty"`
		Subject  string  `json:"subject,omitempty"`
		Relation string  `json:"relation,omitempty"`
		Object   *string `json:"object,omitempty"`
		Reason   string  `json:"reason,omitempty"`
	}{a.UUID, a.Subject, a.Relation, a.Object, a.Reason}
	var out brain.ForgetResult
	err := c.post("/api/v1/forget", req, &out)
	return out, err
}

func (c *Client) Describe(entity string, depth int, at *string) (brain.DescribeResult, error) {
	q := url.Values{}
	q.Set("depth", strconv.Itoa(depth))
	setOpt(q, "at", at)
	var out brain.DescribeResult
	err := c.get("/api/v1/describe/"+url.PathEscape(entity), q, &out)
	return out, err
}

func (c *Client) Ls(typ, at *string) (brain.LsResult, error) {
	q := url.Values{}
	setOpt(q, "type", typ)
	setOpt(q, "at", at)
	var out brain.LsResult
	err := c.get("/api/v1/ls", q, &out)
	return out, err
}

func (c *Client) Relations() (brain.RelationsResult, error) {
	var out brain.RelationsResult
	err := c.get("/api/v1/relations", nil, &out)
	return out, err
}

func (c *Client) Search(text string, at *string) (brain.SearchResult, error) {
	q := url.Values{}
	q.Set("q", text)
	setOpt(q, "at", at)
	var out brain.SearchResult
	err := c.get("/api/v1/search", q, &out)
	return out, err
}

func (c *Client) Path(a, b string, maxDepth int) (brain.PathResult, error) {
	q := url.Values{}
	q.Set("from", a)
	q.Set("to", b)
	q.Set("max_depth", strconv.Itoa(maxDepth))
	var out brain.PathResult
	err := c.get("/api/v1/path", q, &out)
	return out, err
}

func (c *Client) Log(since *string, n *int) (brain.RecordsResult, error) {
	q := url.Values{}
	setOpt(q, "since", since)
	if n != nil {
		q.Set("n", strconv.Itoa(*n))
	}
	var out brain.RecordsResult
	err := c.get("/api/v1/log", q, &out)
	return out, err
}

func (c *Client) History(subject string, relation *string) (brain.RecordsResult, error) {
	q := url.Values{}
	setOpt(q, "relation", relation)
	var out brain.RecordsResult
	err := c.get("/api/v1/history/"+url.PathEscape(subject), q, &out)
	return out, err
}

func (c *Client) Prime(budget int) (brain.PrimeResult, error) {
	q := url.Values{}
	q.Set("budget", strconv.Itoa(budget))
	var out brain.PrimeResult
	err := c.get("/api/v1/prime", q, &out)
	return out, err
}

func (c *Client) Dream() (brain.DreamResult, error) {
	var out brain.DreamResult
	err := c.get("/api/v1/dream", nil, &out)
	return out, err
}

// Records downloads the whole log (export.jsonl), uuid-sorted.
func (c *Client) Records() ([]model.Record, error) {
	body, err := c.do(http.MethodGet, "/api/v1/export.jsonl", nil, nil)
	if err != nil {
		return nil, err
	}
	var warn bytes.Buffer
	recs, err := store.Read(bytes.NewReader(body), &warn)
	if err != nil {
		return nil, brain.StoreErr(fmt.Errorf("reading the export from %s: %w", c.URL, err))
	}
	return recs, nil
}

func setOpt(q url.Values, name string, v *string) {
	if v != nil {
		q.Set(name, *v)
	}
}

func (c *Client) get(path string, q url.Values, out any) error {
	body, err := c.do(http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return c.decode(body, out)
}

func (c *Client) post(path string, req, out any) error {
	b, err := model.Marshal(req)
	if err != nil {
		return brain.StoreErr(err)
	}
	body, err := c.do(http.MethodPost, path, nil, b)
	if err != nil {
		return err
	}
	return c.decode(body, out)
}

// decode fills out from a success body and passes its warnings on.
func (c *Client) decode(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return c.notMyself()
	}
	var w struct {
		Warnings []string `json:"warnings"`
	}
	if json.Unmarshal(body, &w) == nil && c.Warn != nil {
		for _, m := range w.Warnings {
			c.Warn(m)
		}
	}
	return nil
}

func (c *Client) notMyself() error {
	return brain.StoreErr(fmt.Errorf("unexpected response from %s: not a myself server?", redact(c.URL)))
}

// do sends one request and returns the body of a 200 or 201 answer. Any
// other status becomes the server's error, a connection failure becomes
// "cannot reach". A GET is retried once if the connection failed.
func (c *Client) do(method, path string, q url.Values, payload []byte) ([]byte, error) {
	target := c.URL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	attempts := 1
	if method == http.MethodGet {
		attempts = 2
	}
	var err error
	for i := 0; i < attempts; i++ {
		var body []byte
		var retry bool
		if body, retry, err = c.once(method, target, payload); err == nil || !retry {
			return body, err
		}
	}
	return nil, err
}

// once makes one request. retry reports a connection failure that is worth
// trying again (not a timeout, which would only double the wait).
func (c *Client) once(method, target string, payload []byte) (body []byte, retry bool, err error) {
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, target, rd)
	if err != nil {
		return nil, false, brain.Usagef("invalid server URL %q", redact(c.URL))
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	hc2 := *hc
	hc2.Timeout = c.Timeout
	if hc2.Timeout <= 0 {
		hc2.Timeout = DefaultTimeout
	}
	resp, err := hc2.Do(req)
	if err != nil {
		var ue *url.Error
		reason := err
		if errors.As(err, &ue) {
			reason = ue.Err
		}
		timeout := ue != nil && ue.Timeout()
		return nil, !timeout, brain.StoreErr(fmt.Errorf("cannot reach %s: %v", redact(c.URL), reason))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, true, brain.StoreErr(fmt.Errorf("cannot reach %s: %v", redact(c.URL), err))
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return data, false, nil
	}
	return nil, false, c.serverError(resp.StatusCode, data)
}

// serverError turns an error answer into an ExitError with the server's
// message and exit code. Auth statuses always exit 3.
func (c *Client) serverError(status int, data []byte) error {
	var eb struct {
		Error    string `json:"error"`
		ExitCode int    `json:"exit_code"`
	}
	if json.Unmarshal(data, &eb) != nil || eb.Error == "" {
		return brain.StoreErr(fmt.Errorf("%s answered %d %s: not a myself server?", redact(c.URL), status, http.StatusText(status)))
	}
	code := eb.ExitCode
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable:
		code = brain.ExitStore
	}
	if code < brain.ExitUsage || code > brain.ExitStore {
		code = brain.ExitStore
	}
	return &brain.ExitError{Code: code, Err: errors.New(eb.Error)}
}
