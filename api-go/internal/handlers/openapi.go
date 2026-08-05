package handlers

import (
	_ "embed"
	"net/http"

	"openposterdb/internal/httpx"
)

// openapiSpec is the OpenAPI 3.0 description served at /api/openapi.json. It
// is hand-authored (no codegen dependency) and covers every route registered
// in internal/router/routes.go. Update the JSON when adding/removing routes
// rather than relying on annotations.
//
//go:embed static/openapi.json
var openapiSpec []byte

// HandleOpenAPISpec serves the embedded spec. When DISABLE_PUBLIC_PAGES=true
// (configured on AppState), the route is wrapped with admin auth by the
// router, so this handler itself doesn't need to gate.
func HandleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	// Long CDN-friendly cache: a CDN edge can cache this for 24h and serve it
	// to documentation tools without hitting the origin.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}
