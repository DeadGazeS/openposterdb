package handlers

import (
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// TestCDNHandler_HashMiss proves unknown/expired hashes produce a clean 404
// (not a mux-route 404, so the user's edge can distinguish "hash expired,
// please refetch authenticated URL" from a generic miss).
func TestCDNHandler_HashMiss(t *testing.T) {
	reg := services.NewHashRegistry(0)
	deps := &ImageDeps{}

	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return *deps }, reg)(w, r)
	})
	req := httptest.NewRequest("GET", "/c/deadbeef/imdb/poster-default/tt999.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hash miss: got %d want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "settings hash") {
		t.Errorf("body should explain the hash miss, got %q", rec.Body.String())
	}
}

// TestCDNHandler_MethodNotAllowed proves OPTIONS/PUT/etc. on the CDN route
// are rejected with 405 (CDN fetches are GET only).
func TestCDNHandler_MethodNotAllowed(t *testing.T) {
	reg := services.NewHashRegistry(0)
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return ImageDeps{} }, reg)(w, r)
	})
	req := httptest.NewRequest("POST", "/c/abc/imdb/poster-default/x.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: got %d want 405", rec.Code)
	}
}

// TestCDNHandler_HeadAllowed proves HEAD reaches the handler (not 405): a
// HEAD probe on the authed URL follows HandleImage's CDN redirect here.
func TestCDNHandler_HeadAllowed(t *testing.T) {
	reg := services.NewHashRegistry(0)
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return ImageDeps{} }, reg)(w, r)
	})
	req := httptest.NewRequest("HEAD", "/c/unknown/imdb/poster-default/x.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("HEAD unknown hash: got %d want 404 (same as GET)", rec.Code)
	}
}

// TestCDNHandler_InvalidID proves the path validator rejects ".." and
// similar traversal attempts (ValidateIDValue).
func TestCDNHandler_InvalidID(t *testing.T) {
	reg := services.NewHashRegistry(0)
	settings := &services.RenderSettings{ImageSource: "t"}
	hash := reg.Register(settings)
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return ImageDeps{} }, reg)(w, r)
	})
	req := httptest.NewRequest("GET", "/c/"+hash+"/imdb/poster-default/..jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal id: got %d want 400", rec.Code)
	}
}

// guard against accidental import creep
var _ = strings.ToUpper
