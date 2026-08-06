package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"openposterdb/internal/errors"
	"openposterdb/internal/httpx"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

const freeAPIKey = "t0-free-rpdb"

func marshalLayoutResponse(l services.ImageLayout) string {
	s, err := services.MarshalLayout(&l)
	if err != nil {
		return ""
	}
	return s
}

// ImageServeConfig carries the server-level settings needed by the image
// generation pipeline. Built from config.Config by the router.
type ImageServeConfig struct {
	CacheDir            string
	ExternalCacheOnly   bool
	EnableCDNRedirects  bool
	RatingsMinStaleSecs uint64
	RatingsMaxAgeSecs   uint64
	ImageStaleSecs      uint64
	ImageQuality        uint8
	Caches              *services.MemCacheSet
	Inflight            *image.InflightSet
}

// ImageDeps bundles the dependencies shared by the image-serving handlers.
type ImageDeps struct {
	DB        *sql.DB
	Config    *ImageServeConfig
	TMDB      *services.TmdbClient
	OMDB      *services.OmdbClient
	MDBList   *services.MdblistClient
	Trakt     *services.TraktClient
	Fanart    *services.FanartClient
	CDNHashes *services.HashRegistry
}

// ImageQuery represents all query parameters for image endpoints.
func resolveSettings(db *sql.DB, apiKey string, isFreeAPIKeyEnabled func() bool, globalsCache *services.RenderSettings) (*services.RenderSettings, error) {
	if apiKey == freeAPIKey {
		return resolveFreeSettings(db, isFreeAPIKeyEnabled, globalsCache)
	}

	keyHash := services.HashAPIKey(apiKey)
	k, err := services.FindAPIKeyByHash(db, keyHash)
	if err != nil || k == nil {
		return nil, err
	}

	s := services.GetEffectiveRenderSettings(db, k.ID, globalsCache)
	return &s, nil
}

// HandleImage serves the public image endpoints. deps is a per-request
// resolver that returns the current rating-provider client snapshot — callers
// MUST pass a resolver (not a snapshot value) so a TMDB/OMDB/etc key
// rotation in the admin UI reaches in-flight requests on the next call
// (without the resolver the handler closure captures one snapshot and never
// sees the new clients — bug fixed 2026-08-06).
func HandleImage(deps func() ImageDeps, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		d := deps()
		if d.TMDB == nil {
			httpx.WriteError(w, 503, "TMDB API key not configured — image generation unavailable")
			return
		}

		apiKey := r.PathValue("apiKey")
		rest := r.PathValue("rest")

		if apiKey == "" || rest == "" {
			httpx.WriteError(w, 400, "missing path parameters")
			return
		}

		// Parse rest into idType, image-kind, and idValue
		// rest format: {idType}/poster-default/{idValue}.jpg
		//           or: {idType}/logo-default/{idValue}.png
		//           or: {idType}/backdrop-default/{idValue}.jpg
		//           or: {idType}/episode-default/{idValue}.jpg
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 3 {
			httpx.WriteError(w, 400, "invalid path")
			return
		}
		idTypeStr := parts[0]
		imageKind := parts[1]
		idValue := parts[2]

		var kind string
		switch imageKind {
		case "poster-default":
			kind = "poster"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		case "logo-default":
			kind = "logo"
			idValue = strings.TrimSuffix(idValue, ".png")
		case "backdrop-default":
			kind = "backdrop"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		case "episode-default":
			kind = "episode"
			idValue = strings.TrimSuffix(idValue, ".jpg")
		default:
			httpx.WriteError(w, 400, "invalid image path")
			return
		}

		if err := services.ValidateIDValue(idValue); err != nil {
			httpx.WriteError(w, 400, "invalid id value")
			return
		}

		// Parse query parameters
		query := parseImageQuery(r)

		// Validate API key and resolve settings
		if apiKey == freeAPIKey && !isFreeAPIKeyEnabled() {
			httpx.WriteError(w, 403, "free api key is disabled")
			return
		}

		var settings *services.RenderSettings
		if apiKey == freeAPIKey {
			globals, _ := services.GetGlobalSettings(d.DB)
			s := services.ParseGlobalRenderSettings(globals)
			settings = &s
		} else {
			keyHash := services.HashAPIKey(apiKey)
			k, err := services.FindAPIKeyByHash(d.DB, keyHash)
			if err != nil || k == nil {
				httpx.WriteError(w, 401, "invalid api key")
				return
			}
			s := services.GetEffectiveRenderSettings(d.DB, k.ID, nil)
			settings = &s
		}

		// Apply lang override from query
		if query.Lang != nil && *query.Lang != "" {
			if err := services.ValidateLang(*query.Lang); err == nil {
				settings.Lang = *query.Lang
			}
		}

		// Apply query parameter overrides
		settings = applyQueryOverrides(settings, query, kind)

		// CDN redirect: when enabled, register the effective settings under a
		// hash and 302 to /c/{hash}/... so a CDN edge can deduplicate across
		// users. Free-key requests skip the redirect (the free key is public
		// and the hash registry is small; we don't want to leak free settings).
		if d.Config.EnableCDNRedirects && apiKey != freeAPIKey && d.CDNHashes != nil {
			if hash := d.CDNHashes.Register(settings); hash != "" {
				ext := ".jpg"
				if kind == "logo" {
					ext = ".png"
				}
				target := "/c/" + hash + "/" + idTypeStr + "/" + imageKind + "/" + idValue + ext
				w.Header().Set("Location", target)
				w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=3600")
				w.WriteHeader(http.StatusFound)
				return
			}
		}

		bytes, contentType, err := image.ServeImage(image.ServeParams{
			DB: d.DB, TMDB: d.TMDB, OMDB: d.OMDB, MDBList: d.MDBList, Trakt: d.Trakt, Fanart: d.Fanart,
			IDType: idTypeStr, IDValue: idValue, Kind: kind,
			Settings: settings, RatingsLimit: query.RatingsLimit,
			CacheDir: d.Config.CacheDir, ExternalCacheOnly: d.Config.ExternalCacheOnly,
			RatingsMinStaleSecs: d.Config.RatingsMinStaleSecs, RatingsMaxAgeSecs: d.Config.RatingsMaxAgeSecs,
			ImageStaleSecs: d.Config.ImageStaleSecs, Quality: d.Config.ImageQuality,
			ImageSizeStr: query.ImageSize, Caches: d.Config.Caches,
			Inflight: d.Config.Inflight,
		})
		if err != nil {
			if appErr, ok := err.(*errors.AppError); ok {
				httpx.WriteError(w, appErr.Status, appErr.Message)
			} else {
				httpx.WriteError(w, 500, err.Error())
			}
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600, stale-while-revalidate=86400")
		w.Write(bytes)
	}
}

func HandleIsValid(db *sql.DB, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiKey := r.PathValue("apiKey")
		if apiKey == "" {
			httpx.WriteJSON(w, 400, map[string]interface{}{"error": "missing api key"})
			return
		}

		if apiKey == freeAPIKey {
			if !isFreeAPIKeyEnabled() {
				httpx.WriteJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
				return
			}
			httpx.WriteJSON(w, 200, map[string]interface{}{"valid": true})
			return
		}

		keyHash := services.HashAPIKey(apiKey)
		_, err := services.FindAPIKeyByHash(db, keyHash)
		if err != nil {
			httpx.WriteJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
			return
		}

		httpx.WriteJSON(w, 200, map[string]interface{}{"valid": true})
	}
}
