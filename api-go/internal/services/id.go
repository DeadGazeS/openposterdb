package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	IMDbID          *string
	TMDbID          uint64
	TVDBID          *uint64
	KitsuID         *uint64
	MALID           *uint64
	MediaType       MediaType
	PosterPath      *string
	ReleaseDate     *string
	DirectPosterURL *string
	DirectCoverURL  *string
	SourceProvider  string // "" for TMDB/IMDB/TVDB, "kitsu" or "mal" for the new sources
	Episode         *EpisodeInfo
}

// IDClients bundles the per-source resolver clients. TMDB is required (the
// existing pipeline needs it for IMDb / TMDB / TVDB resolution and for
// poster_path / backdrop_path fetches); Kitsu and AniList are optional —
// when nil, the resolver returns NewOther for that source, and the
// fallback chain in image/serve.go simply moves on to the next source.
// KitsuIMDbMapper is optional too — when nil, the "find IMDB equivalent
// via TheBeastLT cross-ref" step is silently skipped.
type IDClients struct {
	TMDB             *TmdbClient
	Kitsu            *KitsuClient
	AniList          *AniListClient
	KitsuIMDbMapper  *KitsuIMDbMapper
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
	ID          uint64  `json:"id"`
	PosterPath  *string `json:"poster_path"`
	ReleaseDate *string `json:"release_date"`
	FirstAir    *string `json:"first_air_date"`
	Popularity  float64 `json:"popularity"`
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
		return resolveIMDBCtx(ctx, idValue, clients.TMDB)
	case IDTypeTMDB:
		return resolveTMDBCtx(ctx, idValue, clients.TMDB)
	case IDTypeTVDB:
		return resolveTVDBCtx(ctx, idValue, clients.TMDB)
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

// ResolveIDCached resolves an ID, caching successful results in the given
// in-memory cache (key "idtype/idvalue", matching the Rust id_cache). A nil
// cache bypasses caching. Errors are never cached, so a transient TMDB failure
// doesn't poison the cache.
func ResolveIDCachedCtx(ctx context.Context, cache *MemCache, idType IDType, idValue string, clients IDClients) (*ResolvedID, error) {
	if cache != nil {
		key := idType.String() + "/" + idValue
		if v, ok := cache.Get(key); ok {
			return v.(*ResolvedID), nil
		}
		resolved, err := ResolveIDCtx(ctx, idType, idValue, clients)
		if err == nil {
			cache.Set(key, resolved, 1)
		}
		return resolved, err
	}
	return ResolveIDCtx(ctx, idType, idValue, clients)
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
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/find/%s", imdbID), map[string]string{"external_source": "imdb_id"}, &result); err != nil {
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
			IMDbID:      &imdbID,
			TMDbID:      bestMovie.ID,
			MediaType:   MediaTypeMovie,
			PosterPath:  bestMovie.PosterPath,
			ReleaseDate: bestMovie.ReleaseDate,
		}, nil
	}
	if bestTV != nil {
		return &ResolvedID{
			IMDbID:      &imdbID,
			TMDbID:      bestTV.ID,
			MediaType:   MediaTypeTV,
			PosterPath:  bestTV.PosterPath,
			ReleaseDate: bestTV.FirstAir,
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
		IMDbID      *string          `json:"imdb_id"`
		PosterPath  *string          `json:"poster_path"`
		ReleaseDate *string          `json:"release_date"`
		FirstAir    *string          `json:"first_air_date"`
		ExternalIDs *tmdbExternalIDs `json:"external_ids"`
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
	if details.ExternalIDs != nil {
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
		IMDbID:      imdbID,
		TMDbID:      tmdbID,
		TVDBID:      tvdbID,
		MediaType:   mediaType,
		PosterPath:  details.PosterPath,
		ReleaseDate: releaseDate,
	}, nil
}

func resolveTVDBCtx(ctx context.Context, tvdbIDVal string, tmdb *TmdbClient) (*ResolvedID, error) {
	if rest, ok := strings.CutPrefix(tvdbIDVal, "episode-"); ok {
		return resolveTVDBEpisodeCtx(ctx, rest, tvdbIDVal, tmdb)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/find/%s", tvdbIDVal), map[string]string{"external_source": "tvdb_id"}, &result); err != nil {
		return nil, err
	}

	if len(result.TVEpisodeResults) > 0 {
		ep := result.TVEpisodeResults[0]
		return resolveEpisodeDetailsCtx(ctx, tmdb, ep.ShowID, ep.SeasonNumber, ep.EpisodeNumber, ep.StillPath, nil)
	}

	tvdbNum, _ := strconv.ParseUint(tvdbIDVal, 10, 64)

	if len(result.TVResults) > 0 {
		tv := result.TVResults[0]
		var details struct {
			ExternalIDs *struct {
				IMDbID *string `json:"imdb_id"`
			} `json:"external_ids"`
			PosterPath *string `json:"poster_path"`
			FirstAir   *string `json:"first_air_date"`
		}
		tmdb.GetCtx(ctx, fmt.Sprintf("/tv/%d", tv.ID), map[string]string{"append_to_response": "external_ids"}, &details)

		var imdb *string
		if details.ExternalIDs != nil && details.ExternalIDs.IMDbID != nil {
			imdb = details.ExternalIDs.IMDbID
		}

		return &ResolvedID{
			IMDbID:      imdb,
			TMDbID:      tv.ID,
			TVDBID:      &tvdbNum,
			MediaType:   MediaTypeTV,
			PosterPath:  details.PosterPath,
			ReleaseDate: details.FirstAir,
		}, nil
	}

	if len(result.MovieResults) > 0 {
		movie := result.MovieResults[0]
		var details struct {
			IMDbID      *string `json:"imdb_id"`
			PosterPath  *string `json:"poster_path"`
			ReleaseDate *string `json:"release_date"`
		}
		tmdb.GetCtx(ctx, fmt.Sprintf("/movie/%d", movie.ID), nil, &details)

		return &ResolvedID{
			IMDbID:      details.IMDbID,
			TMDbID:      movie.ID,
			TVDBID:      nil,
			MediaType:   MediaTypeMovie,
			PosterPath:  details.PosterPath,
			ReleaseDate: details.ReleaseDate,
		}, nil
	}

	return nil, apperr.NewIDNotFound(fmt.Sprintf("%s (not found on TMDB via TVDB lookup)", tvdbIDVal))
}

func resolveEpisodeDetailsCtx(ctx context.Context, tmdb *TmdbClient, showID uint64, season, episode uint32, hintStillPath, hintIMDbID *string) (*ResolvedID, error) {
	type epDetails struct {
		StillPath   *string `json:"still_path"`
		AirDate     *string `json:"air_date"`
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
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/find/%s", seriesIMDbID), map[string]string{"external_source": "imdb_id"}, &result); err != nil {
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

	seStr := upper[idx+2:]
	seParts := strings.SplitN(seStr, "E", 2)
	if len(seParts) != 2 {
		return nil, apperr.NewInvalidIDType(fullID)
	}
	season, _ := strconv.ParseUint(seParts[0], 10, 32)
	episode, _ := strconv.ParseUint(seParts[1], 10, 32)

	return resolveEpisodeDetailsCtx(ctx, tmdb, showID, uint32(season), uint32(episode), nil, nil)
}

func resolveTVDBEpisodeCtx(ctx context.Context, rest, fullID string, tmdb *TmdbClient) (*ResolvedID, error) {
	seriesTVDBID, season, episode := parseEpisodeExternal(rest, fullID)
	if seriesTVDBID == "" {
		return nil, apperr.NewInvalidIDType(fullID)
	}

	var result findResult
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/find/%s", seriesTVDBID), map[string]string{"external_source": "tvdb_id"}, &result); err != nil {
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

	seStr := upper[idx+2:]
	parts := strings.SplitN(seStr, "E", 2)
	if len(parts) != 2 {
		return "", 0, 0
	}

	season, _ := strconv.ParseUint(parts[0], 10, 32)
	episode, _ := strconv.ParseUint(parts[1], 10, 32)
	if season > 10000 || episode > 100000 {
		return "", 0, 0
	}

	return externalID, uint32(season), uint32(episode)
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

	mediaType := MediaTypeTV
	if anime.Subtype == KitsuSubtypeMovie {
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
	resolved := &ResolvedID{
		MediaType:      MediaTypeTV,
		MALID:          &malID,
		SourceProvider: "mal",
	}
	resolved.DirectPosterURL = media.CoverImageExtra
	resolved.DirectCoverURL = media.BannerImage

	return resolved, nil
}
