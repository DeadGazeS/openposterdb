package services

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Settings cache for the public image path, which used to read the API key,
// its per-key settings row and every global setting (plus several JSON
// decodes) from SQLite on every request — cache hits included.
//
// Invalidation is a per-database generation counter bumped by every
// services-layer write to api_keys, api_key_settings or global_settings
// (bumpSettingsGen). An entry is only served if it was computed at the
// current generation, so admin changes take effect on the next request. The
// TTL only bounds staleness for writes that bypass the services layer.
//
// Each entry is tagged with the generation read *before* its DB query, so a
// write racing the query leaves the entry already stale.

const settingsCacheTTL = 60 * time.Second

type settingsCache struct {
	gen  atomic.Uint64
	keys *MemCache // key hash -> keyEntry
	eff  *MemCache // api key id -> effEntry

	mu      sync.Mutex
	globals *globalsEntry
}

type keyEntry struct {
	key *APIKey // nil = no such key (cached so random keys don't hit the DB)
	gen uint64
}

type effEntry struct {
	s   RenderSettings
	gen uint64
}

type globalsEntry struct {
	raw    map[string]string
	parsed RenderSettings
	gen    uint64
	at     time.Time
}

var settingsCaches sync.Map // *sql.DB -> *settingsCache

func settingsCacheFor(db *sql.DB) *settingsCache {
	if c, ok := settingsCaches.Load(db); ok {
		return c.(*settingsCache)
	}
	c, _ := settingsCaches.LoadOrStore(db, &settingsCache{
		keys: NewMemCache(0, 10_000, settingsCacheTTL, 0),
		eff:  NewMemCache(0, 10_000, settingsCacheTTL, 0),
	})
	return c.(*settingsCache)
}

// bumpSettingsGen invalidates every cached settings entry for db. Called by
// all services-layer writers of api_keys / api_key_settings / global_settings.
func bumpSettingsGen(db *sql.DB) {
	if c, ok := settingsCaches.Load(db); ok {
		c.(*settingsCache).gen.Add(1)
	}
}

// FindAPIKeyByHashCachedCtx is FindAPIKeyByHashCtx through the cache. Unknown
// keys are cached too (as nil); read errors never are. The result is a copy.
func FindAPIKeyByHashCachedCtx(ctx context.Context, db *sql.DB, keyHash string) (*APIKey, error) {
	c := settingsCacheFor(db)
	gen := c.gen.Load()
	if v, ok := c.keys.Get(keyHash); ok {
		if e := v.(keyEntry); e.gen == gen {
			return copyAPIKey(e.key), nil
		}
	}
	k, err := FindAPIKeyByHashCtx(ctx, db, keyHash)
	if err != nil {
		return nil, err
	}
	c.keys.Set(keyHash, keyEntry{key: k, gen: gen}, 1)
	return copyAPIKey(k), nil
}

func copyAPIKey(k *APIKey) *APIKey {
	if k == nil {
		return nil
	}
	cp := *k
	return &cp
}

// globalsCached returns the cached raw + parsed global settings, reloading
// them when stale. ok is false when the read failed (nothing is cached then;
// raw is nil and parsed holds the defaults).
func (c *settingsCache) globalsCached(ctx context.Context, db *sql.DB) (raw map[string]string, parsed RenderSettings, ok bool) {
	gen := c.gen.Load()
	c.mu.Lock()
	g := c.globals
	c.mu.Unlock()
	if g != nil && g.gen == gen && time.Since(g.at) < settingsCacheTTL {
		return g.raw, g.parsed, true
	}
	m, err := GetGlobalSettingsCtx(ctx, db)
	if err != nil {
		return nil, DefaultRenderSettings(), false
	}
	g = &globalsEntry{raw: m, parsed: ParseGlobalRenderSettings(m), gen: gen, at: time.Now()}
	c.mu.Lock()
	if c.globals == nil || c.globals.gen <= gen {
		c.globals = g
	}
	c.mu.Unlock()
	return g.raw, g.parsed, true
}

// GlobalRenderSettingsCachedCtx returns the parsed global render settings
// (defaults when the read fails). The result is a value copy; its Colors map
// is shared and must be treated as read-only (as everywhere else).
func GlobalRenderSettingsCachedCtx(ctx context.Context, db *sql.DB) RenderSettings {
	_, parsed, _ := settingsCacheFor(db).globalsCached(ctx, db)
	return parsed
}

// GlobalSettingCachedCtx returns one raw global setting ("" when unset or on
// a read error).
func GlobalSettingCachedCtx(ctx context.Context, db *sql.DB, key string) string {
	raw, _, _ := settingsCacheFor(db).globalsCached(ctx, db)
	return raw[key]
}

// EffectiveRenderSettingsCachedCtx is GetEffectiveRenderSettingsCtx through
// the cache. A result built while the per-key or global read failed is
// returned but not cached.
func EffectiveRenderSettingsCachedCtx(ctx context.Context, db *sql.DB, apiKeyID int64) RenderSettings {
	c := settingsCacheFor(db)
	gen := c.gen.Load()
	key := strconv.FormatInt(apiKeyID, 10)
	if v, ok := c.eff.Get(key); ok {
		if e := v.(effEntry); e.gen == gen {
			return e.s
		}
	}
	_, globals, globalsOK := c.globalsCached(ctx, db)
	perKey, err := GetAPIKeySettingsCtx(ctx, db, apiKeyID)
	s := effectiveFromPerKey(ctx, db, apiKeyID, perKey, err, &globals)
	if err == nil && globalsOK {
		c.eff.Set(key, effEntry{s: s, gen: gen}, 1)
	}
	return s
}
