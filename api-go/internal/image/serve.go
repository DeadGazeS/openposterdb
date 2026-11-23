package image

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	Mappings            *services.IDMappings
	TVDB                *services.TVDBClient
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
	// Served, when set, receives the cache key/path the response was served
	// under (the CDN redirect records it for the /c/ route).
	Served *services.CDNServed
	// freshRatings bypasses the ratings stale-while-revalidate (background
	// refreshes must render with current badges).
	freshRatings bool
}

// serveColdCache answers a request whose id is not in the in-memory ID cache
// (typically the first request after a restart) from the rendered cache,
// without resolving the id or fetching ratings: the cache key is rebuilt from
// the title's stored rating sources (available_ratings, written with every
// render). On a hit the full pipeline runs in the background
// (warmAfterColdHit), so new ratings or settings still reach the image.
func (p ServeParams) serveColdCache(contentType string) ([]byte, bool) {
	if p.DB == nil || p.Caches == nil || p.Caches.IDs == nil {
		return nil, false
	}
	if p.idCacheWarm() {
		return nil, false
	}
	idKey := p.IDType + "/" + p.IDValue
	limit := ratingsLimitForKind(p.Kind, p.Settings, p.RatingsLimit)
	sources, _, releaseDate, err := services.ReadAvailableRatingsCtx(p.Context, p.DB, idKey)
	// No row: the sources are unknown — unless no ratings are shown at all
	// (limit 0), when the key doesn't depend on them.
	if err != nil && !(limit == 0 && stderrors.Is(err, sql.ErrNoRows)) || !services.ValidSourcesString(sources) {
		return nil, false
	}
	ratingsSuffix := services.BadgesSuffixFromAvailable(sources, p.Settings.RatingsOrder, p.Settings.RatingsExclude, limit)
	cacheKey, cachePath, err := p.cacheKeyFor(p.IDType, p.IDValue, p.renderSuffixFromRatings(ratingsSuffix))
	if err != nil {
		return nil, false
	}
	var bytes []byte
	if p.Caches.ImageMem != nil {
		if v, ok := p.Caches.ImageMem.Get(cacheKey); ok {
			bytes = v.([]byte)
		}
	}
	if bytes == nil && !p.ExternalCacheOnly {
		if entry, err := services.ReadCache(cachePath, 0); err == nil {
			bytes = entry.Bytes
			if p.Caches.ImageMem != nil {
				p.Caches.ImageMem.Set(cacheKey, bytes, int64(len(bytes)))
			}
		}
	}
	if bytes == nil {
		return nil, false
	}
	slog.Debug("image cold cache hit", "kind", p.Kind, "cache_key", cacheKey)
	if p.Served != nil {
		*p.Served = services.CDNServed{CacheKey: cacheKey, CachePath: cachePath, ContentType: contentType, ReleaseDate: releaseDate}
	}
	if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
		slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
	}
	goBackground(func() { p.warmAfterColdHit(cacheKey) })
	return bytes, true
}

// idCacheWarm reports whether the in-memory ID cache holds the request's id
// under a key its resolution caches: the type the id's form names
// (services.EffectiveIDType: imdb/movie-123 is cached as tmdb/movie-123) and,
// for a kitsu/mal id translated to the standard pipeline (toggle off), the
// translated id — from the offline anime list only, no lookups here.
func (p ServeParams) idCacheWarm() bool {
	idType := p.IDType
	if eff, err := services.EffectiveIDType(p.IDType, p.IDValue); err == nil {
		idType = eff
	}
	if _, warm := p.Caches.IDs.Peek(idType + "/" + p.IDValue); warm {
		return true
	}
	var entry services.AnimeIDs
	found := false
	if id, err := strconv.ParseUint(p.IDValue, 10, 64); err == nil {
		switch {
		case idType == "kitsu" && !p.Settings.UseKitsu:
			entry, found = p.Mappings.AnimeByKitsu(id)
		case idType == "mal" && !p.Settings.UseMAL:
			entry, found = p.Mappings.AnimeByMAL(id)
		}
	}
	if !found {
		return false
	}
	t, v, ok := standardIDFor(entry)
	if !ok {
		return false
	}
	_, warm := p.Caches.IDs.Peek(t.String() + "/" + v)
	return warm
}

// warmAfterColdHit follows up a request served by serveColdCache: it resolves
// the id and fetches the ratings (filling the ID and ratings caches), then
// re-renders when the ratings changed the cache key or the rendered file is
// stale — checked against the file itself, not the in-memory entry the cold
// hit may just have filled. Shares the background refresh slots and back-off
// with refreshStale; the back-off is only cleared by a re-render.
func (p ServeParams) warmAfterColdHit(staleKey string) {
	if refreshFailures.blocked(staleKey, time.Now()) {
		return
	}
	release, ok := refreshSlots.tryAcquire()
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	bg := p
	bg.Context = ctx
	bg.Served = nil
	idType, err := services.ParseIDType(bg.IDType)
	if err != nil {
		return
	}
	resolved, err := bg.resolveTitle(idType)
	if err == nil {
		var cacheKey, cachePath string
		var releaseDate *string
		_, cacheKey, cachePath, releaseDate, _, _, err = bg.prepareRender(resolved)
		if err == nil && bg.renderIsCurrent(cacheKey, cachePath, releaseDate) {
			return
		}
		if err == nil {
			err = bg.rerender(resolved, bg.imageSize())
		}
	}
	if err != nil {
		slog.Debug("warm-up after cold cache hit failed", "cache_key", staleKey, "error", err)
		refreshFailures.failed(staleKey, time.Now())
		return
	}
	refreshFailures.succeeded(staleKey)
}

// renderIsCurrent reports whether cacheKey's render exists and is not stale
// (with external_cache_only, whether it is in memory).
func (p ServeParams) renderIsCurrent(cacheKey, cachePath string, releaseDate *string) bool {
	if p.ExternalCacheOnly {
		if p.Caches.ImageMem == nil {
			return false
		}
		_, ok := p.Caches.ImageMem.Peek(cacheKey)
		return ok
	}
	staleSecs := services.ComputeStaleSecs(derefStr(releaseDate), p.RatingsMinStaleSecs, p.RatingsMaxAgeSecs)
	stale, err := services.CacheIsStale(cachePath, staleSecs)
	return err == nil && !stale
}

// resolveKitsuMalCtx resolves a kitsu:N or mal:N URL. The UseKitsu and
// UseMAL settings independently control each source:
//   - UseKitsu=true, UseMAL=true (defaults): direct Kitsu/MAL call →
//     Kitsu/MAL artwork (posterImage.original / coverImage.extraLarge /
//     bannerImage).
//   - Either false: try to find an IMDB equivalent first (via the
//     offline anime list, Kitsu or MAL id → IMDb). If a mapping exists, re-resolve via the standard IMDb
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
		// UseKitsu=false: try to find an IMDb/TMDB equivalent via the
		// offline anime list.
		if clients.Mappings != nil {
			if t, v, ok := lookupKitsuStandard(ctx, cache, idValue, clients); ok {
				slog.Debug("kitsu equivalent translation hit",
					"kitsu_id_or_slug", idValue,
					"id", t.String()+"/"+v)
				if r, err := services.ResolveIDCachedCtx(ctx, cache, t, v, clients); err == nil {
					return r, nil
				}
				slog.Debug("kitsu translation resolve failed, falling back to direct", "id", t.String()+"/"+v)
			}
		}
	case services.IDTypeMAL:
		if useMAL {
			return services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
		}
		// UseMAL=false: try the offline anime list for an IMDb/TMDB cross-ref.
		if malID, parseErr := strconv.ParseUint(idValue, 10, 64); parseErr == nil && malID > 0 {
			if t, v, ok := malStandardID(ctx, clients, malID); ok {
				slog.Debug("mal equivalent translation hit",
					"mal_id", malID,
					"id", t.String()+"/"+v)
				if r, err := services.ResolveIDCachedCtx(ctx, cache, t, v, clients); err == nil {
					return r, nil
				}
				slog.Debug("mal translation resolve failed, falling back to direct", "id", t.String()+"/"+v)
			}
		}
	}
	// Fallback: direct Kitsu/MAL. Per the user's spec: "Even if disabled,
	// Kitsu / MAL may still be used when a imdb / tmdb / tvdb key can't be
	// found" — same call, semantically the user's last-resort path.
	return services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
}

// imdbForMAL returns the IMDb id of a MAL anime from the offline anime
// list; AniList's externalLinks (rarely carrying IMDb) only stand in while
// the list hasn't loaded.
func imdbForMAL(ctx context.Context, clients services.IDClients, malID uint64) *string {
	if clients.Mappings.AnimeReady() {
		if a, ok := clients.Mappings.AnimeByMAL(malID); ok {
			imdb, _, _ := a.RatingsIDs()
			return imdb
		}
		return nil
	}
	if clients.AniList != nil {
		return clients.AniList.LookupIMDBByMAL(ctx, malID)
	}
	return nil
}

// kitsuForMAL returns the Kitsu id of a MAL anime from the offline anime
// list; Kitsu's /mappings endpoint only stands in while the list hasn't
// loaded.
func kitsuForMAL(ctx context.Context, m *services.IDMappings, kitsu *services.KitsuClient, malID uint64) *uint64 {
	if m.AnimeReady() {
		return m.KitsuForMAL(malID)
	}
	if kitsu != nil {
		return kitsu.AnimeIDByMALCtx(ctx, malID)
	}
	return nil
}

// standardIDFor returns the id an anime-list entry translates to in the
// standard pipeline: its IMDb id, else its TMDB id of known kind — the pair
// AnimeIDs.RatingsIDs gives the ratings path (ratingsTargetFor), so artwork
// and ratings come from the same title. ok is false when neither is usable.
func standardIDFor(a services.AnimeIDs) (services.IDType, string, bool) {
	imdbID, tmdbID, kind := a.RatingsIDs()
	if imdbID != nil {
		return services.IDTypeIMDB, *imdbID, true
	}
	if tmdbID != 0 {
		return services.IDTypeTMDB, services.FormatTMDbIDValue(tmdbID, kind, nil), true
	}
	return 0, "", false
}

// lookupKitsuStandard accepts either a numeric Kitsu id or a slug and
// returns its standard id (standardIDFor). Numeric values are looked up
// directly; slug values are first resolved to a numeric id via the Kitsu
// slug-filter endpoint.
func lookupKitsuStandard(ctx context.Context, cache *services.MemCache, idOrSlug string, clients services.IDClients) (services.IDType, string, bool) {
	mapper := clients.Mappings
	if mapper == nil || idOrSlug == "" {
		return 0, "", false
	}
	kitsuID, parseErr := strconv.ParseUint(idOrSlug, 10, 64)
	if parseErr != nil || kitsuID == 0 {
		// Slug (e.g. "fairy-tail-2018"): the anime list is keyed by the
		// numeric id, so resolve the slug through the cached Kitsu lookup
		// first. The direct-artwork fallback after a miss reuses the same
		// cache entry.
		if clients.Kitsu == nil {
			return 0, "", false
		}
		r, err := services.ResolveIDCachedCtx(ctx, cache, services.IDTypeKitsu, idOrSlug, clients)
		if err != nil || r == nil || r.KitsuID == nil {
			return 0, "", false
		}
		kitsuID = *r.KitsuID
	}
	a, ok := mapper.AnimeByKitsu(kitsuID)
	if !ok {
		return 0, "", false
	}
	return standardIDFor(a)
}

// malStandardID returns the standard id (standardIDFor) of a MAL anime from
// the offline anime list; while the list hasn't loaded, AniList's IMDb link
// (imdbForMAL) stands in.
func malStandardID(ctx context.Context, clients services.IDClients, malID uint64) (services.IDType, string, bool) {
	if clients.Mappings.AnimeReady() {
		if a, ok := clients.Mappings.AnimeByMAL(malID); ok {
			return standardIDFor(a)
		}
		return 0, "", false
	}
	if imdbID := imdbForMAL(ctx, clients, malID); imdbID != nil {
		return services.IDTypeIMDB, *imdbID, true
	}
	return 0, "", false
}

// resolveWithFallback resolves an id as the type its form names
// (services.EffectiveIDType: tt… is IMDb, movie-/series- is TMDB, whatever
// the URL says; this replaced a blind imdb → tmdb → tvdb retry chain that
// cost a lookup per wrong guess). The use_kitsu_mal checkbox only changes
// how explicit kitsu:N / mal:N ids are handled:
//   - ON:  kitsu:N / mal:N → direct Kitsu/MAL → Kitsu/MAL artwork.
//   - OFF: kitsu:N / mal:N → IMDB-equivalent lookup → fallback to direct.
//
// An IMDb id TMDB doesn't know still falls back to the anime table.
func resolveWithFallback(ctx context.Context, cache *services.MemCache, idType services.IDType, idValue string, clients services.IDClients, useKitsu, useMAL bool) (*services.ResolvedID, error) {
	eff, err := services.EffectiveIDType(idType.String(), idValue)
	if err != nil {
		return nil, err
	}
	if effType, perr := services.ParseIDType(eff); perr == nil && effType != idType {
		slog.Debug("image request: id resolved by its form", "requested", idType.String(), "resolved_as", eff, "id", idValue)
		idType = effType
	}
	if idType == services.IDTypeKitsu || idType == services.IDTypeMAL {
		return resolveKitsuMalCtx(ctx, cache, idType, idValue, clients, useKitsu, useMAL)
	}
	resolved, err := services.ResolveIDCachedCtx(ctx, cache, idType, idValue, clients)
	if err == nil {
		return resolved, nil
	}
	if r := resolveIMDbViaAnimeTable(ctx, cache, idValue, clients); r != nil {
		return r, nil
	}
	return nil, err
}

// resolveIMDbViaAnimeTable is the last resort for an IMDb id ("tt…") that
// TMDB doesn't know — common for some anime movies (e.g. tt19861160, Sailor
// Moon Cosmos): the offline anime list maps it back to a Kitsu entry, whose
// artwork is used (it's the only artwork there is, whatever the "Use Kitsu
// artwork" setting says). The returned copy carries the IMDb id so
// ratingsTargetFor can still fetch ratings by IMDb id. nil when the id isn't
// in the table or Kitsu can't resolve it.
func resolveIMDbViaAnimeTable(ctx context.Context, cache *services.MemCache, idValue string, clients services.IDClients) *services.ResolvedID {
	if !strings.HasPrefix(idValue, "tt") || clients.Mappings == nil || clients.Kitsu == nil {
		return nil
	}
	kitsuID := clients.Mappings.KitsuByIMDb(idValue)
	if kitsuID == nil {
		return nil
	}
	r, err := services.ResolveIDCachedCtx(ctx, cache, services.IDTypeKitsu, strconv.FormatUint(*kitsuID, 10), clients)
	if err != nil || r == nil {
		slog.Info("imdb id not on TMDB; anime-table kitsu entry failed to resolve", "imdb_id", idValue, "kitsu_id", *kitsuID, "error", err)
		return nil
	}
	slog.Info("imdb id not on TMDB; serving its anime entry from Kitsu", "imdb_id", idValue, "kitsu_id", *kitsuID)
	out := *r // r may be the memcached resolver result — never mutate it
	imdb := idValue
	out.IMDbID = &imdb
	return &out
}

// renderTimeout bounds a coalesced or background render, which runs detached
// from any single request's cancellation.
const renderTimeout = 2 * time.Minute

// resolveTitle resolves the requested id as the type its form names (see
// resolveWithFallback), uplifts an episode to its series for the poster,
// logo and backdrop kinds, and applies the anime artwork preference and the
// title identity. The cache key downstream stays the requested
// (idType, idValue).
func (p ServeParams) resolveTitle(idType services.IDType) (*services.ResolvedID, error) {
	resolved, err := resolveWithFallback(p.Context, p.Caches.IDs, idType, p.IDValue, p.idClients(), p.Settings.UseKitsu, p.Settings.UseMAL)
	if err != nil {
		// Logged so a request that "returns nothing" (the media server then
		// shows its own artwork) is visible and explained.
		slog.Info("image not resolved", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "error", err)
		return nil, err
	}

	if p.Kind != "episode" && resolved.MediaType == services.MediaTypeEpisode {
		if resolved.Episode != nil {
			seriesID := services.FormatTMDbIDValue(resolved.Episode.ShowTMDbID, services.MediaTypeTV, nil)
			resolved, err = services.ResolveIDCachedCtx(p.Context, p.Caches.IDs, services.IDTypeTMDB, seriesID, p.idClients())
			if err != nil {
				return nil, err
			}
		}
	}

	if p.Kind == "episode" && resolved.MediaType != services.MediaTypeEpisode {
		return nil, apperr.NewBadRequest("not an episode - use poster/logo/backdrop endpoint")
	}

	// Anime artwork preference (Prefer Kitsu / Prefer MAL) — swaps the
	// direct image URLs before rendering; refreshStale reuses this result.
	resolved = p.applyAnimeArtworkPreference(resolved)
	// Title identity (grouping key, ratings cross-ref, release date fill) —
	// after the artwork swap so a Prefer-Kitsu swap's Kitsu id is used.
	return p.withTitleIdentity(resolved), nil
}

func (p ServeParams) imageSize() services.ImageSize {
	if p.ImageSizeStr != nil {
		return services.ParseImageSize(*p.ImageSizeStr)
	}
	return services.ImageSizeMedium
}

func ServeImage(p ServeParams) ([]byte, string, error) {
	if p.Context == nil {
		p.Context = context.Background()
	}
	start := time.Now()
	defer func() {
		services.LogSlow("image request", time.Since(start).Milliseconds(), "kind", p.Kind, "id", p.IDType+"/"+p.IDValue)
	}()
	idType, err := services.ParseIDType(p.IDType)
	if err != nil {
		return nil, "", err
	}

	if err := services.ValidateIDValue(p.IDValue); err != nil {
		return nil, "", err
	}
	if err := services.ValidateIDFormat(p.IDType, p.IDValue); err != nil {
		return nil, "", err
	}

	if p.TMDB == nil {
		return nil, "", apperr.NewOther("TMDB API key not configured — image generation unavailable")
	}
	if p.Caches == nil {
		p.Caches = &services.MemCacheSet{}
	}

	contentType := ImageContentType(p.Kind)
	imageSize := p.imageSize()

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

	// Cold start: the rendered image may already be on disk while nothing
	// about the id is in memory. Serve it without any upstream call.
	if b, ok := p.serveColdCache(contentType); ok {
		return b, contentType, nil
	}

	// An upstream outage during resolution moments ago isn't retried on
	// every request for this id (see resolveFailures).
	resolveKey := p.IDType + "/" + p.IDValue
	if err := resolveFailures.recent(resolveKey); err != nil {
		slog.Debug("id resolution failed recently, not retrying yet", "id", resolveKey, "error", err)
		return nil, "", err
	}
	resolved, err := p.resolveTitle(idType)
	if err != nil {
		resolveFailures.rememberUpstream(resolveKey, err)
		return nil, "", err
	}

	// Fetch ratings, apply preferences, and build the cache key.
	badges, cacheKey, cachePath, releaseDate, imageTypeChar, available, err := p.prepareRender(resolved)
	if err != nil {
		return nil, "", err
	}
	if p.Served != nil {
		*p.Served = services.CDNServed{CacheKey: cacheKey, CachePath: cachePath, ContentType: contentType, ReleaseDate: releaseDate}
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
				if stale, err := services.CacheIsStale(cachePath, revalStaleSecs); err == nil && stale {
					slog.Debug("image mem cache entry stale, background refresh", "kind", p.Kind, "cache_key", cacheKey)
					goBackground(func() { p.refreshStale(resolved, imageSize, cacheKey) })
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
			goBackground(func() { p.refreshStale(resolved, imageSize, cacheKey) })
			if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
				slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
			}
			return entry.Bytes, contentType, nil
		}
	}

	// A render of this key that failed moments ago (no artwork, upstream
	// down) isn't retried on every request: see renderFailures.
	if err := renderFailures.recent(cacheKey); err != nil {
		slog.Debug("render failed recently, not retrying yet", "kind", p.Kind, "cache_key", cacheKey, "error", err)
		return nil, "", err
	}

	// Render (fetch artwork + generate + persist), coalesced so concurrent
	// requests for the same cache key share a single render. The shared
	// render must not die with the leader's request: detach it from the
	// caller's cancellation (keeping its values) and bound it instead.
	render := func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(p.Context), renderTimeout)
		defer cancel()
		rp := p
		rp.Context = ctx
		b, err := rp.renderArtwork(resolved, badges, cacheKey, cachePath, releaseDate, imageTypeChar, available, imageSize)
		if err != nil {
			renderFailures.remember(cacheKey, err)
		} else {
			renderFailures.forget(cacheKey)
		}
		return b, err
	}
	var rendered []byte
	if p.Inflight != nil {
		rendered, err = p.Inflight.RunCoalescedCtx(p.Context, cacheKey, render)
	} else {
		rendered, err = render()
	}
	if err != nil {
		return nil, "", err
	}

	slog.Debug("image generated", "kind", p.Kind, "cache_key", cacheKey, "badges", len(badges))
	return rendered, contentType, nil
}

// ratingsTargetFor finds the standard (TMDB-resolved) title behind a
// Kitsu/MAL-resolved one so its ratings can be fetched, through the offline
// anime list: the Kitsu (or MAL) entry's IMDb id resolves through TMDB /find
// (cached in Caches.IDs); an entry without one but with a TMDB id of known
// kind (movie / tv) resolves by that id. Returns nil when no IMDb/TMDB match
// exists. The list maps season entries to the whole show, so a season's
// poster gets the show's ratings.
func (p ServeParams) ratingsTargetFor(resolved *services.ResolvedID) (*services.ResolvedID, *uint64) {
	kitsuID := resolved.KitsuID
	if kitsuID == nil && resolved.MALID != nil {
		kitsuID = kitsuForMAL(p.Context, p.Mappings, p.Kitsu, *resolved.MALID)
	}
	// An IMDb id may already be known (the IMDb→Kitsu anime fallback in
	// resolveWithFallback carries the requested one).
	imdbID := resolved.IMDbID
	var tmdbID uint64
	tmdbKind := resolved.MediaType
	entry, found := services.AnimeIDs{}, false
	if kitsuID != nil {
		entry, found = p.Mappings.AnimeByKitsu(*kitsuID)
	}
	if !found && resolved.MALID != nil {
		entry, found = p.Mappings.AnimeByMAL(*resolved.MALID)
	}
	if found {
		listIMDb, listTMDB, listKind := entry.RatingsIDs()
		if imdbID == nil {
			imdbID = listIMDb
		}
		tmdbID, tmdbKind = listTMDB, listKind
	} else if imdbID == nil && resolved.MALID != nil && !p.Mappings.AnimeReady() {
		imdbID = imdbForMAL(p.Context, p.idClients(), *resolved.MALID)
	}
	if imdbID == nil && tmdbID != 0 {
		target, err := services.ResolveIDCachedCtx(p.Context, p.Caches.IDs, services.IDTypeTMDB,
			services.FormatTMDbIDValue(tmdbID, tmdbKind, nil), services.IDClients{TMDB: p.TMDB})
		if err == nil && target != nil {
			slog.Debug("kitsu/mal ratings cross-ref", "id", p.IDType+"/"+p.IDValue, "tmdb_id", target.TMDbID)
			return target, kitsuID
		}
	}
	if imdbID == nil {
		slog.Info("kitsu/mal title has no imdb match — rendering without ratings",
			"id", p.IDType+"/"+p.IDValue,
			"kitsu_id", derefUint(kitsuID),
			"mal_id", derefUint(resolved.MALID),
			"anime_list_loaded", p.Mappings.AnimeReady())
		return nil, kitsuID
	}
	target, err := services.ResolveIDCachedCtx(p.Context, p.Caches.IDs, services.IDTypeIMDB, *imdbID, services.IDClients{TMDB: p.TMDB})
	if err != nil || target == nil || target.TMDbID == 0 {
		// TMDB doesn't know this IMDb id (common for some anime movies). The
		// IMDb id alone still gets ratings from OMDb / MDBList / Trakt; only
		// the TMDB rating is skipped (fetchTmdbRatingCtx guards TMDbID 0).
		slog.Info("kitsu/mal title has no TMDB match — ratings via its IMDb id only", "id", p.IDType+"/"+p.IDValue, "imdb_id", *imdbID, "error", err)
		return &services.ResolvedID{IMDbID: imdbID, MediaType: resolved.MediaType}, kitsuID
	}
	slog.Debug("kitsu/mal ratings cross-ref", "id", p.IDType+"/"+p.IDValue, "imdb_id", *imdbID, "tmdb_id", target.TMDbID)
	return target, kitsuID
}

// withTitleIdentity returns a copy of resolved carrying the per-request
// title identity (never mutates it — the resolver result is memcached):
//   - TitleKey: "tmdb:<movie-/series-/episode-…>" for TMDB titles and for
//     Kitsu/MAL titles cross-referenced to one, else "kitsu:<id>" /
//     "mal:<id>". Stored in image_meta.title_key so the admin list groups
//     every image of one title (all IDs, slug included) together.
//   - RatingsTarget: the cross-referenced IMDb/TMDB title prepareRender
//     fetches ratings for (looked up once here, whatever the ratings limit).
//   - ReleaseDate / Title: filled from the cross-ref when Kitsu/AniList
//     gave none.
func (p ServeParams) withTitleIdentity(resolved *services.ResolvedID) *services.ResolvedID {
	if resolved == nil {
		return nil
	}
	out := *resolved
	if resolved.SourceProvider == "" {
		switch {
		case resolved.TMDbID != 0 || resolved.Episode != nil:
			out.TitleKey = "tmdb:" + services.FormatTMDbIDValue(resolved.TMDbID, resolved.MediaType, resolved.Episode)
		case resolved.TVDBID != nil:
			// A TVDB-only title (TMDB doesn't know it).
			kind := "series"
			if resolved.MediaType == services.MediaTypeMovie {
				kind = "movie"
			}
			out.TitleKey = fmt.Sprintf("tvdb:%s-%d", kind, *resolved.TVDBID)
		}
		return &out
	}
	target, kitsuID := p.ratingsTargetFor(resolved)
	out.RatingsTarget = target
	if p.Caches != nil {
		services.RememberRatingsTarget(p.Caches.IDs, p.IDType, p.IDValue, target)
	}
	switch {
	case target != nil && target.TMDbID == 0 && target.IMDbID != nil:
		out.TitleKey = "imdb:" + *target.IMDbID // IMDb-only (no TMDB entry)
	case target != nil:
		out.TitleKey = "tmdb:" + services.FormatTMDbIDValue(target.TMDbID, target.MediaType, nil)
		if out.ReleaseDate == nil {
			out.ReleaseDate = target.ReleaseDate
		}
		if out.Title == nil {
			out.Title = target.Title
		}
	case kitsuID != nil:
		out.TitleKey = fmt.Sprintf("kitsu:%d", *kitsuID)
	case resolved.MALID != nil:
		out.TitleKey = fmt.Sprintf("mal:%d", *resolved.MALID)
	}
	return &out
}

// applyAnimeArtworkPreference implements Settings.AnimeArtwork for a
// Kitsu/MAL-resolved title (SourceProvider set) while both UseKitsu and
// UseMAL are on: "kitsu" swaps a MAL-sourced title to Kitsu's poster/cover
// (MAL → Kitsu via /mappings, then GetAnimeCtx), "mal" swaps a Kitsu-sourced
// title to AniList's cover/banner (via the MAL id Kitsu's mappings carry).
// "id" or anything unavailable keeps the requested ID's own artwork — a
// failed lookup never errors the request. Returns a copy when it changes
// anything: resolved may be the memcached resolver result.
func (p ServeParams) applyAnimeArtworkPreference(resolved *services.ResolvedID) *services.ResolvedID {
	if resolved == nil || resolved.SourceProvider == "" || p.Settings == nil || !p.Settings.UseKitsu || !p.Settings.UseMAL {
		return resolved
	}
	pref := services.ParseAnimeArtwork(string(p.Settings.AnimeArtwork))
	switch {
	case pref == services.AnimeArtworkKitsu && resolved.SourceProvider == "mal":
		if p.Kitsu == nil {
			return resolved
		}
		kitsuID := resolved.KitsuID
		if kitsuID == nil && resolved.MALID != nil {
			kitsuID = kitsuForMAL(p.Context, p.Mappings, p.Kitsu, *resolved.MALID)
		}
		if kitsuID == nil {
			slog.Info("prefer-kitsu artwork: no kitsu id for mal title, keeping mal artwork", "id", p.IDType+"/"+p.IDValue)
			return resolved
		}
		anime, _, err := p.Kitsu.GetAnimeCtx(p.Context, strconv.FormatUint(*kitsuID, 10))
		if err != nil || anime == nil {
			slog.Info("prefer-kitsu artwork: kitsu lookup failed, keeping mal artwork", "id", p.IDType+"/"+p.IDValue, "kitsu_id", *kitsuID, "error", err)
			return resolved
		}
		out := *resolved
		id := anime.ID
		out.KitsuID = &id
		out.DirectPosterURL = firstNonEmpty(anime.PosterImageOriginal, resolved.DirectPosterURL)
		out.DirectCoverURL = firstNonEmpty(anime.CoverImageOriginal, resolved.DirectCoverURL)
		out.SourceProvider = "kitsu"
		return &out
	case pref == services.AnimeArtworkMAL && resolved.SourceProvider == "kitsu":
		if p.AniList == nil || resolved.MALID == nil {
			slog.Info("prefer-mal artwork: no mal id for kitsu title, keeping kitsu artwork", "id", p.IDType+"/"+p.IDValue)
			return resolved
		}
		media, err := p.AniList.MediaByMALCtx(p.Context, *resolved.MALID)
		if err != nil || media == nil {
			slog.Info("prefer-mal artwork: anilist lookup failed, keeping kitsu artwork", "id", p.IDType+"/"+p.IDValue, "mal_id", *resolved.MALID, "error", err)
			return resolved
		}
		out := *resolved
		out.DirectPosterURL = firstNonEmpty(media.CoverImageExtra, resolved.DirectPosterURL)
		out.DirectCoverURL = firstNonEmpty(media.BannerImage, resolved.DirectCoverURL)
		out.SourceProvider = "mal"
		return &out
	}
	return resolved
}

// firstNonEmpty returns preferred when it holds a URL, else fallback — so a
// preferred provider missing one image kind keeps the original for that kind.
func firstNonEmpty(preferred, fallback *string) *string {
	if preferred != nil && *preferred != "" {
		return preferred
	}
	return fallback
}

// idClients bundles the resolver clients of this request.
func (p ServeParams) idClients() services.IDClients {
	return services.IDClients{TMDB: p.TMDB, Kitsu: p.Kitsu, AniList: p.AniList, Mappings: p.Mappings, TVDB: p.TVDB}
}

// derefUint returns *v, or 0 for nil, so optional ids log as numbers
// rather than pointer addresses.
func derefUint(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

// prepareRender fetches ratings (when the kind's limit allows), applies
// preferences, and computes the cache key/path for the request's ID form.
// It is the first half of the render pipeline, shared by ServeImage and the
// background-refresh path.
//
// available is the title's full set of rating sources (before preferences)
// when ratings were fetched, nil otherwise; renderArtwork stores it for
// serveColdCache.
func (p ServeParams) prepareRender(resolved *services.ResolvedID) (badges []services.RatingBadge, cacheKey, cachePath string, releaseDate *string, imageTypeChar string, available *string, err error) {
	limit := ratingsLimitForKind(p.Kind, p.Settings, p.RatingsLimit)

	var rawBadges []services.RatingBadge
	// A direct image source (kitsu/mal path) carries no TMDb/IMDb id, and
	// FetchRatingsCtx would 404 on TMDbID=0. withTitleIdentity already
	// cross-referenced the title to its IMDb/TMDB equivalent (RatingsTarget)
	// purely for ratings — the artwork still comes from Kitsu/MAL. No match
	// → ratings skipped as before.
	target := resolved
	if resolved.SourceProvider != "" {
		target = resolved.RatingsTarget
	}
	// A TVDB-only title without an IMDb id has nothing to fetch ratings by.
	if target != nil && target.TMDbID == 0 && target.Episode == nil && (target.IMDbID == nil || *target.IMDbID == "") {
		target = nil
	}
	if limit > 0 && target != nil {
		// Shadowed on purpose: the ratings query below reads the
		// cross-referenced title; the outer resolved (artwork, release
		// date) is untouched.
		resolved := target
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

		fetchRatings := services.FetchRatingsCachedCtx
		if p.freshRatings {
			fetchRatings = services.FetchRatingsFreshCtx
		}
		rawBadges = fetchRatings(p.Context, p.Caches.Ratings,
			services.RatingsClients{TMDB: p.TMDB, OMDB: p.OMDB, MDBList: p.MDBList, Trakt: p.Trakt},
			services.RatingsQuery{ResolvedTMDbID: resolved.TMDbID, MediaType: mediaType, IMDbID: imdbID,
				EpisodeShowTMDbID: epShowID, EpisodeSeason: epSeason, EpisodeEpisode: epEpisode,
				ReleaseDate: resolved.ReleaseDate},
		)
		slog.Debug("ratings fetched",
			"kind", p.Kind,
			"id", p.IDType+"/"+p.IDValue,
			"badges", len(rawBadges),
			"sources", badgeSourceString(rawBadges),
		)
		s := services.AvailableSourcesString(rawBadges)
		available = &s
	} else if limit > 0 {
		slog.Debug("ratings skipped", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "reason", "no imdb/tmdb match for kitsu/mal title")
		// Nothing to fetch ratings by: known to have none (for serveColdCache).
		none := ""
		available = &none
	} else {
		slog.Debug("ratings skipped", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "reason", "ratings_limit=0")
	}

	badges = services.ApplyRatingPreferences(rawBadges, p.Settings.RatingsOrder, p.Settings.RatingsExclude, limit)

	suffix := p.renderSuffix(badges)
	cacheKey, cachePath, err = p.cacheKeyFor(p.IDType, p.IDValue, suffix)
	releaseDate = resolved.ReleaseDate
	imageTypeChar = ImageDbValue(p.Kind)
	return badges, cacheKey, cachePath, releaseDate, imageTypeChar, available, err
}

// renderSuffix builds the settings+ratings cache suffix for the given badges.
// It is ID-independent, so it can be reused for alternate-ID cache keys.
func (p ServeParams) renderSuffix(badges []services.RatingBadge) string {
	return p.renderSuffixFromRatings(services.BadgesCacheSuffix(badges))
}

func (p ServeParams) renderSuffixFromRatings(ratingsSuffix string) string {
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
func (p ServeParams) renderArtwork(resolved *services.ResolvedID, badges []services.RatingBadge, cacheKey, cachePath string, releaseDate *string, imageTypeChar string, available *string, imageSize services.ImageSize) ([]byte, error) {
	imageBytes, err := p.fetchBaseArtwork(resolved, imageSize)
	if err != nil {
		return nil, err
	}

	// Render badges onto the artwork, at most RENDER_CONCURRENCY at a time
	// (the slot is taken after the upstream fetch, so a slow download never
	// holds one). Declined with 503 + Retry-After when the queue is full.
	release, err := currentRenderSlots().acquire(p.Context)
	if err != nil {
		return nil, err
	}
	renderStart := time.Now()
	rendered, err := GenerateImage(imageBytes, badges, p.Settings, p.Kind, p.Quality, &imageSize)
	release()
	services.LogSlow("render", time.Since(renderStart).Milliseconds(), "kind", p.Kind, "cache_key", cacheKey)
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
	if err := services.UpsertImageMetaCtx(p.Context, p.DB, cacheKey, releaseDate, imageTypeChar, resolved.TitleKey, resolved.Title); err != nil {
		slog.Warn("failed to upsert image meta", "cache_key", cacheKey, "error", err)
	}
	// The sources this render's key was built from, so serveColdCache can
	// rebuild the key without fetching ratings (requested id form only).
	if available != nil && p.DB != nil {
		if err := services.UpsertAvailableRatingsCtx(p.Context, p.DB, p.IDType+"/"+p.IDValue, *available, releaseDate); err != nil {
			slog.Warn("failed to upsert available ratings", "id", p.IDType+"/"+p.IDValue, "error", err)
		}
	}
	if err := services.TouchImageAccessCtx(p.Context, p.DB, cacheKey); err != nil {
		slog.Warn("failed to touch image access", "cache_key", cacheKey, "error", err)
	}

	// Cross-ID cache writes (fire-and-forget, logged on failure).
	p.writeCrossIDCache(resolved, badges, rendered, available)

	return rendered, nil
}

// fetchBaseArtwork downloads the artwork to render on, trying in order:
//  1. the direct Kitsu/AniList URL of a Kitsu/MAL-resolved title;
//  2. TMDB in the requested language (selectImageRanked tiers) — or
//     Fanart.tv first when it is the configured image source;
//  3. Fanart.tv whenever a Fanart client exists (after TMDB);
//  4. TMDB's default poster_path / backdrop_path;
//  5. TVDB's artwork (with a TVDB client).
//
// A TVDB-only title (TMDbID 0) skips the TMDB steps.
//
// A failed step falls through to the next one; only a failure of the last
// step is returned. (nil, nil) never happens: no artwork is a 404.
func (p ServeParams) fetchBaseArtwork(resolved *services.ResolvedID, imageSize services.ImageSize) ([]byte, error) {
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
	if imageBytes != nil {
		return imageBytes, nil
	}

	fanartFirst := p.Settings.ImageSource.IsFanart() && p.Fanart != nil
	tryFanart := func() []byte {
		if p.Fanart == nil {
			return nil
		}
		return fetchFanartArtworkCtx(p.Context, p.Fanart, p.TVDB, resolved, p.Kind, p.Settings, p.CacheDir, p.ExternalCacheOnly)
	}
	if fanartFirst {
		if b := tryFanart(); b != nil {
			return b, nil
		}
	}
	// Kitsu/MAL resolvers don't carry a TMDb id. When the direct URL produced
	// nothing — e.g. logo/episode kinds, which Kitsu/MAL don't carry —
	// calling TMDB /tv/0 or /movie/0 would only surface a cryptic 404.
	if resolved.SourceProvider != "" && resolved.TMDbID == 0 {
		slog.Warn("no artwork for kitsu/mal-resolved title", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "reason", "kitsu/mal don't carry this kind of art and no TMDB id was resolved")
		return nil, apperr.NewIDNotFound(fmt.Sprintf("no %s artwork available for %s/%s: Kitsu / MAL don't carry %s artwork — use an imdb / tmdb / tvdb key instead", p.Kind, p.IDType, p.IDValue, p.Kind))
	}
	if b := selectedTmdbArtworkCtx(p.Context, p.TMDB, p.CacheDir, p.ExternalCacheOnly, p.ImageStaleSecs, resolved, p.Kind, p.Settings, imageSize); b != nil {
		return b, nil
	}
	if !fanartFirst {
		if b := tryFanart(); b != nil {
			return b, nil
		}
	}
	b, err := defaultTmdbArtworkCtx(p.Context, p.TMDB, p.CacheDir, p.ExternalCacheOnly, p.ImageStaleSecs, resolved, p.Kind, imageSize)
	if b == nil {
		if tb := p.tvdbArtwork(resolved); tb != nil {
			return tb, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if b == nil {
		slog.Warn("no artwork available", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue)
		return nil, apperr.NewIDNotFound("no " + p.Kind + " artwork available for this title")
	}
	return b, nil
}

// tvdbArtwork is the last artwork fallback: the TVDB record's artwork
// (highest score in the request language, its Portuguese sibling, English).
// nil without a TVDB client or id, or when TVDB has none.
func (p ServeParams) tvdbArtwork(resolved *services.ResolvedID) []byte {
	if p.TVDB == nil || p.Kind == "episode" {
		return nil
	}
	var tvdbID *uint64
	if resolved.MediaType == services.MediaTypeMovie {
		tvdbID = resolved.TVDBID
		if tvdbID == nil && resolved.IMDbID != nil {
			if id, err := p.TVDB.RemoteIDCtx(p.Context, *resolved.IMDbID, services.MediaTypeMovie); err == nil && id != 0 {
				tvdbID = &id
			}
		}
	} else {
		tvdbID = services.SeriesTVDBIDCtx(p.Context, resolved, p.TVDB)
	}
	if tvdbID == nil {
		return nil
	}
	var rec *services.TVDBRecord
	var err error
	if resolved.MediaType == services.MediaTypeMovie {
		rec, err = p.TVDB.MovieCtx(p.Context, *tvdbID)
	} else {
		rec, err = p.TVDB.SeriesCtx(p.Context, *tvdbID)
	}
	if err != nil {
		slog.Info("TVDB artwork lookup failed", "kind", p.Kind, "tvdb_id", *tvdbID, "error", err)
		return nil
	}
	url := services.SelectTVDBArtwork(rec, p.Kind, p.Settings.Lang, p.Settings.Textless && p.Kind == "poster")
	if url == "" {
		return nil
	}
	httpClient := p.TVDB.HTTP
	b, err := fetchDirectImageBytesCtx(p.Context, httpClient, url)
	if err != nil {
		slog.Info("TVDB artwork download failed", "kind", p.Kind, "url", url, "error", err)
		return nil
	}
	slog.Debug("artwork from TVDB", "kind", p.Kind, "id", p.IDType+"/"+p.IDValue, "tvdb_id", *tvdbID)
	return b
}

// fetchDirectImageBytesCtx downloads a base artwork from a fully-qualified URL.
// Used for Kitsu/MAL resolved entries whose ResolvedID carries DirectPosterURL
// or DirectCoverURL — those providers don't go through TMDB's CDN, so we fetch
// the bytes straight from the URL the resolver returned. The function does no
// retry: Kitsu's posterImage and AniList's coverImage are static CDN URLs and
// the caller (renderArtwork) logs and falls through to TMDB/Fanart on a miss.
func fetchDirectImageBytesCtx(ctx context.Context, httpClient *http.Client, url string) ([]byte, error) {
	if !allowedDirectArtworkURL(url) {
		return nil, fmt.Errorf("direct artwork URL not allowed: %s", url)
	}
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
	return services.ReadAllLimited(resp.Body, services.MaxArtworkBytes)
}

// refreshStale regenerates a stale cache entry in the background with fresh
// ratings. Guarded by the inflight set so concurrent stale requests for the
// same key trigger only one regeneration. Best-effort: failures are logged.
//
// Runs with an independent background context so the work outlives the client
// request that triggered it (this is server-side cache maintenance, not part
// of the user's response). At most refreshSlotsMax run at once — a refresh
// that finds no free slot is skipped (the next request for the stale entry
// tries again) — and a key whose refresh failed is backed off
// (refreshFailures) instead of being retried on every request.
func (p ServeParams) refreshStale(resolved *services.ResolvedID, imageSize services.ImageSize, staleKey string) {
	now := time.Now()
	if refreshFailures.blocked(staleKey, now) {
		return
	}
	release, ok := refreshSlots.tryAcquire()
	if !ok {
		slog.Debug("background refresh skipped: all refresh slots busy", "cache_key", staleKey)
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	bgParams := p
	bgParams.Context = ctx
	if err := bgParams.rerender(resolved, imageSize); err != nil {
		refreshFailures.failed(staleKey, time.Now())
		return
	}
	refreshFailures.succeeded(staleKey)
}

// rerender renders resolved again with fresh ratings (the body of a
// background refresh; the caller holds a refresh slot).
func (p ServeParams) rerender(resolved *services.ResolvedID, imageSize services.ImageSize) error {
	p.freshRatings = true
	badges, cacheKey, cachePath, releaseDate, imageTypeChar, available, err := p.prepareRender(resolved)
	if err != nil {
		slog.Debug("background refresh: ratings failed", "id", p.IDType+"/"+p.IDValue, "error", err)
		return err
	}
	render := func() ([]byte, error) {
		return p.renderArtwork(resolved, badges, cacheKey, cachePath, releaseDate, imageTypeChar, available, imageSize)
	}
	if p.Inflight != nil {
		_, err = p.Inflight.RunCoalesced(cacheKey, render)
	} else {
		_, err = render()
	}
	if err != nil {
		slog.Debug("background refresh failed", "kind", p.Kind, "cache_key", cacheKey, "error", err)
	}
	return err
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

// selectedTmdbArtworkPath returns the TMDB image the settings ask for: the
// episode still, the default poster for English posters with text, or the
// best /images entry (selectImageRanked / SelectBackdrop). "" when there is
// none or the /images list could not be fetched.
func selectedTmdbArtworkPath(ctx context.Context, tmdb *services.TmdbClient, resolved *services.ResolvedID, kind string, settings *services.RenderSettings) string {
	switch {
	case kind == "episode":
		if resolved.Episode != nil && resolved.Episode.StillPath != nil {
			return *resolved.Episode.StillPath
		}
		return ""
	case kind == "poster" && settings.Lang == "en" && !settings.Textless:
		if resolved.PosterPath != nil {
			return *resolved.PosterPath
		}
		return ""
	}
	if resolved.TMDbID == 0 {
		return ""
	}
	mediaType := "tv"
	if resolved.MediaType == services.MediaTypeMovie {
		mediaType = "movie"
	}
	images, err := tmdb.GetImagesCtx(ctx, mediaType, resolved.TMDbID, settings.Lang, resolved.OriginalLanguage)
	if err != nil {
		slog.Info("TMDB image list failed, trying the next artwork source", "kind", kind, "tmdb_id", resolved.TMDbID, "error", err)
		return ""
	}
	var selected *services.TmdbImage
	switch kind {
	case "poster":
		selected = services.SelectPoster(images.Posters, settings.Lang, resolved.OriginalLanguage, settings.Textless)
	case "logo":
		selected = services.SelectImage(images.Logos, settings.Lang, resolved.OriginalLanguage, false)
	case "backdrop":
		selected = services.SelectBackdrop(images.Backdrops, settings.Lang)
	}
	if selected == nil {
		return ""
	}
	return selected.FilePath
}

// defaultTmdbArtworkPath is the last TMDB fallback: the title's own
// poster_path (posters, episodes without a still) or backdrop_path.
func defaultTmdbArtworkPath(resolved *services.ResolvedID, kind string) string {
	var p *string
	switch kind {
	case "poster", "episode":
		p = resolved.PosterPath
	case "backdrop":
		p = resolved.BackdropPath
	}
	if p == nil {
		return ""
	}
	return *p
}

// selectedTmdbArtworkCtx downloads the selected TMDB artwork; nil when there
// is none or the download failed (the caller moves on to the next source).
func selectedTmdbArtworkCtx(ctx context.Context, tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, imageSize services.ImageSize) []byte {
	filePath := selectedTmdbArtworkPath(ctx, tmdb, resolved, kind, settings)
	if filePath == "" {
		return nil
	}
	b, err := tmdb.FetchBaseImageCtx(ctx, cacheDir, filePath, imageSize.TmdbSize(), imageStaleSecs, externalCacheOnly)
	if err != nil {
		slog.Info("TMDB artwork download failed, trying the next artwork source", "kind", kind, "path", filePath, "error", err)
		return nil
	}
	return b
}

// defaultTmdbArtworkCtx downloads defaultTmdbArtworkPath; (nil, nil) when the
// title has none.
func defaultTmdbArtworkCtx(ctx context.Context, tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, imageSize services.ImageSize) ([]byte, error) {
	filePath := defaultTmdbArtworkPath(resolved, kind)
	if filePath == "" {
		return nil, nil
	}
	return tmdb.FetchBaseImageCtx(ctx, cacheDir, filePath, imageSize.TmdbSize(), imageStaleSecs, externalCacheOnly)
}

// fetchTmdbArtworkCtx is the TMDB part of the chain on its own (selected
// artwork, then the default): used where Fanart doesn't apply.
func fetchTmdbArtworkCtx(ctx context.Context, tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, imageSize services.ImageSize) ([]byte, error) {
	if b := selectedTmdbArtworkCtx(ctx, tmdb, cacheDir, externalCacheOnly, imageStaleSecs, resolved, kind, settings, imageSize); b != nil {
		return b, nil
	}
	return defaultTmdbArtworkCtx(ctx, tmdb, cacheDir, externalCacheOnly, imageStaleSecs, resolved, kind, imageSize)
}

// fetchFanartArtworkCtx downloads the base artwork from fanart.tv. Movies are
// looked up by TMDB id, series only by TVDB id (fanart.tv has no TMDB index
// for TV) — a series without a known TVDB id is skipped. Returns nil when
// unavailable so the caller moves on.
func fetchFanartArtworkCtx(ctx context.Context, fanart *services.FanartClient, tvdbClient *services.TVDBClient, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, cacheDir string, externalCacheOnly bool) []byte {
	if kind == "episode" {
		return nil
	}

	var images *services.FanartImages
	var err error
	switch resolved.MediaType {
	case services.MediaTypeMovie:
		if resolved.TMDbID == 0 {
			return nil
		}
		images, err = fanart.GetMovieImagesCtx(ctx, resolved.TMDbID)
	default:
		tvdb := services.SeriesTVDBIDCtx(ctx, resolved, tvdbClient)
		if tvdb == nil {
			slog.Debug("fanart skipped: series has no TVDB id", "tmdb_id", resolved.TMDbID)
			return nil
		}
		images, err = fanart.GetTVImagesCtx(ctx, *tvdb)
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
	// Backdrops prefer textless ("00") and untagged art, like on TMDB.
	lang, textless := settings.Lang, settings.Textless && kind == "poster"
	if kind == "backdrop" {
		lang, textless = "", true
	}
	selected, _, ok := services.SelectFanartImage(candidates, lang, textless)
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

// directArtworkHosts are the CDNs Kitsu and AniList serve artwork from. The
// direct-artwork URLs come from those providers' API responses and were
// fetched as-is; restricting them to https on these domains keeps a bad or
// compromised upstream response from making the server fetch arbitrary
// (e.g. internal) URLs. A miss is logged and the render falls back to TMDB.
var directArtworkHosts = []string{"kitsu.app", "kitsu.io", "anilist.co", "anili.st", "thetvdb.com"}

func allowedDirectArtworkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range directArtworkHosts {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
