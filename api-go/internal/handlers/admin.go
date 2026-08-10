package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// HandleUserPrefs reads (GET) or updates (PUT, partial merge) the
// authenticated admin user's UI preferences (e.g. the sidebar disclaimer
// minimised state), persisted per-user so they survive login sessions.
func HandleUserPrefs(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := GetAuthUser(r)
		if user == nil {
			httpx.WriteError(w, 401, "Unauthorized")
			return
		}

		switch r.Method {
		case http.MethodGet:
			prefs, err := services.GetUserPrefsCtx(r.Context(), db, user.Username)
			if err != nil {
				httpx.WriteError(w, 500, "Failed to load preferences")
				return
			}
			httpx.WriteJSON(w, 200, prefs)
		case http.MethodPut:
			var updates map[string]string
			if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
				httpx.WriteError(w, 400, "Invalid JSON")
				return
			}
			prefs, err := services.GetUserPrefsCtx(r.Context(), db, user.Username)
			if err != nil {
				httpx.WriteError(w, 500, "Failed to load preferences")
				return
			}
			maps.Copy(prefs, updates)
			if err := services.SetUserPrefsCtx(r.Context(), db, user.Username, prefs); err != nil {
				httpx.WriteError(w, 500, "Failed to save preferences")
				return
			}
			httpx.WriteJSON(w, 200, prefs)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	}
}

func HandleStats(db *sql.DB, cacheDir string, caches *services.MemCacheSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		posterCount, _ := services.CountImageMetaCtx(r.Context(), db, "p")
		logoCount, _ := services.CountImageMetaCtx(r.Context(), db, "l")
		backdropCount, _ := services.CountImageMetaCtx(r.Context(), db, "b")
		episodeCount, _ := services.CountImageMetaCtx(r.Context(), db, "e")
		apiKeyCount, _ := services.CountAPIKeysCtx(r.Context(), db)

		var memCacheEntries, idCacheEntries, ratingsCacheEntries int64
		var imageMemCacheMB float64
		if caches != nil {
			if caches.ImageMem != nil {
				memCacheEntries = caches.ImageMem.Len()
				imageMemCacheMB = float64(caches.ImageMem.WeightedBytes()) / (1024 * 1024)
			}
			if caches.IDs != nil {
				idCacheEntries = caches.IDs.Len()
			}
			if caches.Ratings != nil {
				ratingsCacheEntries = caches.Ratings.Len()
			}
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"total_images":          posterCount + logoCount + backdropCount + episodeCount,
			"total_api_keys":        apiKeyCount,
			"cached_posters":        posterCount,
			"cached_logos":          logoCount,
			"cached_backdrops":      backdropCount,
			"cached_episodes":       episodeCount,
			"api_key_count":         apiKeyCount,
			"mem_cache_entries":     memCacheEntries,
			"id_cache_entries":      idCacheEntries,
			"ratings_cache_entries": ratingsCacheEntries,
			"image_mem_cache_mb":    imageMemCacheMB,
		})
	}
}

func HandleGetSettings(db *sql.DB, freeKeyEnabled, freeKeyLocked, fanartAvailable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		globals, err := services.GetGlobalSettingsCtx(r.Context(), db)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to load settings")
			return
		}
		s := services.ParseGlobalRenderSettings(globals)

		resp := services.SettingsResponseMap(&s)
		resp["fanart_available"] = fanartAvailable
		resp["free_api_key_enabled"] = freeKeyEnabled
		resp["free_api_key_locked"] = freeKeyLocked

		httpx.WriteJSON(w, 200, resp)
	}
}

type updateSettingsRequest struct {
	ImageSource            *string                             `json:"image_source"`
	Lang                   *string                             `json:"lang"`
	Textless               *bool                               `json:"textless"`
	RatingsLimit           *int32                              `json:"ratings_limit"`
	RatingsOrder           *string                             `json:"ratings_order"`
	RatingsExclude         *string                             `json:"ratings_exclude"`
	FreeAPIKeyEnabled      *bool                               `json:"free_api_key_enabled"`
	PosterLayout           *services.ImageLayout               `json:"poster_layout"`
	LogoRatingsLimit       *int32                              `json:"logo_ratings_limit"`
	BackdropRatingsLimit   *int32                              `json:"backdrop_ratings_limit"`
	PosterBadgeStyle       *string                             `json:"poster_badge_style"`
	LogoBadgeStyle         *string                             `json:"logo_badge_style"`
	BackdropBadgeStyle     *string                             `json:"backdrop_badge_style"`
	PosterLabelStyle       *string                             `json:"poster_label_style"`
	LogoLabelStyle         *string                             `json:"logo_label_style"`
	BackdropLabelStyle     *string                             `json:"backdrop_label_style"`
	PosterBadgeDirection   *string                             `json:"poster_badge_direction"`
	PosterFit              *string                             `json:"poster_fit"`
	PosterTextSize         *int32                              `json:"poster_text_size"`
	LogoTextSize           *int32                              `json:"logo_text_size"`
	BackdropTextSize       *int32                              `json:"backdrop_text_size"`
	PosterBadgeSize        *int32                              `json:"poster_badge_size"`
	PosterBadgeWidth       *int32                              `json:"poster_badge_width"`
	PosterBadgeHeight      *int32                              `json:"poster_badge_height"`
	LogoBadgeSize          *int32                              `json:"logo_badge_size"`
	LogoBadgeWidth         *int32                              `json:"logo_badge_width"`
	LogoBadgeHeight        *int32                              `json:"logo_badge_height"`
	BackdropBadgeSize      *int32                              `json:"backdrop_badge_size"`
	BackdropBadgeWidth     *int32                              `json:"backdrop_badge_width"`
	BackdropBadgeHeight    *int32                              `json:"backdrop_badge_height"`
	PosterLogoSize         *int32                              `json:"poster_logo_size"`
	LogoLogoSize           *int32                              `json:"logo_logo_size"`
	BackdropLogoSize       *int32                              `json:"backdrop_logo_size"`
	LogoLayout             *services.ImageLayout               `json:"logo_layout"`
	BackdropLayout         *services.ImageLayout               `json:"backdrop_layout"`
	BackdropBadgeDirection *string                             `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX     *int32                              `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY     *int32                              `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit    *int32                              `json:"episode_ratings_limit"`
	EpisodeBadgeStyle      *string                             `json:"episode_badge_style"`
	EpisodeLabelStyle      *string                             `json:"episode_label_style"`
	EpisodeTextSize        *int32                              `json:"episode_text_size"`
	EpisodeBadgeSize       *int32                              `json:"episode_badge_size"`
	EpisodeBadgeWidth      *int32                              `json:"episode_badge_width"`
	EpisodeBadgeHeight     *int32                              `json:"episode_badge_height"`
	EpisodeLogoSize        *int32                              `json:"episode_logo_size"`
	EpisodeLayout          *services.ImageLayout               `json:"episode_layout"`
	EpisodeBadgeDirection  *string                             `json:"episode_badge_direction"`
	EpisodeBlur            *bool                               `json:"episode_blur"`
	PosterBadgeShape       *string                             `json:"poster_badge_shape"`
	LogoBadgeShape         *string                             `json:"logo_badge_shape"`
	BackdropBadgeShape     *string                             `json:"backdrop_badge_shape"`
	EpisodeBadgeShape      *string                             `json:"episode_badge_shape"`
	PosterBadgeAlpha       *int32                              `json:"poster_badge_alpha"`
	LogoBadgeAlpha         *int32                              `json:"logo_badge_alpha"`
	BackdropBadgeAlpha     *int32                              `json:"backdrop_badge_alpha"`
	EpisodeBadgeAlpha      *int32                              `json:"episode_badge_alpha"`
	Colors                 *map[string]services.SourceColorSet `json:"colors"`
}

func HandleUpdateSettings(db *sql.DB, freeKeyLocked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		var req updateSettingsRequest
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}

		globals, err := services.GetGlobalSettingsCtx(r.Context(), db)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to load settings")
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
		if req.PosterLayout != nil {
			s.PosterLayout = *req.PosterLayout
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
		if req.PosterFit != nil {
			s.PosterFit = services.PosterFit(*req.PosterFit)
		}
		if req.PosterTextSize != nil {
			s.PosterTextSize = services.ClampScalePercent(*req.PosterTextSize)
		}
		if req.LogoTextSize != nil {
			s.LogoTextSize = services.ClampScalePercent(*req.LogoTextSize)
		}
		if req.BackdropTextSize != nil {
			s.BackdropTextSize = services.ClampScalePercent(*req.BackdropTextSize)
		}
		if req.PosterBadgeSize != nil {
			s.PosterBadgeSize = services.ClampScalePercent(*req.PosterBadgeSize)
		}
		if req.PosterBadgeWidth != nil {
			s.PosterBadgeWidth = services.ClampScalePercent(*req.PosterBadgeWidth)
		}
		if req.PosterBadgeHeight != nil {
			s.PosterBadgeHeight = services.ClampScalePercent(*req.PosterBadgeHeight)
		}
		if req.LogoBadgeSize != nil {
			s.LogoBadgeSize = services.ClampScalePercent(*req.LogoBadgeSize)
		}
		if req.LogoBadgeWidth != nil {
			s.LogoBadgeWidth = services.ClampScalePercent(*req.LogoBadgeWidth)
		}
		if req.LogoBadgeHeight != nil {
			s.LogoBadgeHeight = services.ClampScalePercent(*req.LogoBadgeHeight)
		}
		if req.BackdropBadgeSize != nil {
			s.BackdropBadgeSize = services.ClampScalePercent(*req.BackdropBadgeSize)
		}
		if req.BackdropBadgeWidth != nil {
			s.BackdropBadgeWidth = services.ClampScalePercent(*req.BackdropBadgeWidth)
		}
		if req.BackdropBadgeHeight != nil {
			s.BackdropBadgeHeight = services.ClampScalePercent(*req.BackdropBadgeHeight)
		}
		if req.PosterLogoSize != nil {
			s.PosterLogoSize = services.ClampScalePercent(*req.PosterLogoSize)
		}
		if req.LogoLogoSize != nil {
			s.LogoLogoSize = services.ClampScalePercent(*req.LogoLogoSize)
		}
		if req.BackdropLogoSize != nil {
			s.BackdropLogoSize = services.ClampScalePercent(*req.BackdropLogoSize)
		}
		if req.LogoLayout != nil {
			s.LogoLayout = *req.LogoLayout
		}
		if req.BackdropLayout != nil {
			s.BackdropLayout = *req.BackdropLayout
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
		if req.EpisodeTextSize != nil {
			s.EpisodeTextSize = services.ClampScalePercent(*req.EpisodeTextSize)
		}
		if req.EpisodeBadgeSize != nil {
			s.EpisodeBadgeSize = services.ClampScalePercent(*req.EpisodeBadgeSize)
		}
		if req.EpisodeBadgeWidth != nil {
			s.EpisodeBadgeWidth = services.ClampScalePercent(*req.EpisodeBadgeWidth)
		}
		if req.EpisodeBadgeHeight != nil {
			s.EpisodeBadgeHeight = services.ClampScalePercent(*req.EpisodeBadgeHeight)
		}
		if req.EpisodeLogoSize != nil {
			s.EpisodeLogoSize = services.ClampScalePercent(*req.EpisodeLogoSize)
		}
		if req.EpisodeLayout != nil {
			s.EpisodeLayout = *req.EpisodeLayout
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
				httpx.WriteError(w, 400, err.Error())
				return
			}
			s.Colors = services.NormalizeSourceColors(*req.Colors)
		}

		if err := services.ValidateRenderSettings(&s); err != nil {
			httpx.WriteError(w, 400, err.Error())
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

		if err := services.SetGlobalSettingsBatchCtx(r.Context(), db, batch); err != nil {
			httpx.WriteError(w, 500, "Failed to save settings")
			return
		}
		if err := services.PruneStaleColorSettingsCtx(r.Context(), db, globals, batch); err != nil {
			slog.Warn("failed to prune stale color settings", "error", err)
		}

		slog.Debug("global settings updated", "image_source", s.ImageSource, "lang", s.Lang, "ratings_order", s.RatingsOrder)
		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleListImages(db *sql.DB, imageType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		page := httpx.ParseIntParam(r, "page", 1)
		pageSize := httpx.ParseIntParam(r, "page_size", 50)

		items, total, err := services.ListImageMetaByKindCtx(r.Context(), db, imageType, page, pageSize)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to list images")
			return
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	}
}

func HandlePurgeAll(db *sql.DB, cacheDir string, externalCacheOnly bool, caches *services.MemCacheSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		var dirsCleared int64 = 0
		if !externalCacheOnly {
			staged, _ := services.StageCacheForClear(cacheDir)
			dirsCleared = int64(len(staged))
			go services.RemoveStagedDirs(staged)
		}

		metaDeleted, _ := services.DeleteAllImageMetaCtx(r.Context(), db)
		ratingsDeleted, _ := services.DeleteAllAvailableRatingsCtx(r.Context(), db)

		// Drop every in-memory entry so a purge is immediately visible.
		if caches != nil {
			if caches.ImageMem != nil {
				caches.ImageMem.Clear()
			}
			if caches.IDs != nil {
				caches.IDs.Clear()
			}
			if caches.Ratings != nil {
				caches.Ratings.Clear()
			}
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"ok":                  true,
			"external_cache_only": externalCacheOnly,
			"dirs_cleared":        dirsCleared,
			"meta_deleted":        metaDeleted,
			"ratings_deleted":     ratingsDeleted,
		})
	}
}
