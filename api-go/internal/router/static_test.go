package router

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStaticRouter(t *testing.T) *Router {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>x</title>"+strings.Repeat(" ", 2000)), 0o644))
	must(os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte(strings.Repeat("console.log('hello');\n", 200)), 0o644))
	must(os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte{0, 0, 1, 0}, 0o644))
	return &Router{staticDir: dir, fs: http.FileServer(http.Dir(dir))}
}

func TestStatic_CacheHeaders(t *testing.T) {
	r := newStaticRouter(t)
	for path, want := range map[string]string{
		"/assets/app-abc123.js": "public, max-age=31536000, immutable",
		"/":                     "no-cache",
		"/settings":             "no-cache", // SPA fallback
		"/favicon.ico":          "public, max-age=86400",
	} {
		rec := httptest.NewRecorder()
		r.serveStatic(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, want)
		}
	}
}

func TestStatic_GzipsJS(t *testing.T) {
	r := newStaticRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	rec := httptest.NewRecorder()
	Gzip(http.HandlerFunc(r.serveStatic)).ServeHTTP(rec, req) // as ServeHTTP wires it
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("JS must be gzipped, headers %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil || !strings.HasPrefix(string(b), "console.log") {
		t.Fatalf("bad gzip body: %v %q", err, b[:min(len(b), 40)])
	}
}

// A directory must not produce an http.FileServer listing.
func TestStatic_NoDirectoryListing(t *testing.T) {
	r := newStaticRouter(t)
	rec := httptest.NewRecorder()
	r.serveStatic(rec, httptest.NewRequest(http.MethodGet, "/assets/", nil))
	if strings.Contains(rec.Body.String(), "app-abc123.js") {
		t.Fatal("/assets/ must not list files")
	}
}
