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

// respStub answers every request with handler(req); calls counts requests.
type respStub struct {
	calls   atomic.Int32
	mu      sync.Mutex
	keys    []string // the API key each request carried
	handler func(req *http.Request) (int, http.Header, string)
}

func (s *respStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	key := req.URL.Query().Get("apikey")
	if key == "" {
		key = req.Header.Get("trakt-api-key")
	}
	s.mu.Lock()
	s.keys = append(s.keys, key)
	s.mu.Unlock()
	status, h, body := s.handler(req)
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestRetryAfter_Parsing(t *testing.T) {
	mk := func(status int, v string) *http.Response {
		return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{v}}}
	}
	if d, ok := RetryAfter(mk(429, "60")); !ok || d != time.Minute {
		t.Errorf("seconds: %v %v", d, ok)
	}
	if d, ok := RetryAfter(mk(503, time.Now().Add(2*time.Hour).UTC().Format(http.TimeFormat))); !ok || d < 119*time.Minute {
		t.Errorf("HTTP date: %v %v", d, ok)
	}
	if _, ok := RetryAfter(mk(500, "60")); ok {
		t.Error("Retry-After on a 500 must be ignored")
	}
	if _, ok := RetryAfter(mk(429, "soon")); ok {
		t.Error("garbage must be ignored")
	}
}

// 429 with Retry-After: 60 (beyond the 2 s retry policy) → exactly one
// request, the key is blocked, and further calls don't reach the provider
// until the block ends.
func TestMDBList_RetryAfterBlocksKey(t *testing.T) {
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) {
		return 429, http.Header{"Retry-After": []string{"60"}}, `{"error":"limit"}`
	}}
	c := NewMdblistClient([]string{"only"}, &http.Client{Transport: stub})
	if _, err := c.GetRatingsCtx(t.Context(), "tt1", "movie"); err == nil {
		t.Fatal("rate-limited call must fail")
	}
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1 (no retry against a 60 s Retry-After)", n)
	}
	if _, ok := c.KeyPool.ActiveKey(); ok {
		t.Fatal("the only key must be blocked")
	}
	if until := c.KeyPool.BlockedUntil(); time.Until(until) < 55*time.Second {
		t.Errorf("blocked until %v, want ~60 s from now", until)
	}
	if _, err := c.GetRatingsCtx(t.Context(), "tt2", "movie"); err == nil || stub.calls.Load() != 1 {
		t.Fatalf("blocked pool still called the provider: calls=%d err=%v", stub.calls.Load(), err)
	}
}

// Two OMDb keys, the first at its daily limit (401 "Request limit
// reached!") → the same call is answered with the second key.
func TestOMDb_LimitedKeyRotatesWithinCall(t *testing.T) {
	stub := &respStub{handler: func(req *http.Request) (int, http.Header, string) {
		if req.URL.Query().Get("apikey") == "first" {
			return 401, nil, `{"Response":"False","Error":"Request limit reached!"}`
		}
		return 200, nil, `{"imdbRating":"8.1","Ratings":[]}`
	}}
	c := NewOmdbClient([]string{"first", "second"}, &http.Client{Transport: stub})
	resp, err := c.GetRatingsCtx(t.Context(), "tt1")
	if err != nil || resp.IMDBRating == nil || *resp.IMDBRating != "8.1" {
		t.Fatalf("got %+v %v", resp, err)
	}
	if strings.Join(stub.keys, ",") != "first,second" {
		t.Errorf("keys used: %v", stub.keys)
	}
	// The limited key stays blocked: the next call goes straight to "second".
	c.GetRatingsCtx(t.Context(), "tt2")
	if last := stub.keys[len(stub.keys)-1]; last != "second" || len(stub.keys) != 3 {
		t.Errorf("keys used: %v", stub.keys)
	}
}

// An invalid OMDb key (also a 401) is an error, not a rate limit.
func TestOMDb_InvalidKeyIsNotALimit(t *testing.T) {
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) {
		return 401, nil, `{"Response":"False","Error":"Invalid API key!"}`
	}}
	c := NewOmdbClient([]string{"bad", "other"}, &http.Client{Transport: stub})
	if _, err := c.GetRatingsCtx(t.Context(), "tt1"); err == nil {
		t.Fatal("want an error")
	}
	if n := stub.calls.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (no rotation for an invalid key)", n)
	}
}

// Trakt rotates its pool on 429 like MDBList.
func TestTrakt_429RotatesKey(t *testing.T) {
	stub := &respStub{handler: func(req *http.Request) (int, http.Header, string) {
		if req.Header.Get("trakt-api-key") == "a" {
			return 429, nil, `{}`
		}
		return 200, nil, `{"rating":8.0,"votes":500}`
	}}
	saved := TraktRetry
	TraktRetry.MaxRetries = 0
	t.Cleanup(func() { TraktRetry = saved })
	c := NewTraktClient([]string{"a", "b"}, &http.Client{Transport: stub})
	r, err := c.GetMovieRatingCtx(t.Context(), "tt1")
	if err != nil || r == nil || r.Votes != 500 {
		t.Fatalf("got %+v %v", r, err)
	}
	if strings.Join(stub.keys, ",") != "a,b" {
		t.Errorf("keys used: %v", stub.keys)
	}
}

func TestAPIKeyPool_BlockExpires(t *testing.T) {
	now := time.Now()
	p := NewAPIKeyPool([]string{"k"})
	p.now = func() time.Time { return now }
	p.Report429For("k", now.Add(time.Minute))
	if _, ok := p.ActiveKey(); ok {
		t.Fatal("blocked")
	}
	now = now.Add(61 * time.Second)
	if k, ok := p.ActiveKey(); !ok || k != "k" {
		t.Fatal("block must expire")
	}
}

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }
func str(v string) *string   { return &v }

func badgeKeys(bs []RatingBadge) string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Source.Key+"="+b.Value)
	}
	return strings.Join(out, " ")
}

// MDBList wins per source; OMDb fills IMDb / RT / Metacritic it lacks; the
// direct Trakt rating fills Trakt; the order is the Rust one.
func TestMergeRatings_GapFillingAndOrder(t *testing.T) {
	mdb := &MdblistResponse{Ratings: []MdblistRating{
		{Source: "imdb", Value: f64(7.9), Votes: i64(100000)},
		{Source: "letterboxd", Value: f64(3.9)},
	}}
	omdb := &OmdbResponse{IMDBRating: str("7.5"), Metascore: str("71"), Ratings: []OmdbRating{{Source: "Rotten Tomatoes", Value: "88%"}}}
	tmdb := &RatingBadge{Source: SourceTmdb, Value: "80%"}
	trakt := &RatingBadge{Source: SourceTrakt, Value: "82%"}
	got := badgeKeys(mergeRatings(tmdb, omdbBadges(omdb), mdb, trakt))
	want := "imdb=7.9 tmdb=80% rt=88% mc=71 trakt=82% lb=3.9"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Without MDBList, OMDb's values come in the same order as with it (the
// OMDb-only path used to put TMDB first and RT before IMDb), and "N/A"
// placeholders are dropped.
func TestMergeRatings_OMDbOnly(t *testing.T) {
	omdb := &OmdbResponse{IMDBRating: str("6.4"), Metascore: str("N/A"), Ratings: []OmdbRating{{Source: "Rotten Tomatoes", Value: "N/A"}}}
	tmdb := &RatingBadge{Source: SourceTmdb, Value: "64%"}
	if got := badgeKeys(mergeRatings(tmdb, omdbBadges(omdb), nil, nil)); got != "imdb=6.4 tmdb=64%" {
		t.Errorf("got %s", got)
	}
}

func TestMdblistBadges_ValueAndVoteChecks(t *testing.T) {
	resp := &MdblistResponse{Ratings: []MdblistRating{
		{Source: "imdb", Value: f64(0), Votes: i64(1000)},      // no value
		{Source: "letterboxd", Value: f64(4.1), Votes: i64(3)}, // too few votes
		{Source: "trakt", Score: f64(90), Votes: i64(2)},       // too few votes
		{Source: "tomatoes", Score: f64(95), Votes: i64(6)},    // critics: few reviews are normal
		{Source: "metacritic", Score: f64(70), Votes: i64(4)},
	}}
	if got := badgeKeys(MdblistBadges(resp)); got != "rt=95% mc=70" {
		t.Errorf("got %s", got)
	}
}

func TestTmdbRating_NeedsVotes(t *testing.T) {
	for body, want := range map[string]bool{
		`{"vote_average":10,"vote_count":1}`:    false,
		`{"vote_average":7.2,"vote_count":250}`: true,
	} {
		tmdb := NewTmdbClient("k", &http.Client{Transport: routeStub{"/movie/1": body}})
		b, err := fetchTmdbRatingCtx(t.Context(), tmdb, 1, "movie", 0, 0, 0)
		if err != nil || (b != nil) != want {
			t.Errorf("%s: badge %v err %v", body, b, err)
		}
	}
}

// Trakt episode ratings are fetched with the show's IMDb id.
func TestFetchRatings_TraktEpisode(t *testing.T) {
	var traktPath atomic.Value
	stub := &respStub{handler: func(req *http.Request) (int, http.Header, string) {
		switch {
		case strings.Contains(req.URL.Host, "trakt.tv"):
			traktPath.Store(req.URL.Path)
			return 200, nil, `{"rating":8.6,"votes":1200}`
		case strings.Contains(req.URL.Path, "/tv/1396/external_ids"):
			return 200, nil, `{"imdb_id":"tt0903747"}`
		}
		return 404, nil, `{}`
	}}
	hc := &http.Client{Transport: stub}
	clients := RatingsClients{TMDB: NewTmdbClient("k", hc), Trakt: NewTraktClient([]string{"t"}, hc)}
	res := FetchRatingsCtx(t.Context(), clients, RatingsQuery{MediaType: "episode", EpisodeShowTMDbID: 1396, EpisodeSeason: 5, EpisodeEpisode: 14})
	if got := badgeKeys(res.Badges); got != "trakt=86%" {
		t.Fatalf("badges %q", got)
	}
	if p, _ := traktPath.Load().(string); p != "/shows/tt0903747/seasons/5/episodes/14/ratings" {
		t.Errorf("trakt path %q", p)
	}
	if res.Failed != 0 {
		t.Errorf("failed = %d (a TMDB 404 for the episode rating is not a failure)", res.Failed)
	}
}

func TestFetchRatings_CountsFailures(t *testing.T) {
	stub := &respStub{handler: func(*http.Request) (int, http.Header, string) { return 400, nil, `{}` }}
	hc := &http.Client{Transport: stub}
	res := FetchRatingsCtx(t.Context(), RatingsClients{OMDB: NewOmdbClient([]string{"k"}, hc)}, imdbOnly("tt1"))
	if !res.AllFailed() || res.Attempted != 1 {
		t.Errorf("attempted %d failed %d, want 1/1", res.Attempted, res.Failed)
	}
}

func TestRatingsRefreshFor(t *testing.T) {
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"1999-03-31": ratingsRefreshAfter,
		"2026-11-01": newReleaseRefreshAfter, // 16 days old
		"2027-01-01": newReleaseRefreshAfter, // not out yet
		"garbage":    newReleaseRefreshAfter,
	}
	for date, want := range cases {
		if got := ratingsRefreshFor(&date, now); got != want {
			t.Errorf("%s: %v, want %v", date, got, want)
		}
	}
	if got := ratingsRefreshFor(nil, now); got != newReleaseRefreshAfter {
		t.Errorf("unknown date: %v", got)
	}
}

// Stored values outside the enums go through the Parse* functions (unknown
// → default) instead of reaching the renderer and the cache key verbatim.
func TestParseGlobalRenderSettings_InvalidEnumsFallBack(t *testing.T) {
	s := ParseGlobalRenderSettings(map[string]string{
		"image_source":           "zz",
		"poster_label_style":     "zz",
		"poster_badge_direction": "zz",
		"poster_fit":             "zz",
		"poster_badge_shape":     "zz",
	})
	if s.ImageSource != ImageSourceTMDB || s.PosterLabelStyle != LabelStyleOfficial ||
		s.PosterBadgeDirection != BadgeDirectionDefault || s.PosterFit != PosterFitNative || s.PosterBadgeShape != BadgeShapeRounded {
		t.Errorf("got %q %q %q %q %q", s.ImageSource, s.PosterLabelStyle, s.PosterBadgeDirection, s.PosterFit, s.PosterBadgeShape)
	}
}
