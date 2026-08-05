package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

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
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		if !isFreeAPIKeyEnabled() {
			httpx.WriteError(w, 401, "Unauthorized")
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
