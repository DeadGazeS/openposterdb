package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

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

		globals, err := services.GetGlobalSettingsCtx(r.Context(), db)
		if err != nil {
			slog.Warn("free-key GetGlobalSettings failed; falling back to defaults", "error", err)
		}
		var settings services.RenderSettings
		if len(globals) > 0 {
			settings = services.ParseGlobalRenderSettings(globals)
		} else {
			settings = services.DefaultRenderSettings()
		}

		httpx.WriteJSON(w, 200, services.SettingsResponseMap(&settings))
	}
}
