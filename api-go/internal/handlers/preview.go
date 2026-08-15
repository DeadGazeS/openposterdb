package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"golang.org/x/image/font"

	"openposterdb/internal/httpx"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

const (
	previewPosterRatingsLimit       = 3
	previewLogoBackdropRatingsLimit = 5
)

// loadPreviewFonts resolves the value + label font faces for a preview render
// at the given text size. On missing font it writes a 500 response and returns
// ok=false; callers should `return` on !ok.
func loadPreviewFonts(w http.ResponseWriter, textSize services.ScalePercent) (labelFace, valueFace font.Face, ok bool) {
	labelFace, valueFace = image.GetFontFacesAt(float64(textSize))
	if valueFace == nil || labelFace == nil {
		httpx.WriteJSON(w, 500, map[string]string{"error": "font not loaded"})
		return nil, nil, false
	}
	return labelFace, valueFace, true
}

// writePreviewImage writes the preview response headers (Content-Type +
// Cache-Control) and the rendered bytes. Content-type per kind:
// image/jpeg for poster/backdrop/episode, image/png for logo.
func writePreviewImage(w http.ResponseWriter, contentType string, rendered []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
}

func sampleBadges() []services.RatingBadge {
	return []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
		{Source: services.SourceTmdb, Value: "100%"},
		{Source: services.SourceRt, Value: "100%"},
		{Source: services.SourceRtAudience, Value: "100%"},
		{Source: services.SourceMetacritic, Value: "100"},
		{Source: services.SourceTrakt, Value: "100%"},
		{Source: services.SourceLetterboxd, Value: "5.0"},
		{Source: services.SourceMal, Value: "10.00"},
		{Source: services.SourceMdblist, Value: "100"},
		{Source: services.SourceEbert, Value: "4.0"},
	}
}

// previewLayout returns the layout from the query, falling back to the kind
// default.
func previewLayout(query *ImageQuery, kind string) services.ImageLayout {
	def := services.DefaultLayout(kind)
	if query.Layout != nil && *query.Layout != "" {
		return services.UnmarshalLayout(*query.Layout, &def)
	}
	return def
}

// previewScales returns the clamped text/badge/logo scale percentages from the
// preview query, each defaulting to 100 when absent.
func previewScales(query *ImageQuery) (text, badge, logo services.ScalePercent) {
	text = services.DefaultScalePercent()
	badge = services.DefaultScalePercent()
	logo = services.DefaultScalePercent()
	if query.TextSize != nil {
		text = services.ClampScalePercent(*query.TextSize)
	}
	if query.BadgeSize != nil {
		badge = services.ClampScalePercent(*query.BadgeSize)
	}
	if query.LogoSize != nil {
		logo = services.ClampScalePercent(*query.LogoSize)
	}
	return
}

// previewBadgeMultiplier returns the badge frame multiplier for a kind at the
// given badge-size percentage (100 = the kind's default badge size).
func previewBadgeMultiplier(kind string, badge services.ScalePercent) float32 {
	def := float32(1.2)
	if kind == "episode" {
		def = 1.45
	}
	return def * badge.Percent()
}

func parsePreviewImageSize(raw *string, kind string) (*services.ImageSize, error) {
	if raw == nil {
		return nil, nil
	}
	s := services.ParseImageSize(*raw)
	if s == services.ImageSizeSmall && kind != "backdrop" && kind != "episode" {
		return nil, fmt.Errorf("imageSize 'small' is only valid for backdrops and episodes")
	}
	return &s, nil
}

type PreviewHandler struct {
	ctx context.Context
	db  *sql.DB
	cfg *PreviewConfig
}

type PreviewConfig struct {
	CacheDir          string
	ExternalCacheOnly bool
	ImageQuality      uint8
	TMDB              *services.TmdbClient
	OMDB              *services.OmdbClient
	MDBList           *services.MdblistClient
	Trakt             *services.TraktClient
	Kitsu             *services.KitsuClient
	AniList           *services.AniListClient
	KitsuIMDbMapper   *services.KitsuIMDbMapper
}

// demoArtworkBytes returns the base artwork for the demo title (IMDb
// tt12637874; S01E01 for the episode preview), preferring the on-disk base
// cache. Falls back to the built-in sample gradient if TMDB is unavailable or
// the title can't be resolved, so the settings page never errors on artwork.
func (p *PreviewHandler) demoArtworkBytes(kind string) ([]byte, error) {
	if p.cfg.TMDB == nil {
		return sampleArtwork(kind), nil
	}
	bytes, err := image.DemoArtworkCtx(p.ctx, p.cfg.TMDB, p.cfg.CacheDir, p.cfg.ExternalCacheOnly, 0, kind, services.ImageSizeMedium)
	if err != nil || len(bytes) == 0 {
		return sampleArtwork(kind), nil
	}
	return bytes, nil
}

func sampleArtwork(kind string) []byte {
	switch kind {
	case "logo":
		return image.SampleLogoPNG
	case "backdrop", "episode":
		return image.SampleBackdropPNG
	default:
		return image.SamplePosterPNG
	}
}

// demoBadges returns the demo title's ratings for the preview of the given
// kind. Ratings are fetched once from the external providers and persisted in
// available_ratings so subsequent previews are served from the cache. When no
// ratings can be fetched (or no providers are configured) it falls back to the
// built-in sample badge set so every source's colors stay testable.
func (p *PreviewHandler) demoBadges(kind string) []services.RatingBadge {
	idValue := image.DemoIMDBID
	if kind == "episode" {
		idValue = image.DemoEpisodeID
	}
	idKey := "imdb/" + idValue

	if p.cfg.TMDB != nil {
		if sources, _, _, err := services.ReadAvailableRatingsCtx(p.ctx, p.db, idKey); err == nil {
			if badges := services.UnmarshalRatingBadges(sources); len(badges) > 0 {
				return badges
			}
		}

		if badges := p.fetchDemoRatings(idValue); len(badges) > 0 {
			if encoded := services.MarshalRatingBadges(badges); encoded != "" {
				// Best-effort cache of preview-renderable ratings; the preview
				// itself doesn't depend on this row.
				if err := services.UpsertAvailableRatingsCtx(p.ctx, p.db, idKey, encoded, nil); err != nil {
					slog.Warn("preview upsert available ratings failed", "id_key", idKey, "error", err)
				}
			}
			return badges
		}
	}

	return sampleBadges()
}

func (p *PreviewHandler) fetchDemoRatings(idValue string) []services.RatingBadge {
	resolved, err := services.ResolveIDCtx(p.ctx, services.IDTypeIMDB, idValue, services.IDClients{TMDB: p.cfg.TMDB})
	if err != nil {
		return nil
	}
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
	var showID uint64
	var season, episode uint32
	if resolved.Episode != nil {
		showID = resolved.Episode.ShowTMDbID
		season = resolved.Episode.SeasonNumber
		episode = resolved.Episode.EpisodeNumber
	}
	result := services.FetchRatingsCtx(
		p.ctx,
		services.RatingsClients{TMDB: p.cfg.TMDB, OMDB: p.cfg.OMDB, MDBList: p.cfg.MDBList, Trakt: p.cfg.Trakt},
		services.RatingsQuery{ResolvedTMDbID: resolved.TMDbID, MediaType: mediaType, IMDbID: imdbID,
			EpisodeShowTMDbID: showID, EpisodeSeason: season, EpisodeEpisode: episode},
	)
	return result.Badges
}

func NewPreviewHandler(ctx context.Context, db *sql.DB, cfg *PreviewConfig) *PreviewHandler {
	return &PreviewHandler{ctx: ctx, db: db, cfg: cfg}
}

// previewBuild is the shared prologue across HandlePoster/Logo/Backdrop/Episode.
// It parses the query, resolves sizes/scales/ratings/badge settings/layout, and
// loads fonts + colours + badges. On error it writes the 400/500 response and
// returns ok=false; the caller should just `return`. The four kind handlers
// add 1-2 kind-specific extras (posterFit, edgeInsets, blur) and then call
// the kind-specific render function with build.RenderParams.
type previewBuild struct {
	// Query-derived values the handlers inspect after this returns.
	query *ImageQuery

	// Per-kind scale + size values.
	textSize        services.ScalePercent
	badgeSize       services.ScalePercent
	logoSize        services.ScalePercent
	targetWidth     uint32
	badgeMultiplier float32
	badgeScale      float32
	logoScale       float32
	textScale       float32

	// Ratings: limit, order, exclude, layout, badge style/label/shape/alpha
	// appearance — resolved once here and passed through RenderParams.
	ratingsLimit   int32
	ratingsOrder   string
	ratingsExclude string
	layout         services.ImageLayout
	badgeStyle     services.BadgeStyle
	labelStyle     services.LabelStyle
	appearance     services.BadgeAppearance

	// Font faces + colours + badges — passed to RenderParams.
	labelFace font.Face
	valueFace font.Face
	colors    map[string]services.SourceColorSet
	badges    []services.RatingBadge

	// Quality from config (constant per request, but kept here so the kind
	// handler doesn't reach back into p.cfg for it).
	quality uint8
}

func (p *PreviewHandler) previewBuild(w http.ResponseWriter, r *http.Request, kind string) (*previewBuild, bool) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, kind)
	if err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": err.Error()})
		return nil, false
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	textSize, badgeSize, logoSize := previewScales(query)

	var targetWidth uint32
	switch kind {
	case "poster":
		targetWidth = resolvedSize.PosterTargetWidth()
	case "logo":
		targetWidth = resolvedSize.LogoTargetWidth()
	case "backdrop":
		targetWidth = resolvedSize.BackdropTargetWidth()
	case "episode":
		targetWidth = resolvedSize.EpisodeTargetWidth()
	}

	badgeMultiplier := previewBadgeMultiplier(kind, badgeSize)
	badgeScale := resolvedSize.BadgeScale(kind) * badgeMultiplier
	logoScale := logoSize.Percent()
	textScale := textSize.Percent()

	// Per-kind default ratings limit: poster/episode use the smaller poster
	// limit; logo/backdrop use the larger one. An explicit ?ratings_limit
	// beats the layout total when valid; otherwise we fall back to the
	// layout total below (matching the serve path's override ?? layout.Total()
	// semantics).
	defaultLimit := int32(previewLogoBackdropRatingsLimit)
	if kind == "poster" || kind == "episode" {
		defaultLimit = int32(previewPosterRatingsLimit)
	}
	ratingsLimit := defaultLimit
	overrideValid := query.RatingsLimit != nil && services.ValidateRatingsLimit(*query.RatingsLimit) == nil
	if overrideValid {
		ratingsLimit = *query.RatingsLimit
	}

	defaultOrder := services.DefaultRatingsOrder()
	ratingsOrder := defaultOrder
	if query.RatingsOrder != nil && *query.RatingsOrder != "" {
		ratingsOrder = *query.RatingsOrder
	}

	ratingsExclude := ""
	if query.RatingsExclude != nil {
		ratingsExclude = *query.RatingsExclude
	}

	layout := previewLayout(query, kind)
	if !overrideValid {
		ratingsLimit = layout.Total()
	}

	// Per-kind default badge style: poster uses the project's default (the
	// resolver picks the right shape-aware style); logo/backdrop/episode use
	// the top-bottom style as their base.
	defaultBadgeStyle := services.BadgeStyleLogoTB
	if kind == "poster" {
		defaultBadgeStyle = services.BadgeStyleDefault
	}
	rawBadgeStyle := defaultBadgeStyle
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	// badgeDirection matters for poster/backdrop/episode; the logo badge
	// layout is direction-agnostic, so logo just resolves the raw style
	// directly. Other kinds resolve the raw style against the query direction
	// (or the default-resolved one).
	badgeStyle := rawBadgeStyle.ResolveDefault()
	if kind != "logo" {
		direction := services.BadgeDirectionDefault.ResolveDefault()
		if query.BadgeDirection != nil {
			direction = services.BadgeDirection(*query.BadgeDirection).ResolveDefault()
		}
		badgeStyle = rawBadgeStyle.Resolve(direction)
	}

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	alpha := services.DefaultBadgeAlpha()
	if query.BadgeAlpha != nil {
		alpha = services.ClampBadgeAlpha(*query.BadgeAlpha)
	}
	appearance := services.BadgeAppearance{Shape: shape, Alpha: alpha, Style: badgeStyle, Width: services.DefaultScalePercent(), Height: services.DefaultScalePercent()}
	if query.BadgeWidth != nil {
		appearance.Width = services.ClampScalePercent(*query.BadgeWidth)
	}
	if query.BadgeHeight != nil {
		appearance.Height = services.ClampScalePercent(*query.BadgeHeight)
	}

	badges := p.demoBadges(kind)
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)

	labelFace, valueFace, ok := loadPreviewFonts(w, textSize)
	if !ok {
		return nil, false
	}

	colors := parsePreviewColors(r)

	return &previewBuild{
		query:           query,
		textSize:        textSize,
		badgeSize:       badgeSize,
		logoSize:        logoSize,
		targetWidth:     targetWidth,
		badgeMultiplier: badgeMultiplier,
		badgeScale:      badgeScale,
		logoScale:       logoScale,
		textScale:       textScale,
		ratingsLimit:    ratingsLimit,
		ratingsOrder:    ratingsOrder,
		ratingsExclude:  ratingsExclude,
		layout:          layout,
		badgeStyle:      badgeStyle,
		labelStyle:      labelStyle,
		appearance:      appearance,
		labelFace:       labelFace,
		valueFace:       valueFace,
		colors:          colors,
		badges:          badges,
		quality:         p.cfg.ImageQuality,
	}, true
}

func (p *PreviewHandler) HandlePoster(w http.ResponseWriter, r *http.Request) {
	b, ok := p.previewBuild(w, r, "poster")
	if !ok {
		return
	}

	posterFit := services.PosterFitNative
	if b.query.Fit != nil {
		posterFit = services.PosterFit(*b.query.Fit)
	}

	posterBytes, err := p.demoArtworkBytes("poster")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	// Pass the demo artwork through unchanged so every fit previews exactly
	// what the real image endpoint renders (native = the full poster at its
	// natural ratio; cover/pad/blur shape the 2:3 canvas). The web preview box
	// is sized from the rendered image's natural dimensions, so the portrait
	// box is stable for the 2:3 demo poster under every fit.
	posterBytes, err = image.PosterPreviewArtwork(posterBytes, posterFit, b.targetWidth)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderPosterSync(posterBytes, image.RenderParams{
		Badges: b.badges, ValueFontFace: b.valueFace, LabelFontFace: b.labelFace,
		Quality: b.quality, Layout: b.layout, BadgeStyle: b.badgeStyle,
		LabelStyle: b.labelStyle, Appearance: b.appearance, TargetWidth: b.targetWidth,
		BadgeScale: b.badgeScale, BadgeMultiplier: b.badgeMultiplier, TextScale: b.textScale,
		LogoScale: b.logoScale, PosterFit: posterFit, Colors: b.colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	writePreviewImage(w, "image/jpeg", rendered)
}

func (p *PreviewHandler) HandleLogo(w http.ResponseWriter, r *http.Request) {
	b, ok := p.previewBuild(w, r, "logo")
	if !ok {
		return
	}

	logoBytes, err := p.demoArtworkBytes("logo")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderLogoSync(logoBytes, image.RenderParams{
		Badges: b.badges, ValueFontFace: b.valueFace, LabelFontFace: b.labelFace,
		Layout: b.layout, BadgeStyle: b.badgeStyle, LabelStyle: b.labelStyle,
		Appearance: b.appearance, TargetWidth: b.targetWidth, BadgeScale: b.badgeScale,
		BadgeMultiplier: b.badgeMultiplier, TextScale: b.textScale, LogoScale: b.logoScale,
		Colors: b.colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	writePreviewImage(w, "image/png", rendered)
}

func (p *PreviewHandler) HandleBackdrop(w http.ResponseWriter, r *http.Request) {
	b, ok := p.previewBuild(w, r, "backdrop")
	if !ok {
		return
	}

	edgeInsetX := int32(0)
	if b.query.EdgeInsetX != nil {
		edgeInsetX = *b.query.EdgeInsetX
	}
	edgeInsetY := int32(0)
	if b.query.EdgeInsetY != nil {
		edgeInsetY = *b.query.EdgeInsetY
	}

	backdropBytes, err := p.demoArtworkBytes("backdrop")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderBackdropSync(backdropBytes, image.RenderParams{
		Badges: b.badges, ValueFontFace: b.valueFace, LabelFontFace: b.labelFace,
		Quality: b.quality, Layout: b.layout, BadgeStyle: b.badgeStyle,
		LabelStyle: b.labelStyle, Appearance: b.appearance, TargetWidth: b.targetWidth,
		BadgeScale: b.badgeScale, BadgeMultiplier: b.badgeMultiplier, TextScale: b.textScale,
		LogoScale: b.logoScale, EdgeInsetX: edgeInsetX, EdgeInsetY: edgeInsetY, Colors: b.colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	writePreviewImage(w, "image/jpeg", rendered)
}

func (p *PreviewHandler) HandleEpisode(w http.ResponseWriter, r *http.Request) {
	b, ok := p.previewBuild(w, r, "episode")
	if !ok {
		return
	}

	blur := false
	if b.query.Blur != nil {
		blur = *b.query.Blur
	}

	episodeBytes, err := p.demoArtworkBytes("episode")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderEpisodeSync(episodeBytes, image.RenderParams{
		Badges: b.badges, ValueFontFace: b.valueFace, LabelFontFace: b.labelFace,
		Quality: b.quality, Layout: b.layout, BadgeStyle: b.badgeStyle,
		LabelStyle: b.labelStyle, Appearance: b.appearance, TargetWidth: b.targetWidth,
		BadgeScale: b.badgeScale, BadgeMultiplier: b.badgeMultiplier, TextScale: b.textScale,
		LogoScale: b.logoScale, Blur: blur, Colors: b.colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	writePreviewImage(w, "image/jpeg", rendered)
}

// --- Admin image list/file serving ---
func parsePreviewColors(r *http.Request) map[string]services.SourceColorSet {
	raw := r.URL.Query().Get("colors")
	if raw == "" {
		return nil
	}
	var colors map[string]services.SourceColorSet
	if err := json.Unmarshal([]byte(raw), &colors); err != nil {
		return nil
	}
	if services.ValidateSourceColors(colors) != nil {
		return nil
	}
	return colors
}

func HandleClearKind(db *sql.DB, cacheDir string, imageType string, externalCacheOnly bool, caches *services.MemCacheSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		dirCleared := false
		if !externalCacheOnly {
			subdir := services.ImageSubdir(imageType)
			if subdir != "" {
				_, err := services.StageDirForClear(cacheDir, subdir)
				dirCleared = err == nil
			}
		}

		metaDeleted, _ := services.DeleteImageMetaByKindCtx(r.Context(), db, imageType)

		// The kind isn't recoverable from the mem cache keys cheaply, so a
		// per-kind purge drops the whole image mem cache (rare admin action).
		if caches != nil && caches.ImageMem != nil {
			caches.ImageMem.Clear()
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"dir_cleared":         dirCleared,
			"meta_deleted":        metaDeleted,
		})
	}
}

func HandlePurgeTitle(db *sql.DB, cacheDir string, imageType, idType, idValue string, externalCacheOnly bool, caches *services.MemCacheSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		scope := r.URL.Query().Get("scope")

		var filesDeleted int64 = 0
		if !externalCacheOnly {
			subdir := services.ImageSubdir(imageType)
			if subdir != "" {
				if scope == "variant" {
					ext := "jpg"
					if imageType == "l" {
						ext = "png"
					}
					filesDeleted, _ = services.PurgeVariantFile(cacheDir, subdir, idType, idValue, ext)
				} else {
					filesDeleted, _ = services.PurgeTitleFiles(cacheDir, subdir, idType, idValue)
				}
			}
		}

		var metaDeleted int64 = 0
		if scope == "variant" {
			metaDeleted, _ = services.DeleteImageMetaExactCtx(r.Context(), db, imageType, idType+"/"+idValue)
		} else {
			metaDeleted, _ = services.DeleteImageMetaForTitleCtx(r.Context(), db, imageType, idType, idValue)
		}

		idKey := idType + "/" + idValue
		if scope != "variant" {
			services.DeleteAvailableRatingsCtx(r.Context(), db, idKey)
		}

		// Invalidate the matching in-memory entries so the purge is visible
		// immediately (image mem keys are "idType/cacheValue").
		if caches != nil {
			if caches.ImageMem != nil {
				caches.ImageMem.DeletePrefix(idType + "/" + idValue)
			}
			if caches.IDs != nil {
				caches.IDs.DeletePrefix(idType + "/" + idValue)
			}
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"files_deleted":       filesDeleted,
			"meta_deleted":        metaDeleted,
		})
	}
}
