package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_DisabledWhenOriginEmpty(t *testing.T) {
	h := CORS("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "ok")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header when origin empty, got %q", got)
	}
}

func TestCORS_AllowedOriginEchoed(t *testing.T) {
	const origin = "http://localhost:5173"
	h := CORS(origin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", origin)
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Allow-Origin: got %q want %q", got, origin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials: got %q want true", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary: got %q want Origin", got)
	}
}

func TestCORS_DeniedOriginNoHeaders(t *testing.T) {
	const configured = "http://app.example"
	h := CORS(configured)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://attacker.example")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("denied origin must not get Allow-Origin, got %q", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	const origin = "http://localhost:5173"
	called := false
	h := CORS(origin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true // next handler must NOT run for a preflight
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/status", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("preflight must short-circuit before next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status: got %d want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("preflight missing Allow-Methods")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("preflight missing Allow-Headers")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("preflight Max-Age: got %q want 86400", got)
	}
}

func TestCORS_NonPreflightOPTIONSBypassesMiddleware(t *testing.T) {
	// OPTIONS without Access-Control-Request-Method is not a CORS preflight;
	// the request should fall through to the next handler.
	const origin = "http://localhost:5173"
	called := false
	h := CORS(origin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/some/route", nil)
	req.Header.Set("Origin", origin)
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("non-preflight OPTIONS must reach the next handler")
	}
}

func TestIsCORSOriginAllowed(t *testing.T) {
	cases := []struct {
		configured, request string
		want                bool
	}{
		{"", "", false},
		{"", "http://x", false},
		{"http://x", "", false},
		{"http://x", "http://x", true},
		{"http://X", "HTTP://X", true}, // case-insensitive
		{"http://x", "http://y", false},
	}
	for _, c := range cases {
		if got := isCORSOriginAllowed(c.configured, c.request); got != c.want {
			t.Errorf("isCORSOriginAllowed(%q,%q)=%v want %v", c.configured, c.request, got, c.want)
		}
	}
}
