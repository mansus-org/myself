package server

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// The web UI at /app: static files embedded from web/, no build step. Every
// /app path that is not a file gets index.html, and the app routes itself.
//
//go:embed web
var webFiles embed.FS

// uiCSP allows only the app's own files: no inline script or style, no
// third-party origins. The API is same-origin, so connect-src 'self' covers it.
const uiCSP = "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; manifest-src 'self'; base-uri 'none'; " +
	"form-action 'none'; frame-ancestors 'none'"

// uiTypes are set explicitly: distroless images have no mime.types file.
var uiTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
}

type uiFile struct {
	data  []byte
	ctype string
	etag  string
}

// registerUI serves the web UI at /app and /app/*.
func (s *Server) registerUI() {
	files := loadUI()
	s.mux.HandleFunc("GET /app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /app/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/app/")
		f, ok := files[name]
		if !ok {
			// A missing asset is a 404; anything else is a client route.
			if strings.HasPrefix(name, "assets/") {
				uiHeaders(w)
				http.NotFound(w, r)
				return
			}
			name, f = "index.html", files["index.html"]
		}
		uiHeaders(w)
		h := w.Header()
		h.Set("Content-Type", f.ctype)
		h.Set("ETag", f.etag)
		if strings.HasPrefix(name, "assets/fonts/") {
			// Fonts never change under the same name.
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Revalidate everything else, so a deploy never mixes old and new modules.
			h.Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(f.data))
	})
}

func uiHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", uiCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

// loadUI reads the embedded files into memory with their type and ETag.
// Directory entries are left out, so they fall back to index.html.
func loadUI() map[string]uiFile {
	root, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	files := map[string]uiFile{}
	err = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		ctype, ok := uiTypes[path.Ext(p)]
		if !ok {
			ctype = "application/octet-stream"
		}
		sum := sha256.Sum256(data)
		files[p] = uiFile{data, ctype, `"` + hex.EncodeToString(sum[:8]) + `"`}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return files
}

// registerSkill serves the agent skill at /skill.md, open to anyone, so an
// agent can fetch it from a link.
func (s *Server) registerSkill(skill []byte) {
	s.mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		uiHeaders(w)
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(skill)
	})
}
