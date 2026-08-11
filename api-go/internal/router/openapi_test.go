package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPIRouteReturns200WithCachedJSON(t *testing.T) {
	r := testRouter(t)
	req := httptest.NewRequest("GET", "/api/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/openapi.json: got %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control should include max-age, got %q", cc)
	}

	var doc struct {
		OpenAPI string                     `json:"openapi"`
		Info    struct{ Title string }     `json:"info"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not valid JSON: %v\nbody=%q (len=%d)", err, rec.Body.String(), rec.Body.Len())
	}
	if doc.OpenAPI != "3.0.3" {
		t.Fatalf("openapi version: got %q want 3.0.3", doc.OpenAPI)
	}
	if _, ok := doc.Paths["/api/openapi.json"]; !ok {
		t.Fatalf("/api/openapi.json path missing from spec")
	}
	// A couple of sanity probes — the most-touched endpoints must be
	// documented so tools like /api/key/me/preview show up.
	for _, p := range []string{
		"/api/auth/status",
		"/api/keys",
		"/api/admin/settings",
		"/api/key/me/preview/{kind}",
		"/{apiKey}/{rest}",
		"/c/{hash}/{rest}",
	} {
		if _, ok := doc.Paths[p]; !ok {
			t.Errorf("spec missing path %s", p)
		}
	}
}

func TestOpenAPIRouteMethodNotAllowed(t *testing.T) {
	r := testRouter(t)
	req := httptest.NewRequest("POST", "/api/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/openapi.json: got %d want 405", rec.Code)
	}
}
