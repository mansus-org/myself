package server

import (
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"testing"
)

func get(t *testing.T, e *env, method, path string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestUI(t *testing.T) {
	e := newAdminEnv(t)

	resp, _ := get(t, e, "GET", "/app", nil)
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/app/" {
		t.Fatalf("/app: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// The index, and client routes that fall back to it.
	for _, p := range []string{"/app/", "/app/index.html", "/app/e/relay", "/app/e/a%2Fb", "/app/graph", "/app/tokens/"} {
		resp, body := get(t, e, "GET", p, nil)
		if resp.StatusCode != 200 || !strings.Contains(body, `<script type="module" src="/app/assets/app.js">`) {
			t.Fatalf("%s: %d %.80q", p, resp.StatusCode, body)
		}
		h := resp.Header
		if h.Get("Content-Type") != "text/html; charset=utf-8" || h.Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s headers: %v", p, h)
		}
		csp := h.Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Fatalf("%s: CSP %q lacks %q", p, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Fatalf("CSP allows inline code: %q", csp)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: no nosniff", p)
		}
	}

	// Assets, with their types and cache headers.
	for _, c := range []struct{ path, ctype, cache string }{
		{"/app/assets/app.js", "text/javascript; charset=utf-8", "no-cache"},
		{"/app/assets/screens/graph.js", "text/javascript; charset=utf-8", "no-cache"},
		{"/app/assets/base.css", "text/css; charset=utf-8", "no-cache"},
		{"/app/assets/fonts/atkinson-next.woff2", "font/woff2", "public, max-age=31536000, immutable"},
	} {
		resp, body := get(t, e, "GET", c.path, nil)
		if resp.StatusCode != 200 || body == "" {
			t.Fatalf("%s: %d", c.path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != c.ctype {
			t.Fatalf("%s: type %q, want %q", c.path, got, c.ctype)
		}
		if got := resp.Header.Get("Cache-Control"); got != c.cache {
			t.Fatalf("%s: cache %q, want %q", c.path, got, c.cache)
		}
		etag := resp.Header.Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", c.path)
		}
		if resp, _ := get(t, e, "GET", c.path, map[string]string{"If-None-Match": etag}); resp.StatusCode != http.StatusNotModified {
			t.Fatalf("%s revalidation: %d", c.path, resp.StatusCode)
		}
	}

	// Every module reachable from app.js by a static relative import is served.
	// importPath accepts import braces over several lines; dynamic import() is not followed.
	importPath := regexp.MustCompile(`(?m)^\s*import\b[^;]*\bfrom\s+"([^"]+)"`)
	seen := map[string]bool{}
	queue := []string{"/app/assets/app.js"}
	for len(queue) > 0 {
		mod := queue[0]
		queue = queue[1:]
		if seen[mod] {
			continue
		}
		seen[mod] = true
		resp, js := get(t, e, "GET", mod, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("module %s: %d", mod, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Fatalf("module %s: type %q", mod, got)
		}
		for _, m := range importPath.FindAllStringSubmatch(js, -1) {
			rel := m[1]
			if !strings.HasPrefix(rel, "./") && !strings.HasPrefix(rel, "../") {
				continue
			}
			queue = append(queue, path.Clean(path.Join(path.Dir(mod), rel)))
		}
	}
	if !seen["/app/assets/screens/graph.js"] || !seen["/app/assets/ui.js"] {
		t.Fatal("the import walk did not follow imports: want app.js -> screens/graph.js and screens -> ui.js")
	}

	// Every stylesheet the index links is served as CSS.
	linkTag := regexp.MustCompile(`<link\s+[^>]*rel="stylesheet"[^>]*>`)
	hrefAttr := regexp.MustCompile(`href="([^"]+)"`)
	_, index := get(t, e, "GET", "/app/", nil)
	tags := linkTag.FindAllString(index, -1)
	if len(tags) == 0 {
		t.Fatal("index: no stylesheet links")
	}
	for _, tag := range tags {
		m := hrefAttr.FindStringSubmatch(tag)
		if m == nil {
			t.Fatalf("stylesheet link without href: %s", tag)
		}
		resp, _ := get(t, e, "GET", m[1], nil)
		if resp.StatusCode != 200 {
			t.Fatalf("stylesheet %s: %d", m[1], resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != "text/css; charset=utf-8" {
			t.Fatalf("stylesheet %s: type %q", m[1], got)
		}
	}

	// A missing asset is a real 404, not the index.
	if resp, body := get(t, e, "GET", "/app/assets/nope.js", nil); resp.StatusCode != 404 || strings.Contains(body, "<script") {
		t.Fatalf("missing asset: %d", resp.StatusCode)
	}
	if resp, _ := get(t, e, "HEAD", "/app/", nil); resp.StatusCode != 200 {
		t.Fatalf("HEAD /app/: %d", resp.StatusCode)
	}
	if resp, _ := get(t, e, "POST", "/app/", nil); resp.StatusCode != 405 {
		t.Fatalf("POST /app/: %d", resp.StatusCode)
	}
	// The landing page and the API are untouched.
	if _, body := get(t, e, "GET", "/", nil); !strings.Contains(body, "<h1>myself</h1>") {
		t.Fatalf("landing: %q", body)
	}
	if resp, _ := get(t, e, "GET", "/api/v1/whoami", nil); resp.StatusCode != 401 {
		t.Fatalf("api: %d", resp.StatusCode)
	}
}
