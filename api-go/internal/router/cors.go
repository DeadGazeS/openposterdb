package router

import (
	"net/http"
	"strings"
)

// CORS returns a middleware that sets Access-Control-Allow-Origin and related
// headers so a browser on a different origin can call the API.
//
// If `origin` is empty, the middleware is a pass-through (same-origin is the
// only supported access pattern). The matching is exact: the request's Origin
// header must equal `origin` byte-for-byte; otherwise no CORS headers are
// returned and the browser will block the request (same as having no
// middleware at all).
//
// This keeps the implementation small and safe: we never echo back a `*` (which
// the browser would refuse to combine with credentials), and we never allow
// arbitrary Origin values.
func CORS(origin string) func(http.Handler) http.Handler {
	if origin == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	allowedMethods := "GET, POST, PUT, DELETE, OPTIONS"
	allowedHeaders := "Content-Type, Authorization, X-Requested-With"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Origin") == origin &&
				r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isCORSOriginAllowed reports whether the given configured origin matches the
// incoming Origin header. Used by callers that need to apply per-response
// Access-Control-Allow-Origin (e.g. streaming endpoints that write headers
// before the middleware runs).
func isCORSOriginAllowed(configured, requestOrigin string) bool {
	if configured == "" || requestOrigin == "" {
		return false
	}
	return strings.EqualFold(configured, requestOrigin)
}
