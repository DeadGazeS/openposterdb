package services

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func withRatingsRefreshAfter(t *testing.T, d time.Duration) {
	t.Helper()
	old, oldEmpty := ratingsRefreshAfter, emptyRatingsTTL
	ratingsRefreshAfter, emptyRatingsTTL = d, d
	t.Cleanup(func() {
		waitNoRefresh(t) // background refreshes read these vars
		ratingsRefreshAfter, emptyRatingsTTL = old, oldEmpty
	})
}

// waitNoRefresh blocks until no background ratings refresh is running.
func waitNoRefresh(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		busy := false
		ratingsRefreshing.Range(func(any, any) bool { busy = true; return false })
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("background ratings refresh never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitCalls(t *testing.T, s *omdbStub, n int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.calls.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("calls = %d, want %d", s.calls.Load(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	waitNoRefresh(t) // the refresh has stored its result
}

// Past the refresh age a hot title gets its cached badges at once (so a cached
// image never waits on the providers to build its cache key) while one
// background refresh fetches fresh ones for the next request.
func TestFetchRatingsCached_StaleWhileRevalidate(t *testing.T) {
	withRatingsRefreshAfter(t, 20*time.Millisecond)
	stub := &omdbStub{}
	clients := RatingsClients{OMDB: NewOmdbClient([]string{"k"}, &http.Client{Transport: stub})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	q := imdbOnly("tt0000005")

	if got := FetchRatingsCachedCtx(t.Context(), cache, clients, q); len(got) != 1 || got[0].Value != "5.0" {
		t.Fatalf("first fetch: %v", got)
	}
	time.Sleep(30 * time.Millisecond)
	stub.delay = 300 * time.Millisecond
	stub.fixed = "9.9"

	start := time.Now()
	got := FetchRatingsCachedCtx(t.Context(), cache, clients, q)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("stale entry must be served without waiting (took %v)", d)
	}
	if len(got) != 1 || got[0].Value != "5.0" {
		t.Fatalf("stale badges expected, got %v", got)
	}
	FetchRatingsCachedCtx(t.Context(), cache, clients, q) // no second refresh
	waitCalls(t, stub, 2)
	if got := FetchRatingsCachedCtx(t.Context(), cache, clients, q); got[0].Value != "9.9" {
		t.Fatalf("after the refresh: %v", got)
	}
	if n := stub.calls.Load(); n != 2 {
		t.Fatalf("calls = %d, want 2 (one refresh)", n)
	}
}

// A refresh that comes back empty (provider errors are swallowed, so this is
// usually an outage) keeps the previous badges.
func TestFetchRatingsCached_EmptyRefreshKeepsBadges(t *testing.T) {
	withRatingsRefreshAfter(t, 20*time.Millisecond)
	stub := &omdbStub{}
	clients := RatingsClients{OMDB: NewOmdbClient([]string{"k"}, &http.Client{Transport: stub})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	q := imdbOnly("tt0000006")
	FetchRatingsCachedCtx(t.Context(), cache, clients, q)
	time.Sleep(30 * time.Millisecond)
	stub.empty = true
	FetchRatingsCachedCtx(t.Context(), cache, clients, q) // triggers the refresh
	waitCalls(t, stub, 2)
	if got := FetchRatingsCachedCtx(t.Context(), cache, clients, q); len(got) != 1 || got[0].Value != "6.0" {
		t.Fatalf("an empty refresh wiped the badges: %v", got)
	}
}

// The ID cache serves a resolution past its refresh age while one background
// lookup refreshes it (a hot cached image used to wait on TMDB /find hourly);
// a failed refresh keeps the old resolution.
func TestResolveIDCached_StaleWhileRevalidate(t *testing.T) {
	old := idRefreshAfter
	idRefreshAfter = 20 * time.Millisecond
	t.Cleanup(func() { waitNoIDRefresh(t); idRefreshAfter = old })

	tr := &upstreamCounter{status: 200, body: `{"movie_results":[{"id":278,"poster_path":"/a.jpg","popularity":1}],"tv_results":[]}`}
	var slow atomic.Bool
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if slow.Load() {
			time.Sleep(300 * time.Millisecond)
			return (&upstreamCounter{status: 401, body: `{}`}).RoundTrip(r)
		}
		return tr.RoundTrip(r)
	})
	clients := IDClients{TMDB: NewTmdbClient("k", &http.Client{Transport: transport})}
	cache := NewMemCache(0, 100, time.Hour, 0)

	if _, err := ResolveIDCachedCtx(t.Context(), cache, IDTypeIMDB, "tt0111161", clients); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	slow.Store(true) // the refresh is slow and then fails

	start := time.Now()
	r, err := ResolveIDCachedCtx(t.Context(), cache, IDTypeIMDB, "tt0111161", clients)
	if err != nil || r.TMDbID != 278 {
		t.Fatalf("stale resolution expected: %v %v", r, err)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("stale entry must be served without waiting (took %v)", d)
	}
	waitNoIDRefresh(t)
	if r, err := ResolveIDCached(cache, IDTypeIMDB, "tt0111161", clients); err != nil || r.TMDbID != 278 {
		t.Fatalf("a failed refresh must keep the old resolution: %v %v", r, err)
	}
}

func waitNoIDRefresh(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		busy := false
		idRefreshing.Range(func(any, any) bool { busy = true; return false })
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("background id refresh never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
