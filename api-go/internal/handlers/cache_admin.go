package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/httpx"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

func HandleImageFile(db *sql.DB, cacheDir string, imageType, idType, idValue string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		ext := "jpg"
		contentType := "image/jpeg"
		if imageType == "l" {
			ext = "png"
			contentType = "image/png"
		}

		subdir := services.ImageSubdir(imageType)
		if subdir == "" {
			httpx.WriteError(w, 400, "invalid image type")
			return
		}

		fileBase := strings.ReplaceAll(idValue, ":", "_")
		path, err := services.TypedCachePath(cacheDir, subdir, idType, fileBase, ext)
		if err != nil {
			httpx.WriteError(w, 400, "invalid path")
			return
		}

		entry, err := services.ReadCache(path, 0)
		if err != nil {
			httpx.WriteError(w, 404, "image not found")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(entry.Bytes)
	}
}

func HandleFetchImage(deps ImageDeps, imageType, idType, idValue string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		if err := services.ValidateIDValue(idValue); err != nil {
			httpx.WriteError(w, 400, "invalid id value")
			return
		}

		if deps.TMDB == nil {
			httpx.WriteError(w, 503, "TMDB API key not configured — image generation unavailable")
			return
		}

		kind, ok := kindFromType(imageType)
		if !ok {
			httpx.WriteError(w, 400, "invalid image type")
			return
		}

		slog.Debug("admin fetch requested", "kind", kind, "id", idType+"/"+idValue)

		globals, err := services.GetGlobalSettingsCtx(r.Context(), deps.DB)
		if err != nil {
			slog.Warn("admin fetch GetGlobalSettings failed", "error", err)
		}
		settings := services.ParseGlobalRenderSettings(globals)

		bytes, contentType, err := image.ServeImage(image.ServeParams{
			Context: r.Context(), DB: deps.DB, TMDB: deps.TMDB, OMDB: deps.OMDB, MDBList: deps.MDBList, Trakt: deps.Trakt, Fanart: deps.Fanart,
			Kitsu: deps.Kitsu, AniList: deps.AniList, KitsuIMDbMapper: deps.KitsuIMDbMapper,
			IDType: idType, IDValue: idValue, Kind: kind,
			Settings: &settings,
			CacheDir: deps.Config.CacheDir, ExternalCacheOnly: deps.Config.ExternalCacheOnly,
			RatingsMinStaleSecs: deps.Config.RatingsMinStaleSecs, RatingsMaxAgeSecs: deps.Config.RatingsMaxAgeSecs,
			ImageStaleSecs: deps.Config.ImageStaleSecs, Quality: deps.Config.ImageQuality,
			Caches: deps.Config.Caches,
		})
		if err != nil {
			if appErr, ok := err.(*apperr.AppError); ok {
				httpx.WriteError(w, appErr.Status, appErr.Message)
			} else {
				httpx.WriteAppError(w, err)
			}
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600, stale-while-revalidate=86400")
		w.Write(bytes)
	}
}

func kindFromType(imageType string) (string, bool) {
	switch imageType {
	case "p":
		return "poster", true
	case "l":
		return "logo", true
	case "b":
		return "backdrop", true
	case "e":
		return "episode", true
	}
	return "", false
}

// parsePreviewColors reads the `colors` query parameter (JSON object of
// per-source color overrides) used for live previews of unsaved changes.
