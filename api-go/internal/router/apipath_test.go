package router

import "testing"

// The public free key is not 64 hex characters; with the SPA served
// (STATIC_DIR, always set in the Docker image) its image and isValid URLs
// must still reach the API instead of the SPA's index.html.
func TestIsAPIPath(t *testing.T) {
	cases := map[string]bool{
		"/api/auth/login": true,
		"/api":            true,
		"/c/abc":          true,
		"/" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" + "/imdb/poster-default/tt1.jpg": true,
		"/t0-free-rpdb/imdb/poster-default/tt0111161.jpg":                                                         true,
		"/t0-free-rpdb/isValid":  true,
		"/t0-free-rpdbx/isValid": false,
		"/settings":              false,
		"/assets/app.js":         false,
		"/":                      false,
	}
	for p, want := range cases {
		if got := isAPIPath(p); got != want {
			t.Errorf("isAPIPath(%q) = %v, want %v", p, got, want)
		}
	}
}
