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
		"/t0-free-rpdb/isValid": true,
		// Any other key with an image or isValid path shape reaches the API
		// too, so an unknown key is a JSON 401 instead of the SPA's index.html
		// (media servers would otherwise get HTML with a 200).
		"/t0-free-rpdbx/isValid":                              true,
		"/invalid-key-here/imdb/poster-default/tt0111161.jpg": true,
		"/somekey/tmdb/poster-default/movie-550.jpg":          true,
		"/somekey/tvdb/logo-default/81189.png":                true,
		"/somekey/kitsu/poster-default/1.jpg":                 true,
		"/somekey/mal/poster-default/1.jpg":                   true,
		"/somekey/imdb":                                       true,
		// SPA routes stay with the SPA.
		"/settings":                  false,
		"/admin/keys":                false,
		"/admin/settings/image":      false,
		"/admin/posters":             false,
		"/individual-image-settings": false,
		"/key-settings":              false,
		"/docs":                      false,
		"/legal":                     false,
		"/does/not/exist":            false,
		"/assets/app.js":             false,
		"/assets/imdb/x.js":          false,
		"/":                          false,
	}
	for p, want := range cases {
		if got := isAPIPath(p); got != want {
			t.Errorf("isAPIPath(%q) = %v, want %v", p, got, want)
		}
	}
}
