// Package server is `myself serve` (SPEC section 10): the landing page at /,
// /healthz, the REST API at /api/v1 and MCP at /mcp over the same brain
// functions as the CLI, and the web app at /app.
package server

import (
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

// Config configures a Server.
type Config struct {
	File      string           // the brain file; tokens.json goes next to it
	Site      fs.FS            // served at /; nil serves nothing there
	AdminHash string           // lowercase hex sha256 of the admin token; empty disables the API
	Log       io.Writer        // request log; nil discards
	Now       func() time.Time // nil means time.Now
	Version   string           // reported to MCP clients; empty means "dev"
	Skill     []byte           // served at GET /skill.md; nil serves nothing there
}

// Server is the HTTP server. It is an http.Handler.
type Server struct {
	mux       *http.ServeMux
	cache     *cache
	tokens    *Tokens
	adminHash string
	log       *log.Logger
	now       func() time.Time
	version   string
	writeMu   sync.Mutex
}

// New builds the server and registers /, /healthz, /api/v1, /mcp and /app.
func New(cfg Config) (*Server, error) {
	tokens, err := OpenTokens(filepath.Join(filepath.Dir(cfg.File), "tokens.json"))
	if err != nil {
		return nil, err
	}
	out := cfg.Log
	if out == nil {
		out = io.Discard
	}
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	s := &Server{
		mux:       http.NewServeMux(),
		cache:     &cache{path: cfg.File},
		tokens:    tokens,
		adminHash: cfg.AdminHash,
		log:       log.New(out, "", log.LstdFlags|log.LUTC),
		now:       now,
		version:   version,
	}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})
	if cfg.Skill != nil {
		s.registerSkill(cfg.Skill)
	}
	if cfg.Site != nil {
		// No method in the pattern, so /api/, /mcp and /app/ never conflict
		// with it; other methods are refused here instead.
		files := http.FileServerFS(cfg.Site)
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			files.ServeHTTP(w, r)
		})
	}
	s.registerAPI()
	s.registerMCP()
	s.registerUI()
	return s, nil
}

// Handle registers h on the server's mux (Go 1.22 pattern syntax). Every
// request is logged; wrap h with Auth to require a token.
func (s *Server) Handle(pattern string, h http.Handler) { s.mux.Handle(pattern, h) }

// Store returns a brain.Store for one request, backed by the shared cache.
func (s *Server) Store() *RequestStore { return &RequestStore{c: s.cache} }

// LockWrites serializes writers within this process, so a remember
// --replace or forget computes its forgets from the state it appends to.
// Hold it around brain.Remember, brain.Forget and anything else that reads
// then writes.
func (s *Server) LockWrites() (unlock func()) {
	s.writeMu.Lock()
	return s.writeMu.Unlock
}

// Tokens returns the agent token set.
func (s *Server) Tokens() *Tokens { return s.tokens }

type ctxKey int

const (
	identityKey ctxKey = iota
	logKey
)

// reqLog is filled in by Auth so the request log line can name the token.
type reqLog struct{ name string }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// ServeHTTP logs one line per request: method, path (no query), status,
// duration and token name ("-" without one). Tokens are never logged.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rl := &reqLog{name: "-"}
	sw := &statusWriter{ResponseWriter: w}
	s.mux.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), logKey, rl)))
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	s.log.Printf("%s %s %d %s %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond), rl.name)
}
