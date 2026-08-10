package handlers

import (
	"database/sql"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// HandleExportSettings returns a JSON settings backup. Query params
// include_service_keys and include_api_keys control which secrets are included.
func HandleExportSettings(db *sql.DB, keys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		q := r.URL.Query()
		includeServiceKeys := q.Get("include_service_keys") == "1" || q.Get("include_service_keys") == "true"
		includeAPIKeys := q.Get("include_api_keys") == "1" || q.Get("include_api_keys") == "true"

		payload, err := services.BuildExportPayloadCtx(r.Context(), db, keys, includeServiceKeys, includeAPIKeys)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to export settings")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="openposterdb-settings.json"`)
		httpx.WriteJSON(w, 200, payload)
	}
}

// HandleImportSettings restores a settings backup produced by
// HandleExportSettings. API keys that don't exist yet are recreated with a new
// value, which is returned in the response.
func HandleImportSettings(db *sql.DB, keys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		var payload services.ExportPayload
		if err := httpx.DecodeJSON(r, &payload); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}
		if payload.Kind != "openposterdb/settings" {
			httpx.WriteError(w, 400, "not a settings export file")
			return
		}

		result, err := services.ApplyImportPayloadCtx(r.Context(), db, keys, &payload)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to import settings")
			return
		}

		httpx.WriteJSON(w, 200, result)
	}
}
