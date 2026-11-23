package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"openposterdb/internal/services"
)

// ?imageSize= is validated as in the Rust version: unknown values and
// "small" for posters/logos are a 400, before the "TMDB not configured" 503
// (so the answer doesn't depend on whether provider keys are set).
func TestHandleImage_ImageSizeValidation(t *testing.T) {
	db := newHandlersTestDB(t)
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKey(db, "test", hash, prefix, "", 1); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	deps := ImageDeps{DB: db, Config: &ImageServeConfig{}} // no TMDB client
	mux := http.NewServeMux()
	mux.Handle("/{apiKey}/{rest...}", HandleImage(func() ImageDeps { return deps }, func() (bool, error) { return false, nil }))

	for _, tc := range []struct {
		path string
		want int
	}{
		{"imdb/poster-default/tt0111161.jpg?imageSize=small", 400},
		{"imdb/poster-default/tt0111161.jpg?imageSize=huge", 400},
		{"imdb/logo-default/tt0111161.png?imageSize=small", 400},
		{"imdb/backdrop-default/tt0111161.jpg?imageSize=huge", 400},
		// Valid sizes get past validation and stop at the missing TMDB key.
		{"imdb/poster-default/tt0111161.jpg?imageSize=large", 503},
		{"imdb/poster-default/tt0111161.jpg?imageSize=verylarge", 503},
		{"imdb/poster-default/tt0111161.jpg?imageSize=medium", 503},
		{"imdb/backdrop-default/tt0111161.jpg?imageSize=small", 503},
		{"tmdb/episode-default/episode-1399-S1E1.jpg?imageSize=small", 503},
		{"imdb/poster-default/tt0111161.jpg", 503},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+raw+"/"+tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}
