package image

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// fallbackStub routes TMDB API calls to canned results so resolveWithFallback
// can be exercised in isolation. The behaviour is per-path:
//   - /find/{id}?external_source=imdb_id → success when id starts with "tt",
//     empty results otherwise (so strict imdb calls fail for non-imdb ids).
//   - /find/{id}?external_source=tvdb_id → success when id is pure numeric,
//     empty otherwise.
//   - /tv/{N} → success for any N (series format).
//   - /movie/{N} → success for any N (movie format).
// Anything else → empty results, so unspecified cases look like a miss.
type fallbackStub struct{}

func (fallbackStub) RoundTrip(req *http.Request) (*http.Response, error) {
	q := req.URL.Query()
	// TmdbClient prefixes every path with /3, so /find/{id} arrives as
	// /3/find/{id}, /tv/{n} as /3/tv/{n}, /movie/{n} as /3/movie/{n}.
	path := req.URL.Path
	switch {
	case strings.Contains(path, "/find/"):
		idx := strings.Index(path, "/find/")
		id := path[idx+len("/find/"):]
		ext := q.Get("external_source")
		switch ext {
		case "imdb_id":
			if strings.HasPrefix(id, "tt") {
				return jsonResp(`{"movie_results":[{"id":278,"poster_path":"/abc.jpg","release_date":null,"popularity":100.0}],"tv_results":[],"tv_episode_results":[]}`), nil
			}
		case "tvdb_id":
			if isAllDigits(id) {
				return jsonResp(`{"movie_results":[],"tv_results":[{"id":1396,"poster_path":"/abc.jpg","first_air_date":null,"popularity":100.0}],"tv_episode_results":[]}`), nil
			}
		}
		return jsonResp(`{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`), nil
	case strings.Contains(path, "/tv/") || strings.Contains(path, "/movie/"):
		return jsonResp(`{"imdb_id":"tt9999999","poster_path":"/abc.jpg","release_date":null,"first_air_date":null,"external_ids":{"imdb_id":"tt9999999"}}`), nil
	}
	return jsonResp(`{}`), nil
}

func jsonResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       readCloser{bytes.NewReader([]byte(body))},
	}
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func newResolveTestTMDB(t *testing.T) *services.TmdbClient {
	t.Helper()
	return services.NewTmdbClient("test-key", &http.Client{Transport: fallbackStub{}})
}

// TestResolveWithFallback_StrictHit verifies a strict-source success skips
// the fallback chain entirely.
func TestResolveWithFallback_StrictHit(t *testing.T) {
	tmdb := newResolveTestTMDB(t)
	resolved, err := resolveWithFallback(context.Background(), nil, services.IDTypeIMDB, "tt1234567", services.IDClients{TMDB: tmdb}, true, true)
	if err != nil {
		t.Fatalf("strict imdb hit failed: %v", err)
	}
	if resolved == nil || resolved.TMDbID != 278 {
		t.Errorf("resolved = %+v, want TMDbID=278", resolved)
	}
}

// TestResolveWithFallback_TMDBSourceSeries verifies the user's example:
// /imdb/poster-default/series-124364.jpg — strict imdb fails (no tt prefix),
// fallback tmdb succeeds via the series- prefix → /tv/124364.
func TestResolveWithFallback_TMDBSourceSeries(t *testing.T) {
	tmdb := newResolveTestTMDB(t)
	resolved, err := resolveWithFallback(context.Background(), nil, services.IDTypeIMDB, "series-124364", services.IDClients{TMDB: tmdb}, true, true)
	if err != nil {
		t.Fatalf("fallback to tmdb failed: %v", err)
	}
	if resolved == nil || resolved.TMDbID == 0 {
		t.Errorf("resolved = %+v, want non-zero TMDbID", resolved)
	}
}

// TestResolveWithFallback_IMDBSourceFromTMDB verifies the inverse direction:
// /tmdb/poster-default/tt1234567.jpg — strict tmdb fails (needs movie-/
// series-/episode- prefix), fallback imdb succeeds via /find/tt... with
// external_source=imdb_id.
func TestResolveWithFallback_IMDBSourceFromTMDB(t *testing.T) {
	tmdb := newResolveTestTMDB(t)
	resolved, err := resolveWithFallback(context.Background(), nil, services.IDTypeTMDB, "tt1234567", services.IDClients{TMDB: tmdb}, true, true)
	if err != nil {
		t.Fatalf("fallback to imdb failed: %v", err)
	}
	if resolved == nil || resolved.TMDbID != 278 {
		t.Errorf("resolved = %+v, want TMDbID=278 from imdb /find", resolved)
	}
}

// TestResolveWithFallback_TVDBSourceBareNumeric verifies a bare numeric id
// falls through imdb + tmdb (both reject the format) and hits tvdb via
// /find/{id}?external_source=tvdb_id.
func TestResolveWithFallback_TVDBSourceBareNumeric(t *testing.T) {
	tmdb := newResolveTestTMDB(t)
	resolved, err := resolveWithFallback(context.Background(), nil, services.IDTypeIMDB, "81189", services.IDClients{TMDB: tmdb}, true, true)
	if err != nil {
		t.Fatalf("fallback to tvdb failed: %v", err)
	}
	if resolved == nil || resolved.TMDbID != 1396 {
		t.Errorf("resolved = %+v, want TMDbID=1396 from tvdb /find", resolved)
	}
}

// TestResolveWithFallback_AllMiss verifies that when no source resolves
// the idValue, the helper returns the strict error (the most informative
// for the original request) and never an empty ResolvedID.
func TestResolveWithFallback_AllMiss(t *testing.T) {
	tmdb := newResolveTestTMDB(t)
	resolved, err := resolveWithFallback(context.Background(), nil, services.IDTypeIMDB, "garbage", services.IDClients{TMDB: tmdb}, true, true)
	if err == nil {
		t.Fatalf("expected error, got resolved = %+v", resolved)
	}
	if resolved != nil {
		t.Errorf("expected nil resolved on all-miss, got %+v", resolved)
	}
}

// TestResolveWithFallback_OrderIsIMDBThenTMDBThenTVDB verifies the fallback
// chain priority by recording every HTTP attempt: with idValue "movie-99"
// (movie- prefix so the tmdb resolver actually issues an HTTP call) and
// strict source = imdb, we expect imdb (miss), tmdb (miss via /movie/99),
// tvdb (miss via /find/.../tvdb_id) in that exact order. tmdb without a
// prefix would silently fail validation, so the prefix forces a third
// recorded attempt that proves tmdb is between imdb and tvdb.
func TestResolveWithFallback_OrderIsIMDBThenTMDBThenTVDB(t *testing.T) {
	var hits []string
	tracker := &orderTrackingStub{hits: &hits}
	tmdb := services.NewTmdbClient("test-key", &http.Client{Transport: tracker})

	_, _ = resolveWithFallback(context.Background(), nil, services.IDTypeIMDB, "movie-99", services.IDClients{TMDB: tmdb}, true, true)
	want := []string{"imdb_id", "movie/series", "tvdb_id"}
	if len(hits) != len(want) {
		t.Fatalf("expected %d hits %v, got %d: %v", len(want), want, len(hits), hits)
	}
	for i, h := range want {
		if hits[i] != h {
			t.Errorf("hits[%d] = %q, want %q (order is wrong)", i, hits[i], h)
		}
	}
}

// orderTrackingStub records which /find external_source was hit (or which
// /tv|/movie fallback was used) so the fallback order can be asserted.
// /find returns empty results (miss); /tv and /movie return 404 (tmdb
// source fails without triggering the retry logic, so the chain proceeds
// to the tvdb fallback).
type orderTrackingStub struct {
	hits *[]string
}

func (o *orderTrackingStub) RoundTrip(req *http.Request) (*http.Response, error) {
	q := req.URL.Query()
	path := req.URL.Path
	switch {
	case strings.Contains(path, "/find/"):
		ext := q.Get("external_source")
		*o.hits = append(*o.hits, ext)
		return jsonResp(`{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`), nil
	case strings.Contains(path, "/tv/") || strings.Contains(path, "/movie/"):
		*o.hits = append(*o.hits, "movie/series")
		// 404 isn't retried (5xx is), and the TMDB client surfaces non-200
		// as an error → tmdb source fails → fallback chain proceeds.
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       readCloser{bytes.NewReader([]byte("orderTrackingStub: forced 404"))},
		}, nil
	}
	return jsonResp(`{}`), nil
}
