package services

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type upstreamCounter struct {
	calls  atomic.Int32
	status int
	body   string
}

func (c *upstreamCounter) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return &http.Response{
		StatusCode: c.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(c.body)),
	}, nil
}

func TestTmdbGetImages_Cached(t *testing.T) {
	tr := &upstreamCounter{status: 200, body: `{"posters":[{"file_path":"/a.jpg"}]}`}
	c := NewTmdbClient("k", &http.Client{Transport: tr})
	for i := 0; i < 3; i++ {
		img, err := c.GetImagesCtx(t.Context(), "movie", 278, "en")
		if err != nil || len(img.Posters) != 1 {
			t.Fatalf("GetImages: %v %+v", err, img)
		}
	}
	if _, err := c.GetImagesCtx(t.Context(), "movie", 278, "de"); err != nil {
		t.Fatal(err)
	}
	if n := tr.calls.Load(); n != 2 {
		t.Fatalf("upstream calls = %d, want 2 (one per lang)", n)
	}
}

func TestFanartImages_Cached(t *testing.T) {
	tr := &upstreamCounter{status: 200, body: `{"movieposter":[{"id":"1","url":"u","lang":"en"}]}`}
	c := NewFanartClient([]string{"k"}, &http.Client{Transport: tr})
	for i := 0; i < 3; i++ {
		if _, err := c.GetMovieImagesCtx(t.Context(), 278); err != nil {
			t.Fatal(err)
		}
	}
	if n := tr.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

// Unknown ids (a definite 404) are cached briefly so media servers polling
// ids TMDB doesn't know don't hit /find on every request; transient errors
// are still never cached.
func TestResolveIDCached_NegativeCacheOnlyNotFound(t *testing.T) {
	cache := NewMemCache(0, 100, 0, 0)

	notFound := &upstreamCounter{status: 200, body: `{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`}
	clients := IDClients{TMDB: NewTmdbClient("k", &http.Client{Transport: notFound})}
	for i := 0; i < 3; i++ {
		if _, err := ResolveIDCachedCtx(t.Context(), cache, IDTypeIMDB, "tt0000001", clients); err == nil {
			t.Fatal("want not-found")
		}
	}
	if n := notFound.calls.Load(); n != 1 {
		t.Fatalf("not-found: upstream calls = %d, want 1", n)
	}

	broken := &upstreamCounter{status: 401, body: `{}`} // non-retryable, not a 404
	clients = IDClients{TMDB: NewTmdbClient("k", &http.Client{Transport: broken})}
	for i := 0; i < 2; i++ {
		ResolveIDCachedCtx(t.Context(), cache, IDTypeIMDB, "tt0000002", clients)
	}
	if n := broken.calls.Load(); n < 2 {
		t.Fatalf("transient error must not be cached: upstream calls = %d", n)
	}
}
