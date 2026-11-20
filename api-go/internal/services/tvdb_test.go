package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tvdbFake is a minimal TVDB v4 server: /login hands out "tok-N", the data
// endpoints require the current token.
type tvdbFake struct {
	logins   atomic.Int32
	requests atomic.Int32
	mu       sync.Mutex
	token    string
	handler  func(w http.ResponseWriter, r *http.Request) // data endpoints
}

func (f *tvdbFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/login" {
		n := f.logins.Add(1)
		var body struct {
			APIKey string `json:"apikey"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.APIKey != "key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.token = "tok-" + string(rune('0'+n))
		tok := f.token
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond) // let concurrent callers pile up
		w.Write([]byte(`{"status":"success","data":{"token":"` + tok + `"}}`))
		return
	}
	f.requests.Add(1)
	f.mu.Lock()
	want := "Bearer " + f.token
	f.mu.Unlock()
	if r.Header.Get("Authorization") != want {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.handler(w, r)
}

func newFakeTVDB(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*TVDBClient, *tvdbFake) {
	t.Helper()
	fake := &tvdbFake{handler: handler}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c := NewTVDBClient("key", srv.Client())
	c.BaseURL = srv.URL
	c.backoff = func(int) time.Duration { return time.Millisecond }
	return c, fake
}

const seriesExtendedJSON = `{"status":"success","data":{"id":81189,"name":"Breaking Bad","firstAired":"2008-01-20","originalLanguage":"eng",
 "remoteIds":[{"id":"tt0903747","type":2,"sourceName":"IMDB"},{"id":"1396","type":12,"sourceName":"TheMovieDB.com"}],
 "artworks":[
  {"id":1,"image":"https://artworks.thetvdb.com/p-eng-low.jpg","type":2,"language":"eng","score":10},
  {"id":2,"image":"https://artworks.thetvdb.com/p-eng-high.jpg","type":2,"language":"eng","score":99},
  {"id":3,"image":"https://artworks.thetvdb.com/p-deu.jpg","type":2,"language":"deu","score":5},
  {"id":4,"image":"https://artworks.thetvdb.com/bg-null.jpg","type":3,"language":null,"score":1},
  {"id":5,"image":"https://artworks.thetvdb.com/bg-eng.jpg","type":3,"language":"eng","score":50},
  {"id":6,"image":"https://artworks.thetvdb.com/logo-eng.png","type":23,"language":"eng","score":3}
 ]}}`

// The login token is shared: concurrent first calls log in once, later calls
// reuse it.
func TestTVDB_LoginTokenReused(t *testing.T) {
	c, fake := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(seriesExtendedJSON))
	})
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// distinct ids so the record cache doesn't hide the logins
			if _, err := c.SeriesCtx(t.Context(), uint64(100+i)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, err := c.SeriesCtx(t.Context(), 200); err != nil {
		t.Fatal(err)
	}
	if n := fake.logins.Load(); n != 1 {
		t.Errorf("logins = %d, want 1", n)
	}
}

// An expired token (401) is renewed once and the request retried.
func TestTVDB_ExpiredTokenRenewed(t *testing.T) {
	c, fake := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(seriesExtendedJSON))
	})
	if _, err := c.SeriesCtx(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.token = "rotated" // the server forgets the token
	fake.mu.Unlock()
	if _, err := c.SeriesCtx(t.Context(), 2); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	if n := fake.logins.Load(); n != 2 {
		t.Errorf("logins = %d, want 2", n)
	}
}

// A failed login is an error, never a not-found (which would be cached as a
// miss).
func TestTVDB_FailedLoginIsNotNotFound(t *testing.T) {
	c, _ := newFakeTVDB(t, nil)
	c.apiKey = "wrong"
	_, err := c.SeriesCtx(t.Context(), 1)
	if err == nil || isIDNotFound(err) {
		t.Fatalf("err = %v, want a non-404 error", err)
	}
}

func TestTVDB_ExtendedRecordParsed(t *testing.T) {
	c, _ := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/series/81189/extended" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(seriesExtendedJSON))
	})
	r, err := c.SeriesCtx(t.Context(), 81189)
	if err != nil {
		t.Fatal(err)
	}
	if r.IMDbID != "tt0903747" || r.TMDbID != 1396 || r.ReleaseDate != "2008-01-20" || r.Name != "Breaking Bad" || r.IsMovie {
		t.Errorf("record = %+v", r)
	}
	if _, err := c.MovieCtx(t.Context(), 81189); !isIDNotFound(err) {
		t.Errorf("movie 81189: err = %v, want not found", err)
	}
}

// /search/remoteid lists entities in no set order; the requested kind wins.
func TestTVDB_RemoteIDPicksRequestedKind(t *testing.T) {
	c, fake := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search/remoteid/tt0903747") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"data":[{"season":{"id":30272}},{"series":{"id":81189}},{"movie":{"id":555}}]}`))
	})
	if id, err := c.RemoteIDCtx(t.Context(), "tt0903747", MediaTypeTV); err != nil || id != 81189 {
		t.Errorf("series: id=%d err=%v, want 81189", id, err)
	}
	if id, err := c.RemoteIDCtx(t.Context(), "tt0903747", MediaTypeMovie); err != nil || id != 555 {
		t.Errorf("movie: id=%d err=%v, want 555", id, err)
	}
	before := fake.requests.Load()
	if id, _ := c.RemoteIDCtx(t.Context(), "tt0903747", MediaTypeTV); id != 81189 {
		t.Errorf("cached: id=%d", id)
	}
	if fake.requests.Load() != before {
		t.Error("second lookup was not served from cache")
	}
	if id, err := c.RemoteIDCtx(t.Context(), "tt0000001", MediaTypeTV); err != nil || id != 0 {
		t.Errorf("unknown: id=%d err=%v, want 0/nil", id, err)
	}
}

// 429 is retried with backoff; the request eventually succeeds.
func TestTVDB_429Retried(t *testing.T) {
	var n atomic.Int32
	c, _ := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(seriesExtendedJSON))
	})
	if _, err := c.SeriesCtx(t.Context(), 81189); err != nil {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 3 {
		t.Errorf("attempts = %d, want 3", n.Load())
	}
}

func TestSelectTVDBArtwork(t *testing.T) {
	var out struct {
		Data tvdbExtended `json:"data"`
	}
	if err := json.Unmarshal([]byte(seriesExtendedJSON), &out); err != nil {
		t.Fatal(err)
	}
	r := out.Data.record(false)
	cases := []struct {
		kind, lang string
		textless   bool
		want       string
	}{
		{"poster", "en", false, "p-eng-high.jpg"},  // highest score in the language
		{"poster", "de", false, "p-deu.jpg"},       // requested language first
		{"poster", "fr", false, "p-eng-high.jpg"},  // missing → English
		{"backdrop", "de", false, "bg-null.jpg"},   // backgrounds prefer untagged
		{"logo", "ja", false, "logo-eng.png"},      // → English
		{"episode", "en", false, ""},               // not a TVDB kind here
	}
	for _, c := range cases {
		got := SelectTVDBArtwork(r, c.kind, c.lang, c.textless)
		if !strings.HasSuffix(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("%s/%s: got %q, want …%s", c.kind, c.lang, got, c.want)
		}
	}
	// Portuguese: pt-BR is TVDB "pt", its sibling "por" comes next.
	r.Artworks = []TVDBArtwork{
		{Image: "https://artworks.thetvdb.com/por.jpg", Type: 2, Language: strp("por"), Score: 1},
		{Image: "https://artworks.thetvdb.com/eng.jpg", Type: 2, Language: strp("eng"), Score: 9},
	}
	if got := SelectTVDBArtwork(r, "poster", "pt-BR", false); !strings.HasSuffix(got, "por.jpg") {
		t.Errorf("pt-BR: got %q, want the Portuguese sibling", got)
	}
}

func strp(s string) *string { return &s }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// A tvdb: id TMDB /find doesn't know resolves through the TVDB record's
// TMDB cross-link; one without any cross-link is served from TVDB itself.
func TestResolveTVDB_FallsBackToTVDBClient(t *testing.T) {
	tvdb, _ := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/series/81189/extended":
			w.Write([]byte(seriesExtendedJSON))
		case "/movies/4242/extended":
			w.Write([]byte(`{"data":{"id":4242,"name":"Lonely Film","year":"2020","first_release":{"date":"2020-05-01"},"remoteIds":[],"artworks":[]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	tmdb := NewTmdbClient("k", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/find/"):
			return jsonResponse(200, `{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`), nil
		case strings.HasSuffix(req.URL.Path, "/tv/1396"):
			return jsonResponse(200, `{"name":"Breaking Bad","poster_path":"/bb.jpg","external_ids":{"imdb_id":"tt0903747","tvdb_id":null}}`), nil
		}
		return jsonResponse(404, `{}`), nil
	})})
	clients := IDClients{TMDB: tmdb, TVDB: tvdb}

	r, err := ResolveIDCtx(t.Context(), IDTypeTVDB, "81189", clients)
	if err != nil {
		t.Fatal(err)
	}
	if r.TMDbID != 1396 || r.MediaType != MediaTypeTV || r.TVDBID == nil || *r.TVDBID != 81189 {
		t.Errorf("cross-linked: got %+v", r)
	}

	r, err = ResolveIDCtx(t.Context(), IDTypeTVDB, "4242", clients)
	if err != nil {
		t.Fatal(err)
	}
	if r.TMDbID != 0 || r.MediaType != MediaTypeMovie || r.TVDBID == nil || *r.TVDBID != 4242 || r.Title == nil || *r.Title != "Lonely Film" {
		t.Errorf("TVDB-only: got %+v", r)
	}

	if _, err := ResolveIDCtx(t.Context(), IDTypeTVDB, "999", clients); !isIDNotFound(err) {
		t.Errorf("unknown everywhere: err = %v, want not found", err)
	}
}
