package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"openposterdb/internal/services"
)

func HandleStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		posterCount, _ := services.CountImageMeta(db, "p")
		logoCount, _ := services.CountImageMeta(db, "l")
		backdropCount, _ := services.CountImageMeta(db, "b")
		episodeCount, _ := services.CountImageMeta(db, "e")
		apiKeyCount, _ := services.CountAPIKeys(db)

		writeJSON(w, 200, map[string]interface{}{
			"total_images":           posterCount + logoCount + backdropCount + episodeCount,
			"total_api_keys":          apiKeyCount,
			"cached_posters":         posterCount,
			"cached_logos":           logoCount,
			"cached_backdrops":       backdropCount,
			"cached_episodes":        episodeCount,
			"api_key_count":          apiKeyCount,
		})
	}
}

func HandleGetSettings(db *sql.DB, freeKeyEnabled, freeKeyLocked, fanartAvailable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		globals, err := services.GetGlobalSettings(db)
		if err != nil {
			writeError(w, 500, "Failed to load settings")
			return
		}
		s := services.ParseGlobalRenderSettings(globals)

		writeJSON(w, 200, map[string]interface{}{
			"image_source":               string(s.ImageSource),
			"lang":                       s.Lang,
			"textless":                   s.Textless,
			"fanart_available":           fanartAvailable,
			"ratings_limit":              s.RatingsLimit,
			"ratings_order":              s.RatingsOrder,
			"ratings_exclude":            s.RatingsExclude,
			"free_api_key_enabled":       freeKeyEnabled,
			"free_api_key_locked":        freeKeyLocked,
			"poster_position":            string(s.PosterPosition),
			"logo_ratings_limit":         s.LogoRatingsLimit,
			"backdrop_ratings_limit":     s.BackdropRatingsLimit,
			"poster_badge_style":         string(s.PosterBadgeStyle),
			"logo_badge_style":           string(s.LogoBadgeStyle),
			"backdrop_badge_style":       string(s.BackdropBadgeStyle),
			"poster_label_style":         string(s.PosterLabelStyle),
			"logo_label_style":           string(s.LogoLabelStyle),
			"backdrop_label_style":       string(s.BackdropLabelStyle),
			"poster_badge_direction":     string(s.PosterBadgeDirection),
			"poster_badge_split":         s.PosterBadgeSplit,
			"poster_fit":                 string(s.PosterFit),
			"poster_badge_size":          string(s.PosterBadgeSize),
			"logo_badge_size":            string(s.LogoBadgeSize),
			"backdrop_badge_size":        string(s.BackdropBadgeSize),
			"backdrop_position":          string(s.BackdropPosition),
			"backdrop_badge_direction":   string(s.BackdropBadgeDirection),
			"backdrop_edge_inset_x":      s.BackdropEdgeInsetX,
			"backdrop_edge_inset_y":      s.BackdropEdgeInsetY,
			"episode_ratings_limit":      s.EpisodeRatingsLimit,
			"episode_badge_style":        string(s.EpisodeBadgeStyle),
			"episode_label_style":        string(s.EpisodeLabelStyle),
			"episode_badge_size":         string(s.EpisodeBadgeSize),
			"episode_position":           string(s.EpisodePosition),
			"episode_badge_direction":    string(s.EpisodeBadgeDirection),
			"episode_blur":               s.EpisodeBlur,
			"poster_badge_shape":         string(s.PosterBadgeShape),
			"logo_badge_shape":           string(s.LogoBadgeShape),
			"backdrop_badge_shape":       string(s.BackdropBadgeShape),
			"episode_badge_shape":        string(s.EpisodeBadgeShape),
			"poster_badge_alpha":         int32(s.PosterBadgeAlpha),
			"logo_badge_alpha":           int32(s.LogoBadgeAlpha),
			"backdrop_badge_alpha":       int32(s.BackdropBadgeAlpha),
			"episode_badge_alpha":        int32(s.EpisodeBadgeAlpha),
			"colors":                     services.EffectiveSourceColors(&s),
		})
	}
}

type updateSettingsRequest struct {
	ImageSource             *string `json:"image_source"`
	Lang                    *string `json:"lang"`
	Textless                *bool   `json:"textless"`
	RatingsLimit            *int32  `json:"ratings_limit"`
	RatingsOrder            *string `json:"ratings_order"`
	RatingsExclude          *string `json:"ratings_exclude"`
	FreeAPIKeyEnabled       *bool   `json:"free_api_key_enabled"`
	PosterPosition          *string `json:"poster_position"`
	LogoRatingsLimit        *int32  `json:"logo_ratings_limit"`
	BackdropRatingsLimit    *int32  `json:"backdrop_ratings_limit"`
	PosterBadgeStyle        *string `json:"poster_badge_style"`
	LogoBadgeStyle          *string `json:"logo_badge_style"`
	BackdropBadgeStyle      *string `json:"backdrop_badge_style"`
	PosterLabelStyle        *string `json:"poster_label_style"`
	LogoLabelStyle          *string `json:"logo_label_style"`
	BackdropLabelStyle      *string `json:"backdrop_label_style"`
	PosterBadgeDirection    *string `json:"poster_badge_direction"`
	PosterBadgeSplit        *bool   `json:"poster_badge_split"`
	PosterFit               *string `json:"poster_fit"`
	PosterBadgeSize         *string `json:"poster_badge_size"`
	LogoBadgeSize           *string `json:"logo_badge_size"`
	BackdropBadgeSize       *string `json:"backdrop_badge_size"`
	BackdropPosition        *string `json:"backdrop_position"`
	BackdropBadgeDirection  *string `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX      *int32  `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY      *int32  `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit     *int32  `json:"episode_ratings_limit"`
	EpisodeBadgeStyle       *string `json:"episode_badge_style"`
	EpisodeLabelStyle       *string `json:"episode_label_style"`
	EpisodeBadgeSize        *string `json:"episode_badge_size"`
	EpisodePosition         *string `json:"episode_position"`
	EpisodeBadgeDirection   *string `json:"episode_badge_direction"`
	EpisodeBlur             *bool   `json:"episode_blur"`
	PosterBadgeShape        *string `json:"poster_badge_shape"`
	LogoBadgeShape          *string `json:"logo_badge_shape"`
	BackdropBadgeShape      *string `json:"backdrop_badge_shape"`
	EpisodeBadgeShape       *string `json:"episode_badge_shape"`
	PosterBadgeAlpha        *int32  `json:"poster_badge_alpha"`
	LogoBadgeAlpha          *int32  `json:"logo_badge_alpha"`
	BackdropBadgeAlpha      *int32  `json:"backdrop_badge_alpha"`
	EpisodeBadgeAlpha       *int32  `json:"episode_badge_alpha"`
	Colors                  *map[string]services.SourceColorSet `json:"colors"`
}

func HandleUpdateSettings(db *sql.DB, freeKeyLocked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, 405, "Method not allowed")
			return
		}

		var req updateSettingsRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}

		globals, err := services.GetGlobalSettings(db)
		if err != nil {
			writeError(w, 500, "Failed to load settings")
			return
		}
		s := services.ParseGlobalRenderSettings(globals)

		if req.ImageSource != nil {
			s.ImageSource = services.ImageSource(*req.ImageSource)
		}
		if req.Lang != nil {
			s.Lang = *req.Lang
		}
		if req.Textless != nil {
			s.Textless = *req.Textless
		}
		if req.RatingsLimit != nil {
			s.RatingsLimit = *req.RatingsLimit
		}
		if req.RatingsOrder != nil {
			s.RatingsOrder = *req.RatingsOrder
		}
		if req.RatingsExclude != nil {
			s.RatingsExclude = *req.RatingsExclude
		}
		if req.PosterPosition != nil {
			s.PosterPosition = services.BadgePosition(*req.PosterPosition)
		}
		if req.LogoRatingsLimit != nil {
			s.LogoRatingsLimit = *req.LogoRatingsLimit
		}
		if req.BackdropRatingsLimit != nil {
			s.BackdropRatingsLimit = *req.BackdropRatingsLimit
		}
		if req.PosterBadgeStyle != nil {
			s.PosterBadgeStyle = services.BadgeStyle(*req.PosterBadgeStyle)
		}
		if req.LogoBadgeStyle != nil {
			s.LogoBadgeStyle = services.BadgeStyle(*req.LogoBadgeStyle)
		}
		if req.BackdropBadgeStyle != nil {
			s.BackdropBadgeStyle = services.BadgeStyle(*req.BackdropBadgeStyle)
		}
		if req.PosterLabelStyle != nil {
			s.PosterLabelStyle = services.LabelStyle(*req.PosterLabelStyle)
		}
		if req.LogoLabelStyle != nil {
			s.LogoLabelStyle = services.LabelStyle(*req.LogoLabelStyle)
		}
		if req.BackdropLabelStyle != nil {
			s.BackdropLabelStyle = services.LabelStyle(*req.BackdropLabelStyle)
		}
		if req.PosterBadgeDirection != nil {
			s.PosterBadgeDirection = services.BadgeDirection(*req.PosterBadgeDirection)
		}
		if req.PosterBadgeSplit != nil {
			s.PosterBadgeSplit = *req.PosterBadgeSplit
		}
		if req.PosterFit != nil {
			s.PosterFit = services.PosterFit(*req.PosterFit)
		}
		if req.PosterBadgeSize != nil {
			s.PosterBadgeSize = services.BadgeSize(*req.PosterBadgeSize)
		}
		if req.LogoBadgeSize != nil {
			s.LogoBadgeSize = services.BadgeSize(*req.LogoBadgeSize)
		}
		if req.BackdropBadgeSize != nil {
			s.BackdropBadgeSize = services.BadgeSize(*req.BackdropBadgeSize)
		}
		if req.BackdropPosition != nil {
			s.BackdropPosition = services.BadgePosition(*req.BackdropPosition)
		}
		if req.BackdropBadgeDirection != nil {
			s.BackdropBadgeDirection = services.BadgeDirection(*req.BackdropBadgeDirection)
		}
		if req.BackdropEdgeInsetX != nil {
			s.BackdropEdgeInsetX = services.ClampEdgeInset(*req.BackdropEdgeInsetX)
		}
		if req.BackdropEdgeInsetY != nil {
			s.BackdropEdgeInsetY = services.ClampEdgeInset(*req.BackdropEdgeInsetY)
		}
		if req.EpisodeRatingsLimit != nil {
			s.EpisodeRatingsLimit = *req.EpisodeRatingsLimit
		}
		if req.EpisodeBadgeStyle != nil {
			s.EpisodeBadgeStyle = services.BadgeStyle(*req.EpisodeBadgeStyle)
		}
		if req.EpisodeLabelStyle != nil {
			s.EpisodeLabelStyle = services.LabelStyle(*req.EpisodeLabelStyle)
		}
		if req.EpisodeBadgeSize != nil {
			s.EpisodeBadgeSize = services.BadgeSize(*req.EpisodeBadgeSize)
		}
		if req.EpisodePosition != nil {
			s.EpisodePosition = services.BadgePosition(*req.EpisodePosition)
		}
		if req.EpisodeBadgeDirection != nil {
			s.EpisodeBadgeDirection = services.BadgeDirection(*req.EpisodeBadgeDirection)
		}
		if req.EpisodeBlur != nil {
			s.EpisodeBlur = *req.EpisodeBlur
		}
		if req.PosterBadgeShape != nil {
			s.PosterBadgeShape = services.BadgeShape(*req.PosterBadgeShape)
		}
		if req.LogoBadgeShape != nil {
			s.LogoBadgeShape = services.BadgeShape(*req.LogoBadgeShape)
		}
		if req.BackdropBadgeShape != nil {
			s.BackdropBadgeShape = services.BadgeShape(*req.BackdropBadgeShape)
		}
		if req.EpisodeBadgeShape != nil {
			s.EpisodeBadgeShape = services.BadgeShape(*req.EpisodeBadgeShape)
		}
		if req.PosterBadgeAlpha != nil {
			s.PosterBadgeAlpha = services.ClampBadgeAlpha(*req.PosterBadgeAlpha)
		}
		if req.LogoBadgeAlpha != nil {
			s.LogoBadgeAlpha = services.ClampBadgeAlpha(*req.LogoBadgeAlpha)
		}
		if req.BackdropBadgeAlpha != nil {
			s.BackdropBadgeAlpha = services.ClampBadgeAlpha(*req.BackdropBadgeAlpha)
		}
		if req.EpisodeBadgeAlpha != nil {
			s.EpisodeBadgeAlpha = services.ClampBadgeAlpha(*req.EpisodeBadgeAlpha)
		}
		if req.Colors != nil {
			if err := services.ValidateSourceColors(*req.Colors); err != nil {
				writeError(w, 400, err.Error())
				return
			}
			s.Colors = services.NormalizeSourceColors(*req.Colors)
		}

		if err := services.ValidateRenderSettings(&s); err != nil {
			writeError(w, 400, err.Error())
			return
		}

		batch := services.RenderSettingsToMap(&s)
		if !freeKeyLocked && req.FreeAPIKeyEnabled != nil {
			if *req.FreeAPIKeyEnabled {
				batch["free_api_key_enabled"] = "true"
			} else {
				batch["free_api_key_enabled"] = "false"
			}
		}

		if err := services.SetGlobalSettingsBatch(db, batch); err != nil {
			writeError(w, 500, "Failed to save settings")
			return
		}
		if err := services.PruneStaleColorSettings(db, globals, batch); err != nil {
			slog.Warn("failed to prune stale color settings", "error", err)
		}

		slog.Debug("global settings updated", "image_source", s.ImageSource, "lang", s.Lang, "ratings_order", s.RatingsOrder)
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleListImages(db *sql.DB, imageType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		page := parseIntParam(r, "page", 1)
		pageSize := parseIntParam(r, "page_size", 50)

		items, total, err := services.ListImageMetaByKind(db, imageType, page, pageSize)
		if err != nil {
			writeError(w, 500, "Failed to list images")
			return
		}

		writeJSON(w, 200, map[string]interface{}{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	}
}

func HandlePurgeAll(db *sql.DB, cacheDir string, externalCacheOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}

		var dirsCleared int64 = 0
		if !externalCacheOnly {
			staged, _ := services.StageCacheForClear(cacheDir)
			dirsCleared = int64(len(staged))
			go services.RemoveStagedDirs(staged)
		}

		metaDeleted, _ := services.DeleteAllImageMeta(db)
		ratingsDeleted, _ := services.DeleteAllAvailableRatings(db)

		writeJSON(w, 200, map[string]interface{}{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"dirs_cleared":        dirsCleared,
			"meta_deleted":        metaDeleted,
			"ratings_deleted":     ratingsDeleted,
		})
	}
}

func parseIntParam(r *http.Request, name string, def int64) int64 {
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

func decodeJSONBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
