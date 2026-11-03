package handlers

import (
	"net/http"
	"strings"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// CDNLookup is satisfied by services.HashRegistry; defined as an interface so
// the handler doesn't depend on the concrete type (and tests can stub it).
type CDNLookup interface {
	LookupServed(key string) (services.CDNServed, bool)
}

// HandleCDNImage serves the content-addressed /c/{hash}/{rest...} route used
// behind CDN edges. The hash is registered by HandleImage when it issues a
// 302 redirect.
//
// It serves only the cache entry an authenticated request recorded for the
// URL (HandleImage renders before redirecting); anything else is a 404 the
// edge must not cache.
//
// deps is a per-request resolver (matching HandleImage) so that updates to
// ImageServeConfig (CacheDir / ImageStaleSecs) are visible to in-flight
// requests without restarting the server.
func HandleCDNImage(deps func() ImageDeps, registry CDNLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// HEAD is accepted because HandleImage's CDN redirect keeps the
		// method, so a HEAD probe on the authed URL lands here.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		d := deps()

		hash := r.PathValue("hash")
		rest := r.PathValue("rest")

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

		// Serve only what an authenticated request rendered for this URL
		// (recorded by HandleImage before its redirect): no id resolution,
		// ratings fetch or render here, so a settings hash grants nothing
		// beyond already-produced images.
		notFound := func() {
			// Don't let the edge cache the miss; the authed URL re-renders.
			w.Header().Set("Cache-Control", "no-store")
			httpx.WriteError(w, http.StatusNotFound, "image not found — request the original image URL")
		}
		served, ok := registry.LookupServed(services.CDNServedKey(hash, idTypeStr, kind, idValue, r.URL.Query().Get("imageSize")))
		if !ok || d.Config == nil {
			notFound()
			return
		}
		var bytes []byte
		if c := d.Config.Caches; c != nil && c.ImageMem != nil {
			if v, ok := c.ImageMem.Get(served.CacheKey); ok {
				bytes = v.([]byte)
			}
		}
		if bytes == nil && !d.Config.ExternalCacheOnly {
			if entry, err := services.ReadCache(served.CachePath, 0); err == nil {
				bytes = entry.Bytes
			}
		}
		if bytes == nil {
			notFound()
			return
		}

		// CDN-cached for a long time; the settings hash is the cache key, so
		// the CDN edge sees at most one entry per hash per image.
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		w.Header().Set("Content-Type", served.ContentType)
		w.Write(bytes)
	}
}
