package handlers

import (
	"net/http"
	"strings"
	"time"

	"openposterdb/internal/httpx"
	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"
)

// CDNLookup is satisfied by services.HashRegistry; defined as an interface so
// the handler doesn't depend on the concrete type (and tests can stub it).
type CDNLookup interface {
	Lookup(hash string) (*services.RenderSettings, bool)
}

// HandleCDNImage serves the content-addressed /c/{hash}/{rest...} route used
// behind CDN edges. The hash is registered by HandleImage when it issues a
// 302 redirect.
//
// The handler is read-only — it looks up the hash in the registry, computes
// the cache key from the effective settings, and serves the cached bytes if
// present. If the hash is unknown, the cache key doesn't exist, or the
// cached file is missing, the request gets a 404 — the edge should retry the
// original authenticated URL so HandleImage re-registers the hash and
// re-populates the cache.
func HandleCDNImage(deps ImageDeps, registry CDNLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		hash := r.PathValue("hash")
		rest := r.PathValue("rest")

		settings, ok := registry.Lookup(hash)
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "settings hash not found or expired")
			return
		}

		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 3 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid path")
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
			httpx.WriteError(w, http.StatusBadRequest, "invalid image path")
			return
		}

		if err := services.ValidateIDValue(idValue); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid id value")
			return
		}

		// Compute the cache key the original authenticated request would have
		// written: idType + idValue + variant + suffix. lang override is
		// honoured so a /c/.../?lang=de hit still finds the right cache entry
		// when the authed URL used lang=en.
		query := r.URL.Query()
		s := *settings
		if lang := query.Get("lang"); lang != "" {
			if err := services.ValidateLang(lang); err == nil {
				s.Lang = lang
			}
		}
		if lang := query.Get("lang"); lang != "" {
			if err := services.ValidateLang(lang); err == nil {
				s.Lang = lang
			}
		}
		suffix := services.SettingsCacheSuffix(&s, kind, nil)
		variant := ""
		cacheValue := idValue + variant + suffix
		cacheKey := idTypeStr + "/" + cacheValue
		cachePath, err := services.TypedCachePath(deps.Config.CacheDir, services.ImageSubdir(kind), idTypeStr, cacheValue, services.ImageExt(kind))
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		entry, err := services.ReadCache(cachePath, deps.Config.ImageStaleSecs)
		if err != nil || entry.IsStale {
			httpx.WriteError(w, http.StatusNotFound, "cached image not found")
			return
		}
		if _, err := deps.DB.Exec(`UPDATE image_meta SET updated_at = ? WHERE cache_key = ?`, time.Now().Unix(), cacheKey); err != nil {
			// best-effort; cache hit is unaffected
			_ = err
		}

		// CDN-cached for a long time; the settings hash is the cache key, so
		// the CDN edge sees at most one entry per hash per image.
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		w.Header().Set("Content-Type", appimg.ImageContentType(kind))
		w.Write(entry.Bytes)
	}
}
