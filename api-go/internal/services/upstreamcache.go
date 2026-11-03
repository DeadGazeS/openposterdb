package services

import "sync/atomic"

// Per-client upstream caches (TMDB/fanart image lists, Kitsu/AniList
// lookups) tag every entry with a process-wide generation. Clients are
// recreated on key rotation, so instead of tracking every live cache,
// ClearUpstreamCaches bumps the generation and all older entries miss.

var upstreamGen atomic.Uint64

// ClearUpstreamCaches invalidates every per-client upstream cache (admin
// "Clear cache").
func ClearUpstreamCaches() { upstreamGen.Add(1) }

type upstreamEntry struct {
	v   any
	gen uint64
}

func upstreamGet(c *MemCache, key string) (any, bool) {
	v, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	e, ok := v.(upstreamEntry)
	if !ok || e.gen != upstreamGen.Load() {
		c.Delete(key)
		return nil, false
	}
	return e.v, true
}

func upstreamSet(c *MemCache, key string, v any) {
	c.Set(key, upstreamEntry{v: v, gen: upstreamGen.Load()}, 1)
}
