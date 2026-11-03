package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A raw "/../x" path must not probe the filesystem outside staticDir: an
// existing and a missing outside file have to get the same answer.
func TestStatic_NoExistenceOracleOutsideRoot(t *testing.T) {
	r := newStaticRouter(t)
	outside := filepath.Join(filepath.Dir(r.staticDir), "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	get := func(p string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = p // as sent with curl --path-as-is
		rec := httptest.NewRecorder()
		r.serveStatic(rec, req)
		return rec.Code, rec.Header().Get("Cache-Control")
	}
	c1, h1 := get("/../outside.txt")
	c2, h2 := get("/../missing.txt")
	if c1 != c2 || h1 != h2 {
		t.Fatalf("existing vs missing outside file differ: %d/%q vs %d/%q", c1, h1, c2, h2)
	}
}
