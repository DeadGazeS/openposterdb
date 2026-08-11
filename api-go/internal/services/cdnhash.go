package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
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

// HashRegistry maps a settings hash to the most recently registered render
// settings. It is populated by HandleImage when a redirect is emitted, and
// read by HandleCDNImage when a /c/{hash}/... request arrives. Entries expire
// after `ttl`; a background janitor sweeps them up.
//
// The registry is process-local — across restarts, entries are lost (the
// first authenticated request after a restart repopulates the hash, and the
// CDN edge cache survives the restart).
type HashRegistry struct {
	ttl time.Duration

	mu    sync.RWMutex
	store map[string]hashEntry
}

type hashEntry struct {
	settings *RenderSettings
	expires  time.Time
}

// NewHashRegistry creates a registry with the given TTL per entry. ttl=0
// disables expiration (entries live forever).
func NewHashRegistry(ttl time.Duration) *HashRegistry {
	return &HashRegistry{
		ttl:   ttl,
		store: make(map[string]hashEntry),
	}
}

// Register records the settings under their hash and returns the hash.
func (r *HashRegistry) Register(s *RenderSettings) string {
	if r == nil || s == nil {
		return ""
	}
	h := SettingsHash(s)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[h] = hashEntry{settings: s, expires: time.Now().Add(r.ttl)}
	return h
}

// Lookup returns the settings for the given hash. The second return is false
// on miss (unknown hash or expired entry).
func (r *HashRegistry) Lookup(hash string) (*RenderSettings, bool) {
	if r == nil || hash == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.store[hash]
	if !ok {
		return nil, false
	}
	if r.ttl > 0 && time.Now().After(e.expires) {
		return nil, false
	}
	return e.settings, true
}

// Len returns the current number of live entries (test/diagnostic helper).
func (r *HashRegistry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.store)
}

// SweepExpired removes all expired entries; call from a janitor goroutine.
func (r *HashRegistry) SweepExpired() int {
	if r == nil || r.ttl <= 0 {
		return 0
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for h, e := range r.store {
		if now.After(e.expires) {
			delete(r.store, h)
			removed++
		}
	}
	return removed
}
