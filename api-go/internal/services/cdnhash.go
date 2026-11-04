package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// SettingsHash returns a stable 32-char hex digest of the effective render
// settings. It is kind-independent on purpose: a poster fetch and a logo
// fetch with identical settings produce the same hash, so a CDN can deduplicate
// entries across kinds and images.
//
// JSON encoding with Go's default encoder is deterministic for the field order
// of the RenderSettings struct (and its nested ImageLayout), which is what we
// rely on here.
func SettingsHash(s *RenderSettings) string {
	if s == nil {
		return ""
	}
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16]) // 32 hex chars
}

// HashRegistry is the /c/ index: which cache entry a /c/{hash}/... URL
// serves. HandleImage records it when it issues a redirect; HandleCDNImage
// only reads it. (It used to also keep hash -> settings for /c/ to render
// from; /c/ became index-only, so nothing read that map, yet every authed
// request wrote to it under a global lock.)
//
// The index is process-local: after a restart the first authenticated
// request repopulates an entry (the CDN edge cache survives the restart).
type HashRegistry struct {
	// served maps a /c/ URL (CDNServedKey) to the cache entry the
	// authenticated route rendered for it. /c/ serves only from this index —
	// it never resolves ids, fetches ratings or renders, so a settings hash
	// (deterministic, computable for the defaults) can't be used to make
	// the server do upstream work without an API key.
	served *MemCache
}

// CDNServed is the cache entry a /c/ URL maps to.
type CDNServed struct {
	CacheKey    string
	CachePath   string
	ContentType string
}

// CDNServedKey identifies one /c/ URL: settings hash, id, kind and the
// ?imageSize the redirect carries (not part of the settings hash).
func CDNServedKey(hash, idType, kind, idValue, imageSize string) string {
	return hash + "|" + idType + "|" + kind + "|" + idValue + "|" + imageSize
}

// RecordServed remembers which cache entry a /c/ URL serves. Nil-safe.
func (r *HashRegistry) RecordServed(key string, s CDNServed) {
	if r == nil || r.served == nil {
		return
	}
	r.served.Set(key, s, 1)
}

// LookupServed returns the cache entry recorded for a /c/ URL.
func (r *HashRegistry) LookupServed(key string) (CDNServed, bool) {
	if r == nil || r.served == nil {
		return CDNServed{}, false
	}
	v, ok := r.served.Get(key)
	if !ok {
		return CDNServed{}, false
	}
	return v.(CDNServed), true
}

// NewHashRegistry creates an empty /c/ index.
func NewHashRegistry() *HashRegistry {
	return &HashRegistry{
		// A CDN re-fetches /c/ URLs after its own cache expires (a day), so
		// entries live 48 h. Bounded.
		served: NewMemCache(0, 100_000, 48*time.Hour, 0),
	}
}

// Register returns the settings hash for a redirect ("" when r or s is nil,
// i.e. CDN redirects are off).
func (r *HashRegistry) Register(s *RenderSettings) string {
	if r == nil || s == nil {
		return ""
	}
	return SettingsHash(s)
}

// SweepExpired drops expired index entries; call from a janitor goroutine.
func (r *HashRegistry) SweepExpired() int {
	if r == nil || r.served == nil {
		return 0
	}
	return r.served.SweepExpired()
}
