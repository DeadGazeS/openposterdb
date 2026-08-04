package services

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type memEntry struct {
	key      string
	value    any
	weight   int64
	inserted time.Time
	lastUsed time.Time
	elem     *list.Element
}

// MemCache is a thread-safe in-memory cache with weight-based capacity, TTL and
// idle-TTL eviction — the Go counterpart of the Rust implementation's moka
// caches. A zero maxBytes/maxEntries means "unlimited" for that dimension.
type MemCache struct {
	mu         sync.Mutex
	maxBytes   int64
	maxEntries int
	ttl        time.Duration
	idleTTL    time.Duration
	entries    map[string]*memEntry
	lru        *list.List // front = most recently used
	totalBytes int64
}

// NewMemCache creates a cache. ttl is the entry lifetime from insertion;
// idleTTL additionally expires entries not accessed for that long (0 disables
// either). Entries are evicted least-recently-used once capacity is exceeded.
func NewMemCache(maxBytes int64, maxEntries int, ttl, idleTTL time.Duration) *MemCache {
	return &MemCache{
		maxBytes:   maxBytes,
		maxEntries: maxEntries,
		ttl:        ttl,
		idleTTL:    idleTTL,
		entries:    make(map[string]*memEntry),
		lru:        list.New(),
	}
}

// Get returns the cached value, refreshing its idle timer. Expired entries are
// removed on access.
func (c *MemCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	now := time.Now()
	if (c.ttl > 0 && now.Sub(e.inserted) > c.ttl) || (c.idleTTL > 0 && now.Sub(e.lastUsed) > c.idleTTL) {
		c.remove(e)
		return nil, false
	}
	e.lastUsed = now
	c.lru.MoveToFront(e.elem)
	return e.value, true
}

// Set inserts or replaces an entry with the given weight (typically bytes for
// image data), then evicts expired and least-recently-used entries until within
// capacity.
func (c *MemCache) Set(key string, value any, weight int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if e, ok := c.entries[key]; ok {
		c.totalBytes += weight - e.weight
		e.value = value
		e.weight = weight
		e.inserted = now
		e.lastUsed = now
		c.lru.MoveToFront(e.elem)
	} else {
		e := &memEntry{key: key, value: value, weight: weight, inserted: now, lastUsed: now}
		e.elem = c.lru.PushFront(e)
		c.entries[key] = e
		c.totalBytes += weight
	}
	c.evict(now)
}

func (c *MemCache) evict(now time.Time) {
	// Expired sweep.
	for _, e := range c.entries {
		if (c.ttl > 0 && now.Sub(e.inserted) > c.ttl) || (c.idleTTL > 0 && now.Sub(e.lastUsed) > c.idleTTL) {
			c.remove(e)
		}
	}
	// Capacity eviction: least-recently-used from the back.
	for (c.maxBytes > 0 && c.totalBytes > c.maxBytes) || (c.maxEntries > 0 && len(c.entries) > c.maxEntries) {
		back := c.lru.Back()
		if back == nil {
			break
		}
		c.remove(back.Value.(*memEntry))
	}
}

func (c *MemCache) remove(e *memEntry) {
	c.lru.Remove(e.elem)
	delete(c.entries, e.key)
	c.totalBytes -= e.weight
}

// Delete removes one entry.
func (c *MemCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.remove(e)
	}
}

// DeletePrefix removes every entry whose key starts with prefix (used by
// per-title / per-kind cache purges).
func (c *MemCache) DeletePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if strings.HasPrefix(k, prefix) {
			c.remove(e)
		}
	}
}

// Clear removes all entries.
func (c *MemCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*memEntry)
	c.lru.Init()
	c.totalBytes = 0
}

// Len returns the number of cached entries.
func (c *MemCache) Len() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.entries))
}

// WeightedBytes returns the total weight of all entries (bytes for image data).
func (c *MemCache) WeightedBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalBytes
}

// MemCacheSet bundles the three in-memory caches shared across the process:
// rendered images, ID resolutions, and fetched ratings. All fields may be nil
// (caching disabled).
type MemCacheSet struct {
	ImageMem *MemCache
	IDs      *MemCache
	Ratings  *MemCache
}
