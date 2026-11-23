package services

import (
	"bytes"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	apperr "openposterdb/internal/errors"
)

// routeStub answers TMDB API calls by path substring; the fragments a test
// uses must not overlap (map order is random).
type routeStub map[string]string

func (s routeStub) RoundTrip(req *http.Request) (*http.Response, error) {
	for frag, body := range s {
		if strings.Contains(req.URL.Path, frag) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte(body)))}, nil
		}
	}
	return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}

// An IMDb-requested series resolves its TVDB id (fanart.tv indexes series by
// it) together with the original language and default backdrop.
func TestResolveIMDB_SeriesGetsTVDBID(t *testing.T) {
	tmdb := NewTmdbClient("k", &http.Client{Transport: routeStub{
		"/find/tt0903747": `{"movie_results":[],"tv_results":[{"id":1396,"name":"Breaking Bad","poster_path":"/p.jpg","original_language":"en","popularity":9}]}`,
		"/tv/1396":        `{"backdrop_path":"/b.jpg","external_ids":{"tvdb_id":81189,"imdb_id":"tt0903747"}}`,
	}})
	r, err := ResolveIDCtx(t.Context(), IDTypeIMDB, "tt0903747", IDClients{TMDB: tmdb})
	if err != nil {
		t.Fatal(err)
	}
	if r.TVDBID == nil || *r.TVDBID != 81189 {
		t.Errorf("TVDBID = %v, want 81189", r.TVDBID)
	}
	if r.BackdropPath == nil || *r.BackdropPath != "/b.jpg" || r.OriginalLanguage != "en" {
		t.Errorf("backdrop %v lang %q", r.BackdropPath, r.OriginalLanguage)
	}
}

// A tvdb: id that /find maps to a movie keeps the requested TVDB id, and the
// poster falls back to the /find entry when the details have none.
func TestResolveTVDB_MovieKeepsTVDBIDAndPosterFallback(t *testing.T) {
	tmdb := NewTmdbClient("k", &http.Client{Transport: routeStub{
		"/find/12345": `{"movie_results":[{"id":550,"title":"Fight Club","poster_path":"/find.jpg","backdrop_path":"/fb.jpg"}],"tv_results":[]}`,
		"/movie/550":  `{"imdb_id":"tt0137523","poster_path":null,"original_language":"en"}`,
	}})
	r, err := ResolveIDCtx(t.Context(), IDTypeTVDB, "12345", IDClients{TMDB: tmdb})
	if err != nil {
		t.Fatal(err)
	}
	if r.TVDBID == nil || *r.TVDBID != 12345 {
		t.Errorf("TVDBID = %v, want 12345", r.TVDBID)
	}
	if r.PosterPath == nil || *r.PosterPath != "/find.jpg" {
		t.Errorf("PosterPath = %v, want /find.jpg", r.PosterPath)
	}
	if r.BackdropPath == nil || *r.BackdropPath != "/fb.jpg" {
		t.Errorf("BackdropPath = %v, want /fb.jpg", r.BackdropPath)
	}
}

func TestParseSeasonEpisode_Bounds(t *testing.T) {
	ok := map[string][2]uint32{"1E2": {1, 2}, "10000E100000": {10000, 100000}, "0E0": {0, 0}}
	for in, want := range ok {
		s, e, valid := parseSeasonEpisode(in)
		if !valid || s != want[0] || e != want[1] {
			t.Errorf("%q: got %d/%d/%v", in, s, e, valid)
		}
	}
	for _, in := range []string{"10001E1", "1E100001", "xE1", "1Ey", "1", "99999999999E1"} {
		if _, _, valid := parseSeasonEpisode(in); valid {
			t.Errorf("%q accepted", in)
		}
	}
	// The tmdb episode form is checked too, before any request.
	tmdb := NewTmdbClient("k", &http.Client{Transport: routeStub{}})
	if _, err := ResolveIDCtx(t.Context(), IDTypeTMDB, "episode-1396-S99999E1", IDClients{TMDB: tmdb}); !isBadRequest(err) {
		t.Errorf("out-of-range tmdb episode: err %v, want 400", err)
	}
}

func isBadRequest(err error) bool {
	var ae *apperr.AppError
	return stderrors.As(err, &ae) && ae.Status == http.StatusBadRequest
}

func TestSeriesTVDBID_LearnedFromOtherProvider(t *testing.T) {
	own := uint64(7)
	if got := SeriesTVDBID(&ResolvedID{TMDbID: 1, TVDBID: &own}); got == nil || *got != 7 {
		t.Errorf("own id: %v", got)
	}
	if SeriesTVDBID(&ResolvedID{TMDbID: 987654}) != nil {
		t.Error("unknown series must have no TVDB id")
	}
	RememberTVDBID(987654, 4242)
	if got := SeriesTVDBID(&ResolvedID{TMDbID: 987654}); got == nil || *got != 4242 {
		t.Errorf("learned id: %v", got)
	}
}

// Ids MDBList knows for a series TMDB has without them (IMDb, TVDB) reach the
// cached resolution afterwards, without mutating the cached value.
func TestResolveIDCached_GetsIDsLearnedFromMDBList(t *testing.T) {
	const tmdbID = 818181
	item := `{"ids":{"imdb":"tt8888","tmdb":818181,"tvdb":4321},"ratings":[]}`
	mdb := NewMdblistClient([]string{"k"}, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			return jsonResponse(200, "["+item+"]"), nil
		}
		return jsonResponse(200, item), nil
	})})
	FetchRatingsCtx(t.Context(), RatingsClients{MDBList: mdb}, RatingsQuery{ResolvedTMDbID: tmdbID, MediaType: "tv"})

	cache := NewMemCache(0, 10, 0, 0)
	cached := &ResolvedID{TMDbID: tmdbID, MediaType: MediaTypeTV}
	now := time.Now()
	cache.Set("tmdb/series-818181", idEntry{resolved: cached, refreshAfter: now.Add(time.Hour), fetched: now}, 1)
	r, err := ResolveIDCachedCtx(t.Context(), cache, IDTypeTMDB, "series-818181", IDClients{})
	if err != nil {
		t.Fatal(err)
	}
	if r.IMDbID == nil || *r.IMDbID != "tt8888" {
		t.Errorf("IMDb id not backfilled: %v", r.IMDbID)
	}
	if r.TVDBID == nil || *r.TVDBID != 4321 {
		t.Errorf("TVDB id not backfilled: %v", r.TVDBID)
	}
	if cached.IMDbID != nil || cached.TVDBID != nil {
		t.Error("cached resolution mutated")
	}
	// A movie with the same TMDB number is another title.
	movie := &ResolvedID{TMDbID: tmdbID, MediaType: MediaTypeMovie}
	cache.Set("tmdb/movie-818181", idEntry{resolved: movie, refreshAfter: now.Add(time.Hour), fetched: now}, 1)
	if r, _ := ResolveIDCachedCtx(t.Context(), cache, IDTypeTMDB, "movie-818181", IDClients{}); r.IMDbID != nil || r.TVDBID != nil {
		t.Errorf("movie got the series' ids: %v %v", r.IMDbID, r.TVDBID)
	}
}
