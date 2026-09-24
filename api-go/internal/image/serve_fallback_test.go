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
//
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
				return jsonResp(`{"movie_results":[{"id":278,"title":"The Shawshank Redemption","poster_path":"/abc.jpg","release_date":null,"popularity":100.0}],"tv_results":[],"tv_episode_results":[]}`), nil
			}
		case "tvdb_id":
			if isAllDigits(id) {
				return jsonResp(`{"movie_results":[],"tv_results":[{"id":1396,"poster_path":"/abc.jpg","first_air_date":null,"popularity":100.0}],"tv_episode_results":[]}`), nil
			}
		}
		return jsonResp(`{"movie_results":[],"tv_results":[],"tv_episode_results":[]}`), nil
	case strings.Contains(path, "/tv/") || strings.Contains(path, "/movie/"):
		return jsonResp(`{"imdb_id":"tt9999999","title":"Stub Title","name":"Stub Title","poster_path":"/abc.jpg","release_date":null,"first_air_date":null,"external_ids":{"imdb_id":"tt9999999"}}`), nil
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

// crossRefStub serves the three hosts ratingsTargetFor touches: the
// TheBeastLT Kitsu→IMDb table (one row, kitsu 3936 → tt0000278), Kitsu's
// /mappings endpoint (MAL 5114 → kitsu 3936, anything else → no rows), and
// TMDB (delegated to fallbackStub: /find tt… → movie 278).
type crossRefStub struct{}

func (crossRefStub) RoundTrip(req *http.Request) (*http.Response, error) {
	switch {
	case req.URL.Host == "raw.githubusercontent.com":
		return jsonResp(`[{"kitsu_id":3936,"imdb_id":"tt0000278","title":"FMA:B"}]`), nil
	case req.URL.Host == "kitsu.io" && strings.HasSuffix(req.URL.Path, "/mappings"):
		q := req.URL.Query()
		if q.Get("filter[externalId]") != "5114" {
			return jsonResp(`{"data":[]}`), nil
		}
		// Like the real API (verified 2026-09-24): relationship linkage
		// data is only present with include=item; otherwise links only.
		if q.Get("include") == "item" {
			return jsonResp(`{"data":[{"id":"412","type":"mappings","relationships":{"item":{"data":{"type":"anime","id":"3936"}}}}]}`), nil
		}
		return jsonResp(`{"data":[{"id":"412","type":"mappings","relationships":{"item":{"links":{"related":"https://kitsu.io/api/edge/mappings/412/item"}}}}]}`), nil
	case req.URL.Host == "kitsu.io" && strings.HasSuffix(req.URL.Path, "/anime") && req.URL.Query().Get("filter[slug]") == "fmab":
		return jsonResp(`{"data":[{"id":"3936","type":"anime","attributes":{"slug":"fmab","canonicalTitle":"FMA:B","subtype":"TV","posterImage":{"original":"https://kitsu.test/poster.jpg"},"coverImage":{"original":"https://kitsu.test/cover.jpg"}}}],"included":[]}`), nil
	case req.URL.Host == "kitsu.io" && strings.HasSuffix(req.URL.Path, "/anime/3936"):
		return jsonResp(`{"data":{"id":"3936","type":"anime","attributes":{"slug":"fmab","canonicalTitle":"FMA:B","subtype":"TV","startDate":"2009-04-05","posterImage":{"original":"https://kitsu.test/poster.jpg"},"coverImage":{"original":"https://kitsu.test/cover.jpg"}}},"included":[{"id":"1","type":"mappings","attributes":{"externalSite":"myanimelist/anime","externalId":"5114"}}]}`), nil
	case req.URL.Host == "graphql.anilist.co":
		return jsonResp(`{"data":{"Media":{"id":5114,"idMal":5114,"title":{"romaji":"FMA:B"},"coverImage":{"extraLarge":"https://anilist.test/cover.jpg"},"bannerImage":"https://anilist.test/banner.jpg","startDate":{"year":2009,"month":4,"day":5},"externalLinks":[]}}}`), nil
	}
	return fallbackStub{}.RoundTrip(req)
}

func newCrossRefParams(t *testing.T) ServeParams {
	t.Helper()
	httpClient := &http.Client{Transport: crossRefStub{}}
	mapper := services.NewKitsuIMDbMapper(httpClient)
	if err := mapper.Load(context.Background()); err != nil {
		t.Fatalf("mapper load: %v", err)
	}
	return ServeParams{
		Context:         context.Background(),
		TMDB:            services.NewTmdbClient("test-key", httpClient),
		Kitsu:           services.NewKitsuClient(httpClient),
		KitsuIMDbMapper: mapper,
		Caches:          &services.MemCacheSet{},
		IDType:          "kitsu",
		IDValue:         "3936",
	}
}

// TestRatingsTargetFor_KitsuMapped: a kitsu-resolved title in the TheBeastLT
// table cross-references to its TMDB title so rating badges can be fetched.
func TestRatingsTargetFor_KitsuMapped(t *testing.T) {
	p := newCrossRefParams(t)
	kitsuID := uint64(3936)
	target, _ := p.ratingsTargetFor(&services.ResolvedID{KitsuID: &kitsuID, SourceProvider: "kitsu"})
	if target == nil || target.TMDbID != 278 {
		t.Fatalf("target = %+v, want TMDbID=278", target)
	}
}

// TestRatingsTargetFor_MALViaKitsuMapping: a mal-resolved title (no Kitsu id)
// goes MAL → Kitsu (/mappings) → IMDb → TMDB.
func TestRatingsTargetFor_MALViaKitsuMapping(t *testing.T) {
	p := newCrossRefParams(t)
	malID := uint64(5114)
	target, _ := p.ratingsTargetFor(&services.ResolvedID{MALID: &malID, SourceProvider: "mal"})
	if target == nil || target.TMDbID != 278 {
		t.Fatalf("target = %+v, want TMDbID=278", target)
	}
}

// TestRatingsTargetFor_NoMatch: no mapping anywhere → nil, so prepareRender
// skips ratings exactly as before.
func TestRatingsTargetFor_NoMatch(t *testing.T) {
	p := newCrossRefParams(t)
	kitsuID := uint64(1)
	malID := uint64(1)
	if target, _ := p.ratingsTargetFor(&services.ResolvedID{KitsuID: &kitsuID, SourceProvider: "kitsu"}); target != nil {
		t.Errorf("unmapped kitsu: target = %+v, want nil", target)
	}
	if target, _ := p.ratingsTargetFor(&services.ResolvedID{MALID: &malID, SourceProvider: "mal"}); target != nil {
		t.Errorf("unmapped mal: target = %+v, want nil", target)
	}
}

func strp(s string) *string { return &s }

func animeParams(t *testing.T, pref services.AnimeArtwork) ServeParams {
	t.Helper()
	p := newCrossRefParams(t)
	p.AniList = services.NewAniListClient(&http.Client{Transport: crossRefStub{}})
	s := services.DefaultRenderSettings()
	s.AnimeArtwork = pref
	p.Settings = &s
	return p
}

// malResolved / kitsuResolved mimic resolveMALCtx / resolveKitsuCtx output.
func malResolved() *services.ResolvedID {
	malID := uint64(5114)
	return &services.ResolvedID{MALID: &malID, SourceProvider: "mal",
		DirectPosterURL: strp("https://anilist.test/cover.jpg"), DirectCoverURL: strp("https://anilist.test/banner.jpg")}
}

func kitsuResolved() *services.ResolvedID {
	kitsuID, malID := uint64(3936), uint64(5114)
	return &services.ResolvedID{KitsuID: &kitsuID, MALID: &malID, SourceProvider: "kitsu",
		DirectPosterURL: strp("https://kitsu.test/poster.jpg"), DirectCoverURL: strp("https://kitsu.test/cover.jpg")}
}

// TestAnimeArtwork_PreferKitsuOnMAL: a mal: title switches to Kitsu's art
// (MAL → Kitsu via /mappings, then /anime/{id}); the input is not mutated.
func TestAnimeArtwork_PreferKitsuOnMAL(t *testing.T) {
	p := animeParams(t, services.AnimeArtworkKitsu)
	in := malResolved()
	out := p.applyAnimeArtworkPreference(in)
	if out.SourceProvider != "kitsu" || *out.DirectPosterURL != "https://kitsu.test/poster.jpg" || *out.DirectCoverURL != "https://kitsu.test/cover.jpg" {
		t.Fatalf("got provider=%s poster=%v cover=%v, want Kitsu art", out.SourceProvider, *out.DirectPosterURL, *out.DirectCoverURL)
	}
	if out.KitsuID == nil || *out.KitsuID != 3936 {
		t.Errorf("KitsuID = %v, want 3936", out.KitsuID)
	}
	if in.SourceProvider != "mal" || *in.DirectPosterURL != "https://anilist.test/cover.jpg" {
		t.Error("input ResolvedID was mutated (it may be the memcached resolver result)")
	}
}

// TestAnimeArtwork_PreferMALOnKitsu: a kitsu: title switches to AniList art.
func TestAnimeArtwork_PreferMALOnKitsu(t *testing.T) {
	p := animeParams(t, services.AnimeArtworkMAL)
	out := p.applyAnimeArtworkPreference(kitsuResolved())
	if out.SourceProvider != "mal" || *out.DirectPosterURL != "https://anilist.test/cover.jpg" || *out.DirectCoverURL != "https://anilist.test/banner.jpg" {
		t.Fatalf("got provider=%s poster=%v, want AniList art", out.SourceProvider, *out.DirectPosterURL)
	}
}

// TestAnimeArtwork_NoOpCases: match-the-ID, same provider, a toggle off, a
// failed lookup and normal (TMDB) titles all keep the input unchanged.
func TestAnimeArtwork_NoOpCases(t *testing.T) {
	cases := []struct {
		name string
		pref services.AnimeArtwork
		mod  func(*ServeParams)
		in   *services.ResolvedID
	}{
		{"match id on mal", services.AnimeArtworkMatchID, nil, malResolved()},
		{"prefer kitsu on kitsu", services.AnimeArtworkKitsu, nil, kitsuResolved()},
		{"toggle off", services.AnimeArtworkKitsu, func(p *ServeParams) { p.Settings.UseMAL = false }, malResolved()},
		{"normal title", services.AnimeArtworkKitsu, nil, &services.ResolvedID{TMDbID: 278}},
		{"kitsu lookup fails", services.AnimeArtworkKitsu, nil, func() *services.ResolvedID {
			r := malResolved()
			id := uint64(1) // not mapped by the stub
			r.MALID = &id
			return r
		}()},
	}
	for _, c := range cases {
		p := animeParams(t, c.pref)
		if c.mod != nil {
			c.mod(&p)
		}
		if out := p.applyAnimeArtworkPreference(c.in); out != c.in {
			t.Errorf("%s: expected the input unchanged, got %+v", c.name, out)
		}
	}
}

// TestFilterAnimeAlternates: kitsu/mal cross copies are written only when a
// request under that ID would render the same artwork.
func TestFilterAnimeAlternates(t *testing.T) {
	alts := []altID{{"kitsu", "3936"}, {"mal", "5114"}, {"imdb", "tt1"}}
	types := func(a []altID) string {
		var s []string
		for _, x := range a {
			s = append(s, x.idType)
		}
		return strings.Join(s, ",")
	}
	cases := []struct {
		name     string
		pref     services.AnimeArtwork
		useMAL   bool
		resolved *services.ResolvedID
		want     string
	}{
		{"match id, kitsu render", services.AnimeArtworkMatchID, true, kitsuResolved(), "kitsu,imdb"},
		{"match id, mal render", services.AnimeArtworkMatchID, true, malResolved(), "mal,imdb"},
		{"prefer kitsu, kitsu render", services.AnimeArtworkKitsu, true, kitsuResolved(), "kitsu,mal,imdb"},
		{"prefer kitsu but fell back to mal art", services.AnimeArtworkKitsu, true, malResolved(), "imdb"},
		{"prefer mal, mal render", services.AnimeArtworkMAL, true, malResolved(), "kitsu,mal,imdb"},
		{"mal toggle off, kitsu render", services.AnimeArtworkMatchID, false, kitsuResolved(), "kitsu,imdb"},
		{"normal title keeps all", services.AnimeArtworkMatchID, true, &services.ResolvedID{TMDbID: 278}, "kitsu,mal,imdb"},
	}
	for _, c := range cases {
		s := services.DefaultRenderSettings()
		s.AnimeArtwork = c.pref
		s.UseMAL = c.useMAL
		p := ServeParams{Settings: &s}
		if got := types(p.filterAnimeAlternates(c.resolved, alts)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWithTitleIdentity: every image of one title gets the same TitleKey so
// the admin list can group them; the input (memcached) is never mutated.
func TestWithTitleIdentity(t *testing.T) {
	p := animeParams(t, services.AnimeArtworkMatchID)

	tmdb := p.withTitleIdentity(&services.ResolvedID{TMDbID: 278, MediaType: services.MediaTypeMovie})
	if tmdb.TitleKey != "tmdb:movie-278" {
		t.Errorf("tmdb title: TitleKey %q, want tmdb:movie-278", tmdb.TitleKey)
	}

	// kitsu 3936 → tt0000278 (mapper) → TMDB movie 278: same key as above.
	in := kitsuResolved()
	in.ReleaseDate = nil
	k := p.withTitleIdentity(in)
	if k.TitleKey != "tmdb:movie-278" || k.RatingsTarget == nil || k.RatingsTarget.TMDbID != 278 {
		t.Errorf("mapped kitsu: TitleKey %q target %+v, want tmdb:movie-278 / 278", k.TitleKey, k.RatingsTarget)
	}
	if in.TitleKey != "" || in.RatingsTarget != nil {
		t.Error("input ResolvedID was mutated")
	}

	// mal 5114 → kitsu 3936 → same title.
	if m := p.withTitleIdentity(malResolved()); m.TitleKey != "tmdb:movie-278" {
		t.Errorf("mapped mal: TitleKey %q, want tmdb:movie-278", m.TitleKey)
	}

	// No cross-ref: fall back to the provider id.
	kitsuID, malID := uint64(1), uint64(1)
	if u := p.withTitleIdentity(&services.ResolvedID{KitsuID: &kitsuID, SourceProvider: "kitsu"}); u.TitleKey != "kitsu:1" || u.RatingsTarget != nil {
		t.Errorf("unmapped kitsu: TitleKey %q target %v, want kitsu:1 / nil", u.TitleKey, u.RatingsTarget)
	}
	if u := p.withTitleIdentity(&services.ResolvedID{MALID: &malID, SourceProvider: "mal"}); u.TitleKey != "mal:1" {
		t.Errorf("unmapped mal: TitleKey %q, want mal:1", u.TitleKey)
	}
}

// TestKitsuMALReleaseDate: resolvers carry the entry's own start date.
func TestKitsuMALReleaseDate(t *testing.T) {
	httpClient := &http.Client{Transport: crossRefStub{}}
	clients := services.IDClients{TMDB: services.NewTmdbClient("k", httpClient), Kitsu: services.NewKitsuClient(httpClient), AniList: services.NewAniListClient(httpClient)}
	for _, c := range []struct {
		idType services.IDType
		value  string
	}{{services.IDTypeKitsu, "3936"}, {services.IDTypeMAL, "5114"}} {
		r, err := services.ResolveIDCtx(context.Background(), c.idType, c.value, clients)
		if err != nil {
			t.Fatalf("%v %s: %v", c.idType, c.value, err)
		}
		if r.ReleaseDate == nil || *r.ReleaseDate != "2009-04-05" {
			t.Errorf("%v %s: ReleaseDate %v, want 2009-04-05", c.idType, c.value, r.ReleaseDate)
		}
	}
}

// TestResolverTitles: every resolver keeps the display name it already gets
// (TMDB title/name, Kitsu canonicalTitle, AniList English → romaji), and a
// Kitsu/MAL entry without one borrows the cross-referenced TMDB title.
func TestResolverTitles(t *testing.T) {
	httpClient := &http.Client{Transport: crossRefStub{}}
	clients := services.IDClients{TMDB: services.NewTmdbClient("k", httpClient), Kitsu: services.NewKitsuClient(httpClient), AniList: services.NewAniListClient(httpClient)}
	for _, c := range []struct {
		idType services.IDType
		value  string
		want   string
	}{
		{services.IDTypeIMDB, "tt0111161", "The Shawshank Redemption"},
		{services.IDTypeTMDB, "movie-278", "Stub Title"},
		{services.IDTypeKitsu, "3936", "FMA:B"},
		{services.IDTypeMAL, "5114", "FMA:B"}, // no English title in the stub → romaji
	} {
		r, err := services.ResolveIDCtx(context.Background(), c.idType, c.value, clients)
		if err != nil {
			t.Fatalf("%v %s: %v", c.idType, c.value, err)
		}
		if r.Title == nil || *r.Title != c.want {
			t.Errorf("%v %s: Title %v, want %q", c.idType, c.value, r.Title, c.want)
		}
	}

	p := animeParams(t, services.AnimeArtworkMatchID)
	in := kitsuResolved() // no Title set
	if out := p.withTitleIdentity(in); out.Title == nil || *out.Title != "The Shawshank Redemption" {
		t.Errorf("cross-ref title fallback: got %v, want the TMDB title", out.Title)
	}
}

// TestKitsuSlugTranslatesWithToggleOff: with "Use Kitsu artwork" off, a
// Kitsu slug is translated to its IMDb/TMDB title like a numeric id
// (previously only numeric ids translated; slugs fell back to Kitsu art).
func TestKitsuSlugTranslatesWithToggleOff(t *testing.T) {
	p := newCrossRefParams(t)
	clients := services.IDClients{TMDB: p.TMDB, Kitsu: p.Kitsu, KitsuIMDbMapper: p.KitsuIMDbMapper}
	for _, id := range []string{"3936", "fmab"} {
		r, err := resolveKitsuMalCtx(context.Background(), nil, services.IDTypeKitsu, id, clients, false, true)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if r.SourceProvider != "" || r.TMDbID != 278 {
			t.Errorf("%s: got provider=%q tmdb=%d, want the TMDB title 278", id, r.SourceProvider, r.TMDbID)
		}
	}
}
