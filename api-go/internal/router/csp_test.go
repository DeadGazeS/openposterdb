package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The web UI is served with a CSP that forbids inline/third-party scripts.
// (Checked end to end in a headless browser: landing, docs, login, settings
// and logout render without violations.)
func TestStatic_ContentSecurityPolicy(t *testing.T) {
	r := newStaticRouter(t)
	for _, p := range []string{"/", "/settings", "/assets/app-abc123.js"} {
		rec := httptest.NewRecorder()
		r.serveStatic(rec, httptest.NewRequest(http.MethodGet, p, nil))
		csp := rec.Header().Get("Content-Security-Policy")
		for _, want := range []string{"script-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", p, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
			t.Errorf("%s: CSP must not allow inline/eval scripts: %q", p, csp)
		}
	}
}
