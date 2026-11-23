package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	apperr "openposterdb/internal/errors"
)

type IDType int

const (
	IDTypeIMDB IDType = iota
	IDTypeTMDB
	IDTypeTVDB
	IDTypeKitsu
	IDTypeMAL
)

func (t IDType) String() string {
	switch t {
	case IDTypeIMDB:
		return "imdb"
	case IDTypeTMDB:
		return "tmdb"
	case IDTypeTVDB:
		return "tvdb"
	case IDTypeKitsu:
		return "kitsu"
	case IDTypeMAL:
		return "mal"
	}
	return ""
}

func ParseIDType(s string) (IDType, error) {
	switch s {
	case "imdb":
		return IDTypeIMDB, nil
	case "tmdb":
		return IDTypeTMDB, nil
	case "tvdb":
		return IDTypeTVDB, nil
	case "kitsu":
		return IDTypeKitsu, nil
	case "mal":
		return IDTypeMAL, nil
	default:
		return 0, apperr.NewInvalidIDType(s)
	}
}

type MediaType int

const (
	MediaTypeMovie MediaType = iota
	MediaTypeTV
	MediaTypeEpisode
)

func (m MediaType) String() string {
	switch m {
	case MediaTypeMovie:
		return "movie"
	case MediaTypeTV:
		return "tv"
	case MediaTypeEpisode:
		return "episode"
	}
	return ""
}

// KitsuSubtypeTV / KitsuSubtypeMovie / KitsuSubtypeOVA etc. cover the Kitsu
// anime.subtype enum ("TV" / "Movie" / "OVA" / "ONA" / "Special" / "Music").
// Defined here so id.go can translate without importing kitsu.go's private
// types — keeping kitsu.go the sole owner of KitsuAnime's full shape.
const (
	KitsuSubtypeTV      = "TV"
	KitsuSubtypeMovie   = "Movie"
	KitsuSubtypeOVA     = "OVA"
	KitsuSubtypeONA     = "ONA"
	KitsuSubtypeSpecial = "Special"
	KitsuSubtypeMusic   = "Music"
)

type EpisodeInfo struct {
	ShowTMDbID    uint64
	SeasonNumber  uint32
	EpisodeNumber uint32
	StillPath     *string
}

// ResolvedID carries everything the image-rendering pipeline needs after
// a single ID resolve. KitsuID / MALID are populated when the resolver
// pulled from a non-TMDB source, so the cross-id cache write in
// image/crossid.go can write alternate forms. DirectPosterURL and
// DirectCoverURL carry Kitsu's posterImage.original / coverImage.original
// or AniList's coverImage.extraLarge / bannerImage — when populated, the
// art fetch path skips TMDB entirely for that request.
type ResolvedID struct {
	IMDbID       *string
	TMDbID       uint64
	TVDBID       *uint64
	KitsuID      *uint64
	MALID        *uint64
	MediaType    MediaType
	PosterPath   *string
	BackdropPath *string // TMDB default backdrop, the last backdrop fallback
	// OriginalLanguage is TMDB's original_language ("ja", "ko", …): one of
	// the artwork selection tiers.
	OriginalLanguage string
	ReleaseDate      *string
	Title            *string // display name from the resolving source (image_meta.title)
	DirectPosterURL  *string
	DirectCoverURL   *string
	SourceProvider   string // "" for TMDB/IMDB/TVDB, "kitsu" or "mal" for the new sources

	// Set per request by image.ServeParams.withTitleIdentity on a copy (the
	// resolver result itself is memcached and never carries them):
	// TitleKey groups every cached image of one title in the admin list
	// (image_meta.title_key); RatingsTarget is the IMDb/TMDB title a
	// Kitsu/MAL-resolved title was cross-referenced to for ratings.
	TitleKey      string
	RatingsTarget *ResolvedID
	Episode       *EpisodeInfo
}

// IDClients bundles the per-source resolver clients. TMDB is required (the
// existing pipeline needs it for IMDb / TMDB / TVDB resolution and for
// poster_path / backdrop_path fetches); Kitsu and AniList are optional —
// when nil, the resolver returns NewOther for that source, and the
// fallback chain in image/serve.go simply moves on to the next source.
// Mappings (the offline id lists) and TVDB are optional too: when nil, the
// steps that use them are skipped.
type IDClients struct {
	TMDB     *TmdbClient
	Kitsu    *KitsuClient
	AniList  *AniListClient
	Mappings *IDMappings
	TVDB     *TVDBClient
}

func FormatTMDbIDValue(tmdbID uint64, mediaType MediaType, episode *EpisodeInfo) string {
	switch mediaType {
	case MediaTypeMovie:
		return fmt.Sprintf("movie-%d", tmdbID)
	case MediaTypeTV:
		return fmt.Sprintf("series-%d", tmdbID)
	case MediaTypeEpisode:
		if episode != nil {
			return fmt.Sprintf("episode-%d-S%dE%d", episode.ShowTMDbID, episode.SeasonNumber, episode.EpisodeNumber)
		}
		return fmt.Sprintf("series-%d", tmdbID)
	}
	return fmt.Sprintf("series-%d", tmdbID)
}

type findEntry struct {
	ID               uint64  `json:"id"`
	Title            *string `json:"title"` // movies
	Name             *string `json:"name"`  // TV
	PosterPath       *string `json:"poster_path"`
	BackdropPath     *string `json:"backdrop_path"`
	OriginalLanguage string  `json:"original_language"`
	ReleaseDate      *string `json:"release_date"`
	FirstAir         *string `json:"first_air_date"`
	Popularity       float64 `json:"popularity"`
}

// tvDetails is the part of /tv/{id}?append_to_response=external_ids the
// resolvers read.
type tvDetails struct {
	PosterPath       *string `json:"poster_path"`
	BackdropPath     *string `json:"backdrop_path"`
	OriginalLanguage string  `json:"original_language"`
	FirstAir         *string `json:"first_air_date"`
	Name             *string `json:"name"`
	ExternalIDs      *struct {
		IMDbID *string `json:"imdb_id"`
		TVDBID *uint64 `json:"tvdb_id"`
	} `json:"external_ids"`
}

func (d *tvDetails) tvdbID() *uint64 {
	if d.ExternalIDs != nil && d.ExternalIDs.TVDBID != nil && *d.ExternalIDs.TVDBID != 0 {
		return d.ExternalIDs.TVDBID
	}
	return nil
}

func (d *tvDetails) imdbID() *string {
	if d.ExternalIDs != nil && d.ExternalIDs.IMDbID != nil && *d.ExternalIDs.IMDbID != "" {
		return d.ExternalIDs.IMDbID
	}
	return nil
}

func getTVDetailsCtx(ctx context.Context, tmdb *TmdbClient, id uint64) (*tvDetails, error) {
	var d tvDetails
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/tv/%d", id), map[string]string{"append_to_response": "external_ids"}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// firstPath returns the first non-empty image path.
func firstPath(paths ...*string) *string {
	for _, p := range paths {
		if p != nil && *p != "" {
			return p
		}
	}
	return nil
}

type episodeFindEntry struct {
	ShowID        uint64  `json:"show_id"`
	SeasonNumber  uint32  `json:"season_number"`
	EpisodeNumber uint32  `json:"episode_number"`
	StillPath     *string `json:"still_path"`
}

type findResult struct {
	MovieResults     []findEntry        `json:"movie_results"`
	TVResults        []findEntry        `json:"tv_results"`
	TVEpisodeResults []episodeFindEntry `json:"tv_episode_results"`
}

func ResolveIDCtx(ctx context.Context, idType IDType, idValue string, clients IDClients) (*ResolvedID, error) {
	switch idType {
	case IDTypeIMDB:
		r, err := resolveIMDBCtx(ctx, idValue, clients.TMDB)
		return backfillSeriesTVDB(r, clients.Mappings), err
	case IDTypeTMDB:
		r, err := resolveTMDBCtx(ctx, idValue, clients.TMDB)
		return backfillSeriesTVDB(r, clients.Mappings), err
	case IDTypeTVDB:
		return resolveTVDBWithFallbacksCtx(ctx, idValue, clients)
	case IDTypeKitsu:
		if clients.Kitsu == nil {
			return nil, apperr.NewOther("Kitsu client not configured")
		}
		return resolveKitsuCtx(ctx, idValue, clients.Kitsu)
	case IDTypeMAL:
		if clients.AniList == nil {
			return nil, apperr.NewOther("AniList client not configured")
		}
		return resolveMALCtx(ctx, idValue, clients.AniList)
	}
	return nil, apperr.NewInvalidIDType(idType.String())
}

func ResolveID(idType IDType, idValue string, clients IDClients) (*ResolvedID, error) {
	return ResolveIDCtx(context.Background(), idType, idValue, clients)
}

// idNotFoundTTL is how long a definite "unknown id" (404) is remembered, so a
// media server re-requesting an id TMDB doesn't know doesn't hit /find each
// time, while a title added upstream later is picked up within minutes.
const idNotFoundTTL = 10 * time.Minute

// idMiss is a negative ID-cache entry; it carries its own, shorter expiry.
type idMiss struct {
	err   error
	until time.Time
}

// ResolveIDCachedCtx resolves an ID, caching successful results in the given
// in-memory cache (key "idtype/idvalue", matching the Rust id_cache). A nil
// cache bypasses caching. A 404 (id not found) is cached for idNotFoundTTL;
// every other error is never cached, so a transient TMDB failure doesn't
// poison the cache.
func ResolveIDCachedCtx(ctx context.Context, cache *MemCache, idType IDType, idValue string, clients IDClients) (*ResolvedID, error) {
	if cache == nil {
		return ResolveIDCtx(ctx, idType, idValue, clients)
	}
	key := idType.String() + "/" + idValue
	flightKey := fmt.Sprintf("%p|%s", cache, key)
	if v, ok := cache.Get(key); ok {
		switch e := v.(type) {
		case idEntry:
			if time.Since(e.fetched) <= staleServeMax {
				if time.Now().After(e.refreshAfter) {
					refreshIDInBackground(cache, key, flightKey, idType, idValue, clients)
				}
				return withLearnedIDs(e.resolved), nil
			}
			cache.Delete(key) // past the hard cap: resolve synchronously
		case idMiss:
			if time.Now().Before(e.until) {
				return nil, e.err
			}
			cache.Delete(key)
		}
	}
	// Concurrent misses for the same id share one upstream lookup.
	resolved, err := idFlight.do(ctx, flightKey, func(fctx context.Context) (*ResolvedID, error) {
		resolved, err := ResolveIDCtx(fctx, idType, idValue, clients)
		if err == nil {
			now := time.Now()
			cache.Set(key, idEntry{resolved: resolved, refreshAfter: now.Add(idRefreshAfter), fetched: now}, 1)
		} else if isIDNotFound(err) {
			cache.Set(key, idMiss{err: err, until: time.Now().Add(idNotFoundTTL)}, 1)
		}
		return resolved, err
	})
	if err != nil {
		return nil, err
	}
	return withLearnedIDs(resolved), nil
}

// idRefreshAfter is the age after which a cached resolution is refreshed in
// the background (it is still served meanwhile); the cache's TTL is the hard
// limit.
var idRefreshAfter = time.Hour

type idEntry struct {
	resolved     *ResolvedID
	refreshAfter time.Time
	fetched      time.Time // last successful resolve; staleServeMax counts from here
}

// staleServeMax caps how long a value is served after its last successful
// fetch while refreshes keep failing (MemCache.Set resets the cache's own
// TTL on every re-store, so the cap is tracked in the entry).
var staleServeMax = 24 * time.Hour

var idRefreshing sync.Map // flight key -> struct{}

// refreshIDInBackground re-resolves an id whose cached entry is past its
// refresh age. Only a success replaces the entry; a failure keeps the old
// resolution (retried after a short delay rather than on every request).
func refreshIDInBackground(cache *MemCache, key, flightKey string, idType IDType, idValue string, clients IDClients) {
	if _, running := idRefreshing.LoadOrStore(flightKey, struct{}{}); running {
		return
	}
	go func() {
		defer idRefreshing.Delete(flightKey)
		idFlight.do(context.Background(), flightKey, func(fctx context.Context) (*ResolvedID, error) {
			resolved, err := ResolveIDCtx(fctx, idType, idValue, clients)
			now := time.Now()
			if err == nil {
				cache.Set(key, idEntry{resolved: resolved, refreshAfter: now.Add(idRefreshAfter), fetched: now}, 1)
				return resolved, nil
			}
			if isIDNotFound(err) {
				// A definitive answer: the id is gone or remapped upstream.
				cache.Set(key, idMiss{err: err, until: now.Add(idNotFoundTTL)}, 1)
				return nil, err
			}
			// Transient failure: keep the old resolution (retry soon), but
			// only up to staleServeMax after its last successful fetch.
			if v, ok := cache.Get(key); ok {
				if e, ok := v.(idEntry); ok {
					if now.Sub(e.fetched) <= staleServeMax {
						cache.Set(key, idEntry{resolved: e.resolved, refreshAfter: now.Add(idNotFoundTTL), fetched: e.fetched}, 1)
					} else {
						cache.Delete(key)
					}
				}
			}
			return nil, err
		})
	}()
}

var idFlight flightGroup[*ResolvedID]

func isIDNotFound(err error) bool {
	var appErr *apperr.AppError
	return stderrors.As(err, &appErr) && appErr.Status == http.StatusNotFound
}

// ResolveIDCached resolves an ID, caching successful results in the given
// in-memory cache (key "idtype/idvalue", matching the Rust id_cache). A nil
// cache bypasses caching. Errors are never cached, so a transient TMDB failure
// doesn't poison the cache.
func ResolveIDCached(cache *MemCache, idType IDType, idValue string, clients IDClients) (*ResolvedID, error) {
	return ResolveIDCachedCtx(context.Background(), cache, idType, idValue, clients)
}

func resolveIMDBCtx(ctx context.Context, imdbID string, tmdb *TmdbClient) (*ResolvedID, error) {
	if rest, ok := strings.CutPrefix(imdbID, "episode-"); ok {
		return resolveIMDBEpisodeCtx(ctx, rest, imdbID, tmdb)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, "/find/"+neturl.PathEscape(imdbID), map[string]string{"external_source": "imdb_id"}, &result); err != nil {
		return nil, err
	}

	if len(result.TVEpisodeResults) > 0 {
		ep := result.TVEpisodeResults[0]
		return resolveEpisodeDetailsCtx(ctx, tmdb, ep.ShowID, ep.SeasonNumber, ep.EpisodeNumber, ep.StillPath, &imdbID)
	}

	bestMovie := findBestEntry(result.MovieResults)
	bestTV := findBestEntry(result.TVResults)

	pickMovie := bestMovie != nil && (bestTV == nil || bestMovie.Popularity >= bestTV.Popularity)
	if pickMovie && bestMovie != nil {
		return &ResolvedID{
			IMDbID:           &imdbID,
			TMDbID:           bestMovie.ID,
			MediaType:        MediaTypeMovie,
			PosterPath:       bestMovie.PosterPath,
			BackdropPath:     bestMovie.BackdropPath,
			OriginalLanguage: bestMovie.OriginalLanguage,
			ReleaseDate:      bestMovie.ReleaseDate,
			Title:            titleOf(bestMovie.Title, bestMovie.Name),
		}, nil
	}
	if bestTV != nil {
		// /find has no TVDB id; fanart.tv indexes series by it, so it is
		// resolved here once (and cached with the id) instead of per render.
		// The series' tvdb/ alternate cache key comes from it too.
		details, err := getTVDetailsCtx(ctx, tmdb, bestTV.ID)
		if err != nil {
			return nil, err // not cached; a partial result would be
		}
		return &ResolvedID{
			IMDbID:           &imdbID,
			TMDbID:           bestTV.ID,
			TVDBID:           details.tvdbID(),
			MediaType:        MediaTypeTV,
			PosterPath:       firstPath(bestTV.PosterPath, details.PosterPath),
			BackdropPath:     firstPath(bestTV.BackdropPath, details.BackdropPath),
			OriginalLanguage: firstNonEmptyString(bestTV.OriginalLanguage, details.OriginalLanguage),
			ReleaseDate:      bestTV.FirstAir,
			Title:            titleOf(bestTV.Name, bestTV.Title),
		}, nil
	}

	return nil, apperr.NewIDNotFound(fmt.Sprintf("%s (not found on TMDB)", imdbID))
}

func resolveTMDBCtx(ctx context.Context, idValue string, tmdb *TmdbClient) (*ResolvedID, error) {
	if rest, ok := strings.CutPrefix(idValue, "episode-"); ok {
		return resolveTMDBEpisodeCtx(ctx, rest, idValue, tmdb)
	}

	var mediaType MediaType
	var tmdbID uint64
	var err error

	if rest, ok := strings.CutPrefix(idValue, "movie-"); ok {
		mediaType = MediaTypeMovie
		tmdbID, err = strconv.ParseUint(rest, 10, 64)
	} else if rest, ok := strings.CutPrefix(idValue, "series-"); ok {
		mediaType = MediaTypeTV
		tmdbID, err = strconv.ParseUint(rest, 10, 64)
	} else {
		return nil, apperr.NewInvalidIDType("tmdb id must be prefixed with movie-, series-, or episode-: " + idValue)
	}
	if err != nil {
		return nil, apperr.NewInvalidIDType(idValue)
	}

	type tmdbExternalIDs struct {
		IMDbID *string `json:"imdb_id"`
		TVDBID *uint64 `json:"tvdb_id"`
	}
	type tmdbDetails struct {
		IMDbID           *string          `json:"imdb_id"`
		PosterPath       *string          `json:"poster_path"`
		BackdropPath     *string          `json:"backdrop_path"`
		OriginalLanguage string           `json:"original_language"`
		ReleaseDate      *string          `json:"release_date"`
		FirstAir         *string          `json:"first_air_date"`
		Title            *string          `json:"title"`
		Name             *string          `json:"name"`
		ExternalIDs      *tmdbExternalIDs `json:"external_ids"`
	}

	var path string
	switch mediaType {
	case MediaTypeMovie:
		path = fmt.Sprintf("/movie/%d", tmdbID)
	case MediaTypeTV:
		path = fmt.Sprintf("/tv/%d", tmdbID)
	}

	params := map[string]string{}
	if mediaType == MediaTypeTV {
		params["append_to_response"] = "external_ids"
	}

	var details tmdbDetails
	if err := tmdb.GetCtx(ctx, path, params, &details); err != nil {
		return nil, err
	}

	var imdbID *string
	if details.IMDbID != nil && *details.IMDbID != "" {
		imdbID = details.IMDbID
	} else if details.ExternalIDs != nil && details.ExternalIDs.IMDbID != nil && *details.ExternalIDs.IMDbID != "" {
		imdbID = details.ExternalIDs.IMDbID
	}

	var tvdbID *uint64
	if details.ExternalIDs != nil && details.ExternalIDs.TVDBID != nil && *details.ExternalIDs.TVDBID != 0 {
		tvdbID = details.ExternalIDs.TVDBID
	}

	var releaseDate *string
	switch mediaType {
	case MediaTypeMovie:
		releaseDate = details.ReleaseDate
	case MediaTypeTV:
		releaseDate = details.FirstAir
	default:
		releaseDate = details.ReleaseDate
	}

	return &ResolvedID{
		IMDbID:           imdbID,
		TMDbID:           tmdbID,
		TVDBID:           tvdbID,
		MediaType:        mediaType,
		PosterPath:       details.PosterPath,
		BackdropPath:     details.BackdropPath,
		OriginalLanguage: details.OriginalLanguage,
		ReleaseDate:      releaseDate,
		Title:            titleOf(details.Title, details.Name),
	}, nil
}

func resolveTVDBCtx(ctx context.Context, tvdbIDVal string, tmdb *TmdbClient) (*ResolvedID, error) {
	if rest, ok := strings.CutPrefix(tvdbIDVal, "episode-"); ok {
		return resolveTVDBEpisodeCtx(ctx, rest, tvdbIDVal, tmdb)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, "/find/"+neturl.PathEscape(tvdbIDVal), map[string]string{"external_source": "tvdb_id"}, &result); err != nil {
		return nil, err
	}

	if len(result.TVEpisodeResults) > 0 {
		ep := result.TVEpisodeResults[0]
		return resolveEpisodeDetailsCtx(ctx, tmdb, ep.ShowID, ep.SeasonNumber, ep.EpisodeNumber, ep.StillPath, nil)
	}

	tvdbNum, _ := strconv.ParseUint(tvdbIDVal, 10, 64)
	var requestedTVDB *uint64
	if tvdbNum != 0 {
		requestedTVDB = &tvdbNum
	}

	if len(result.TVResults) > 0 {
		tv := result.TVResults[0]
		details, err := getTVDetailsCtx(ctx, tmdb, tv.ID)
		if err != nil {
			return nil, err // not cached; a partial result would be
		}
		return &ResolvedID{
			IMDbID:           details.imdbID(),
			TMDbID:           tv.ID,
			TVDBID:           requestedTVDB,
			MediaType:        MediaTypeTV,
			PosterPath:       firstPath(details.PosterPath, tv.PosterPath),
			BackdropPath:     firstPath(details.BackdropPath, tv.BackdropPath),
			OriginalLanguage: firstNonEmptyString(details.OriginalLanguage, tv.OriginalLanguage),
			ReleaseDate:      details.FirstAir,
			Title:            titleOf(details.Name, tv.Name),
		}, nil
	}

	if len(result.MovieResults) > 0 {
		movie := result.MovieResults[0]
		var details struct {
			IMDbID           *string `json:"imdb_id"`
			PosterPath       *string `json:"poster_path"`
			BackdropPath     *string `json:"backdrop_path"`
			OriginalLanguage string  `json:"original_language"`
			ReleaseDate      *string `json:"release_date"`
			Title            *string `json:"title"`
		}
		if err := tmdb.GetCtx(ctx, fmt.Sprintf("/movie/%d", movie.ID), nil, &details); err != nil {
			return nil, err // not cached; a partial result would be
		}

		return &ResolvedID{
			IMDbID:           details.IMDbID,
			TMDbID:           movie.ID,
			TVDBID:           requestedTVDB, // the id this title was requested by
			MediaType:        MediaTypeMovie,
			PosterPath:       firstPath(details.PosterPath, movie.PosterPath),
			BackdropPath:     firstPath(details.BackdropPath, movie.BackdropPath),
			OriginalLanguage: firstNonEmptyString(details.OriginalLanguage, movie.OriginalLanguage),
			ReleaseDate:      firstPath(details.ReleaseDate, movie.ReleaseDate),
			Title:            titleOf(details.Title, movie.Title),
		}, nil
	}

	return nil, apperr.NewIDNotFound(fmt.Sprintf("%s (not found on TMDB via TVDB lookup)", tvdbIDVal))
}

// resolveTVDBWithFallbacksCtx resolves a tvdb: id: first through the
// offline id lists (no /find call), then TMDB /find, and when /find knows
// nothing, through the TVDB API (its record's IMDb / TMDB cross-links, else
// the TVDB record itself).
func resolveTVDBWithFallbacksCtx(ctx context.Context, idValue string, clients IDClients) (*ResolvedID, error) {
	if strings.HasPrefix(idValue, "episode-") {
		return resolveTVDBCtx(ctx, idValue, clients.TMDB)
	}
	tvdbNum, parseErr := strconv.ParseUint(idValue, 10, 64)
	if parseErr == nil && tvdbNum != 0 && clients.Mappings != nil {
		for _, kind := range []MediaType{MediaTypeTV, MediaTypeMovie} {
			if e, ok := clients.Mappings.Lookup(kind, IDTypeTVDB, idValue); ok {
				if r := resolveMappedCtx(ctx, clients.TMDB, kind, e); r != nil {
					r.TVDBID = &tvdbNum
					return r, nil
				}
			}
		}
	}
	r, err := resolveTVDBCtx(ctx, idValue, clients.TMDB)
	if err == nil {
		return backfillSeriesTVDB(r, clients.Mappings), nil
	}
	if !isIDNotFound(err) || clients.TVDB == nil || parseErr != nil || tvdbNum == 0 {
		return nil, err
	}
	viaTVDB, tvdbErr := resolveViaTVDBRecordCtx(ctx, tvdbNum, clients)
	if tvdbErr != nil {
		if isIDNotFound(tvdbErr) {
			return nil, err // TVDB doesn't know it either: the /find answer stands
		}
		slog.Info("TVDB lookup failed", "tvdb_id", tvdbNum, "error", tvdbErr)
		return nil, tvdbErr // transient: not cached as a miss
	}
	return viaTVDB, nil
}

// resolveMappedCtx resolves a title of the given kind through the TMDB (else
// IMDb) id the mapping lists or TVDB cross-link it to. nil when that fails.
func resolveMappedCtx(ctx context.Context, tmdb *TmdbClient, kind MediaType, e IDMap) *ResolvedID {
	if tmdb == nil {
		return nil
	}
	if e.TMDB != 0 {
		if r, err := resolveTMDBCtx(ctx, FormatTMDbIDValue(e.TMDB, kind, nil), tmdb); err == nil {
			if r.IMDbID == nil && e.IMDb != "" {
				imdb := e.IMDb
				r.IMDbID = &imdb
			}
			return r
		}
	}
	if e.IMDb != "" {
		if r, err := resolveIMDBCtx(ctx, e.IMDb, tmdb); err == nil && r.MediaType == kind {
			return r
		}
	}
	return nil
}

// resolveViaTVDBRecordCtx looks a TVDB id up on TVDB itself (series first,
// as TMDB /find does, then movies). A record cross-linked to a title TMDB
// knows resolves to that title; otherwise the TVDB record is served on its
// own (TMDbID 0: artwork from Fanart / TVDB, ratings by IMDb id if any).
func resolveViaTVDBRecordCtx(ctx context.Context, tvdbNum uint64, clients IDClients) (*ResolvedID, error) {
	rec, err := clients.TVDB.SeriesCtx(ctx, tvdbNum)
	if isIDNotFound(err) {
		rec, err = clients.TVDB.MovieCtx(ctx, tvdbNum)
	}
	if err != nil {
		return nil, err
	}
	kind := MediaTypeTV
	if rec.IsMovie {
		kind = MediaTypeMovie
	}
	if r := resolveMappedCtx(ctx, clients.TMDB, kind, IDMap{IMDb: rec.IMDbID, TMDB: rec.TMDbID}); r != nil {
		slog.Info("tvdb id not found by TMDB /find; resolved through its TVDB cross-link", "tvdb_id", tvdbNum, "tmdb_id", r.TMDbID)
		r.TVDBID = &tvdbNum
		return r, nil
	}
	out := &ResolvedID{TVDBID: &tvdbNum, MediaType: kind, Title: titleOf(&rec.Name, nil)}
	if rec.IMDbID != "" {
		imdb := rec.IMDbID
		out.IMDbID = &imdb
	}
	if rec.ReleaseDate != "" {
		d := rec.ReleaseDate
		out.ReleaseDate = &d
	}
	slog.Info("tvdb id not on TMDB; serving it from TVDB", "tvdb_id", tvdbNum, "kind", kind.String())
	return out, nil
}

// backfillSeriesTVDB fills a freshly resolved series' missing TVDB id from
// the offline lists (Fanart indexes series by TVDB id only).
func backfillSeriesTVDB(r *ResolvedID, m *IDMappings) *ResolvedID {
	if r == nil || m == nil || r.MediaType != MediaTypeTV || (r.TVDBID != nil && *r.TVDBID != 0) {
		return r
	}
	if id := m.SeriesTVDB(r.IMDbID, r.TMDbID); id != 0 {
		r.TVDBID = &id
	}
	return r
}

func resolveEpisodeDetailsCtx(ctx context.Context, tmdb *TmdbClient, showID uint64, season, episode uint32, hintStillPath, hintIMDbID *string) (*ResolvedID, error) {
	type epDetails struct {
		StillPath   *string `json:"still_path"`
		AirDate     *string `json:"air_date"`
		Name        *string `json:"name"`
		ExternalIDs *struct {
			IMDbID *string `json:"imdb_id"`
			TVDBID *uint64 `json:"tvdb_id"`
		} `json:"external_ids"`
	}

	var details epDetails
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", showID, season, episode)
	if err := tmdb.GetCtx(ctx, path, map[string]string{"append_to_response": "external_ids"}, &details); err != nil {
		return nil, err
	}

	imdbID := hintIMDbID
	if imdbID == nil {
		if details.ExternalIDs != nil && details.ExternalIDs.IMDbID != nil && *details.ExternalIDs.IMDbID != "" {
			imdbID = details.ExternalIDs.IMDbID
		}
	}

	var tvdbID *uint64
	if details.ExternalIDs != nil {
		tvdbID = details.ExternalIDs.TVDBID
	}

	stillPath := hintStillPath
	if stillPath == nil {
		stillPath = details.StillPath
	}

	posterPath := stillPath
	if posterPath == nil {
		var show struct {
			PosterPath *string `json:"poster_path"`
		}
		if err := tmdb.GetCtx(ctx, fmt.Sprintf("/tv/%d", showID), nil, &show); err == nil {
			posterPath = show.PosterPath
		}
	}

	return &ResolvedID{
		IMDbID:      imdbID,
		TMDbID:      showID,
		TVDBID:      tvdbID,
		MediaType:   MediaTypeEpisode,
		PosterPath:  posterPath,
		ReleaseDate: details.AirDate,
		Title:       titleOf(details.Name, nil),
		Episode: &EpisodeInfo{
			ShowTMDbID:    showID,
			SeasonNumber:  season,
			EpisodeNumber: episode,
			StillPath:     stillPath,
		},
	}, nil
}

func resolveIMDBEpisodeCtx(ctx context.Context, rest, fullID string, tmdb *TmdbClient) (*ResolvedID, error) {
	seriesIMDbID, season, episode := parseEpisodeExternal(rest, fullID)
	if seriesIMDbID == "" {
		return nil, apperr.NewInvalidIDType(fullID)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, "/find/"+neturl.PathEscape(seriesIMDbID), map[string]string{"external_source": "imdb_id"}, &result); err != nil {
		return nil, err
	}

	if len(result.TVResults) == 0 {
		return nil, apperr.NewIDNotFound(fmt.Sprintf("%s (not found as a TV series on TMDB)", seriesIMDbID))
	}

	return resolveEpisodeDetailsCtx(ctx, tmdb, result.TVResults[0].ID, season, episode, nil, nil)
}

func resolveTMDBEpisodeCtx(ctx context.Context, rest, fullID string, tmdb *TmdbClient) (*ResolvedID, error) {
	upper := strings.ToUpper(rest)
	idx := strings.Index(upper, "-S")
	if idx < 0 {
		return nil, apperr.NewInvalidIDType(fullID)
	}
	idStr := rest[:idx]
	if idStr == "" {
		return nil, apperr.NewInvalidIDType(fullID)
	}
	showID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return nil, apperr.NewInvalidIDType(fullID)
	}

	season, episode, ok := parseSeasonEpisode(upper[idx+2:])
	if !ok {
		return nil, apperr.NewInvalidIDType(fullID)
	}
	return resolveEpisodeDetailsCtx(ctx, tmdb, showID, season, episode, nil, nil)
}

// Upper bounds for season/episode numbers in an episode id; anything larger
// is a malformed id, not a TMDB lookup.
const (
	maxSeasonNumber  = 10000
	maxEpisodeNumber = 100000
)

// parseSeasonEpisode parses "<season>E<episode>" (the part after "-S").
func parseSeasonEpisode(se string) (uint32, uint32, bool) {
	parts := strings.SplitN(se, "E", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	season, err1 := strconv.ParseUint(parts[0], 10, 32)
	episode, err2 := strconv.ParseUint(parts[1], 10, 32)
	if err1 != nil || err2 != nil || season > maxSeasonNumber || episode > maxEpisodeNumber {
		return 0, 0, false
	}
	return uint32(season), uint32(episode), true
}

func resolveTVDBEpisodeCtx(ctx context.Context, rest, fullID string, tmdb *TmdbClient) (*ResolvedID, error) {
	seriesTVDBID, season, episode := parseEpisodeExternal(rest, fullID)
	if seriesTVDBID == "" {
		return nil, apperr.NewInvalidIDType(fullID)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, "/find/"+neturl.PathEscape(seriesTVDBID), map[string]string{"external_source": "tvdb_id"}, &result); err != nil {
		return nil, err
	}

	if len(result.TVResults) == 0 {
		return nil, apperr.NewIDNotFound(fmt.Sprintf("%s (not found as a TV series on TMDB via TVDB lookup)", seriesTVDBID))
	}

	return resolveEpisodeDetailsCtx(ctx, tmdb, result.TVResults[0].ID, season, episode, nil, nil)
}

func parseEpisodeExternal(rest, idValue string) (string, uint32, uint32) {
	upper := strings.ToUpper(rest)
	idx := strings.Index(upper, "-S")
	if idx < 0 {
		return "", 0, 0
	}
	externalID := rest[:idx]
	if externalID == "" {
		return "", 0, 0
	}

	season, episode, ok := parseSeasonEpisode(upper[idx+2:])
	if !ok {
		return "", 0, 0
	}
	return externalID, season, episode
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// knownTVDBIDs remembers TVDB ids learned outside the ID resolver (the
// MDBList "ids" block) for series whose TMDB entry has none, keyed by TMDB
// id. Fanart.tv indexes series by TVDB id only.
var knownTVDBIDs = NewMemCache(0, 50000, 7*24*time.Hour, 0)

// RememberTVDBID records a series' TVDB id learned from another provider.
func RememberTVDBID(tmdbID, tvdbID uint64) {
	if tmdbID != 0 && tvdbID != 0 {
		knownTVDBIDs.Set(strconv.FormatUint(tmdbID, 10), tvdbID, 1)
	}
}

// knownIMDbIDs remembers IMDb ids learned outside the ID resolver (the
// MDBList "ids" block) for titles whose TMDB entry has none, keyed by
// "movie/N" or "tv/N" (TMDB numbers are per kind).
var knownIMDbIDs = NewMemCache(0, 50000, 7*24*time.Hour, 0)

// RememberIMDbID records a title's IMDb id learned from another provider;
// mediaType is "movie" or "tv".
func RememberIMDbID(tmdbID uint64, mediaType, imdbID string) {
	if tmdbID != 0 && strings.HasPrefix(imdbID, "tt") && (mediaType == "movie" || mediaType == "tv") {
		knownIMDbIDs.Set(mediaType+"/"+strconv.FormatUint(tmdbID, 10), imdbID, 1)
	}
}

// withLearnedIDs returns r with the IMDb / TVDB ids it lacks filled from the
// ids learned from other providers (RememberIMDbID, RememberTVDBID), so ids
// learned after r was cached still reach its users (ratings, alternate cache
// keys). r itself, possibly a cached value, is never mutated.
func withLearnedIDs(r *ResolvedID) *ResolvedID {
	if r == nil || r.TMDbID == 0 || (r.MediaType != MediaTypeMovie && r.MediaType != MediaTypeTV) {
		return r
	}
	kind := "movie"
	if r.MediaType == MediaTypeTV {
		kind = "tv"
	}
	var imdb *string
	if r.IMDbID == nil || *r.IMDbID == "" {
		if v, ok := knownIMDbIDs.Get(kind + "/" + strconv.FormatUint(r.TMDbID, 10)); ok {
			s := v.(string)
			imdb = &s
		}
	}
	var tvdb *uint64
	if r.MediaType == MediaTypeTV && (r.TVDBID == nil || *r.TVDBID == 0) {
		tvdb = SeriesTVDBID(r)
	}
	if imdb == nil && tvdb == nil {
		return r
	}
	out := *r
	if imdb != nil {
		out.IMDbID = imdb
	}
	if tvdb != nil {
		out.TVDBID = tvdb
	}
	return &out
}

// SeriesTVDBID returns the TVDB id of a resolved series: its own, else one
// learned from another provider, else nil.
func SeriesTVDBID(r *ResolvedID) *uint64 {
	if r == nil {
		return nil
	}
	if r.TVDBID != nil && *r.TVDBID != 0 {
		return r.TVDBID
	}
	if r.TMDbID == 0 {
		return nil
	}
	if v, ok := knownTVDBIDs.Get(strconv.FormatUint(r.TMDbID, 10)); ok {
		id := v.(uint64)
		return &id
	}
	return nil
}

// SeriesTVDBIDCtx is SeriesTVDBID plus the TVDB API: a series whose TVDB id
// is still unknown is looked up by IMDb id via /search/remoteid (a bare
// TMDB number is ambiguous across sources there), and the answer
// remembered. nil when TVDB has none, or the lookup failed.
func SeriesTVDBIDCtx(ctx context.Context, r *ResolvedID, tvdb *TVDBClient) *uint64 {
	if id := SeriesTVDBID(r); id != nil || r == nil || tvdb == nil || r.MediaType == MediaTypeMovie {
		return id
	}
	if r.IMDbID == nil || *r.IMDbID == "" {
		return nil
	}
	id, err := tvdb.RemoteIDCtx(ctx, *r.IMDbID, MediaTypeTV)
	if err != nil {
		slog.Debug("TVDB remote-id lookup failed", "imdb_id", *r.IMDbID, "error", err)
		return nil
	}
	if id == 0 {
		return nil
	}
	RememberTVDBID(r.TMDbID, id)
	return &id
}

func findBestEntry(entries []findEntry) *findEntry {
	var best *findEntry
	for i := range entries {
		e := &entries[i]
		if best == nil || e.Popularity > best.Popularity {
			best = e
		}
	}
	return best
}

// resolveKitsuCtx fetches the Kitsu anime resource for the given idValue
// (numeric Kitsu anime id) and projects it onto ResolvedID with the direct
// poster/cover URLs populated. Subtype is mapped to MediaType (TV/Movie/OVA
// → MediaTypeTV; the existing art pipeline treats everything non-movie the
// same way for poster/logo/backdrop fetches, and Kitsu's per-episode still
// pipeline is out of scope for this resolver). Episode kind on a kitsu: url
// is rejected at the handler level — Kitsu doesn't expose a clean season/episode
// path through the public JSON:API that maps onto our /episode-default/{id}.
// MAL id from the mappings response is preserved on ResolvedID.MALID so the
// cross-id cache write in image/crossid.go can write the alternate form.
func resolveKitsuCtx(ctx context.Context, idValue string, kitsu *KitsuClient) (*ResolvedID, error) {
	if idValue == "" {
		return nil, apperr.NewInvalidIDType("kitsu id must not be empty")
	}

	anime, mappings, err := kitsu.GetAnimeCtx(ctx, idValue)
	if err != nil {
		return nil, err
	}

	// Kitsu sends subtypes lowercase except "TV"/"OVA"/"ONA" ("movie",
	// "special", "music" — verified 2026-09-24), so compare case-insensitively.
	mediaType := MediaTypeTV
	if strings.EqualFold(anime.Subtype, KitsuSubtypeMovie) {
		mediaType = MediaTypeMovie
	}

	kitsuID := anime.ID
	resolved := &ResolvedID{
		MediaType:      mediaType,
		KitsuID:        &kitsuID,
		SourceProvider: "kitsu",
	}
	resolved.DirectPosterURL = anime.PosterImageOriginal
	resolved.DirectCoverURL = anime.CoverImageOriginal
	resolved.ReleaseDate = anime.StartDate
	resolved.Title = titleOf(&anime.CanonicalTitle, nil)
	if malID := MALIDForMapping(mappings); malID != nil {
		resolved.MALID = malID
	}

	return resolved, nil
}

// resolveMALCtx looks up an anime by MAL id through AniList (AniList is the
// only no-auth, no-OAuth source that reliably indexes every MAL anime; Jikan
// sunsets 2026-10-01 and MAL's official v2 API is OAuth-gated). The result
// carries AniList's coverImage.extraLarge as the poster and bannerImage as
// the cover — no TMDB involvement. KitsuID is left nil (the AniList query
// doesn't expose it; adding a second hop to look it up is out of scope).
func resolveMALCtx(ctx context.Context, idValue string, anilist *AniListClient) (*ResolvedID, error) {
	id, err := ParseMALID(idValue)
	if err != nil {
		return nil, err
	}

	media, err := anilist.MediaByMALCtx(ctx, id)
	if err != nil {
		return nil, err
	}

	malID := media.MALID
	mediaType := MediaTypeTV
	if strings.EqualFold(media.Format, "MOVIE") {
		mediaType = MediaTypeMovie
	}
	resolved := &ResolvedID{
		MediaType:      mediaType,
		MALID:          &malID,
		SourceProvider: "mal",
	}
	resolved.DirectPosterURL = media.CoverImageExtra
	resolved.DirectCoverURL = media.BannerImage
	resolved.ReleaseDate = media.StartDate
	resolved.Title = titleOf(&media.TitleEnglish, &media.TitleRomaji)

	return resolved, nil
}

// titleOf returns the first non-empty name (movie "title" vs TV "name",
// AniList English vs romaji), or nil.
func titleOf(primary, fallback *string) *string {
	for _, t := range []*string{primary, fallback} {
		if t != nil && strings.TrimSpace(*t) != "" {
			v := strings.TrimSpace(*t)
			return &v
		}
	}
	return nil
}
