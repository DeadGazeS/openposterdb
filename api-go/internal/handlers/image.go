package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"openposterdb/internal/errors"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

const freeAPIKey = "t0-free-rpdb"

func marshalLayoutResponse(l services.ImageLayout) string {
	s, err := services.MarshalLayout(&l)
	if err != nil {
		return ""
	}
	return s
}

// ImageServeConfig carries the server-level settings needed by the image
// generation pipeline. Built from config.Config by the router.
type ImageServeConfig struct {
	CacheDir            string
	ExternalCacheOnly   bool
	RatingsMinStaleSecs uint64
	RatingsMaxAgeSecs   uint64
	ImageStaleSecs      uint64
	ImageQuality        uint8
}

// ImageQuery represents all query parameters for image endpoints.
type ImageQuery struct {
	Fallback       *string `json:"fallback"`
	Lang           *string `json:"lang"`
	ImageSize      *string `json:"imageSize"`
	RatingsLimit   *int32  `json:"ratings_limit"`
	RatingsOrder   *string `json:"ratings_order"`
	RatingsExclude *string `json:"ratings_exclude"`
	BadgeStyle     *string `json:"badge_style"`
	LabelStyle     *string `json:"label_style"`
	TextSize       *int32  `json:"text_size"`
	BadgeSize      *int32  `json:"badge_size"`
	BadgeWidth     *int32  `json:"badge_width"`
	BadgeHeight    *int32  `json:"badge_height"`
	LogoSize       *int32  `json:"logo_size"`
	BadgeDirection *string `json:"badge_direction"`
	BadgeShape     *string `json:"badge_shape"`
	BadgeAlpha     *int32  `json:"badge_alpha"`
	Layout         *string `json:"layout"`
	ImageSource    *string `json:"image_source"`
	Textless       *bool   `json:"textless"`
	Blur           *bool   `json:"blur"`
	Fit            *string `json:"fit"`
	EdgeInsetX     *int32  `json:"edge_inset_x"`
	EdgeInsetY     *int32  `json:"edge_inset_y"`
}

func (q *ImageQuery) HasOverrides() bool {
	return q.RatingsLimit != nil || q.RatingsOrder != nil || q.RatingsExclude != nil ||
		q.BadgeStyle != nil || q.LabelStyle != nil || q.TextSize != nil ||
		q.BadgeSize != nil || q.LogoSize != nil ||
		q.BadgeDirection != nil || q.BadgeShape != nil || q.BadgeAlpha != nil ||
		q.Layout != nil || q.ImageSource != nil || q.Textless != nil ||
		q.Blur != nil || q.Fit != nil ||
		q.EdgeInsetX != nil || q.EdgeInsetY != nil
}

func applyQueryOverrides(settings *services.RenderSettings, query *ImageQuery, kind string) *services.RenderSettings {
	if !query.HasOverrides() {
		return settings
	}

	s := *settings

	if query.RatingsLimit != nil {
		limit := *query.RatingsLimit
		if err := services.ValidateRatingsLimit(limit); err != nil {
			return settings
		}
		switch kind {
		case "poster":
			s.RatingsLimit = limit
		case "logo":
			s.LogoRatingsLimit = limit
		case "backdrop":
			s.BackdropRatingsLimit = limit
		case "episode":
			s.EpisodeRatingsLimit = limit
		}
	}

	if query.RatingsOrder != nil {
		if err := services.ValidateRatingsOrder(*query.RatingsOrder); err == nil {
			s.RatingsOrder = *query.RatingsOrder
		}
	}

	if query.RatingsExclude != nil {
		if err := services.ValidateRatingsExclude(*query.RatingsExclude); err == nil {
			s.RatingsExclude = *query.RatingsExclude
		}
	}

	if query.BadgeStyle != nil {
		style := services.ParseBadgeStyle(*query.BadgeStyle)
		switch kind {
		case "poster":
			s.PosterBadgeStyle = style
		case "logo":
			s.LogoBadgeStyle = style
		case "backdrop":
			s.BackdropBadgeStyle = style
		case "episode":
			s.EpisodeBadgeStyle = style
		}
	}

	if query.LabelStyle != nil {
		style := services.LabelStyle(*query.LabelStyle)
		switch kind {
		case "poster":
			s.PosterLabelStyle = style
		case "logo":
			s.LogoLabelStyle = style
		case "backdrop":
			s.BackdropLabelStyle = style
		case "episode":
			s.EpisodeLabelStyle = style
		}
	}

	if query.TextSize != nil {
		size := services.ClampScalePercent(*query.TextSize)
		switch kind {
		case "poster":
			s.PosterTextSize = size
		case "logo":
			s.LogoTextSize = size
		case "backdrop":
			s.BackdropTextSize = size
		case "episode":
			s.EpisodeTextSize = size
		}
	}

	if query.BadgeSize != nil {
		size := services.ClampScalePercent(*query.BadgeSize)
		switch kind {
		case "poster":
			s.PosterBadgeSize = size
		case "logo":
			s.LogoBadgeSize = size
		case "backdrop":
			s.BackdropBadgeSize = size
		case "episode":
			s.EpisodeBadgeSize = size
		}
	}

	if query.LogoSize != nil {
		size := services.ClampScalePercent(*query.LogoSize)
		switch kind {
		case "poster":
			s.PosterLogoSize = size
		case "logo":
			s.LogoLogoSize = size
		case "backdrop":
			s.BackdropLogoSize = size
		case "episode":
			s.EpisodeLogoSize = size
		}
	}

	if query.BadgeShape != nil {
		shape := services.BadgeShape(*query.BadgeShape)
		switch kind {
		case "poster":
			s.PosterBadgeShape = shape
		case "logo":
			s.LogoBadgeShape = shape
		case "backdrop":
			s.BackdropBadgeShape = shape
		case "episode":
			s.EpisodeBadgeShape = shape
		}
	}

	if query.BadgeAlpha != nil {
		alpha := services.ClampBadgeAlpha(*query.BadgeAlpha)
		switch kind {
		case "poster":
			s.PosterBadgeAlpha = alpha
		case "logo":
			s.LogoBadgeAlpha = alpha
		case "backdrop":
			s.BackdropBadgeAlpha = alpha
		case "episode":
			s.EpisodeBadgeAlpha = alpha
		}
	}

	if query.Layout != nil {
		def := services.DefaultLayout(kind)
		l := services.UnmarshalLayout(*query.Layout, &def)
		switch kind {
		case "poster":
			s.PosterLayout = l
		case "logo":
			s.LogoLayout = l
		case "backdrop":
			s.BackdropLayout = l
		case "episode":
			s.EpisodeLayout = l
		}
	}

	if kind == "poster" {
		if query.BadgeDirection != nil {
			s.PosterBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Fit != nil {
			s.PosterFit = services.PosterFit(*query.Fit)
		}
		if query.Textless != nil {
			s.Textless = *query.Textless
		}
	}

	if kind == "backdrop" {
		if query.BadgeDirection != nil {
			s.BackdropBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.EdgeInsetX != nil {
			s.BackdropEdgeInsetX = services.ClampEdgeInset(*query.EdgeInsetX)
		}
		if query.EdgeInsetY != nil {
			s.BackdropEdgeInsetY = services.ClampEdgeInset(*query.EdgeInsetY)
		}
	}

	if kind == "logo" {
		// no per-kind logo query params beyond the shared layout
	}

	if kind == "episode" {
		if query.BadgeDirection != nil {
			s.EpisodeBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Blur != nil {
			s.EpisodeBlur = *query.Blur
		}
	}

	if query.ImageSource != nil {
		s.ImageSource = services.ImageSource(*query.ImageSource)
	}

	return &s
}

func resolveSettings(db *sql.DB, apiKey string, isFreeAPIKeyEnabled func() bool, globalsCache *services.RenderSettings) (*services.RenderSettings, error) {
	if apiKey == freeAPIKey {
		return resolveFreeSettings(db, isFreeAPIKeyEnabled, globalsCache)
	}

	keyHash := HashAPIKey(apiKey)
	k, err := services.FindAPIKeyByHash(db, keyHash)
	if err != nil || k == nil {
		return nil, err
	}

	s := services.GetEffectiveRenderSettings(db, k.ID, globalsCache)
	return &s, nil
}

func resolveFreeSettings(db *sql.DB, isFreeAPIKeyEnabled func() bool, globalsCache *services.RenderSettings) (*services.RenderSettings, error) {
	if !isFreeAPIKeyEnabled() {
		return nil, nil
	}

	if globalsCache != nil {
		settings := *globalsCache
		return &settings, nil
	}

	globals, err := services.GetGlobalSettings(db)
	if err != nil {
		defaults := services.DefaultRenderSettings()
		return &defaults, nil
	}
	settings := services.ParseGlobalRenderSettings(globals)
	return &settings, nil
}

type FreeKeySettingsResponse struct {
	ImageSource            string `json:"image_source"`
	Lang                   string `json:"lang"`
	Textless               bool   `json:"textless"`
	RatingsLimit           int32  `json:"ratings_limit"`
	RatingsOrder           string `json:"ratings_order"`
	RatingsExclude         string `json:"ratings_exclude"`
	PosterLayout           string `json:"poster_layout"`
	LogoRatingsLimit       int32  `json:"logo_ratings_limit"`
	BackdropRatingsLimit   int32  `json:"backdrop_ratings_limit"`
	PosterBadgeStyle       string `json:"poster_badge_style"`
	LogoBadgeStyle         string `json:"logo_badge_style"`
	BackdropBadgeStyle     string `json:"backdrop_badge_style"`
	PosterLabelStyle       string `json:"poster_label_style"`
	LogoLabelStyle         string `json:"logo_label_style"`
	BackdropLabelStyle     string `json:"backdrop_label_style"`
	PosterBadgeDirection   string `json:"poster_badge_direction"`
	PosterFit              string `json:"poster_fit"`
	PosterTextSize         int32  `json:"poster_text_size"`
	LogoTextSize           int32  `json:"logo_text_size"`
	BackdropTextSize       int32  `json:"backdrop_text_size"`
	PosterBadgeSize        int32  `json:"poster_badge_size"`
	LogoBadgeSize          int32  `json:"logo_badge_size"`
	BackdropBadgeSize      int32  `json:"backdrop_badge_size"`
	PosterLogoSize         int32  `json:"poster_logo_size"`
	LogoLogoSize           int32  `json:"logo_logo_size"`
	BackdropLogoSize       int32  `json:"backdrop_logo_size"`
	LogoLayout             string `json:"logo_layout"`
	PosterBadgeShape       string `json:"poster_badge_shape"`
	LogoBadgeShape         string `json:"logo_badge_shape"`
	BackdropBadgeShape     string `json:"backdrop_badge_shape"`
	PosterBadgeAlpha       int32  `json:"poster_badge_alpha"`
	LogoBadgeAlpha         int32  `json:"logo_badge_alpha"`
	BackdropBadgeAlpha     int32  `json:"backdrop_badge_alpha"`
	BackdropLayout         string `json:"backdrop_layout"`
	BackdropBadgeDirection string `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX     int32  `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY     int32  `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit    int32  `json:"episode_ratings_limit"`
	EpisodeBadgeStyle      string `json:"episode_badge_style"`
	EpisodeLabelStyle      string `json:"episode_label_style"`
	EpisodeTextSize        int32  `json:"episode_text_size"`
	EpisodeBadgeSize       int32  `json:"episode_badge_size"`
	EpisodeLogoSize        int32  `json:"episode_logo_size"`
	EpisodeLayout          string `json:"episode_layout"`
	EpisodeBadgeDirection  string `json:"episode_badge_direction"`
	EpisodeBlur            bool   `json:"episode_blur"`
	EpisodeBadgeShape      string `json:"episode_badge_shape"`
	EpisodeBadgeAlpha      int32  `json:"episode_badge_alpha"`
}

func freeKeySettingsFromRender(s *services.RenderSettings) FreeKeySettingsResponse {
	return FreeKeySettingsResponse{
		ImageSource:            string(s.ImageSource),
		Lang:                   s.Lang,
		Textless:               s.Textless,
		RatingsLimit:           s.RatingsLimit,
		RatingsOrder:           s.RatingsOrder,
		RatingsExclude:         s.RatingsExclude,
		PosterLayout:           marshalLayoutResponse(s.PosterLayout),
		LogoRatingsLimit:       s.LogoRatingsLimit,
		BackdropRatingsLimit:   s.BackdropRatingsLimit,
		PosterBadgeStyle:       string(s.PosterBadgeStyle),
		LogoBadgeStyle:         string(s.LogoBadgeStyle),
		BackdropBadgeStyle:     string(s.BackdropBadgeStyle),
		PosterLabelStyle:       string(s.PosterLabelStyle),
		LogoLabelStyle:         string(s.LogoLabelStyle),
		BackdropLabelStyle:     string(s.BackdropLabelStyle),
		PosterBadgeDirection:   string(s.PosterBadgeDirection),
		PosterFit:              string(s.PosterFit),
		PosterTextSize:         int32(s.PosterTextSize),
		LogoTextSize:           int32(s.LogoTextSize),
		BackdropTextSize:       int32(s.BackdropTextSize),
		PosterBadgeSize:        int32(s.PosterBadgeSize),
		LogoBadgeSize:          int32(s.LogoBadgeSize),
		BackdropBadgeSize:      int32(s.BackdropBadgeSize),
		PosterLogoSize:         int32(s.PosterLogoSize),
		LogoLogoSize:           int32(s.LogoLogoSize),
		BackdropLogoSize:       int32(s.BackdropLogoSize),
		LogoLayout:             marshalLayoutResponse(s.LogoLayout),
		PosterBadgeShape:       string(s.PosterBadgeShape),
		LogoBadgeShape:         string(s.LogoBadgeShape),
		BackdropBadgeShape:     string(s.BackdropBadgeShape),
		PosterBadgeAlpha:       int32(s.PosterBadgeAlpha),
		LogoBadgeAlpha:         int32(s.LogoBadgeAlpha),
		BackdropBadgeAlpha:     int32(s.BackdropBadgeAlpha),
		BackdropLayout:         marshalLayoutResponse(s.BackdropLayout),
		BackdropBadgeDirection: string(s.BackdropBadgeDirection),
		BackdropEdgeInsetX:     s.BackdropEdgeInsetX,
		BackdropEdgeInsetY:     s.BackdropEdgeInsetY,
		EpisodeRatingsLimit:    s.EpisodeRatingsLimit,
		EpisodeBadgeStyle:      string(s.EpisodeBadgeStyle),
		EpisodeLabelStyle:      string(s.EpisodeLabelStyle),
		EpisodeTextSize:        int32(s.EpisodeTextSize),
		EpisodeBadgeSize:       int32(s.EpisodeBadgeSize),
		EpisodeLogoSize:        int32(s.EpisodeLogoSize),
		EpisodeLayout:          marshalLayoutResponse(s.EpisodeLayout),
		EpisodeBadgeDirection:  string(s.EpisodeBadgeDirection),
		EpisodeBlur:            s.EpisodeBlur,
		EpisodeBadgeShape:      string(s.EpisodeBadgeShape),
		EpisodeBadgeAlpha:      int32(s.EpisodeBadgeAlpha),
	}
}

func HandleFreeKeySettings(db *sql.DB, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		if !isFreeAPIKeyEnabled() {
			writeError(w, 401, "Unauthorized")
			return
		}

		globals, _ := services.GetGlobalSettings(db)
		var settings services.RenderSettings
		if len(globals) > 0 {
			settings = services.ParseGlobalRenderSettings(globals)
		} else {
			settings = services.DefaultRenderSettings()
		}

		resp := freeKeySettingsFromRender(&settings)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func HandleImage(db *sql.DB, cfg *ImageServeConfig, tmdb *services.TmdbClient, omdb *services.OmdbClient, mdblist *services.MdblistClient, trakt *services.TraktClient, fanart *services.FanartClient, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		if tmdb == nil {
			writeError(w, 503, "TMDB API key not configured — image generation unavailable")
			return
		}

		apiKey := r.PathValue("apiKey")
		rest := r.PathValue("rest")

		if apiKey == "" || rest == "" {
			writeError(w, 400, "missing path parameters")
			return
		}

		// Parse rest into idType, image-kind, and idValue
		// rest format: {idType}/poster-default/{idValue}.jpg
		//           or: {idType}/logo-default/{idValue}.png
		//           or: {idType}/backdrop-default/{idValue}.jpg
		//           or: {idType}/episode-default/{idValue}.jpg
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 3 {
			writeError(w, 400, "invalid path")
			return
		}
		idTypeStr := parts[0]
		imageKind := parts[1]
		idValue := parts[2]

		var kind string
		switch imageKind {
		case "poster-default":
			kind = "poster"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		case "logo-default":
			kind = "logo"
			idValue = strings.TrimSuffix(idValue, ".png")
		case "backdrop-default":
			kind = "backdrop"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		case "episode-default":
			kind = "episode"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		default:
			writeError(w, 400, "invalid image path")
			return
		}

		if err := services.ValidateIDValue(idValue); err != nil {
			writeError(w, 400, "invalid id value")
			return
		}

		// Parse query parameters
		query := parseImageQuery(r)

		// Validate API key and resolve settings
		if apiKey == freeAPIKey && !isFreeAPIKeyEnabled() {
			writeError(w, 403, "free api key is disabled")
			return
		}

		var settings *services.RenderSettings
		if apiKey == freeAPIKey {
			globals, _ := services.GetGlobalSettings(db)
			s := services.ParseGlobalRenderSettings(globals)
			settings = &s
		} else {
			keyHash := HashAPIKey(apiKey)
			k, err := services.FindAPIKeyByHash(db, keyHash)
			if err != nil || k == nil {
				writeError(w, 401, "invalid api key")
				return
			}
			s := services.GetEffectiveRenderSettings(db, k.ID, nil)
			settings = &s
		}

		// Apply lang override from query
		if query.Lang != nil && *query.Lang != "" {
			if err := services.ValidateLang(*query.Lang); err == nil {
				settings.Lang = *query.Lang
			}
		}

		// Apply query parameter overrides
		settings = applyQueryOverrides(settings, query, kind)

		bytes, contentType, err := image.ServeImage(
			db, tmdb, omdb, mdblist, trakt, fanart,
			idTypeStr, idValue, kind, settings, query.RatingsLimit,
			cfg.CacheDir, cfg.ExternalCacheOnly,
			cfg.RatingsMinStaleSecs, cfg.RatingsMaxAgeSecs, cfg.ImageStaleSecs,
			cfg.ImageQuality, query.ImageSize,
		)
		if err != nil {
			if appErr, ok := err.(*errors.AppError); ok {
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

func HandleIsValid(db *sql.DB, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		apiKey := r.PathValue("apiKey")
		if apiKey == "" {
			writeJSON(w, 400, map[string]interface{}{"error": "missing api key"})
			return
		}

		if apiKey == freeAPIKey {
			if !isFreeAPIKeyEnabled() {
				writeJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
				return
			}
			writeJSON(w, 200, map[string]interface{}{"valid": true})
			return
		}

		keyHash := HashAPIKey(apiKey)
		_, err := services.FindAPIKeyByHash(db, keyHash)
		if err != nil {
			writeJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
			return
		}

		writeJSON(w, 200, map[string]interface{}{"valid": true})
	}
}

func parseImageQuery(r *http.Request) *ImageQuery {
	q := r.URL.Query()
	query := &ImageQuery{}

	if v := q.Get("fallback"); v != "" {
		query.Fallback = &v
	}
	if v := q.Get("lang"); v != "" {
		query.Lang = &v
	}
	if v := q.Get("imageSize"); v != "" {
		query.ImageSize = &v
	}
	if v := q.Get("ratings_limit"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.RatingsLimit = &n
		}
	}
	if v := q.Get("ratings_order"); v != "" {
		query.RatingsOrder = &v
	}
	if v := q.Get("ratings_exclude"); v != "" {
		query.RatingsExclude = &v
	}
	if v := q.Get("badge_style"); v != "" {
		query.BadgeStyle = &v
	}
	if v := q.Get("label_style"); v != "" {
		query.LabelStyle = &v
	}
	if v := q.Get("badge_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeSize = &n
		}
	}
	if v := q.Get("text_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.TextSize = &n
		}
	}
	if v := q.Get("logo_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.LogoSize = &n
		}
	}
	if v := q.Get("badge_direction"); v != "" {
		query.BadgeDirection = &v
	}
	if v := q.Get("badge_shape"); v != "" {
		query.BadgeShape = &v
	}
	if v := q.Get("badge_alpha"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeAlpha = &n
		}
	}
	if v := q.Get("position"); v != "" {
		_ = v
	}
	if v := q.Get("image_source"); v != "" {
		query.ImageSource = &v
	}
	if v := q.Get("poster_source"); v != "" {
		query.ImageSource = &v
	}
	if v := q.Get("textless"); v != "" {
		b := v == "true"
		query.Textless = &b
	}
	if v := q.Get("fanart_textless"); v != "" {
		b := v == "true"
		query.Textless = &b
	}
	if v := q.Get("blur"); v != "" {
		b := v == "true"
		query.Blur = &b
	}
	if v := q.Get("layout"); v != "" {
		query.Layout = &v
	}
	if v := q.Get("fit"); v != "" {
		query.Fit = &v
	}
	if v := q.Get("edge_inset_x"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.EdgeInsetX = &n
		}
	}
	if v := q.Get("edge_inset_y"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.EdgeInsetY = &n
		}
	}

	return query
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
