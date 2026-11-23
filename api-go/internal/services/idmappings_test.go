package services

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const dinsSeriesCSV = "imdb_id,tmdb_id,tvdb_id,tvmaze_id\n" +
	"tt0903747,1396,81189,169\n" +
	"tt0944947,1399,,82\n" + // no TVDB id: Wikidata may fill it
	"tt123,1,2,3\n" + // malformed IMDb id: skipped
	"tt0000009,9,9\n" // wrong column count: skipped

const wikiSeriesCSV = "imdb_id,tmdb_id,tvdb_id\n" +
	"tt0944947,,121361\n" + // fills Game of Thrones' TVDB id
	"tt0903747,1396,99999\n" + // contradicts dins: skipped
	"tt7777777,,555\n" // a title dins lacks: added

func TestParseMappingCSV(t *testing.T) {
	rows, err := parseMappingCSV([]byte("\xef\xbb\xbf"+dinsSeriesCSV), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 valid rows", len(rows))
	}
	if _, err := parseMappingCSV([]byte("tmdb_id,tvdb_id\n1,2\n"), true); err == nil {
		t.Error("no imdb column: want an error")
	}
	if _, err := parseMappingCSV([]byte("imdb_id,tmdb_id\n"), true); err == nil {
		t.Error("no rows: want an error (an empty download must not replace a good list)")
	}
	// keepTVDB=false drops the column (Wikidata movie TVDB ids aren't trusted).
	rows, _ = parseMappingCSV([]byte("imdb_id,tmdb_id,tvdb_id\ntt0111161,278,190\n"), false)
	if len(rows) != 1 || rows[0].tvdb != 0 {
		t.Errorf("keepTVDB=false: %+v", rows)
	}
}

func TestIDMappings_LookupAndBackfill(t *testing.T) {
	m := NewIDMappings(nil, "")
	if err := m.LoadTables([]byte(dinsSeriesCSV), []byte(wikiSeriesCSV), []byte("imdb_id,tmdb_id\ntt0111161,278\n"), nil); err != nil {
		t.Fatal(err)
	}
	if e, ok := m.Lookup(MediaTypeTV, IDTypeTVDB, "81189"); !ok || e.TMDB != 1396 || e.IMDb != "tt0903747" {
		t.Errorf("tvdb 81189: %+v %v", e, ok)
	}
	if e, ok := m.Lookup(MediaTypeTV, IDTypeTVDB, "99999"); ok {
		t.Errorf("contradicting Wikidata row was indexed: %+v", e)
	}
	if id := m.SeriesTVDB(nil, 1399); id != 121361 {
		t.Errorf("GoT TVDB filled from Wikidata = %d, want 121361", id)
	}
	if e, ok := m.Lookup(MediaTypeTV, IDTypeIMDB, "tt7777777"); !ok || e.TVDB != 555 {
		t.Errorf("Wikidata-only title: %+v %v", e, ok)
	}
	if e, ok := m.Lookup(MediaTypeMovie, IDTypeIMDB, "tt0111161"); !ok || e.TMDB != 278 {
		t.Errorf("movie: %+v %v", e, ok)
	}
	if _, ok := m.Lookup(MediaTypeMovie, IDTypeTVDB, "81189"); ok {
		t.Error("series id found in the movie table")
	}
}

const fribbJSON = `[
 {"mal_id":5114,"kitsu_id":3936,"anilist_id":5114,"imdb_id":"tt1355642","themoviedb_id":{"tv":[31911]},"tvdb_id":85249,"type":"TV"},
 {"mal_id":30,"kitsu_id":21,"imdb_id":["tt0169858","tt0169859"],"themoviedb_id":{"movie":[18491,18492]},"type":"MOVIE"},
 {"mal_id":431,"kitsu_id":399,"themoviedb_id":10494,"type":"MOVIE"},
 {"mal_id":432,"kitsu_id":400,"imdb_id":"tt1355642","type":"OVA"}
]`

func TestIDMappings_AnimeList(t *testing.T) {
	m := NewIDMappings(nil, "")
	if err := m.LoadAnimeList([]byte(fribbJSON)); err != nil {
		t.Fatal(err)
	}
	if imdb := m.IMDbForKitsu(3936); imdb == nil || *imdb != "tt1355642" {
		t.Errorf("kitsu 3936 → %v", imdb)
	}
	if k := m.KitsuForMAL(5114); k == nil || *k != 3936 {
		t.Errorf("mal 5114 → kitsu %v", k)
	}
	// Multi-part film: no single title to stand for the entry.
	if imdb := m.IMDbForKitsu(21); imdb != nil {
		t.Errorf("multi-part entry gave %v", *imdb)
	}
	// TMDB id with the namespace inferred from the type.
	a, _ := m.AnimeByMAL(431)
	if imdb, tmdb, kind := a.RatingsIDs(); imdb != nil || tmdb != 10494 || kind != MediaTypeMovie {
		t.Errorf("mal 431: %v %d %v", imdb, tmdb, kind)
	}
	a, _ = m.AnimeByMAL(5114)
	if _, tmdb, kind := a.RatingsIDs(); tmdb != 31911 || kind != MediaTypeTV {
		t.Errorf("mal 5114: tmdb %d kind %v, want the tv namespace", tmdb, kind)
	}
	// Several entries share an IMDb id: the first listed wins.
	if k := m.KitsuByIMDb("tt1355642"); k == nil || *k != 3936 {
		t.Errorf("tt1355642 → kitsu %v, want 3936", k)
	}
}

// An anime film's entry often carries its franchise show's ids: a TMDB id
// from the tv space and the show's IMDb id, shared with the show's entries.
// Neither stands for the film (mirrors aiometadata's filmIdsFor).
func TestIDMappings_AnimeFilmRefusesFranchiseIDs(t *testing.T) {
	const list = `[
 {"mal_id":1,"kitsu_id":11,"imdb_id":"tt0100001","themoviedb_id":500,"themoviedb_type":"tv","type":"TV"},
 {"mal_id":2,"kitsu_id":12,"imdb_id":"tt0100001","themoviedb_id":500,"themoviedb_type":"tv","type":"MOVIE"},
 {"mal_id":3,"kitsu_id":13,"imdb_id":"tt0100003","themoviedb_id":700,"themoviedb_type":"movie","type":"MOVIE"},
 {"mal_id":4,"kitsu_id":14,"imdb_id":"tt0100001","themoviedb_id":500,"themoviedb_type":"tv","type":"OVA"}
]`
	m := NewIDMappings(nil, "")
	if err := m.LoadAnimeList([]byte(list)); err != nil {
		t.Fatal(err)
	}
	film, _ := m.AnimeByMAL(2)
	if imdb, tmdb, kind := film.RatingsIDs(); imdb != nil || tmdb != 0 || kind != MediaTypeMovie {
		t.Errorf("film with the show's ids: imdb %v tmdb %d kind %v, want nothing (movie)", imdb, tmdb, kind)
	}
	if imdb := m.IMDbForKitsu(12); imdb != nil {
		t.Errorf("film kitsu 12 → %v, want nil (IMDb id shared with the show)", *imdb)
	}
	own, _ := m.AnimeByMAL(3)
	if imdb, tmdb, kind := own.RatingsIDs(); imdb == nil || *imdb != "tt0100003" || tmdb != 700 || kind != MediaTypeMovie {
		t.Errorf("film with its own ids: %v %d %v", imdb, tmdb, kind)
	}
	// Series-like entries keep the shared show ids.
	show, _ := m.AnimeByMAL(1)
	if imdb, tmdb, kind := show.RatingsIDs(); imdb == nil || tmdb != 500 || kind != MediaTypeTV {
		t.Errorf("show: %v %d %v", imdb, tmdb, kind)
	}
	ova, _ := m.AnimeByMAL(4)
	if imdb, tmdb, kind := ova.RatingsIDs(); imdb == nil || tmdb != 500 || kind != MediaTypeTV {
		t.Errorf("ova: %v %d %v", imdb, tmdb, kind)
	}
}

// Downloads go through an ETag check, a broken file never replaces the last
// good snapshot, and a 304 reuses the snapshot.
func TestIDMappings_RefreshSnapshotAndETag(t *testing.T) {
	var serveBroken atomic.Bool
	var conditional atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` && !serveBroken.Load() {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		switch {
		case serveBroken.Load():
			w.Write([]byte("<html>oops</html>"))
		case strings.HasSuffix(r.URL.Path, ".json"):
			w.Write([]byte(fribbJSON))
		case strings.Contains(r.URL.Path, "tv"):
			w.Write([]byte(dinsSeriesCSV))
		default:
			w.Write([]byte("imdb_id,tmdb_id\ntt0111161,278\n"))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	newM := func() *IDMappings {
		m := NewIDMappings(srv.Client(), dir)
		m.sources = map[string]string{
			srcDinsSeries.file: srv.URL + "/dins/tv.csv", srcWikiSeries.file: srv.URL + "/wiki/tv.csv",
			srcDinsMovies.file: srv.URL + "/dins/movie.csv", srcWikiMovies.file: srv.URL + "/wiki/movie.csv",
			srcFribb.file: srv.URL + "/anime.json",
		}
		return m
	}

	m := newM()
	if err := m.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !m.Ready() || !m.AnimeReady() {
		t.Fatal("lists not loaded")
	}
	if _, err := os.Stat(filepath.Join(dir, srcFribb.file)); err != nil {
		t.Fatalf("no snapshot: %v", err)
	}

	// Second refresh: every list answers 304.
	if err := m.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if conditional.Load() != 5 {
		t.Errorf("304 answers = %d, want 5", conditional.Load())
	}

	// A broken upstream keeps the snapshot; a restart loads it offline.
	serveBroken.Store(true)
	if err := m.Refresh(t.Context()); err == nil {
		t.Error("broken download: want an error")
	}
	if !m.AnimeReady() {
		t.Error("broken download dropped the loaded list")
	}
	m2 := newM()
	m2.loadSnapshots()
	if !m2.Ready() || !m2.AnimeReady() {
		t.Fatal("snapshot not usable after a broken download")
	}
	if k := m2.KitsuForMAL(5114); k == nil || *k != 3936 {
		t.Errorf("snapshot lookup: %v", k)
	}
}

// A resolve of a tvdb: id found in the offline list skips TMDB /find.
func TestResolveTVDB_UsesMappingFirst(t *testing.T) {
	m := NewIDMappings(nil, "")
	if err := m.LoadTables([]byte(dinsSeriesCSV), nil, []byte("imdb_id,tmdb_id\ntt0111161,278\n"), nil); err != nil {
		t.Fatal(err)
	}
	var finds atomic.Int32
	tmdb := NewTmdbClient("k", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/find/") {
			finds.Add(1)
			return jsonResponse(200, `{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`), nil
		}
		if strings.HasSuffix(req.URL.Path, "/tv/1396") {
			return jsonResponse(200, `{"name":"Breaking Bad","external_ids":{"imdb_id":"tt0903747"}}`), nil
		}
		return jsonResponse(404, `{}`), nil
	})})
	r, err := ResolveIDCtx(t.Context(), IDTypeTVDB, "81189", IDClients{TMDB: tmdb, Mappings: m})
	if err != nil {
		t.Fatal(err)
	}
	if r.TMDbID != 1396 || r.TVDBID == nil || *r.TVDBID != 81189 {
		t.Errorf("got %+v", r)
	}
	if finds.Load() != 0 {
		t.Errorf("/find called %d times, want 0", finds.Load())
	}
}
