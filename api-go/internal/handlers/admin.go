package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"time"

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

// processStart is when the server started (dashboard uptime).
var processStart = time.Now()

// HandleStats serves the dashboard numbers. externalCacheOnly means there is
// no disk cache (EXTERNAL_CACHE_ONLY), so its size is reported as null.
func HandleStats(db *sql.DB, cacheDir string, externalCacheOnly bool, caches *services.MemCacheSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		// A failed count used to show as 0 on the dashboard.
		var countErr error
		count := func(n int64, err error) int64 {
			if err != nil && countErr == nil {
				countErr = err
			}
			return n
		}
		posterCount := count(services.CountImageMetaCtx(r.Context(), db, "p"))
		logoCount := count(services.CountImageMetaCtx(r.Context(), db, "l"))
		backdropCount := count(services.CountImageMetaCtx(r.Context(), db, "b"))
		episodeCount := count(services.CountImageMetaCtx(r.Context(), db, "e"))
		apiKeyCount := count(services.CountAPIKeysCtx(r.Context(), db))
		if countErr != nil {
			slog.Warn("stats count failed", "error", countErr)
			httpx.WriteError(w, 500, "Failed to read stats")
			return
		}

		var memCacheEntries, idCacheEntries, ratingsCacheEntries, imageRequests int64
		var imageMemCacheMB float64
		var memCacheHitRate *float64 // null until the first image request
		if caches != nil {
			if caches.ImageMem != nil {
				memCacheEntries = caches.ImageMem.Len()
				imageMemCacheMB = float64(caches.ImageMem.WeightedBytes()) / (1024 * 1024)
				hits, misses := caches.ImageMem.HitsMisses()
				if imageRequests = hits + misses; imageRequests > 0 {
					rate := float64(hits) * 100 / float64(imageRequests)
					memCacheHitRate = &rate
				}
			}
			if caches.IDs != nil {
				idCacheEntries = caches.IDs.Len()
			}
			if caches.Ratings != nil {
				ratingsCacheEntries = caches.Ratings.Len()
			}
		}

		// Disk cache size, measured at most every 30 s (or right after a
		// purge). A failed walk shows as "—" rather than failing the whole
		// dashboard.
		var imageDiskCacheMB *float64
		var imageDiskCacheFiles *int64
		if !externalCacheOnly {
			if u, err := services.CachedDiskUsage(r.Context(), cacheDir); err == nil {
				mb := float64(u.Bytes) / (1024 * 1024)
				imageDiskCacheMB, imageDiskCacheFiles = &mb, &u.Files
			} else {
				slog.Warn("measuring the disk cache failed", "error", err)
			}
		}

		// SQLite file size from its own page counters (no path needed; the
		// WAL is not included). Null if the pragmas fail.
		var databaseMB *float64
		var pages, pageSize int64
		if db.QueryRowContext(r.Context(), "PRAGMA page_count").Scan(&pages) == nil &&
			db.QueryRowContext(r.Context(), "PRAGMA page_size").Scan(&pageSize) == nil {
			mb := float64(pages*pageSize) / (1024 * 1024)
			databaseMB = &mb
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"total_images":           posterCount + logoCount + backdropCount + episodeCount,
			"total_api_keys":         apiKeyCount,
			"cached_posters":         posterCount,
			"cached_logos":           logoCount,
			"cached_backdrops":       backdropCount,
			"cached_episodes":        episodeCount,
			"api_key_count":          apiKeyCount,
			"mem_cache_entries":      memCacheEntries,
			"id_cache_entries":       idCacheEntries,
			"ratings_cache_entries":  ratingsCacheEntries,
			"image_mem_cache_mb":     imageMemCacheMB,
			"image_disk_cache_mb":    imageDiskCacheMB,
			"image_disk_cache_files": imageDiskCacheFiles,
			"uptime_seconds":         int64(time.Since(processStart).Seconds()),
			"database_mb":            databaseMB,
			"image_requests":         imageRequests,
			"mem_cache_hit_rate":     memCacheHitRate,
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
		if pageSize > 200 {
			pageSize = 200 // unbounded, page_size=1e9 returned the whole table
		}
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

		var errs []string
		var dirsCleared int64 = 0
		if !externalCacheOnly {
			staged, err := services.StageCacheForClear(cacheDir)
			if err != nil {
				errs = append(errs, "cache dirs: "+err.Error())
			}
			dirsCleared = int64(len(staged))
			go services.RemoveStagedDirs(staged)
		}

		metaDeleted, err := services.DeleteAllImageMetaCtx(r.Context(), db)
		if err != nil {
			errs = append(errs, "image metadata: "+err.Error())
		}
		ratingsDeleted, err := services.DeleteAllAvailableRatingsCtx(r.Context(), db)
		if err != nil {
			errs = append(errs, "available ratings: "+err.Error())
		}

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
		// ...and the per-client upstream caches (TMDB/fanart image lists,
		// Kitsu/AniList lookups), so the next render re-fetches everything.
		services.ClearUpstreamCaches()

		writePurgeResult(w, errs, map[string]any{
			"external_cache_only": externalCacheOnly,
			"dirs_cleared":        dirsCleared,
			"meta_deleted":        metaDeleted,
			"ratings_deleted":     ratingsDeleted,
		})
	}
}
