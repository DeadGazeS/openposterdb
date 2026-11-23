package services

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureLogs sends slog's default logger to a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	saved := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&syncWriter{w: &buf}, nil)))
	t.Cleanup(func() { slog.SetDefault(saved) })
	return &buf
}

type syncWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// failureLines returns the "rating provider failed" log lines.
func failureLines(buf *bytes.Buffer) []string {
	var out []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, "rating provider failed") {
			out = append(out, l)
		}
	}
	return out
}

// A title MDBList doesn't know (404) has no MDBList ratings; that is not a
// provider failure (TMDB and Trakt treat their 404 the same way).
func TestMdblist_NotFoundIsNoRating(t *testing.T) {
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) {
		return 404, nil, `{"error":"Not found"}`
	}}
	c := NewMdblistClient([]string{"k"}, &http.Client{Transport: stub})
	r, err := c.GetRatingsCtx(t.Context(), "tt1", "movie")
	if err != nil || r != nil {
		t.Fatalf("got %+v %v, want no ratings and no error", r, err)
	}

	buf := captureLogs(t)
	res := FetchRatingsCtx(t.Context(), RatingsClients{MDBList: c}, imdbOnly("tt2"))
	if res.Attempted != 1 || res.Failed != 0 || res.AllFailed() {
		t.Errorf("attempted %d failed %d, want 1/0", res.Attempted, res.Failed)
	}
	if l := failureLines(buf); len(l) != 0 {
		t.Errorf("logged %v", l)
	}
}

// 503 with a Retry-After beyond the retry policy blocks the key like a 429.
func TestMDBList_503LongRetryAfterBlocksKey(t *testing.T) {
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) {
		return 503, http.Header{"Retry-After": []string{"60"}}, `{}`
	}}
	c := NewMdblistClient([]string{"only"}, &http.Client{Transport: stub})
	if _, err := c.GetRatingsCtx(t.Context(), "tt1", "movie"); err == nil {
		t.Fatal("want an error")
	}
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
	if until := c.KeyPool.BlockedUntil(); time.Until(until) < 55*time.Second {
		t.Errorf("blocked until %v, want ~60 s from now", until)
	}
}

// A 503 without Retry-After is a plain failure: the key is not blocked.
func TestMDBList_503WithoutRetryAfterDoesNotBlock(t *testing.T) {
	saved := MDBListRetry
	MDBListRetry.MaxRetries = 0
	t.Cleanup(func() { MDBListRetry = saved })
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) { return 503, nil, `{}` }}
	c := NewMdblistClient([]string{"only"}, &http.Client{Transport: stub})
	if _, err := c.GetRatingsCtx(t.Context(), "tt1", "movie"); err == nil {
		t.Fatal("want an error")
	}
	if _, ok := c.KeyPool.ActiveKey(); !ok {
		t.Error("key must not be blocked")
	}
}

// The failure log names the key that made the failed call, not the one the
// pool rotated to afterwards.
func TestFetchRatings_FailureLogNamesKeyUsed(t *testing.T) {
	saved := MDBListRetry
	MDBListRetry.MaxRetries = 0
	t.Cleanup(func() { MDBListRetry = saved })
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) { return 429, nil, `{}` }}
	c := NewMdblistClient([]string{"first", "second"}, &http.Client{Transport: stub})
	buf := captureLogs(t)
	res := FetchRatingsCtx(t.Context(), RatingsClients{MDBList: c}, imdbOnly("tt1"))
	if res.Failed != 1 {
		t.Fatalf("failed = %d", res.Failed)
	}
	// "first" was limited, then "second"; the pool is back on "first".
	if c.KeyPool.ActiveKeyHash() != KeyHash("first") {
		t.Fatalf("setup: active key is not the first one")
	}
	lines := failureLines(buf)
	if len(lines) != 1 || !strings.Contains(lines[0], "key="+KeyHash("second")) {
		t.Errorf("failure log %v, want key=%s", lines, KeyHash("second"))
	}
}

// On the Trakt episode path a failed TMDB lookup of the show's IMDb id is a
// TMDB failure, not a Trakt one under the Trakt key.
func TestFetchRatings_TraktEpisodeLookupFailureIsTMDB(t *testing.T) {
	stub := &respStub{handler: func(req *http.Request) (int, http.Header, string) {
		if strings.Contains(req.URL.Path, "/external_ids") {
			return 400, nil, `{"status_message":"bad"}`
		}
		return 404, nil, `{}`
	}}
	hc := &http.Client{Transport: stub}
	clients := RatingsClients{TMDB: NewTmdbClient("k", hc), Trakt: NewTraktClient([]string{"t"}, hc)}
	buf := captureLogs(t)
	FetchRatingsCtx(t.Context(), clients, RatingsQuery{MediaType: "episode", EpisodeShowTMDbID: 99001, EpisodeSeason: 1, EpisodeEpisode: 1})
	lines := failureLines(buf)
	if len(lines) != 1 {
		t.Fatalf("failure log %v, want one line", lines)
	}
	if !strings.Contains(lines[0], "provider=tmdb") || strings.Contains(lines[0], "key=") {
		t.Errorf("failure log %q, want provider=tmdb without a Trakt key", lines[0])
	}
}

// Without a score the Letterboxd badge comes from the value, whose scale
// depends on the endpoint: 0–5 on the single GET, 0–10 on the batch POST.
func TestMdblistBadges_LetterboxdValueScaleByEndpoint(t *testing.T) {
	stub := &respStub{handler: func(req *http.Request) (int, http.Header, string) {
		if req.Method == http.MethodPost {
			return 200, nil, `[{"ids":{"imdb":"tt1"},"ratings":[{"source":"letterboxd","value":8.4}]},{"ids":{"imdb":"tt2"},"ratings":[{"source":"letterboxd","value":8.4}]}]`
		}
		return 200, nil, `{"ids":{"imdb":"tt3"},"ratings":[{"source":"letterboxd","value":4.2}]}`
	}}
	c := NewMdblistClient([]string{"k"}, &http.Client{Transport: stub})
	got := make([]string, 2)
	var wg sync.WaitGroup
	for i, id := range []string{"tt1", "tt2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, err := c.GetRatingsCtx(t.Context(), id, "movie"); err == nil && r != nil {
				got[i] = badgeKeys(MdblistBadges(r))
			}
		}()
	}
	wg.Wait()
	if got[0] != "lb=4.2" || got[1] != "lb=4.2" {
		t.Errorf("batch: %q", got)
	}
	r, err := c.GetRatingsCtx(t.Context(), "tt3", "movie")
	if err != nil || badgeKeys(MdblistBadges(r)) != "lb=4.2" {
		t.Errorf("single: %v %v", r, err)
	}
}
