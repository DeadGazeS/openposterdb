package image

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/services"
)

var fontData []byte
var loadedFont *sfnt.Font

const (
	// labelFontFaceSize is the point size for rating source labels.
	labelFontFaceSize = 26.0
	// valueFontFaceSize is the point size for rating values, ~23% larger than the label.
	valueFontFaceSize = 32.0
)

func LoadFont(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fontData = data

	f, err := sfnt.Parse(data)
	if err != nil {
		return err
	}
	loadedFont = f
	return nil
}

// GetFontFace returns a fresh font.Face at the label size. opentype.Face is
// NOT safe for concurrent use — it owns a mutable sfnt.Buffer and rasterizer,
// so a single shared face corrupted its glyph data under parallel requests
// (panic: index out of range in sfnt.LoadGlyph). Each caller therefore gets
// its own face; the underlying *sfnt.Font is immutable and safe to share.
func GetFontFace() font.Face {
	return newFace(labelFontFaceSize)
}

// GetValueFontFace returns a fresh font.Face at the value size (~10% larger
// than the label size). See GetFontFace for the concurrency note.
func GetValueFontFace() font.Face {
	return newFace(valueFontFaceSize)
}

func newFace(size float64) font.Face {
	if loadedFont == nil {
		return nil
	}
	face, err := opentype.NewFace(loadedFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	return face
}

// GetFontFacesAt returns fresh label and value font faces scaled by the given
// text-size percentage (100 = the default 26/32pt sizes). Faces are NOT safe to
// share across goroutines, so each caller gets its own; see GetFontFace.
func GetFontFacesAt(textSizePct float64) (font.Face, font.Face) {
	return newFace(labelFontFaceSize * textSizePct / 100.0),
		newFace(valueFontFaceSize * textSizePct / 100.0)
}

func GenerateImage(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize) ([]byte, error) {
	targetW := uint32(580)
	badgeScale := float32(1.0)

	if imageSize != nil {
		switch kind {
		case "poster":
			targetW = imageSize.PosterTargetWidth()
			badgeScale = imageSize.BadgeScale("poster")
		case "logo":
			targetW = imageSize.LogoTargetWidth()
			badgeScale = imageSize.BadgeScale("logo")
		case "backdrop":
			targetW = imageSize.BackdropTargetWidth()
			badgeScale = imageSize.BadgeScale("backdrop")
		case "episode":
			targetW = imageSize.EpisodeTargetWidth()
			badgeScale = imageSize.BadgeScale("episode")
		}
	}

	// Per-kind scales: the badge frame (badgeMultiplier = kind default ×
	// badge_size), the font faces (text_size), and the rating logos (logo_size).
	kindDefault := float32(1.2)
	if kind == "episode" {
		kindDefault = 1.45
	}
	textSizePct := float64(100)
	badgeSizePct := float64(100)
	logoSizePct := float64(100)
	switch kind {
	case "poster":
		textSizePct = float64(settings.PosterTextSize)
		badgeSizePct = float64(settings.PosterBadgeSize)
		logoSizePct = float64(settings.PosterLogoSize)
	case "logo":
		textSizePct = float64(settings.LogoTextSize)
		badgeSizePct = float64(settings.LogoBadgeSize)
		logoSizePct = float64(settings.LogoLogoSize)
	case "backdrop":
		textSizePct = float64(settings.BackdropTextSize)
		badgeSizePct = float64(settings.BackdropBadgeSize)
		logoSizePct = float64(settings.BackdropLogoSize)
	case "episode":
		textSizePct = float64(settings.EpisodeTextSize)
		badgeSizePct = float64(settings.EpisodeBadgeSize)
		logoSizePct = float64(settings.EpisodeLogoSize)
	}
	badgeMultiplier := kindDefault * float32(badgeSizePct) / 100.0
	badgeScale *= badgeMultiplier
	logoScale := float32(logoSizePct) / 100.0

	var labelStyle services.LabelStyle
	var badgeStyle services.BadgeStyle
	var appearance services.BadgeAppearance
	var layout services.ImageLayout

	switch kind {
	case "poster":
		labelStyle = settings.PosterLabelStyle
		badgeStyle = settings.PosterBadgeStyle
		appearance = settings.PosterAppearance()
		layout = settings.PosterLayout
	case "logo":
		labelStyle = settings.LogoLabelStyle
		badgeStyle = settings.LogoBadgeStyle
		appearance = settings.LogoAppearance()
		layout = settings.LogoLayout
	case "backdrop":
		labelStyle = settings.BackdropLabelStyle
		badgeStyle = settings.BackdropBadgeStyle
		appearance = settings.BackdropAppearance()
		layout = settings.BackdropLayout
	case "episode":
		labelStyle = settings.EpisodeLabelStyle
		badgeStyle = settings.EpisodeBadgeStyle
		appearance = settings.EpisodeAppearance()
		layout = settings.EpisodeLayout
	}

	// GetFontFacesAt returns (label, value) — assign in that order.
	labelFace, valueFace := GetFontFacesAt(textSizePct)
	if valueFace == nil || labelFace == nil {
		return nil, fmt.Errorf("font not loaded")
	}
	textScale := float32(textSizePct) / 100.0

	params := RenderParams{
		Badges:          badges,
		ValueFontFace:   valueFace,
		LabelFontFace:   labelFace,
		Quality:         quality,
		Layout:          layout,
		BadgeStyle:      badgeStyle,
		LabelStyle:      labelStyle,
		Appearance:      appearance,
		TargetWidth:     targetW,
		BadgeScale:      badgeScale,
		BadgeMultiplier: badgeMultiplier,
		TextScale:       textScale,
		LogoScale:       logoScale,
		Colors:          settings.Colors,
	}

	switch kind {
	case "poster":
		params.PosterFit = settings.PosterFit
		return RenderPosterSync(imageBytes, params)

	case "logo":
		return RenderLogoSync(imageBytes, params)

	case "backdrop":
		params.EdgeInsetX = settings.BackdropEdgeInsetX
		params.EdgeInsetY = settings.BackdropEdgeInsetY
		return RenderBackdropSync(imageBytes, params)

	case "episode":
		params.Blur = settings.EpisodeBlur
		return RenderEpisodeSync(imageBytes, params)
	}

	return nil, fmt.Errorf("unknown image kind: %s", kind)
}

// --- CDN helpers ---

func ImageResponse(data []byte, contentType string) (int, map[string]string, []byte) {
	return 200, map[string]string{
		"Content-Type":  contentType,
		"Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
	}, data
}

func TmdbPosterVariant(lang string, textless bool) string {
	if lang == "en" {
		if textless {
			return "_t_tl"
		}
		return ""
	}
	if textless {
		return "_t_" + lang + "_tl"
	}
	return "_t_" + lang
}

func ImageContentType(kind string) string {
	switch kind {
	case "logo":
		return "image/png"
	default:
		return "image/jpeg"
	}
}

func EnsureCacheDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

func ImageDbValue(kind string) string {
	switch kind {
	case "logo":
		return "l"
	case "backdrop":
		return "b"
	case "episode":
		return "e"
	default:
		return "p"
	}
}

// ratingsLimitForKind returns the number of ratings to fetch for a kind: the
// explicit ?ratings_limit override when provided and valid, otherwise the
// per-kind layout's total badge capacity. The stored ratings_limit settings
// fields (settings.RatingsLimit etc.) are legacy and are never consulted, so a
// stored 0 for a per-key row can't wrongly suppress all ratings.
func ratingsLimitForKind(kind string, settings *services.RenderSettings, override *int32) int32 {
	limit := settings.PosterLayout.Total()
	switch kind {
	case "logo":
		limit = settings.LogoLayout.Total()
	case "backdrop":
		limit = settings.BackdropLayout.Total()
	case "episode":
		limit = settings.EpisodeLayout.Total()
	}
	if override != nil && services.ValidateRatingsLimit(*override) == nil {
		return *override
	}
	return limit
}

// ServeImage is the full image-generation pipeline shared by the public image
// endpoint and the admin fetch endpoint. It resolves the ID, fetches ratings,
// downloads the base artwork, renders badges, and caches the result to the
// in-memory cache (when provided), the filesystem, and the image_meta table.
//
// ratingsLimit is the explicit ?ratings_limit override from the public image
// query (nil when absent). The stored ratings_limit settings fields are legacy
// and are never consulted for the limit decision.
//
// caches optionally provides the process-wide in-memory caches (rendered
// images, ID resolutions, fetched ratings); nil fields disable that layer.
//
// Returns (image bytes, content type, error).

// ServeParams carries everything ServeImage needs to resolve and render one
// image request.
type ServeParams struct {
	Context             context.Context
	DB                  *sql.DB
	TMDB                *services.TmdbClient
	OMDB                *services.OmdbClient
	MDBList             *services.MdblistClient
	Trakt               *services.TraktClient
	Fanart              *services.FanartClient
	Kitsu               *services.KitsuClient
	AniList             *services.AniListClient
	KitsuIMDbMapper     *services.KitsuIMDbMapper
	IDType              string
	IDValue             string
	Kind                string
	Settings            *services.RenderSettings
	RatingsLimit        *int32
	CacheDir            string
	ExternalCacheOnly   bool
	RatingsMinStaleSecs uint64
	RatingsMaxAgeSecs   uint64
	ImageStaleSecs      uint64
	Quality             uint8
	ImageSizeStr        *string
	Caches              *services.MemCacheSet
	Inflight            *InflightSet
}

// resolveKitsuMalCtx resolves a kitsu:N or mal:N URL. The UseKitsu and
// UseMAL settings independently control each source:
//   - UseKitsu=true, UseMAL=true (defaults): direct Kitsu/MAL call →
//     Kitsu/MAL artwork (posterImage.original / coverImage.extraLarge /
//     bannerImage).
//   - Either false: try to find an IMDB equivalent first (via the
//     appropriate mapping source — KitsuIMDbMapper for kitsu, AniListClient
//     for mal). If a mapping exists, re-resolve via the standard IMDb
//     pipeline (TMDB /find). If the mapping is missing OR the IMDb pipeline
//     rejects it, fall back to the direct Kitsu/MAL call. "No third
//     option" — if both equivalent-finding and direct-call fail, the
//     request errors out; we don't retry via TMDB/TVDB on a kitsu:/
//     mal: id_type path.
//
// Only applies to poster and backdrop kinds — kitsu/mal don't carry logo
// or per-episode still artwork (the admin UI Fetch modal dropdown also
// hides kitsu/mal options for logo/episode kinds as a UI guard).
func resolveKitsuMalCtx(ctx context.Context, cache *services.MemCache, idType services.IDType, idValue string, clients services.IDClients, useKitsu, useMAL bool) (*services.ResolvedID, error) {
	switch idType {
	case services.IDTypeKitsu:
		if useKitsu {
			return services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
		}
		// UseKitsu=false: try to find an IMDB equivalent via the TheBeastLT
		// cross-ref (~775 KB, good coverage).
		if clients.KitsuIMDbMapper != nil {
			if imdbID := lookupKitsuIMDb(idValue, clients.KitsuIMDbMapper); imdbID != nil {
				slog.Debug("kitsu→imdb equivalent translation hit",
					"kitsu_id_or_slug", idValue,
					"imdb_id", *imdbID)
				if r, err := services.ResolveIDCachedCtx(ctx, cache, services.IDTypeIMDB, *imdbID, clients); err == nil {
					return r, nil
				}
				slog.Debug("kitsu→imdb translation resolve failed, falling back to direct", "imdb_id", *imdbID)
			}
		}
	case services.IDTypeMAL:
		if useMAL {
			return services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
		}
		// UseMAL=false: try AniList's externalLinks for an IMDB cross-ref.
		// Coverage is limited (verified 2026-08-15 that 0/6 popular anime
		// had IMDB in AniList's externalLinks), so this is best-effort.
		if clients.AniList != nil {
			if malID, parseErr := strconv.ParseUint(idValue, 10, 64); parseErr == nil && malID > 0 {
				if imdbID := clients.AniList.LookupIMDBByMAL(ctx, malID); imdbID != nil {
					slog.Debug("mal→imdb equivalent translation hit",
						"mal_id", malID,
						"imdb_id", *imdbID)
					if r, err := services.ResolveIDCachedCtx(ctx, cache, services.IDTypeIMDB, *imdbID, clients); err == nil {
						return r, nil
					}
					slog.Debug("mal→imdb translation resolve failed, falling back to direct", "imdb_id", *imdbID)
				}
			}
		}
	}
	// Fallback: direct Kitsu/MAL. Per the user's spec: "Even if disabled,
	// Kitsu / MAL may still be used when a imdb / tmdb / tvdb key can't be
	// found" — same call, semantically the user's last-resort path.
	return services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
}

// lookupKitsuIMDb accepts either a numeric Kitsu id or a slug. Numeric
// values are looked up directly; slug values are first resolved to a
// numeric id via the Kitsu slug-filter endpoint.
func lookupKitsuIMDb(idOrSlug string, mapper *services.KitsuIMDbMapper) *string {
	if mapper == nil || idOrSlug == "" {
		return nil
	}
	if kitsuID, parseErr := strconv.ParseUint(idOrSlug, 10, 64); parseErr == nil && kitsuID > 0 {
		return mapper.LookupIMDB(kitsuID)
	}
	// Slug path: would need a separate KitsuClient.GetAnimeBySlug call to
	// resolve the slug first; for the IMDB translation we accept the
	// limitation that slug IDs only translate when the user has supplied
	// a numeric id previously cached (the slug→numeric bridge is a TODO).
	return nil
}

// resolveStandardCtx is the standard TMDB/IMDb/TVDB pipeline — strict call
// then the imdb → tmdb → tvdb fallback chain. The use_kitsu_mal checkbox
// does not affect this path; it only governs how explicit kitsu:N /
// mal:N URLs are handled (see resolveKitsuMalCtx).
func resolveStandardCtx(ctx context.Context, cache *services.MemCache, idType services.IDType, idValue string, clients services.IDClients) (*services.ResolvedID, error) {
	resolve := func(t services.IDType) (*services.ResolvedID, error) {
		return services.ResolveIDCachedCtx(ctx, cache, t, idValue, clients)
	}
	resolved, strictErr := resolve(idType)
	if strictErr == nil {
		return resolved, nil
	}
	for _, alt := range []services.IDType{services.IDTypeIMDB, services.IDTypeTMDB, services.IDTypeTVDB} {
		if alt == idType {
			continue
		}
		if r, err := resolve(alt); err == nil {
			slog.Debug("image request: standard-chain fallback hit",
				"requested", idType.String(),
				"resolved_via", alt.String(),
				"id", idValue)
			return r, nil
		}
	}
	return nil, strictErr
}

// resolveWithFallback dispatches to the two resolution paths above. The
// use_kitsu_mal checkbox only changes how explicit kitsu:N / mal:N URLs
// are handled — every other id_type (imdb / tmdb / tvdb) uses the standard
// imdb → tmdb → tvdb chain regardless of the checkbox.
//
// Per the user's 2026-08-14 spec ("no third option"):
//   - ON:  kitsu:N / mal:N → direct Kitsu/MAL → Kitsu/MAL artwork.
//   - OFF: kitsu:N / mal:N → IMDB-equivalent lookup → fallback to direct.
//   - Both: imdb:N / tmdb:N / tvdb:N → strict + standard chain.
func resolveWithFallback(ctx context.Context, cache *services.MemCache, idType services.IDType, idValue string, clients services.IDClients, useKitsu, useMAL bool) (*services.ResolvedID, error) {
	if idType == services.IDTypeKitsu || idType == services.IDTypeMAL {
		return resolveKitsuMalCtx(ctx, cache, idType, idValue, clients, useKitsu, useMAL)
	}
	return resolveStandardCtx(ctx, cache, idType, idValue, clients)
}

func ServeImage(p ServeParams) ([]byte, string, error) {
	if p.Context == nil {
		p.Context = context.Background()
	}
	idType, err := services.ParseIDType(p.IDType)
	if err != nil {
		return nil, "", err
	}

	if err := services.ValidateIDValue(p.IDValue); err != nil {
		return nil, "", err
	}

	if p.TMDB == nil {
		return nil, "", apperr.NewOther("TMDB API key not configured — image generation unavailable")
	}
	if p.Caches == nil {
		p.Caches = &services.MemCacheSet{}
	}

	contentType := ImageContentType(p.Kind)
	imageSize := services.ImageSizeMedium
	if p.ImageSizeStr != nil {
		imageSize = services.ParseImageSize(*p.ImageSizeStr)
	}

	slog.Debug("image request", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue)

	// Clone the caller's settings so the badge-direction/style default
	// resolution below (and any future in-place mutation) cannot mutate the
	// caller's struct. RenderSettings mutated fields here are all value types
	// (BadgeDirection / BadgeStyle / etc. are string aliases) so a shallow
	// copy is sufficient; the Colors map is read-only in this function and is
	// shared intentionally. Production handlers always pass a fresh struct per
	// request, but integration tests reuse a struct across calls (would shift
	// the cache key on each invocation), so this guards both.
	settingsCopy := *p.Settings
	p.Settings = &settingsCopy

	// Resolve badge direction/style defaults before cache key construction.
	switch p.Kind {
	case "poster":
		p.Settings.PosterBadgeDirection = p.Settings.PosterBadgeDirection.ResolveDefault()
		p.Settings.PosterBadgeStyle = p.Settings.PosterBadgeStyle.Resolve(p.Settings.PosterBadgeDirection)
	case "backdrop":
		p.Settings.BackdropBadgeDirection = p.Settings.BackdropBadgeDirection.ResolveDefault()
		p.Settings.BackdropBadgeStyle = p.Settings.BackdropBadgeStyle.Resolve(p.Settings.BackdropBadgeDirection)
	case "episode":
		p.Settings.EpisodeBadgeDirection = p.Settings.EpisodeBadgeDirection.ResolveDefault()
		p.Settings.EpisodeBadgeStyle = p.Settings.EpisodeBadgeStyle.Resolve(p.Settings.EpisodeBadgeDirection)
	}

	// Resolve the ID, uplifting episodes to their parent series for poster,
	// logo, and backdrop endpoints. resolveWithFallback retries the same
	// idValue under the other ID sources (imdb → tmdb → tvdb, skipping the
	// requested source) so a misrouted URL like
	// /imdb/poster-default/series-124364.jpg still resolves: the strict
	// imdb call fails (no tt prefix), the fallback tmdb call recognises the
	// series- prefix and returns the show. The cache key downstream stays
	// the originally-requested (idType, idValue) so the next request hits
	// cache directly with no fallback fired.
	resolved, err := resolveWithFallback(p.Context, p.Caches.IDs, idType, p.IDValue, services.IDClients{TMDB: p.TMDB, Kitsu: p.Kitsu, AniList: p.AniList, KitsuIMDbMapper: p.KitsuIMDbMapper}, p.Settings.UseKitsu, p.Settings.UseMAL)
	if err != nil {
		return nil, "", err
	}

	if p.Kind != "episode" && resolved.MediaType == services.MediaTypeEpisode {
		if resolved.Episode != nil {
			seriesID := services.FormatTMDbIDValue(resolved.Episode.ShowTMDbID, services.MediaTypeTV, nil)
			resolved, err = services.ResolveIDCachedCtx(p.Context, p.Caches.IDs, services.IDTypeTMDB, seriesID, services.IDClients{TMDB: p.TMDB, Kitsu: p.Kitsu, AniList: p.AniList, KitsuIMDbMapper: p.KitsuIMDbMapper})
			if err != nil {
				return nil, "", err
			}
		}
	}

	if p.Kind == "episode" && resolved.MediaType != services.MediaTypeEpisode {
		return nil, "", apperr.NewBadRequest("not an episode - use poster/logo/backdrop endpoint")
	}

	// Fetch ratings, apply preferences, and build the cache key.
	badges, cacheKey, cachePath, releaseDate, imageTypeChar, err := p.prepareRender(resolved)
	if err != nil {
		return nil, "", err
	}

	// Serve from the in-memory cache first (fast path; works even with
	// external_cache_only, mirroring the Rust image_mem_cache). When the entry
	// is due for re-validation (60s window) and the release-date staleness says
	// the cached render is out of date, serve the bytes anyway and refresh in
	// the background — mirroring the Rust check_caches behavior so ratings
	// changes propagate through a hot in-memory entry within ~a minute.
	if p.Caches.ImageMem != nil {
		if v, ok, due := p.Caches.ImageMem.GetDue(cacheKey); ok {
			if due && !p.ExternalCacheOnly {
				revalStaleSecs := services.ComputeStaleSecs(derefStr(releaseDate), p.RatingsMinStaleSecs, p.RatingsMaxAgeSecs)
				if entry, err := services.ReadCache(cachePath, revalStaleSecs); err == nil && entry.IsStale {
					slog.Debug("image mem cache entry stale, background refresh", "kind", p.Kind, "cache_key", cacheKey)
					go p.refreshStale(resolved, imageSize)
				}
				p.Caches.ImageMem.Touch(cacheKey)
			}
			slog.Debug("image mem cache hit", "kind", p.Kind, "cache_key", cacheKey)
			if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
				slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
			}
			return v.([]byte), contentType, nil
		}
	}

	// Serve from filesystem cache if present and fresh. A stale entry is
	// served immediately and regenerated in the background so the request is
	// not blocked (mirrors the Rust background refresh; the inflight set
	// guarantees at most one regeneration per cache key).
	staleSecs := services.ComputeStaleSecs(derefStr(releaseDate), p.RatingsMinStaleSecs, p.RatingsMaxAgeSecs)
	if !p.ExternalCacheOnly {
		if entry, err := services.ReadCache(cachePath, staleSecs); err == nil {
			if !entry.IsStale {
				slog.Debug("image cache hit", "kind", p.Kind, "cache_key", cacheKey)
				if p.Caches.ImageMem != nil {
					p.Caches.ImageMem.Set(cacheKey, entry.Bytes, int64(len(entry.Bytes)))
				}
				if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
					slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
				}
				return entry.Bytes, contentType, nil
			}
			slog.Debug("image cache stale, serving + background refresh", "kind", p.Kind, "cache_key", cacheKey)
			if p.Caches.ImageMem != nil {
				p.Caches.ImageMem.Set(cacheKey, entry.Bytes, int64(len(entry.Bytes)))
			}
			go p.refreshStale(resolved, imageSize)
			if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
				slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
			}
			return entry.Bytes, contentType, nil
		}
	}

	// Render (fetch artwork + generate + persist), coalesced so concurrent
	// requests for the same cache key share a single render.
	render := func() ([]byte, error) {
		return p.renderArtwork(resolved, badges, cacheKey, cachePath, releaseDate, imageTypeChar, imageSize)
	}
	var rendered []byte
	if p.Inflight != nil {
		rendered, err = p.Inflight.RunCoalesced(cacheKey, render)
	} else {
		rendered, err = render()
	}
	if err != nil {
		return nil, "", err
	}

	slog.Debug("image generated", "kind", p.Kind, "cache_key", cacheKey, "badges", len(badges))
	return rendered, contentType, nil
}

// prepareRender fetches ratings (when the kind's limit allows), applies
// preferences, and computes the cache key/path for the request's ID form.
// It is the first half of the render pipeline, shared by ServeImage and the
// background-refresh path.
func (p ServeParams) prepareRender(resolved *services.ResolvedID) (badges []services.RatingBadge, cacheKey, cachePath string, releaseDate *string, imageTypeChar string, err error) {
	limit := ratingsLimitForKind(p.Kind, p.Settings, p.RatingsLimit)

	var rawBadges []services.RatingBadge
	// Skip ratings when the resolver returned a direct image source (kitsu/mal
	// path). FetchRatingsCtx hits TMDB /movie/{id} or /tv/{id} which 404s on
	// TMDbID=0, and the new resolvers don't carry an IMDb cross-ref in practice
	// (verified 2026-08-14 against AniList externalLinks + Kitsu /mappings —
	// neither surfaces IMDb for anime). Better to skip than to log a flood of
	// 404s on every kitsu:N / mal:N request.
	if limit > 0 && resolved.SourceProvider == "" {
		mediaType := "movie"
		switch resolved.MediaType {
		case services.MediaTypeTV:
			mediaType = "tv"
		case services.MediaTypeEpisode:
			mediaType = "episode"
		}

		var imdbID *string
		if resolved.IMDbID != nil && *resolved.IMDbID != "" {
			imdbID = resolved.IMDbID
		}

		var epShowID uint64
		var epSeason, epEpisode uint32
		if resolved.Episode != nil {
			epShowID = resolved.Episode.ShowTMDbID
			epSeason = resolved.Episode.SeasonNumber
			epEpisode = resolved.Episode.EpisodeNumber
		}

		rawBadges = services.FetchRatingsCachedCtx(p.Context, p.Caches.Ratings,
			services.RatingsClients{TMDB: p.TMDB, OMDB: p.OMDB, MDBList: p.MDBList, Trakt: p.Trakt},
			services.RatingsQuery{ResolvedTMDbID: resolved.TMDbID, MediaType: mediaType, IMDbID: imdbID,
				EpisodeShowTMDbID: epShowID, EpisodeSeason: epSeason, EpisodeEpisode: epEpisode},
		)
		slog.Debug("ratings fetched",
			"kind", p.Kind,
			"id", p.IDType+"/"+p.IDValue,
			"badges", len(rawBadges),
			"sources", badgeSourceString(rawBadges),
		)
	} else {
		slog.Debug("ratings skipped", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "reason", "ratings_limit=0")
	}

	badges = services.ApplyRatingPreferences(rawBadges, p.Settings.RatingsOrder, p.Settings.RatingsExclude, limit)

	suffix := p.renderSuffix(badges)
	cacheKey, cachePath, err = p.cacheKeyFor(p.IDType, p.IDValue, suffix)
	releaseDate = resolved.ReleaseDate
	imageTypeChar = ImageDbValue(p.Kind)
	return badges, cacheKey, cachePath, releaseDate, imageTypeChar, err
}

// renderSuffix builds the settings+ratings cache suffix for the given badges.
// It is ID-independent, so it can be reused for alternate-ID cache keys.
func (p ServeParams) renderSuffix(badges []services.RatingBadge) string {
	ratingsSuffix := services.BadgesCacheSuffix(badges)
	return services.SettingsCacheSuffixWithRatings(p.Settings, p.Kind, p.ImageSizeStr, ratingsSuffix)
}

// cacheKeyFor computes the cache key and path for a specific ID form using
// the request's variant and suffix (both ID-independent).
func (p ServeParams) cacheKeyFor(idType, idValue, suffix string) (cacheKey, cachePath string, err error) {
	fanartPrimary := p.Settings.ImageSource.IsFanart() && p.Fanart != nil
	variant := cacheVariant(p.Kind, p.Settings, fanartPrimary)
	cacheValue := idValue + variant + suffix
	cacheKey = idType + "/" + cacheValue
	cachePath, err = services.TypedCachePath(p.CacheDir, services.ImageSubdir(p.Kind), idType, cacheValue, services.ImageExt(p.Kind))
	return cacheKey, cachePath, err
}

// renderArtwork fetches the base artwork, renders badges onto it, and persists
// the result (in-memory + filesystem + metadata, plus cross-ID copies). It is
// the coalescable unit: concurrent requests for the same cache key share one
// run via the inflight set.
func (p ServeParams) renderArtwork(resolved *services.ResolvedID, badges []services.RatingBadge, cacheKey, cachePath string, releaseDate *string, imageTypeChar string, imageSize services.ImageSize) ([]byte, error) {
	// Fetch the base artwork. Three paths, tried in order:
	//   1. Direct URL (kitsu/mal resolved) — Kitsu/AniList returned their own
	//      posterImage.original / coverImage.original URLs; fetch them as-is.
	//   2. Fanart.tv (when ImageSource=="f") — TMDB id may be 0 on the kitsu/mal
	//      path, so fanart is skipped automatically via the resolved.MediaType
	//      guard inside fetchFanartArtworkCtx.
	//   3. TMDB poster_path/backdrop_path (the existing default).
	var imageBytes []byte
	if resolved.SourceProvider != "" {
		var directURL *string
		switch p.Kind {
		case "poster":
			directURL = resolved.DirectPosterURL
		case "backdrop":
			directURL = resolved.DirectCoverURL
		}
		if directURL != nil && *directURL != "" {
			httpClient := http.DefaultClient
			if p.TMDB != nil && p.TMDB.HTTP != nil {
				httpClient = p.TMDB.HTTP
			}
			bytes, err := fetchDirectImageBytesCtx(p.Context, httpClient, *directURL)
			if err != nil {
				slog.Warn("direct image fetch failed", "kind", p.Kind, "url", *directURL, "error", err)
			} else {
				imageBytes = bytes
			}
		}
	}
	if imageBytes == nil && p.Settings.ImageSource.IsFanart() && p.Fanart != nil {
		imageBytes = fetchFanartArtworkCtx(p.Context, p.Fanart, p.TMDB, resolved, p.Kind, p.Settings, p.CacheDir, p.ExternalCacheOnly)
	}
	// Kitsu/MAL resolvers don't carry a TMDb id (AniList's externalLinks
	// don't reliably expose IMDB either, and even when they do the
	// translation produces an IMDB id without a corresponding TMDb id until
	// TMDB /find runs). When we got here via Kitsu/MAL and the direct URL
	// path produced nothing — e.g. logo/episode kinds, where Kitsu/MAL
	// don't carry logo/episode artwork — calling TMDB /tv/0 or /movie/0
	// is just going to surface a cryptic 404. Surface a clean error instead.
	if imageBytes == nil && resolved.SourceProvider != "" && resolved.TMDbID == 0 {
		slog.Warn("no artwork for kitsu/mal-resolved title", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "reason", "kitsu/mal don't carry this kind of art and no TMDB id was resolved")
		return nil, apperr.NewIDNotFound(fmt.Sprintf("no %s artwork available for %s/%s: Kitsu / MAL don't carry %s artwork — use an imdb / tmdb / tvdb key instead", p.Kind, p.IDType, p.IDValue, p.Kind))
	}
	if imageBytes == nil {
		var err error
		imageBytes, err = fetchTmdbArtworkCtx(p.Context, p.TMDB, p.CacheDir, p.ExternalCacheOnly, p.ImageStaleSecs, resolved, p.Kind, p.Settings, imageSize)
		if err != nil {
			return nil, err
		}
	}
	if imageBytes == nil {
		slog.Warn("no artwork available", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue)
		return nil, apperr.NewIDNotFound("no " + p.Kind + " artwork available for this title")
	}

	// Render badges onto the artwork.
	rendered, err := GenerateImage(imageBytes, badges, p.Settings, p.Kind, p.Quality, &imageSize)
	if err != nil {
		return nil, apperr.NewImageError(err)
	}

	// Persist: in-memory + filesystem + metadata DB.
	if p.Caches.ImageMem != nil {
		p.Caches.ImageMem.Set(cacheKey, rendered, int64(len(rendered)))
	}
	if !p.ExternalCacheOnly {
		if err := services.WriteCache(cachePath, rendered); err != nil {
			slog.Warn("failed to write image cache", "cache_key", cacheKey, "error", err)
		}
	}
	if err := services.UpsertImageMetaCtx(p.Context, p.DB, cacheKey, releaseDate, imageTypeChar); err != nil {
		slog.Warn("failed to upsert image meta", "cache_key", cacheKey, "error", err)
	}
	if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
		slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
	}

	// Cross-ID cache writes (fire-and-forget, logged on failure).
	p.writeCrossIDCache(resolved, badges, rendered, imageSize)

	return rendered, nil
}

// fetchDirectImageBytesCtx downloads a base artwork from a fully-qualified URL.
// Used for Kitsu/MAL resolved entries whose ResolvedID carries DirectPosterURL
// or DirectCoverURL — those providers don't go through TMDB's CDN, so we fetch
// the bytes straight from the URL the resolver returned. The function does no
// retry: Kitsu's posterImage and AniList's coverImage are static CDN URLs and
// the caller (renderArtwork) logs and falls through to TMDB/Fanart on a miss.
func fetchDirectImageBytesCtx(ctx context.Context, httpClient *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("direct image fetch returned %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// refreshStale regenerates a stale cache entry in the background with fresh
// ratings. Guarded by the inflight set so concurrent stale requests for the
// same key trigger only one regeneration. Best-effort: failures are logged.
//
// Runs with an independent background context so the work outlives the client
// request that triggered it (this is server-side cache maintenance, not part
// of the user's response).
func (p ServeParams) refreshStale(resolved *services.ResolvedID, imageSize services.ImageSize) {
	bgParams := p
	bgParams.Context = context.Background()
	badges, cacheKey, cachePath, releaseDate, imageTypeChar, err := bgParams.prepareRender(resolved)
	if err != nil {
		slog.Debug("background refresh: ratings failed", "id", p.IDType+"/"+p.IDValue, "error", err)
		return
	}
	render := func() ([]byte, error) {
		return bgParams.renderArtwork(resolved, badges, cacheKey, cachePath, releaseDate, imageTypeChar, imageSize)
	}
	var renderErr error
	if p.Inflight != nil {
		_, renderErr = p.Inflight.RunCoalesced(cacheKey, render)
	} else {
		_, renderErr = render()
	}
	if renderErr != nil {
		slog.Debug("background refresh failed", "kind", p.Kind, "cache_key", cacheKey, "error", renderErr)
	}
}

func cacheVariant(kind string, settings *services.RenderSettings, fanartPrimary bool) string {
	switch kind {
	case "poster":
		if fanartPrimary {
			if settings.Textless {
				return "_f_tl"
			}
			return "_f_" + settings.Lang
		}
		return TmdbPosterVariant(settings.Lang, settings.Textless)
	case "logo":
		if fanartPrimary {
			return "_l_f_" + settings.Lang
		}
		return "_l_t_" + settings.Lang
	case "backdrop":
		if fanartPrimary {
			return "_b_f"
		}
		return "_b_t"
	case "episode":
		return ""
	}
	return ""
}

// fetchTmdbArtworkCtx downloads the base artwork for the given kind from TMDB,
// preferring the on-disk base cache. Returns nil bytes when no artwork exists.
func fetchTmdbArtworkCtx(ctx context.Context, tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, imageSize services.ImageSize) ([]byte, error) {
	tmdbSize := imageSize.TmdbSize()

	var filePath string
	if kind == "episode" {
		if resolved.Episode != nil && resolved.Episode.StillPath != nil {
			filePath = *resolved.Episode.StillPath
		} else if resolved.PosterPath != nil {
			filePath = *resolved.PosterPath
		}
	} else if kind == "poster" && (settings.Lang == "en" && !settings.Textless) {
		if resolved.PosterPath != nil {
			filePath = *resolved.PosterPath
		}
	} else {
		mediaType := "tv"
		if resolved.MediaType == services.MediaTypeMovie {
			mediaType = "movie"
		}
		lang := settings.Lang
		if kind == "backdrop" {
			lang = ""
		}
		textless := settings.Textless && kind == "poster"
		images, err := tmdb.GetImagesCtx(ctx, mediaType, resolved.TMDbID, lang)
		if err != nil {
			return nil, err
		}
		var candidates []services.TmdbImage
		switch kind {
		case "poster":
			candidates = images.Posters
		case "logo":
			candidates = images.Logos
		case "backdrop":
			candidates = images.Backdrops
		}
		var selected *services.TmdbImage
		if kind == "poster" {
			selected = services.SelectPoster(candidates, lang, textless)
		} else {
			selected = services.SelectImage(candidates, lang, textless)
		}
		if selected != nil {
			filePath = selected.FilePath
		}
	}

	if filePath == "" {
		return nil, nil
	}

	if externalCacheOnly {
		return tmdb.FetchPosterBytesCtx(ctx, filePath, tmdbSize)
	}

	basePath, err := services.BasePosterPath(cacheDir, filePath, tmdbSize)
	if err != nil {
		return nil, err
	}
	if entry, err := services.ReadCache(basePath, imageStaleSecs); err == nil {
		return entry.Bytes, nil
	}
	bytes, err := tmdb.FetchPosterBytesCtx(ctx, filePath, tmdbSize)
	if err != nil {
		return nil, err
	}
	if err := services.WriteCache(basePath, bytes); err != nil {
		slog.Warn("failed to write base image cache", "path", basePath, "error", err)
	}
	return bytes, nil
}

// fetchTmdbArtwork downloads the base artwork for the given kind from TMDB,
// preferring the on-disk base cache. Returns nil bytes when no artwork exists.
func fetchTmdbArtwork(tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, imageSize services.ImageSize) ([]byte, error) {
	return fetchTmdbArtworkCtx(context.Background(), tmdb, cacheDir, externalCacheOnly, imageStaleSecs, resolved, kind, settings, imageSize)
}

// fetchFanartArtworkCtx downloads the base artwork from fanart.tv when the
// user's image source is fanart. Returns nil bytes when unavailable so the
// caller falls through to TMDB.
func fetchFanartArtworkCtx(ctx context.Context, fanart *services.FanartClient, tmdb *services.TmdbClient, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, cacheDir string, externalCacheOnly bool) []byte {
	if kind == "episode" {
		return nil
	}

	var images *services.FanartImages
	var err error
	switch resolved.MediaType {
	case services.MediaTypeMovie:
		images, err = fanart.GetMovieImagesCtx(ctx, resolved.TMDbID)
	default:
		tvID := resolved.TMDbID
		if resolved.TVDBID != nil && *resolved.TVDBID != 0 {
			tvID = *resolved.TVDBID
		}
		images, err = fanart.GetTVImagesCtx(ctx, tvID)
	}
	if err != nil || images == nil {
		return nil
	}

	var candidates []services.FanartPoster
	switch kind {
	case "poster":
		candidates = images.Posters
	case "logo":
		candidates = images.Logos
	case "backdrop":
		candidates = images.Backdrops
	}
	lang := settings.Lang
	if kind == "backdrop" {
		lang = ""
	}
	selected, _, ok := services.SelectFanartImage(candidates, lang, settings.Textless && kind == "poster")
	if !ok {
		return nil
	}

	if !externalCacheOnly {
		if basePath, err := services.BaseFanartPath(cacheDir, selected.ID, services.ImageExt(kind)); err == nil {
			if entry, err := services.ReadCache(basePath, 0); err == nil {
				return entry.Bytes
			}
			if bytes, err := fanart.FetchPosterBytesCtx(ctx, selected.URL); err == nil {
				// Fire-and-forget cache write — log on failure but don't block the
				// refresh; the bytes are already returned to the caller.
				if err := services.WriteCache(basePath, bytes); err != nil {
					slog.Warn("fanart cache write failed", "base_path", basePath, "error", err)
				}
				return bytes
			}
			return nil
		}
	}
	bytes, err := fanart.FetchPosterBytesCtx(ctx, selected.URL)
	if err != nil {
		return nil
	}
	return bytes
}

// fetchFanartArtwork downloads the base artwork from fanart.tv when the
// user's image source is fanart. Returns nil bytes when unavailable so the
// caller falls through to TMDB.
func fetchFanartArtwork(fanart *services.FanartClient, tmdb *services.TmdbClient, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, cacheDir string, externalCacheOnly bool) []byte {
	return fetchFanartArtworkCtx(context.Background(), fanart, tmdb, resolved, kind, settings, cacheDir, externalCacheOnly)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// DemoIMDBID is the demo title used by the admin preview endpoints. It is a TV
// series; the episode preview resolves its season 1, episode 1 still.
const DemoIMDBID = "tt12637874"
const DemoEpisodeID = "episode-tt12637874-S1E1"

// DemoArtwork fetches the base artwork for the preview endpoints' demo title
// (IMDb tt12637874; S01E01 for the episode preview). It prefers the on-disk
// base cache and returns an error when TMDB is unavailable or no artwork
// exists for the kind.
func DemoArtwork(tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, kind string, imageSize services.ImageSize) ([]byte, error) {
	return DemoArtworkCtx(context.Background(), tmdb, cacheDir, externalCacheOnly, imageStaleSecs, kind, imageSize)
}

// DemoArtworkCtx is the context-aware form of DemoArtwork.
func DemoArtworkCtx(ctx context.Context, tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, kind string, imageSize services.ImageSize) ([]byte, error) {
	idValue := DemoIMDBID
	if kind == "episode" {
		idValue = DemoEpisodeID
	}
	resolved, err := services.ResolveIDCtx(ctx, services.IDTypeIMDB, idValue, services.IDClients{TMDB: tmdb})
	if err != nil {
		return nil, err
	}
	settings := &services.RenderSettings{Lang: "en", Textless: false}
	bytes, err := fetchTmdbArtworkCtx(ctx, tmdb, cacheDir, externalCacheOnly, imageStaleSecs, resolved, kind, settings, imageSize)
	if err != nil {
		return nil, err
	}
	if bytes == nil {
		return nil, apperr.NewIDNotFound("no " + kind + " artwork available for the demo title")
	}
	return bytes, nil
}

func badgeSourceString(badges []services.RatingBadge) string {
	keys := make([]string, len(badges))
	for i, b := range badges {
		keys[i] = b.Source.Key
	}
	return strings.Join(keys, ",")
}
