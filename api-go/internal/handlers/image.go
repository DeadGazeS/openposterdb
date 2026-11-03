package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
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
	DB              *sql.DB
	Config          *ImageServeConfig
	TMDB            *services.TmdbClient
	OMDB            *services.OmdbClient
	MDBList         *services.MdblistClient
	Trakt           *services.TraktClient
	Fanart          *services.FanartClient
	Kitsu           *services.KitsuClient
	AniList         *services.AniListClient
	KitsuIMDbMapper *services.KitsuIMDbMapper
	CDNHashes       *services.HashRegistry
}

// HandleImage serves the public image endpoints. deps is a per-request
// resolver that returns the current rating-provider client snapshot — callers
// MUST pass a resolver (not a snapshot value) so a TMDB/OMDB/etc key
// rotation in the admin UI reaches in-flight requests on the next call
// (without the resolver the handler closure captures one snapshot and never
// sees the new clients — bug fixed 2026-08-06).
func HandleImage(deps func() ImageDeps, isFreeAPIKeyEnabled func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// HEAD is accepted so clients that probe a poster URL before using it
		// (e.g. AIOStreams' poster redirect API) get the same status as GET;
		// net/http drops the body on HEAD.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
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
		// Cached lookups (services/settingscache.go): invalidated by every
		// settings/key write, so a cache hit needs no DB round-trip here.
		if apiKey == freeAPIKey {
			s := services.GlobalRenderSettingsCachedCtx(r.Context(), d.DB)
			settings = &s
		} else {
			keyHash := services.HashAPIKey(apiKey)
			k, err := services.FindAPIKeyByHashCachedCtx(r.Context(), d.DB, keyHash)
			if err != nil || k == nil {
				httpx.WriteError(w, 401, "invalid api key")
				return
			}
			s := services.EffectiveRenderSettingsCachedCtx(r.Context(), d.DB, k.ID)
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
		// users. The image is rendered (or served from cache) here first and
		// its cache entry recorded: /c/ only serves what an authenticated
		// request produced, never rendering or calling upstreams itself.
		// Free-key requests skip the redirect (the free key is public and the
		// hash registry is small; we don't want to leak free settings).
		if d.Config.EnableCDNRedirects && apiKey != freeAPIKey && d.CDNHashes != nil {
			if hash := d.CDNHashes.Register(settings); hash != "" {
				var served services.CDNServed
				params := d.serveParams(r.Context(), idTypeStr, idValue, kind, settings, query.RatingsLimit, query.ImageSize)
				params.Served = &served
				if _, _, err := image.ServeImage(params); err != nil {
					writeServeError(w, err)
					return
				}
				size := ""
				if query.ImageSize != nil {
					size = *query.ImageSize
				}
				d.CDNHashes.RecordServed(services.CDNServedKey(hash, idTypeStr, kind, idValue, size), served)

				ext := ".jpg"
				if kind == "logo" {
					ext = ".png"
				}
				target := "/c/" + hash + "/" + idTypeStr + "/" + imageKind + "/" + idValue + ext
				// imageSize is a request parameter, not a render setting, so
				// it is not part of the hash — carry it on the redirect.
				if size != "" {
					target += "?imageSize=" + url.QueryEscape(size)
				}
				w.Header().Set("Location", target)
				w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=3600")
				w.WriteHeader(http.StatusFound)
				return
			}
		}

		bytes, contentType, err := image.ServeImage(d.serveParams(r.Context(), idTypeStr, idValue, kind, settings, query.RatingsLimit, query.ImageSize))
		if err != nil {
			writeServeError(w, err)
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
		k, err := services.FindAPIKeyByHashCachedCtx(r.Context(), db, keyHash)
		if err != nil || k == nil {
			httpx.WriteJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
			return
		}

		httpx.WriteJSON(w, 200, map[string]interface{}{"valid": true})
	}
}

// serveParams builds the ServeImage parameters shared by the authed image
// route and the /c/ CDN route, so both render (and cache-key) identically.
func (d ImageDeps) serveParams(ctx context.Context, idType, idValue, kind string, settings *services.RenderSettings, ratingsLimit *int32, imageSize *string) image.ServeParams {
	return image.ServeParams{
		Context: ctx, DB: d.DB, TMDB: d.TMDB, OMDB: d.OMDB, MDBList: d.MDBList, Trakt: d.Trakt, Fanart: d.Fanart,
		Kitsu: d.Kitsu, AniList: d.AniList, KitsuIMDbMapper: d.KitsuIMDbMapper,
		IDType: idType, IDValue: idValue, Kind: kind,
		Settings: settings, RatingsLimit: ratingsLimit,
		CacheDir: d.Config.CacheDir, ExternalCacheOnly: d.Config.ExternalCacheOnly,
		RatingsMinStaleSecs: d.Config.RatingsMinStaleSecs, RatingsMaxAgeSecs: d.Config.RatingsMaxAgeSecs,
		ImageStaleSecs: d.Config.ImageStaleSecs, Quality: d.Config.ImageQuality,
		ImageSizeStr: imageSize, Caches: d.Config.Caches,
		Inflight: d.Config.Inflight,
	}
}

func writeServeError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		httpx.WriteError(w, appErr.Status, appErr.Message)
		return
	}
	httpx.WriteError(w, 500, err.Error())
}
