package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"openposterdb/internal/services"
)

func HandleListKeys(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		keys, err := services.ListAPIKeys(db)
		if err != nil {
			writeError(w, 500, "Failed to list keys")
			return
		}

		type keyResp struct {
			ID         int64   `json:"id"`
			Name       string  `json:"name"`
			KeyPrefix  string  `json:"key_prefix"`
			CreatedAt  string  `json:"created_at"`
			LastUsedAt *string `json:"last_used_at"`
		}

		result := make([]keyResp, len(keys))
		for i, k := range keys {
			result[i] = keyResp{
				ID:         k.ID,
				Name:       k.Name,
				KeyPrefix:  k.KeyPrefix,
				CreatedAt:  k.CreatedAt,
				LastUsedAt: k.LastUsedAt,
			}
		}

		writeJSON(w, 200, result)
	}
}

func HandleCreateKey(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := decodeJSONBody(r, &body); err != nil || body.Name == "" {
			writeError(w, 400, "name is required")
			return
		}

		raw, hash, prefix := GenerateAPIKey()

		var createdBy int64 = 1
		id, err := services.CreateAPIKey(db, body.Name, hash, prefix, createdBy)
		if err != nil {
			writeError(w, 500, "Failed to create key")
			return
		}

		writeJSON(w, 201, map[string]interface{}{
			"id":         id,
			"name":       body.Name,
			"key":        raw,
			"key_prefix": prefix,
		})
	}
}

func HandleDeleteKey(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, 400, "invalid id")
			return
		}

		if err := services.DeleteAPIKey(db, id); err != nil {
			writeError(w, 500, "Failed to delete key")
			return
		}

		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleGetKeySettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, 400, "invalid id")
			return
		}

		settings := services.GetEffectiveRenderSettings(db, id, nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
	}
}

func HandleUpdateKeySettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, 400, "invalid id")
			return
		}

		var body services.APIKeySettings
		if err := decodeJSONBody(r, &body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}

		body.APIKeyID = id
		if err := services.UpsertAPIKeySettings(db, &body); err != nil {
			writeError(w, 500, "Failed to update settings")
			return
		}

		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleResetKeySettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, 400, "invalid id")
			return
		}

		if err := services.DeleteAPIKeySettings(db, id); err != nil {
			writeError(w, 500, "Failed to reset settings")
			return
		}

		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleSelfKeyInfo(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			writeError(w, 401, "Unauthorized")
			return
		}

		k, err := services.FindAPIKeyByID(db, apiUser.KeyID)
		if err != nil || k == nil {
			writeError(w, 404, "Not found")
			return
		}

		writeJSON(w, 200, map[string]interface{}{
			"name":       k.Name,
			"key_prefix": k.KeyPrefix,
		})
	}
}

func HandleSelfSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			writeError(w, 401, "Unauthorized")
			return
		}

		settings := services.GetEffectiveRenderSettings(db, apiUser.KeyID, nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
	}
}

func HandleUpdateSelfSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			writeError(w, 401, "Unauthorized")
			return
		}

		var body services.APIKeySettings
		if err := decodeJSONBody(r, &body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}

		body.APIKeyID = apiUser.KeyID
		if err := services.UpsertAPIKeySettings(db, &body); err != nil {
			writeError(w, 500, "Failed to update settings")
			return
		}

		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleResetSelfSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			writeError(w, 401, "Unauthorized")
			return
		}

		if err := services.DeleteAPIKeySettings(db, apiUser.KeyID); err != nil {
			writeError(w, 500, "Failed to reset settings")
			return
		}

		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
