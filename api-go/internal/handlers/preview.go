package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

const (
	previewPosterRatingsLimit       = 3
	previewLogoBackdropRatingsLimit = 5
)

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

func previewRenderSettings(
	kind string,
	badgeStyle services.BadgeStyle,
	labelStyle services.LabelStyle,
	textSize services.ScalePercent,
	badgeSize services.ScalePercent,
	logoSize services.ScalePercent,
	layout services.ImageLayout,
	badgeDirection services.BadgeDirection,
	appearance services.BadgeAppearance,
	ratingsLimit int32,
	ratingsOrder string,
	ratingsExclude string,
) *services.RenderSettings {
	s := services.DefaultRenderSettings()
	s.RatingsOrder = ratingsOrder
	s.RatingsExclude = ratingsExclude

	switch kind {
	case "poster":
		s.RatingsLimit = ratingsLimit
		s.PosterBadgeStyle = badgeStyle
		s.PosterLabelStyle = labelStyle
		s.PosterTextSize = textSize
		s.PosterBadgeSize = badgeSize
		s.PosterLogoSize = logoSize
		s.PosterLayout = layout
		s.PosterBadgeDirection = badgeDirection
		s.PosterBadgeShape = appearance.Shape
		s.PosterBadgeAlpha = appearance.Alpha
	case "logo":
		s.LogoRatingsLimit = ratingsLimit
		s.LogoBadgeStyle = badgeStyle
		s.LogoLabelStyle = labelStyle
		s.LogoTextSize = textSize
		s.LogoBadgeSize = badgeSize
		s.LogoLogoSize = logoSize
		s.LogoBadgeShape = appearance.Shape
		s.LogoBadgeAlpha = appearance.Alpha
	case "backdrop":
		s.BackdropRatingsLimit = ratingsLimit
		s.BackdropBadgeStyle = badgeStyle
		s.BackdropLabelStyle = labelStyle
		s.BackdropTextSize = textSize
		s.BackdropBadgeSize = badgeSize
		s.BackdropLogoSize = logoSize
		s.BackdropLayout = layout
		s.BackdropBadgeDirection = badgeDirection
		s.BackdropBadgeShape = appearance.Shape
		s.BackdropBadgeAlpha = appearance.Alpha
	case "episode":
		s.EpisodeRatingsLimit = ratingsLimit
		s.EpisodeBadgeStyle = badgeStyle
		s.EpisodeLabelStyle = labelStyle
		s.EpisodeTextSize = textSize
		s.EpisodeBadgeSize = badgeSize
		s.EpisodeLogoSize = logoSize
		s.EpisodeLayout = layout
		s.EpisodeBadgeDirection = badgeDirection
		s.EpisodeBadgeShape = appearance.Shape
		s.EpisodeBadgeAlpha = appearance.Alpha
	}
	return &s
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
}

// demoArtworkBytes returns the base artwork for the demo title (IMDb
// tt12637874; S01E01 for the episode preview), preferring the on-disk base
// cache. Falls back to the built-in sample gradient if TMDB is unavailable or
// the title can't be resolved, so the settings page never errors on artwork.
func (p *PreviewHandler) demoArtworkBytes(kind string) ([]byte, error) {
	if p.cfg.TMDB == nil {
		return sampleArtwork(kind), nil
	}
	bytes, err := image.DemoArtwork(p.cfg.TMDB, p.cfg.CacheDir, p.cfg.ExternalCacheOnly, 0, kind, services.ImageSizeMedium)
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
		if sources, _, _, err := services.ReadAvailableRatings(p.db, idKey); err == nil {
			if badges := services.UnmarshalRatingBadges(sources); len(badges) > 0 {
				return badges
			}
		}

		if badges := p.fetchDemoRatings(idValue); len(badges) > 0 {
			if encoded := services.MarshalRatingBadges(badges); encoded != "" {
				_ = services.UpsertAvailableRatings(p.db, idKey, encoded, nil)
			}
			return badges
		}
	}

	return sampleBadges()
}

func (p *PreviewHandler) fetchDemoRatings(idValue string) []services.RatingBadge {
	resolved, err := services.ResolveID(services.IDTypeIMDB, idValue, p.cfg.TMDB)
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
	result := services.FetchRatings(
		services.RatingsClients{TMDB: p.cfg.TMDB, OMDB: p.cfg.OMDB, MDBList: p.cfg.MDBList, Trakt: p.cfg.Trakt},
		services.RatingsQuery{ResolvedTMDbID: resolved.TMDbID, MediaType: mediaType, IMDbID: imdbID,
			EpisodeShowTMDbID: showID, EpisodeSeason: season, EpisodeEpisode: episode},
	)
	return result.Badges
}

func NewPreviewHandler(db *sql.DB, cfg *PreviewConfig) *PreviewHandler {
	return &PreviewHandler{db: db, cfg: cfg}
}

func (p *PreviewHandler) HandlePoster(w http.ResponseWriter, r *http.Request) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, "poster")
	if err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	textSize, badgeSize, logoSize := previewScales(query)

	targetWidth := resolvedSize.PosterTargetWidth()
	badgeMultiplier := previewBadgeMultiplier("poster", badgeSize)
	badgeScale := resolvedSize.BadgeScale("poster") * badgeMultiplier
	logoScale := logoSize.Percent()
	textScale := textSize.Percent()

	ratingsLimit := int32(previewPosterRatingsLimit)
	// An explicit ?ratings_limit beats the layout total only when valid; an
	// absent or invalid override falls back to the layout total (matching the
	// serve path's override ?? layout.Total() semantics).
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

	layout := previewLayout(query, "poster")
	if !overrideValid {
		ratingsLimit = layout.Total()
	}

	rawBadgeStyle := services.BadgeStyleDefault
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeDirection := services.BadgeDirectionDefault.ResolveDefault()
	if query.BadgeDirection != nil {
		badgeDirection = services.BadgeDirection(*query.BadgeDirection).ResolveDefault()
	}

	badgeStyle := rawBadgeStyle.ResolveDefault()
	if query.BadgeDirection != nil {
		badgeStyle = rawBadgeStyle.Resolve(services.BadgeDirection(*query.BadgeDirection))
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

	posterFit := services.PosterFitNative
	if query.Fit != nil {
		posterFit = services.PosterFit(*query.Fit)
	}

	settings := previewRenderSettings("poster", badgeStyle, labelStyle, textSize, badgeSize, logoSize, layout, badgeDirection, appearance, ratingsLimit, ratingsOrder, ratingsExclude)
	settings.PosterFit = posterFit

	badges := p.demoBadges("poster")
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)
	labelFace, valueFace := image.GetFontFacesAt(float64(textSize))
	if valueFace == nil || labelFace == nil {
		httpx.WriteJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	colors := parsePreviewColors(r)

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
	posterBytes, err = image.PosterPreviewArtwork(posterBytes, posterFit, targetWidth)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderPosterSync(posterBytes, image.RenderParams{
		Badges: badges, ValueFontFace: valueFace, LabelFontFace: labelFace,
		Quality: p.cfg.ImageQuality, Layout: layout, BadgeStyle: badgeStyle,
		LabelStyle: labelStyle, Appearance: appearance, TargetWidth: targetWidth,
		BadgeScale: badgeScale, BadgeMultiplier: badgeMultiplier, TextScale: textScale,
		LogoScale: logoScale, PosterFit: posterFit, Colors: colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
}

func (p *PreviewHandler) HandleLogo(w http.ResponseWriter, r *http.Request) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, "logo")
	if err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	textSize, badgeSize, logoSize := previewScales(query)

	targetWidth := resolvedSize.LogoTargetWidth()
	badgeMultiplier := previewBadgeMultiplier("logo", badgeSize)
	badgeScale := resolvedSize.BadgeScale("logo") * badgeMultiplier
	logoScale := logoSize.Percent()
	textScale := textSize.Percent()

	ratingsLimit := int32(previewLogoBackdropRatingsLimit)
	// An explicit ?ratings_limit beats the layout total only when valid; an
	// absent or invalid override falls back to the layout total (matching the
	// serve path's override ?? layout.Total() semantics).
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

	rawBadgeStyle := services.BadgeStyleLogoTB
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.ResolveDefault()

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	alpha := services.DefaultBadgeAlpha()
	if query.BadgeAlpha != nil {
		alpha = services.ClampBadgeAlpha(*query.BadgeAlpha)
	}
	appearance := services.BadgeAppearance{Shape: shape, Alpha: alpha, Width: services.DefaultScalePercent(), Height: services.DefaultScalePercent()}
	if query.BadgeWidth != nil {
		appearance.Width = services.ClampScalePercent(*query.BadgeWidth)
	}
	if query.BadgeHeight != nil {
		appearance.Height = services.ClampScalePercent(*query.BadgeHeight)
	}

	layout := previewLayout(query, "logo")
	if !overrideValid {
		ratingsLimit = layout.Total()
	}

	badges := p.demoBadges("logo")
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)
	labelFace, valueFace := image.GetFontFacesAt(float64(textSize))
	if valueFace == nil || labelFace == nil {
		httpx.WriteJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	colors := parsePreviewColors(r)

	logoBytes, err := p.demoArtworkBytes("logo")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderLogoSync(logoBytes, image.RenderParams{
		Badges: badges, ValueFontFace: valueFace, LabelFontFace: labelFace,
		Layout: layout, BadgeStyle: badgeStyle, LabelStyle: labelStyle,
		Appearance: appearance, TargetWidth: targetWidth, BadgeScale: badgeScale,
		BadgeMultiplier: badgeMultiplier, TextScale: textScale, LogoScale: logoScale,
		Colors: colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
}

func (p *PreviewHandler) HandleBackdrop(w http.ResponseWriter, r *http.Request) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, "backdrop")
	if err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	textSize, badgeSize, logoSize := previewScales(query)

	targetWidth := resolvedSize.BackdropTargetWidth()
	badgeMultiplier := previewBadgeMultiplier("backdrop", badgeSize)
	badgeScale := resolvedSize.BadgeScale("backdrop") * badgeMultiplier
	logoScale := logoSize.Percent()
	textScale := textSize.Percent()

	ratingsLimit := int32(previewLogoBackdropRatingsLimit)
	// An explicit ?ratings_limit beats the layout total only when valid; an
	// absent or invalid override falls back to the layout total (matching the
	// serve path's override ?? layout.Total() semantics).
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

	layout := previewLayout(query, "backdrop")
	if !overrideValid {
		ratingsLimit = layout.Total()
	}

	rawBadgeStyle := services.BadgeStyleLogoTB
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.ResolveDefault()
	if query.BadgeDirection != nil {
		badgeStyle = rawBadgeStyle.Resolve(services.BadgeDirection(*query.BadgeDirection))
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

	edgeInsetX := int32(0)
	if query.EdgeInsetX != nil {
		edgeInsetX = *query.EdgeInsetX
	}
	edgeInsetY := int32(0)
	if query.EdgeInsetY != nil {
		edgeInsetY = *query.EdgeInsetY
	}
	badges := p.demoBadges("backdrop")
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)
	labelFace, valueFace := image.GetFontFacesAt(float64(textSize))
	if valueFace == nil || labelFace == nil {
		httpx.WriteJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	colors := parsePreviewColors(r)

	backdropBytes, err := p.demoArtworkBytes("backdrop")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderBackdropSync(backdropBytes, image.RenderParams{
		Badges: badges, ValueFontFace: valueFace, LabelFontFace: labelFace,
		Quality: p.cfg.ImageQuality, Layout: layout, BadgeStyle: badgeStyle,
		LabelStyle: labelStyle, Appearance: appearance, TargetWidth: targetWidth,
		BadgeScale: badgeScale, BadgeMultiplier: badgeMultiplier, TextScale: textScale,
		LogoScale: logoScale, EdgeInsetX: edgeInsetX, EdgeInsetY: edgeInsetY, Colors: colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
}

func (p *PreviewHandler) HandleEpisode(w http.ResponseWriter, r *http.Request) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, "episode")
	if err != nil {
		httpx.WriteJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	textSize, badgeSize, logoSize := previewScales(query)

	targetWidth := resolvedSize.EpisodeTargetWidth()
	badgeMultiplier := previewBadgeMultiplier("episode", badgeSize)
	badgeScale := resolvedSize.BadgeScale("episode") * badgeMultiplier
	logoScale := logoSize.Percent()
	textScale := textSize.Percent()

	ratingsLimit := int32(previewPosterRatingsLimit)
	// An explicit ?ratings_limit beats the layout total only when valid; an
	// absent or invalid override falls back to the layout total (matching the
	// serve path's override ?? layout.Total() semantics).
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

	layout := previewLayout(query, "episode")
	if !overrideValid {
		ratingsLimit = layout.Total()
	}

	rawBadgeStyle := services.BadgeStyleLogoTB
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.ResolveDefault()
	if query.BadgeDirection != nil {
		badgeStyle = rawBadgeStyle.Resolve(services.BadgeDirection(*query.BadgeDirection))
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

	blur := false
	if query.Blur != nil {
		blur = *query.Blur
	}
	badges := p.demoBadges("episode")
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)
	labelFace, valueFace := image.GetFontFacesAt(float64(textSize))
	if valueFace == nil || labelFace == nil {
		httpx.WriteJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	colors := parsePreviewColors(r)

	episodeBytes, err := p.demoArtworkBytes("episode")
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	rendered, err := image.RenderEpisodeSync(episodeBytes, image.RenderParams{
		Badges: badges, ValueFontFace: valueFace, LabelFontFace: labelFace,
		Quality: p.cfg.ImageQuality, Layout: layout, BadgeStyle: badgeStyle,
		LabelStyle: labelStyle, Appearance: appearance, TargetWidth: targetWidth,
		BadgeScale: badgeScale, BadgeMultiplier: badgeMultiplier, TextScale: textScale,
		LogoScale: logoScale, Blur: blur, Colors: colors,
	})
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
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

		metaDeleted, _ := services.DeleteImageMetaByKind(db, imageType)

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
			metaDeleted, _ = services.DeleteImageMetaExact(db, imageType, idType+"/"+idValue)
		} else {
			metaDeleted, _ = services.DeleteImageMetaForTitle(db, imageType, idType, idValue)
		}

		idKey := idType + "/" + idValue
		if scope != "variant" {
			services.DeleteAvailableRatings(db, idKey)
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
