package handlers

import (
	"database/sql"
	"encoding/json"
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

func HandleGetSettings(db *sql.DB) http.HandlerFunc {
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
		writeJSON(w, 200, globals)
	}
}

func HandleUpdateSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, 405, "Method not allowed")
			return
		}

		var settings map[string]string
		if err := decodeJSONBody(r, &settings); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}

		if err := services.SetGlobalSettingsBatch(db, settings); err != nil {
			writeError(w, 500, "Failed to save settings")
			return
		}

		writeJSON(w, 200, map[string]bool{"success": true})
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
