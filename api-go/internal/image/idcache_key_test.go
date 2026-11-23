package image

import (
	"context"
	"net/http"
	"testing"

	"openposterdb/internal/services"
)

// The cold path checks the ID cache under the key the resolution caches:
// the type the id's form names (imdb/movie-278 is cached as tmdb/movie-278),
// and for a kitsu/mal id translated to the standard pipeline (toggle off),
// the translated id.
func TestIDCacheWarm_UsesResolutionKey(t *testing.T) {
	p := newCrossRefParams(t)
	s := services.DefaultRenderSettings()
	s.UseKitsu, s.UseMAL = false, false
	p.Settings = &s
	p.Caches = &services.MemCacheSet{IDs: services.NewMemCache(0, 100, 0, 0)}

	p.IDType, p.IDValue = "imdb", "movie-278"
	if p.idCacheWarm() {
		t.Fatal("empty cache reported warm")
	}
	p.Caches.IDs.Set("tmdb/movie-278", "x", 1)
	if !p.idCacheWarm() {
		t.Error("imdb/movie-278: resolution key tmdb/movie-278 not checked")
	}

	// kitsu 3936 = MAL 5114 → tt0000278 in the anime list.
	p.Caches.IDs.Set("imdb/tt0000278", "x", 1)
	for _, id := range [][2]string{{"kitsu", "3936"}, {"mal", "5114"}} {
		p.IDType, p.IDValue = id[0], id[1]
		if !p.idCacheWarm() {
			t.Errorf("%s/%s: translated key imdb/tt0000278 not checked", id[0], id[1])
		}
	}
	// With the toggle on the id resolves directly: only its own key counts.
	s.UseKitsu = true
	p.IDType, p.IDValue = "kitsu", "3936"
	if p.idCacheWarm() {
		t.Error("kitsu with toggle on reported warm from the translated key")
	}
}

// standardAnimeList: a film whose IMDb id is refused (shared by two entries)
// but whose movie TMDB id is usable, and one with no IMDb id at all.
const standardAnimeList = `[
 {"kitsu_id":6000,"mal_id":6100,"imdb_id":"tt8888888","themoviedb_id":{"movie":[4242]},"type":"MOVIE"},
 {"kitsu_id":6001,"mal_id":6101,"imdb_id":"tt8888888","type":"MOVIE"},
 {"kitsu_id":6002,"mal_id":6102,"themoviedb_id":{"movie":[4343]},"type":"MOVIE"}
]`

// With the toggles off, a Kitsu/MAL id translates through its TMDB id when
// the IMDb id is refused or missing (as RatingsIDs does for ratings), instead
// of falling back to Kitsu/MAL art.
func TestKitsuMalTranslatesViaTMDBWhenIMDbRefused(t *testing.T) {
	httpClient := &http.Client{Transport: crossRefStub{}}
	mappings := services.NewIDMappings(httpClient, "")
	if err := mappings.LoadAnimeList([]byte(standardAnimeList)); err != nil {
		t.Fatalf("anime list load: %v", err)
	}
	clients := services.IDClients{
		TMDB:     services.NewTmdbClient("test-key", httpClient),
		Kitsu:    services.NewKitsuClient(httpClient),
		AniList:  services.NewAniListClient(httpClient),
		Mappings: mappings,
	}
	cases := []struct {
		idType services.IDType
		id     string
		tmdb   uint64
	}{
		{services.IDTypeKitsu, "6000", 4242},
		{services.IDTypeMAL, "6100", 4242},
		{services.IDTypeKitsu, "6002", 4343},
		{services.IDTypeMAL, "6102", 4343},
	}
	for _, c := range cases {
		r, err := resolveKitsuMalCtx(context.Background(), nil, c.idType, c.id, clients, false, false)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.idType, c.id, err)
		}
		if r.SourceProvider != "" || r.TMDbID != c.tmdb || r.MediaType != services.MediaTypeMovie {
			t.Errorf("%s/%s: got provider=%q tmdb=%d type=%v, want TMDB movie %d", c.idType, c.id, r.SourceProvider, r.TMDbID, r.MediaType, c.tmdb)
		}
	}
}
