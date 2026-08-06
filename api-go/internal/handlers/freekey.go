package handlers

import (
	"database/sql"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

type FreeKeySettingsResponse = map[string]any

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

		httpx.WriteJSON(w, 200, services.SettingsResponseMap(&settings))
	}
}
