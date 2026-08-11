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

func HandleUpdateSettings(db *sql.DB, freeKeyLocked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		// Decode the payload as a raw map so every key flows through the shared
		// RenderSettingFields spec (services/settings.go). The previous
		// hand-written updateSettingsRequest struct was the parallel source of
		// truth that drifted out of sync with RenderSettingsToMap — see the
		// 2026-08-04 badge-width bug. free_api_key_enabled is handled below
		// because it lives outside RenderSettings.
		var payload map[string]json.RawMessage
		if err := httpx.DecodeJSON(r, &payload); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}

		globals, err := services.GetGlobalSettingsCtx(r.Context(), db)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to load settings")
			return
		}
		s := services.ParseGlobalRenderSettings(globals)

		if err := services.ApplyUpdatePayload(&s, payload); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}

		if err := services.ValidateRenderSettings(&s); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}

		batch := services.RenderSettingsToMap(&s)
		if !freeKeyLocked {
			if raw, ok := payload["free_api_key_enabled"]; ok && string(raw) != "null" {
				var enabled bool
				if err := json.Unmarshal(raw, &enabled); err != nil {
					httpx.WriteError(w, 400, "invalid free_api_key_enabled")
					return
				}
				if enabled {
					batch["free_api_key_enabled"] = "true"
				} else {
					batch["free_api_key_enabled"] = "false"
				}
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
		sortBy := r.URL.Query().Get("sort_by")
		sortDir := r.URL.Query().Get("sort_dir")

		items, total, err := services.ListImageMetaByKindCtx(r.Context(), db, imageType, sortBy, sortDir, page, pageSize)
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
