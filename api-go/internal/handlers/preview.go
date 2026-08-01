package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

const (
	previewPosterRatingsLimit     = 3
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
	badgeSize services.BadgeSize,
	position services.BadgePosition,
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
		s.PosterBadgeSize = badgeSize
		s.PosterPosition = position
		s.PosterBadgeDirection = badgeDirection
		s.PosterBadgeShape = appearance.Shape
		s.PosterBadgeBackground = appearance.Background
	case "logo":
		s.LogoRatingsLimit = ratingsLimit
		s.LogoBadgeStyle = badgeStyle
		s.LogoLabelStyle = labelStyle
		s.LogoBadgeSize = badgeSize
		s.LogoBadgeShape = appearance.Shape
		s.LogoBadgeBackground = appearance.Background
	case "backdrop":
		s.BackdropRatingsLimit = ratingsLimit
		s.BackdropBadgeStyle = badgeStyle
		s.BackdropLabelStyle = labelStyle
		s.BackdropBadgeSize = badgeSize
		s.BackdropPosition = position
		s.BackdropBadgeDirection = badgeDirection
		s.BackdropBadgeShape = appearance.Shape
		s.BackdropBadgeBackground = appearance.Background
	case "episode":
		s.EpisodeRatingsLimit = ratingsLimit
		s.EpisodeBadgeStyle = badgeStyle
		s.EpisodeLabelStyle = labelStyle
		s.EpisodeBadgeSize = badgeSize
		s.EpisodePosition = position
		s.EpisodeBadgeDirection = badgeDirection
		s.EpisodeBadgeShape = appearance.Shape
		s.EpisodeBadgeBackground = appearance.Background
	}
	return &s
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
	db   *sql.DB
	cfg  *PreviewConfig
}

type PreviewConfig struct {
	CacheDir          string
	ExternalCacheOnly bool
	ImageQuality      uint8
}

func NewPreviewHandler(db *sql.DB, cfg *PreviewConfig) *PreviewHandler {
	return &PreviewHandler{db: db, cfg: cfg}
}

func (p *PreviewHandler) HandlePoster(w http.ResponseWriter, r *http.Request) {
	query := parseImageQuery(r)

	imageSize, err := parsePreviewImageSize(query.ImageSize, "poster")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	badgeSize := services.BadgeSizeMedium
	if query.BadgeSize != nil {
		badgeSize = services.BadgeSize(*query.BadgeSize)
	}

	targetWidth := resolvedSize.PosterTargetWidth()
	badgeScale := resolvedSize.BadgeScale("poster") * badgeSize.ScaleFactor()

	ratingsLimit := int32(previewPosterRatingsLimit)
	if query.RatingsLimit != nil {
		ratingsLimit = *query.RatingsLimit
	}
	services.ValidateRatingsLimit(ratingsLimit)

	defaultOrder := services.DefaultRatingsOrder()
	ratingsOrder := defaultOrder
	if query.RatingsOrder != nil && *query.RatingsOrder != "" {
		ratingsOrder = *query.RatingsOrder
	}

	ratingsExclude := ""
	if query.RatingsExclude != nil {
		ratingsExclude = *query.RatingsExclude
	}

	position := services.PositionBottomCenter
	if query.Position != nil {
		position = services.BadgePosition(*query.Position)
	}

	rawBadgeStyle := services.BadgeStyleDefault
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeDirection := services.BadgeDirectionDefault.Resolve(position)
	if query.BadgeDirection != nil {
		badgeDirection = services.BadgeDirection(*query.BadgeDirection).Resolve(position)
	}

	badgeStyle := rawBadgeStyle.Resolve(badgeDirection)

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	background := services.BadgeBackgroundDefault
	if query.BadgeBackground != nil {
		background = services.BadgeBackground(*query.BadgeBackground)
	}
	appearance := services.BadgeAppearance{Shape: shape, Background: background}

	split := false
	if query.Split != nil {
		split = *query.Split
	}

	posterFit := services.PosterFitNative
	if query.Fit != nil {
		posterFit = services.PosterFit(*query.Fit)
	}

	settings := previewRenderSettings("poster", badgeStyle, labelStyle, badgeSize, position, badgeDirection, appearance, ratingsLimit, ratingsOrder, ratingsExclude)
	settings.PosterBadgeSplit = split
	settings.PosterFit = posterFit

	badges := sampleBadges()
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)

	valueFace := image.GetValueFontFace()
	labelFace := image.GetFontFace()
	if valueFace == nil || labelFace == nil {
		writeJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	rendered, err := image.RenderPosterSync(image.SamplePosterPNG, badges, valueFace, labelFace, p.cfg.ImageQuality,
		position, badgeStyle, labelStyle, appearance, badgeDirection,
		targetWidth, badgeScale, badgeSize, split, posterFit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
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
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	badgeSize := services.BadgeSizeMedium
	if query.BadgeSize != nil {
		badgeSize = services.BadgeSize(*query.BadgeSize)
	}

	targetWidth := resolvedSize.LogoTargetWidth()
	badgeScale := resolvedSize.BadgeScale("logo") * badgeSize.ScaleFactor()

	ratingsLimit := int32(previewLogoBackdropRatingsLimit)
	if query.RatingsLimit != nil {
		ratingsLimit = *query.RatingsLimit
	}
	services.ValidateRatingsLimit(ratingsLimit)

	defaultOrder := services.DefaultRatingsOrder()
	ratingsOrder := defaultOrder
	if query.RatingsOrder != nil && *query.RatingsOrder != "" {
		ratingsOrder = *query.RatingsOrder
	}

	ratingsExclude := ""
	if query.RatingsExclude != nil {
		ratingsExclude = *query.RatingsExclude
	}

	rawBadgeStyle := services.BadgeStyleHorizontal
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.Resolve(services.BadgeDirectionHorizontal)

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	background := services.BadgeBackgroundDefault
	if query.BadgeBackground != nil {
		background = services.BadgeBackground(*query.BadgeBackground)
	}
	appearance := services.BadgeAppearance{Shape: shape, Background: background}

	badges := sampleBadges()
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)

	valueFace := image.GetValueFontFace()
	labelFace := image.GetFontFace()
	if valueFace == nil || labelFace == nil {
		writeJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	rendered, err := image.RenderLogoSync(image.SampleLogoPNG, badges, valueFace, labelFace,
		badgeStyle, labelStyle, appearance, targetWidth, badgeScale)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
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
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	badgeSize := services.BadgeSizeMedium
	if query.BadgeSize != nil {
		badgeSize = services.BadgeSize(*query.BadgeSize)
	}

	targetWidth := resolvedSize.BackdropTargetWidth()
	badgeScale := resolvedSize.BadgeScale("backdrop") * badgeSize.ScaleFactor()

	ratingsLimit := int32(previewLogoBackdropRatingsLimit)
	if query.RatingsLimit != nil {
		ratingsLimit = *query.RatingsLimit
	}
	services.ValidateRatingsLimit(ratingsLimit)

	defaultOrder := services.DefaultRatingsOrder()
	ratingsOrder := defaultOrder
	if query.RatingsOrder != nil && *query.RatingsOrder != "" {
		ratingsOrder = *query.RatingsOrder
	}

	ratingsExclude := ""
	if query.RatingsExclude != nil {
		ratingsExclude = *query.RatingsExclude
	}

	position := services.PositionTopRight
	if query.Position != nil {
		position = services.BadgePosition(*query.Position)
	}

	badgeDirection := services.BadgeDirectionDefault.Resolve(position)
	if query.BadgeDirection != nil {
		badgeDirection = services.BadgeDirection(*query.BadgeDirection).Resolve(position)
	}

	rawBadgeStyle := services.BadgeStyleVertical
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.Resolve(badgeDirection)

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	background := services.BadgeBackgroundDefault
	if query.BadgeBackground != nil {
		background = services.BadgeBackground(*query.BadgeBackground)
	}
	appearance := services.BadgeAppearance{Shape: shape, Background: background}

	edgeInsetX := int32(0)
	if query.EdgeInsetX != nil {
		edgeInsetX = *query.EdgeInsetX
	}
	edgeInsetY := int32(0)
	if query.EdgeInsetY != nil {
		edgeInsetY = *query.EdgeInsetY
	}

	badges := sampleBadges()
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)

	valueFace := image.GetValueFontFace()
	labelFace := image.GetFontFace()
	if valueFace == nil || labelFace == nil {
		writeJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	rendered, err := image.RenderBackdropSync(image.SampleBackdropPNG, badges, valueFace, labelFace, p.cfg.ImageQuality,
		position, badgeStyle, labelStyle, appearance, badgeDirection,
		targetWidth, badgeScale, badgeSize, edgeInsetX, edgeInsetY)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
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
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	resolvedSize := services.ImageSizeMedium
	if imageSize != nil {
		resolvedSize = *imageSize
	}

	badgeSize := services.BadgeSizeMedium
	if query.BadgeSize != nil {
		badgeSize = services.BadgeSize(*query.BadgeSize)
	}

	targetWidth := resolvedSize.EpisodeTargetWidth()
	badgeScale := resolvedSize.BadgeScale("episode") * badgeSize.ScaleFactor()

	ratingsLimit := int32(previewPosterRatingsLimit)
	if query.RatingsLimit != nil {
		ratingsLimit = *query.RatingsLimit
	}
	services.ValidateRatingsLimit(ratingsLimit)

	defaultOrder := services.DefaultRatingsOrder()
	ratingsOrder := defaultOrder
	if query.RatingsOrder != nil && *query.RatingsOrder != "" {
		ratingsOrder = *query.RatingsOrder
	}

	ratingsExclude := ""
	if query.RatingsExclude != nil {
		ratingsExclude = *query.RatingsExclude
	}

	position := services.PositionTopRight
	if query.Position != nil {
		position = services.BadgePosition(*query.Position)
	}

	badgeDirection := services.BadgeDirectionVertical.Resolve(position)
	if query.BadgeDirection != nil {
		badgeDirection = services.BadgeDirection(*query.BadgeDirection).Resolve(position)
	}

	rawBadgeStyle := services.BadgeStyleVertical
	if query.BadgeStyle != nil {
		rawBadgeStyle = services.BadgeStyle(*query.BadgeStyle)
	}

	labelStyle := services.LabelStyleOfficial
	if query.LabelStyle != nil {
		labelStyle = services.LabelStyle(*query.LabelStyle)
	}

	badgeStyle := rawBadgeStyle.Resolve(badgeDirection)

	shape := services.BadgeShapeRounded
	if query.BadgeShape != nil {
		shape = services.BadgeShape(*query.BadgeShape)
	}
	background := services.BadgeBackgroundDefault
	if query.BadgeBackground != nil {
		background = services.BadgeBackground(*query.BadgeBackground)
	}
	appearance := services.BadgeAppearance{Shape: shape, Background: background}

	blur := false
	if query.Blur != nil {
		blur = *query.Blur
	}

	badges := sampleBadges()
	badges = services.ApplyRatingPreferences(badges, ratingsOrder, ratingsExclude, ratingsLimit)

	valueFace := image.GetValueFontFace()
	labelFace := image.GetFontFace()
	if valueFace == nil || labelFace == nil {
		writeJSON(w, 500, map[string]string{"error": "font not loaded"})
		return
	}

	rendered, err := image.RenderEpisodeSync(image.SampleBackdropPNG, badges, valueFace, labelFace, p.cfg.ImageQuality,
		position, badgeStyle, labelStyle, appearance, badgeDirection,
		targetWidth, badgeScale, badgeSize, blur)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(rendered)
}

// --- Admin image list/file serving ---

func HandleImageFile(db *sql.DB, cacheDir string, imageType, idType, idValue string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		ext := "jpg"
		contentType := "image/jpeg"
		if imageType == "l" {
			ext = "png"
			contentType = "image/png"
		}

		subdir := services.ImageSubdirByType(imageType)
		if subdir == "" {
			writeError(w, 400, "invalid image type")
			return
		}

		fileBase := strings.ReplaceAll(idValue, ":", "_")
		path, err := services.TypedCachePath(cacheDir, subdir, idType, fileBase, ext)
		if err != nil {
			writeError(w, 400, "invalid path")
			return
		}

		data, err := readFile(path)
		if err != nil {
			writeError(w, 404, "image not found")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}

func HandleFetchImage(db *sql.DB, cfg *ImageServeConfig, tmdb *services.TmdbClient, omdb *services.OmdbClient, mdblist *services.MdblistClient, trakt *services.TraktClient, fanart *services.FanartClient, imageType, idType, idValue string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}

		if err := services.ValidateIDValue(idValue); err != nil {
			writeError(w, 400, "invalid id value")
			return
		}

		if tmdb == nil {
			writeError(w, 503, "TMDB API key not configured — image generation unavailable")
			return
		}

		kind, ok := kindFromType(imageType)
		if !ok {
			writeError(w, 400, "invalid image type")
			return
		}

		slog.Debug("admin fetch requested", "kind", kind, "id", idType+"/"+idValue)

		globals, _ := services.GetGlobalSettings(db)
		settings := services.ParseGlobalRenderSettings(globals)

		bytes, contentType, err := image.ServeImage(
			db, tmdb, omdb, mdblist, trakt, fanart,
			idType, idValue, kind, &settings,
			cfg.CacheDir, cfg.ExternalCacheOnly,
			cfg.RatingsMinStaleSecs, cfg.RatingsMaxAgeSecs, cfg.ImageStaleSecs,
			cfg.ImageQuality, nil,
		)
		if err != nil {
			if appErr, ok := err.(*apperr.AppError); ok {
				writeError(w, appErr.Status, appErr.Message)
			} else {
				writeError(w, 500, err.Error())
			}
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600, stale-while-revalidate=86400")
		w.Write(bytes)
	}
}

func kindFromType(imageType string) (string, bool) {
	switch imageType {
	case "p":
		return "poster", true
	case "l":
		return "logo", true
	case "b":
		return "backdrop", true
	case "e":
		return "episode", true
	}
	return "", false
}

func HandleClearKind(db *sql.DB, cacheDir string, imageType string, externalCacheOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, 405, "Method not allowed")
			return
		}

		dirCleared := false
		if !externalCacheOnly {
			subdir := services.ImageSubdirByType(imageType)
			if subdir != "" {
				_, err := services.StageDirForClear(cacheDir, subdir)
				dirCleared = err == nil
			}
		}

		metaDeleted, _ := services.DeleteImageMetaByKind(db, imageType)

		writeJSON(w, 200, map[string]interface{}{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"dir_cleared":         dirCleared,
			"meta_deleted":        metaDeleted,
		})
	}
}

func HandlePurgeTitle(db *sql.DB, cacheDir string, imageType, idType, idValue string, externalCacheOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, 405, "Method not allowed")
			return
		}

		scope := r.URL.Query().Get("scope")

		var filesDeleted int64 = 0
		if !externalCacheOnly {
			subdir := services.ImageSubdirByType(imageType)
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

		writeJSON(w, 200, map[string]interface{}{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"files_deleted":       filesDeleted,
			"meta_deleted":        metaDeleted,
		})
	}
}

func HandleServiceKeys(db *sql.DB, serviceKeys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, serviceKeys.GetStatus())
		case http.MethodPut:
			var update services.ServiceKeysUpdate
			if err := decodeJSONBody(r, &update); err != nil {
				writeError(w, 400, "invalid JSON")
				return
			}
			if err := serviceKeys.UpdateKeys(&update); err != nil {
				writeError(w, 500, "failed to update keys")
				return
			}
			writeJSON(w, 200, serviceKeys.GetStatus())
		default:
			writeError(w, 405, "Method not allowed")
		}
	}
}

func readFile(path string) ([]byte, error) {
	entry, err := services.ReadCache(path, 0)
	if err != nil {
		return nil, err
	}
	return entry.Bytes, nil
}

func parseInt64Param(r *http.Request, name string, def int64) int64 {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return def
	}
	return n
}
