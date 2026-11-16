// Package embed turns text into vectors with an OpenAI-compatible embeddings
// endpoint. It is what lets `myself recall` answer by meaning instead of by
// substring (SPEC section 4, "Backends", and section 5).
//
// Nothing in this package runs unless the environment names an endpoint, so the
// default brain keeps the promise it always made: no network, no model, one
// answer per input. The endpoint is configured by the environment and never by
// a flag, for the same reason the Neo4j password is — a key on a command line
// shows in `ps`.
//
// The endpoint is OpenAI-compatible on purpose: one shape covers OpenAI,
// Ollama's /v1/embeddings, LM Studio, vLLM, Jina, OpenRouter and the rest, and
// swapping models stays a configuration change rather than a code change.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Environment variables selecting the embedding endpoint. The endpoint is on
// when EnvURL is non-empty; a URL without a model is a misconfiguration, never
// a silent "no embeddings".
const (
	EnvURL        = "MYSELF_EMBED_URL"        // e.g. https://api.openai.com/v1/embeddings
	EnvModel      = "MYSELF_EMBED_MODEL"      // e.g. text-embedding-3-small, nomic-embed-text
	EnvKey        = "MYSELF_EMBED_API_KEY"    // bearer token; empty sends none (Ollama, vLLM)
	EnvDimensions = "MYSELF_EMBED_DIMENSIONS" // optional; otherwise learned from the first reply
	EnvTimeout    = "MYSELF_EMBED_TIMEOUT"    // seconds or a Go duration; default 30s

	DefaultTimeout = 30 * time.Second

	// maxTexts is how many texts one request carries. The caller sees one
	// call; a large index is filled in requests of this size so a request never
	// carries a whole brain of vectors at once.
	maxTexts = 64
)

// Config is a resolved embedding endpoint.
type Config struct {
	URL        string
	Model      string
	Key        string
	Dimensions int
	Timeout    time.Duration
}

// FromEnv reads the embedding settings. ok is false when no endpoint is
// selected, in which case recall is unavailable and every other command is
// unchanged.
func FromEnv(getenv func(string) string) (cfg Config, ok bool, err error) {
	endpoint := strings.TrimSpace(getenv(EnvURL))
	if endpoint == "" {
		return Config{}, false, nil
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return Config{}, false, fmt.Errorf("%s %q: want an http(s) URL of an OpenAI-compatible /embeddings endpoint", EnvURL, endpoint)
	}
	cfg = Config{URL: endpoint, Model: strings.TrimSpace(getenv(EnvModel)), Key: strings.TrimSpace(getenv(EnvKey)), Timeout: DefaultTimeout}
	if cfg.Model == "" {
		return Config{}, false, fmt.Errorf("%s is not set; %s selects an embedding endpoint, which needs a model: %w", EnvModel, EnvURL, ErrNoModel)
	}
	if s := strings.TrimSpace(getenv(EnvDimensions)); s != "" {
		n, cerr := strconv.Atoi(s)
		if cerr != nil || n < 1 {
			return Config{}, false, fmt.Errorf("%s %q: want a whole number of dimensions", EnvDimensions, s)
		}
		cfg.Dimensions = n
	}
	if s := strings.TrimSpace(getenv(EnvTimeout)); s != "" {
		if d, perr := time.ParseDuration(s); perr == nil && d > 0 {
			cfg.Timeout = d
		} else if f, perr := strconv.ParseFloat(s, 64); perr == nil && f > 0 {
			cfg.Timeout = time.Duration(f * float64(time.Second))
		} else {
			return Config{}, false, fmt.Errorf("%s %q: want seconds (30) or a duration (500ms, 1m)", EnvTimeout, s)
		}
	}
	return cfg, true, nil
}

// String names the endpoint for logs and startup lines. It never carries the
// key, not even the one someone might have put in the URL's userinfo.
func (c Config) String() string {
	target := c.URL
	if u, perr := url.Parse(target); perr == nil && u.User != nil {
		u.User = nil
		target = u.String()
	}
	return fmt.Sprintf("embeddings %s at %s", c.Model, target)
}

// Embedder is what the rest of the program needs from an embedding endpoint:
// vectors for texts, and the name of the model that made them. A test answers
// it with a fixed vector instead of a network.
type Embedder interface {
	// Embed returns one vector per text, in order.
	Embed(ctx context.Context, texts ...string) ([][]float32, error)
	// Model names the model the vectors came from. A vector is only comparable
	// with vectors from the same model, so the name travels with them.
	Model() string
	// Dimensions is the length of the vectors, or 0 until the first reply.
	Dimensions() int
}

// Client is an Embedder over one OpenAI-compatible /embeddings endpoint. It is
// safe for concurrent use.
type Client struct {
	cfg    Config
	client *http.Client

	mu   sync.Mutex
	dims int
}

// New returns a Client for cfg. Nothing is sent until the first Embed.
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{cfg: cfg, client: &http.Client{Timeout: timeout}, dims: cfg.Dimensions}
}

// Model names the configured model.
func (c *Client) Model() string { return c.cfg.Model }

// Dimensions is the vector length, or 0 while still unknown.
func (c *Client) Dimensions() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dims
}

// String names the endpoint without the key.
func (c *Client) String() string { return c.cfg.String() }

// Embed returns one vector per text, in the order given. It splits a long list
// into requests of maxTexts texts, so a caller never sends a whole brain in one
// POST, and it refuses a reply that does not answer every text.
func (c *Client) Embed(ctx context.Context, texts ...string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	for _, t := range texts {
		if strings.TrimSpace(t) == "" {
			return nil, errors.New("embed: an empty text has no vector")
		}
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxTexts {
		end := min(start+maxTexts, len(texts))
		batch, err := c.embed(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedReply struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// embed sends one request and returns its vectors in the order of the texts.
func (c *Client) embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Model: c.cfg.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Key)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	var reply embedReply
	if jerr := json.Unmarshal(raw, &reply); jerr != nil {
		return nil, fmt.Errorf("%s: unreadable reply (HTTP %d): %v", c, res.StatusCode, firstLine(string(raw)))
	}
	if reply.Error != nil && reply.Error.Message != "" {
		return nil, fmt.Errorf("%s: %s", c, reply.Error.Message)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", c, res.StatusCode, firstLine(string(raw)))
	}
	if len(reply.Data) != len(texts) {
		return nil, fmt.Errorf("%s: answered %d vectors for %d texts", c, len(reply.Data), len(texts))
	}
	// The API documents `index` as the position of the text the vector belongs
	// to, and some servers answer out of order. Sorting on it is what keeps
	// Embed's promise of "one vector per text, in order".
	sort.SliceStable(reply.Data, func(i, j int) bool { return reply.Data[i].Index < reply.Data[j].Index })
	out := make([][]float32, 0, len(reply.Data))
	for i, d := range reply.Data {
		if d.Index != i {
			return nil, fmt.Errorf("%s: indexes out of order or missing: got %d at %d", c, d.Index, i)
		}
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("%s: empty vector for text %d", c, i)
		}
		if err := c.checkDims(len(d.Embedding)); err != nil {
			return nil, err
		}
		out = append(out, d.Embedding)
	}
	return out, nil
}

// checkDims records the vector length on the first reply and refuses a change
// afterwards: vectors of two lengths can never be compared, and mixing them in
// one index is the mistake that must not happen quietly.
func (c *Client) checkDims(n int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dims == 0 {
		c.dims = n
		return nil
	}
	if c.dims != n {
		return fmt.Errorf("%s: returns %d dimensions, but %d were seen before: a brain holds one model's vectors at a time", c, n, c.dims)
	}
	return nil
}

// Text is the sentence a fact becomes: `subject relation object`, the same words
// `describe` prints, so a recall and a read of the fact agree on what the fact
// says.
func Text(subject, relation, object string) string {
	return strings.TrimSpace(subject + " " + relation + " " + object)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ErrNoModel is returned by FromEnv when an endpoint is set without a model, so
// a caller can tell "no endpoint" from "endpoint misconfigured".
var ErrNoModel = errors.New("embed: model not set")
