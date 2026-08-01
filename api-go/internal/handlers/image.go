package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"openposterdb/internal/services"
)

const freeAPIKey = "t0-free-rpdb"

// ImageQuery represents all query parameters for image endpoints.
type ImageQuery struct {
	Fallback         *string `json:"fallback"`
	Lang             *string `json:"lang"`
	ImageSize        *string `json:"imageSize"`
	RatingsLimit     *int32  `json:"ratings_limit"`
	RatingsOrder     *string `json:"ratings_order"`
	RatingsExclude   *string `json:"ratings_exclude"`
	BadgeStyle       *string `json:"badge_style"`
	LabelStyle       *string `json:"label_style"`
	BadgeSize        *string `json:"badge_size"`
	BadgeDirection   *string `json:"badge_direction"`
	BadgeShape       *string `json:"badge_shape"`
	BadgeBackground  *string `json:"badge_background"`
	Position         *string `json:"position"`
	ImageSource      *string `json:"image_source"`
	Textless         *bool   `json:"textless"`
	Blur             *bool   `json:"blur"`
	Split            *bool   `json:"split"`
	Fit              *string `json:"fit"`
	EdgeInsetX       *int32  `json:"edge_inset_x"`
	EdgeInsetY       *int32  `json:"edge_inset_y"`
}

func (q *ImageQuery) HasOverrides() bool {
	return q.RatingsLimit != nil || q.RatingsOrder != nil || q.RatingsExclude != nil ||
		q.BadgeStyle != nil || q.LabelStyle != nil || q.BadgeSize != nil ||
		q.BadgeDirection != nil || q.BadgeShape != nil || q.BadgeBackground != nil ||
		q.Position != nil || q.ImageSource != nil || q.Textless != nil ||
		q.Blur != nil || q.Split != nil || q.Fit != nil ||
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
		style := services.BadgeStyle(*query.BadgeStyle)
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

	if query.BadgeSize != nil {
		size := services.BadgeSize(*query.BadgeSize)
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

	if query.BadgeBackground != nil {
		bg := services.BadgeBackground(*query.BadgeBackground)
		switch kind {
		case "poster":
			s.PosterBadgeBackground = bg
		case "logo":
			s.LogoBadgeBackground = bg
		case "backdrop":
			s.BackdropBadgeBackground = bg
		case "episode":
			s.EpisodeBadgeBackground = bg
		}
	}

	if kind == "poster" {
		if query.BadgeDirection != nil {
			s.PosterBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Position != nil {
			s.PosterPosition = services.BadgePosition(*query.Position)
		}
		if query.Split != nil {
			s.PosterBadgeSplit = *query.Split
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
		if query.Position != nil {
			s.BackdropPosition = services.BadgePosition(*query.Position)
		}
		if query.EdgeInsetX != nil {
			s.BackdropEdgeInsetX = services.ClampEdgeInset(*query.EdgeInsetX)
		}
		if query.EdgeInsetY != nil {
			s.BackdropEdgeInsetY = services.ClampEdgeInset(*query.EdgeInsetY)
		}
	}

	if kind == "episode" {
		if query.BadgeDirection != nil {
			s.EpisodeBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Position != nil {
			s.EpisodePosition = services.BadgePosition(*query.Position)
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
	ImageSource              string `json:"image_source"`
	Lang                     string `json:"lang"`
	Textless                 bool   `json:"textless"`
	RatingsLimit             int32  `json:"ratings_limit"`
	RatingsOrder             string `json:"ratings_order"`
	RatingsExclude           string `json:"ratings_exclude"`
	PosterPosition           string `json:"poster_position"`
	LogoRatingsLimit         int32  `json:"logo_ratings_limit"`
	BackdropRatingsLimit     int32  `json:"backdrop_ratings_limit"`
	PosterBadgeStyle         string `json:"poster_badge_style"`
	LogoBadgeStyle           string `json:"logo_badge_style"`
	BackdropBadgeStyle       string `json:"backdrop_badge_style"`
	PosterLabelStyle         string `json:"poster_label_style"`
	LogoLabelStyle           string `json:"logo_label_style"`
	BackdropLabelStyle       string `json:"backdrop_label_style"`
	PosterBadgeDirection     string `json:"poster_badge_direction"`
	PosterBadgeSplit         bool   `json:"poster_badge_split"`
	PosterFit                string `json:"poster_fit"`
	PosterBadgeSize          string `json:"poster_badge_size"`
	LogoBadgeSize            string `json:"logo_badge_size"`
	BackdropBadgeSize        string `json:"backdrop_badge_size"`
	PosterBadgeShape         string `json:"poster_badge_shape"`
	LogoBadgeShape           string `json:"logo_badge_shape"`
	BackdropBadgeShape       string `json:"backdrop_badge_shape"`
	PosterBadgeBackground    string `json:"poster_badge_background"`
	LogoBadgeBackground      string `json:"logo_badge_background"`
	BackdropBadgeBackground  string `json:"backdrop_badge_background"`
	BackdropPosition         string `json:"backdrop_position"`
	BackdropBadgeDirection   string `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX       int32  `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY       int32  `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit      int32  `json:"episode_ratings_limit"`
	EpisodeBadgeStyle        string `json:"episode_badge_style"`
	EpisodeLabelStyle        string `json:"episode_label_style"`
	EpisodeBadgeSize         string `json:"episode_badge_size"`
	EpisodePosition          string `json:"episode_position"`
	EpisodeBadgeDirection    string `json:"episode_badge_direction"`
	EpisodeBlur              bool   `json:"episode_blur"`
	EpisodeBadgeShape        string `json:"episode_badge_shape"`
	EpisodeBadgeBackground    string `json:"episode_badge_background"`
}

func freeKeySettingsFromRender(s *services.RenderSettings) FreeKeySettingsResponse {
	return FreeKeySettingsResponse{
		ImageSource:              string(s.ImageSource),
		Lang:                     s.Lang,
		Textless:                 s.Textless,
		RatingsLimit:             s.RatingsLimit,
		RatingsOrder:             s.RatingsOrder,
		RatingsExclude:           s.RatingsExclude,
		PosterPosition:           string(s.PosterPosition),
		LogoRatingsLimit:         s.LogoRatingsLimit,
		BackdropRatingsLimit:     s.BackdropRatingsLimit,
		PosterBadgeStyle:         string(s.PosterBadgeStyle),
		LogoBadgeStyle:           string(s.LogoBadgeStyle),
		BackdropBadgeStyle:       string(s.BackdropBadgeStyle),
		PosterLabelStyle:         string(s.PosterLabelStyle),
		LogoLabelStyle:           string(s.LogoLabelStyle),
		BackdropLabelStyle:       string(s.BackdropLabelStyle),
		PosterBadgeDirection:     string(s.PosterBadgeDirection),
		PosterBadgeSplit:         s.PosterBadgeSplit,
		PosterFit:                string(s.PosterFit),
		PosterBadgeSize:          string(s.PosterBadgeSize),
		LogoBadgeSize:            string(s.LogoBadgeSize),
		BackdropBadgeSize:        string(s.BackdropBadgeSize),
		PosterBadgeShape:         string(s.PosterBadgeShape),
		LogoBadgeShape:           string(s.LogoBadgeShape),
		BackdropBadgeShape:       string(s.BackdropBadgeShape),
		PosterBadgeBackground:    string(s.PosterBadgeBackground),
		LogoBadgeBackground:      string(s.LogoBadgeBackground),
		BackdropBadgeBackground:  string(s.BackdropBadgeBackground),
		BackdropPosition:         string(s.BackdropPosition),
		BackdropBadgeDirection:   string(s.BackdropBadgeDirection),
		BackdropEdgeInsetX:       s.BackdropEdgeInsetX,
		BackdropEdgeInsetY:       s.BackdropEdgeInsetY,
		EpisodeRatingsLimit:      s.EpisodeRatingsLimit,
		EpisodeBadgeStyle:        string(s.EpisodeBadgeStyle),
		EpisodeLabelStyle:        string(s.EpisodeLabelStyle),
		EpisodeBadgeSize:         string(s.EpisodeBadgeSize),
		EpisodePosition:          string(s.EpisodePosition),
		EpisodeBadgeDirection:    string(s.EpisodeBadgeDirection),
		EpisodeBlur:              s.EpisodeBlur,
		EpisodeBadgeShape:        string(s.EpisodeBadgeShape),
		EpisodeBadgeBackground:    string(s.EpisodeBadgeBackground),
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

func HandleImage(db *sql.DB, tmdb *services.TmdbClient, omdb *services.OmdbClient, mdblist *services.MdblistClient, trakt *services.TraktClient, fanart *services.FanartClient, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
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

		// Resolve and apply badge direction defaults
		if kind == "poster" {
			settings.PosterBadgeDirection = settings.PosterBadgeDirection.Resolve(settings.PosterPosition)
			settings.PosterBadgeStyle = settings.PosterBadgeStyle.Resolve(settings.PosterBadgeDirection)
		}

		// Parse ID type
		idType, err := services.ParseIDType(idTypeStr)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}

		// Resolve ID
		resolved, err := services.ResolveID(idType, idValue, tmdb)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}

		if resolved.MediaType == services.MediaTypeEpisode && kind != "episode" {
			// Uplift episode to series for poster/logo/backdrop
			if resolved.Episode != nil {
				seriesID := services.FormatTMDbIDValue(resolved.Episode.ShowTMDbID, services.MediaTypeTV, nil)
				resolved, err = services.ResolveID(services.IDTypeTMDB, seriesID, tmdb)
				if err != nil {
					writeError(w, 404, err.Error())
					return
				}
			}
		}

		if kind == "episode" && resolved.MediaType != services.MediaTypeEpisode {
			writeError(w, 400, "not an episode - use poster/logo/backdrop endpoint")
			return
		}

		// Fetch ratings
		limit := settings.RatingsLimit
		switch kind {
		case "logo":
			limit = settings.LogoRatingsLimit
		case "backdrop":
			limit = settings.BackdropRatingsLimit
		case "episode":
			limit = settings.EpisodeRatingsLimit
		}

		var imdbID *string
		if resolved.IMDbID != nil && *resolved.IMDbID != "" {
			imdbID = resolved.IMDbID
		}

		mediaType := "movie"
		switch resolved.MediaType {
		case services.MediaTypeTV:
			mediaType = "tv"
		case services.MediaTypeEpisode:
			mediaType = "episode"
		}

		var epShowID uint64
		var epSeason, epEpisode uint32
		if resolved.Episode != nil {
			epShowID = resolved.Episode.ShowTMDbID
			epSeason = resolved.Episode.SeasonNumber
			epEpisode = resolved.Episode.EpisodeNumber
		}

		_, _, _, _, rawBadges := services.FetchRatings(
			resolved.TMDbID, mediaType, imdbID,
			epShowID, epSeason, epEpisode,
			tmdb, omdb, mdblist, trakt,
		)

		badges := services.ApplyRatingPreferences(rawBadges, settings.RatingsOrder, settings.RatingsExclude, limit)

		// Build cache suffix
		ratingsSuffix := services.BadgesCacheSuffix(badges)
		suffix := services.SettingsCacheSuffixWithRatings(settings, kind, query.ImageSize, ratingsSuffix)

		_ = suffix
		_ = settings

		// Return placeholder - image rendering not yet implemented
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{})
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
		query.BadgeSize = &v
	}
	if v := q.Get("badge_direction"); v != "" {
		query.BadgeDirection = &v
	}
	if v := q.Get("badge_shape"); v != "" {
		query.BadgeShape = &v
	}
	if v := q.Get("badge_background"); v != "" {
		query.BadgeBackground = &v
	}
	if v := q.Get("position"); v != "" {
		query.Position = &v
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
	if v := q.Get("split"); v != "" {
		b := v == "true"
		query.Split = &b
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
