package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"openposterdb/internal/errors"
	"openposterdb/internal/httpx"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

// FreeAPIKey is the public key (enabled in Settings); the router needs it to
// route its URLs to the API when the SPA is served.
const FreeAPIKey = "t0-free-rpdb"

const freeAPIKey = FreeAPIKey

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
	Kitsu     *services.KitsuClient
	AniList   *services.AniListClient
	Mappings  *services.IDMappings
	TVDB      *services.TVDBClient
	CDNHashes *services.HashRegistry
	// LastUsed records which keys serve images (admin "last used"); nil-safe.
	LastUsed KeyRecorder
}

// HandleImage serves the public image endpoints. deps is a per-request
// resolver that returns the current rating-provider client snapshot — callers
// MUST pass a resolver (not a snapshot value) so a TMDB/OMDB/etc key
// rotation in the admin UI reaches in-flight requests on the next call
// (without the resolver the handler closure captures one snapshot and never
// sees the new clients — bug fixed 2026-08-06).
func HandleImage(deps func() ImageDeps, isFreeAPIKeyEnabled FreeKeyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// HEAD is accepted so clients that probe a poster URL before using it
		// (e.g. AIOStreams' poster redirect API) get the same status as GET;
		// net/http drops the body on HEAD.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		d := deps()

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
		// Before any cache lookup: a render stored under an invalid id
		// (e.g. tvdb/movie-2) must not be served.
		if err := services.ValidateIDFormat(idTypeStr, idValue); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}

		// Parse query parameters
		query := parseImageQuery(r)

		// Validate API key and resolve settings
		if apiKey == freeAPIKey && !checkFreeKey(w, isFreeAPIKeyEnabled, "image") {
			return
		}

		var settings *services.RenderSettings
		// Cached lookups (services/settingscache.go): invalidated by every
		// settings/key write, so a cache hit needs no DB round-trip here. A
		// failed settings read is a 500, never a render with the defaults.
		if apiKey == freeAPIKey {
			s, err := services.GlobalRenderSettingsCachedErrCtx(r.Context(), d.DB)
			if err != nil {
				slog.Error("image: global settings read failed", "error", err)
				httpx.WriteError(w, 500, "Internal server error")
				return
			}
			settings = &s
		} else {
			keyHash := services.HashAPIKey(apiKey)
			k, err := services.FindAPIKeyByHashCachedCtx(r.Context(), d.DB, keyHash)
			if err != nil {
				slog.Error("image: api key lookup failed", "error", err)
				httpx.WriteAppError(w, err)
				return
			}
			if k == nil {
				httpx.WriteError(w, 401, "invalid api key")
				return
			}
			recordKey(d.LastUsed, k.ID)
			s, err := services.EffectiveRenderSettingsCachedErrCtx(r.Context(), d.DB, k.ID)
			if err != nil {
				slog.Error("image: key settings read failed", "api_key_id", k.ID, "error", err)
				httpx.WriteError(w, 500, "Internal server error")
				return
			}
			settings = &s
		}

		if query.ImageSize != nil {
			if _, err := services.ValidateImageSize(*query.ImageSize, kind); err != nil {
				httpx.WriteError(w, 400, err.Error())
				return
			}
		}

		// After the key and request checks, so an unknown key is a 401 and a
		// bad request a 400 whether or not provider keys are configured.
		if d.TMDB == nil {
			httpx.WriteError(w, 503, "TMDB API key not configured — image generation unavailable")
			return
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
		writeImage(w, r, bytes, contentType, directImageCacheControl)
	}
}

// directImageCacheControl is the lifetime of the direct image URL. It stays
// short: the URL doesn't change when the key's settings do, so a long
// max-age would keep old renders in clients and proxies. The ETag makes the
// re-check cheap (304, no body).
const directImageCacheControl = "public, max-age=3600, stale-while-revalidate=86400"

// cdnCacheControl is the lifetime of a /c/ URL, whose settings hash changes
// with the settings: max-age follows the title's age
// (services.ComputeCDNMaxAge: about a day for new releases, up to a year for
// titles whose ratings no longer move), stale-while-revalidate is 7× that.
func cdnCacheControl(releaseDate *string, cfg *ImageServeConfig) string {
	var minStale, maxAge uint64
	if cfg != nil {
		minStale, maxAge = cfg.RatingsMinStaleSecs, cfg.RatingsMaxAgeSecs
	}
	age := services.ComputeCDNMaxAge(releaseDate, minStale, maxAge)
	return fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", age, age*7)
}

// writeImage sends a rendered image with the given Cache-Control. The ETag
// is a hash of the bytes; a matching If-None-Match gets a 304.
func writeImage(w http.ResponseWriter, r *http.Request, bytes []byte, contentType, cacheControl string) {
	etag := imageETag(bytes)
	h := w.Header()
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", contentType)
	w.Write(bytes)
}

// imageETag is a strong ETag over the image bytes.
func imageETag(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// etagMatches implements If-None-Match: "*" or any listed tag (weak
// comparison, so W/"x" matches "x").
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}

// FreeKeyCheck reports whether the free API key is enabled; an error means
// the flag could not be read.
type FreeKeyCheck func() (bool, error)

// checkFreeKey answers a free-key request that may not proceed: 403 when the
// key is disabled, 500 when the flag could not be read. It reports whether
// the request may go on.
func checkFreeKey(w http.ResponseWriter, enabled FreeKeyCheck, route string) bool {
	ok, err := enabled()
	if err != nil {
		slog.Error(route+": free api key flag read failed", "error", err)
		httpx.WriteError(w, 500, "Internal server error")
		return false
	}
	if !ok {
		httpx.WriteError(w, 403, "free api key is disabled")
		return false
	}
	return true
}

// HandleIsValid answers /{apiKey}/isValid. HEAD is accepted like on the image
// routes (clients probe with it), and a valid key's use is recorded for the
// admin "last used" column like an image request.
func HandleIsValid(db *sql.DB, isFreeAPIKeyEnabled FreeKeyCheck, lastUsed KeyRecorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiKey := r.PathValue("apiKey")
		if apiKey == "" {
			httpx.WriteJSON(w, 400, map[string]interface{}{"error": "missing api key"})
			return
		}

		if apiKey == freeAPIKey {
			if !checkFreeKey(w, isFreeAPIKeyEnabled, "isValid") {
				return
			}
			httpx.WriteJSON(w, 200, map[string]interface{}{"valid": true})
			return
		}

		keyHash := services.HashAPIKey(apiKey)
		k, err := services.FindAPIKeyByHashCachedCtx(r.Context(), db, keyHash)
		if err != nil {
			slog.Error("isValid: api key lookup failed", "error", err)
			httpx.WriteAppError(w, err)
			return
		}
		if k == nil {
			httpx.WriteJSON(w, 401, map[string]interface{}{"error": "Invalid or missing API key"})
			return
		}
		recordKey(lastUsed, k.ID)

		httpx.WriteJSON(w, 200, map[string]interface{}{"valid": true})
	}
}

// serveParams builds the ServeImage parameters shared by the authed image
// route and the /c/ CDN route, so both render (and cache-key) identically.
func (d ImageDeps) serveParams(ctx context.Context, idType, idValue, kind string, settings *services.RenderSettings, ratingsLimit *int32, imageSize *string) image.ServeParams {
	return image.ServeParams{
		Context: ctx, DB: d.DB, TMDB: d.TMDB, OMDB: d.OMDB, MDBList: d.MDBList, Trakt: d.Trakt, Fanart: d.Fanart,
		Kitsu: d.Kitsu, AniList: d.AniList, Mappings: d.Mappings, TVDB: d.TVDB,
		IDType: idType, IDValue: idValue, Kind: kind,
		Settings: settings, RatingsLimit: ratingsLimit,
		CacheDir: d.Config.CacheDir, ExternalCacheOnly: d.Config.ExternalCacheOnly,
		RatingsMinStaleSecs: d.Config.RatingsMinStaleSecs, RatingsMaxAgeSecs: d.Config.RatingsMaxAgeSecs,
		ImageStaleSecs: d.Config.ImageStaleSecs, Quality: d.Config.ImageQuality,
		ImageSizeStr: imageSize, Caches: d.Config.Caches,
		Inflight: d.Config.Inflight,
	}
}

// writeServeError answers a failed image request. 4xx messages are passed
// through; 5xx details (upstream response bodies, panics) stay in the log and
// the client gets a generic message.
func writeServeError(w http.ResponseWriter, err error) {
	var appErr *errors.AppError
	if !stderrors.As(err, &appErr) || appErr.Status >= 500 {
		slog.Warn("image request failed", "error", err)
	}
	httpx.WriteAppError(w, err)
}
