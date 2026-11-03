package services

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// omdbStub answers OMDb with an imdbRating derived from the requested id
// (tt0000001 -> "1.0", ...) after delay, counting calls.
type omdbStub struct {
	calls atomic.Int32
	delay time.Duration
	empty bool
}

func (s *omdbStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	body := `{"Response":"False"}`
	if !s.empty {
		id := req.URL.Query().Get("i")
		body = `{"imdbRating":"` + id[len(id)-1:] + `.0","Ratings":[]}`
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func imdbOnly(id string) RatingsQuery { return RatingsQuery{MediaType: "movie", IMDbID: &id} }

// Titles without a TMDB id used to share the cache key "0/movie", so every
// IMDb-only title (anime movies TMDB doesn't know) got the first one's ratings.
func TestFetchRatingsCached_IMDbOnlyTitlesDoNotCollide(t *testing.T) {
	stub := &omdbStub{}
	clients := RatingsClients{OMDB: NewOmdbClient([]string{"k"}, &http.Client{Transport: stub})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	a := FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000001"))
	b := FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000002"))
	if len(a) != 1 || len(b) != 1 || a[0].Value == b[0].Value {
		t.Fatalf("IMDb-only titles share ratings: %v vs %v", a, b)
	}
}

func TestFetchRatingsCached_ConcurrentCallsShareOneFetch(t *testing.T) {
	stub := &omdbStub{delay: 50 * time.Millisecond}
	clients := RatingsClients{OMDB: NewOmdbClient([]string{"k"}, &http.Client{Transport: stub})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000003")); len(got) != 1 {
				t.Errorf("got %v", got)
			}
		}()
	}
	wg.Wait()
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("OMDb calls = %d, want 1", n)
	}
}

// An empty badge list (every provider failed, or genuinely no ratings) is only
// cached briefly, so an upstream outage doesn't pin badge-less posters for
// the full ratings TTL.
func TestFetchRatingsCached_EmptyResultShortTTL(t *testing.T) {
	old := emptyRatingsTTL
	emptyRatingsTTL = 30 * time.Millisecond
	defer func() { emptyRatingsTTL = old }()

	stub := &omdbStub{empty: true}
	clients := RatingsClients{OMDB: NewOmdbClient([]string{"k"}, &http.Client{Transport: stub})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000004"))
	FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000004"))
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("within the empty TTL: calls = %d, want 1", n)
	}
	time.Sleep(40 * time.Millisecond)
	stub.empty = false
	if got := FetchRatingsCachedCtx(t.Context(), cache, clients, imdbOnly("tt0000004")); len(got) != 1 {
		t.Fatalf("after the empty TTL the ratings must be refetched, got %v", got)
	}
}

func TestResolveIDCached_ConcurrentCallsShareOneLookup(t *testing.T) {
	tr := &upstreamCounter{status: 200, body: `{"movie_results":[{"id":278,"poster_path":"/a.jpg","popularity":1}],"tv_results":[]}`}
	slow := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		time.Sleep(50 * time.Millisecond)
		return tr.RoundTrip(r)
	})
	clients := IDClients{TMDB: NewTmdbClient("k", &http.Client{Transport: slow})}
	cache := NewMemCache(0, 100, time.Hour, 0)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ResolveIDCachedCtx(t.Context(), cache, IDTypeIMDB, "tt0111161", clients); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := tr.calls.Load(); n != 1 {
		t.Fatalf("TMDB /find calls = %d, want 1", n)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
