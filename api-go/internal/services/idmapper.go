package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
)

// theBeastLTKitsuIMDbURL is the canonical Kitsu→IMDb cross-reference used by
// aiometadata (https://github.com/TheBeastLT/stremio-kitsu-anime). We only
// fetch this when the user opts into the "find IMDB equivalent when Kitsu
// is disabled" behaviour; the file is ~775 KB and the parse is sub-second,
// so the startup cost is bounded and one-shot. Failed fetches leave the
// mapper empty (graceful degradation: disabled-state requests then fall
// through the existing chain rather than translating).
const theBeastLTKitsuIMDbURL = "https://raw.githubusercontent.com/TheBeastLT/stremio-kitsu-anime/master/static/data/imdb_mapping.json"

// KitsuIMDbMapper indexes the TheBeastLT Kitsu→IMDb mapping file. Lookups are
// O(1) on the in-memory map; the underlying array is parsed once on Load
// and never touched again. The mapper is safe for concurrent reads.
type KitsuIMDbMapper struct {
	HTTP   *http.Client
	mu     sync.RWMutex
	byKitsu map[uint64]string // kitsu anime id → imdb id ("tt..." prefix)
	loaded  bool
}

// NewKitsuIMDbMapper returns an empty mapper; call Load to populate it.
func NewKitsuIMDbMapper(httpClient *http.Client) *KitsuIMDbMapper {
	return &KitsuIMDbMapper{HTTP: httpClient, byKitsu: map[uint64]string{}}
}

// KitsuIMDbEntry is one row of the TheBeastLT JSON. Some entries carry
// fromSeason / fromEpisode (used when a single Kitsu anime id maps to a
// different IMDb title per season — e.g. long-running shonen). We currently
// store only the first hit per kitsu_id; future per-episode mapping would
// need a separate SeasonEpisodeKey lookup.
type KitsuIMDbEntry struct {
	KitsuID     uint64 `json:"kitsu_id"`
	IMDBID      string `json:"imdb_id"`
	Title       string `json:"title"`
	FromSeason  *int   `json:"fromSeason,omitempty"`
	FromEpisode *int   `json:"fromEpisode,omitempty"`
}

// Load fetches and parses the mapping JSON. Idempotent — subsequent calls
// are no-ops (so the startup path can call this safely even if some test
// harness re-runs the bootstrap).
func (m *KitsuIMDbMapper) Load(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("nil KitsuIMDbMapper")
	}
	m.mu.Lock()
	if m.loaded {
		m.mu.Unlock()
		return nil
	}
	if m.HTTP == nil {
		m.mu.Unlock()
		return fmt.Errorf("KitsuIMDbMapper: HTTP client not configured")
	}
	m.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, theBeastLTKitsuIMDbURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", kitsuUserAgent)

	// Route through SendWithRetry so transient 429 / 5xx / connection
	// resets retry cleanly with the same backoff the rest of the Kitsu
	// code paths use. The previous direct-m.HTTP.Do path was vulnerable to
	// "invalid Read on closed Body" when the transport reset mid-stream.
	resp, err := SendWithRetry(&MALRetry, func() (*http.Response, error) {
		// Re-build the request inside the retry closure because the previous
		// attempt's request may have been mutated by the transport
		// (http.Client reuses req.Body across retries, which is fine for
		// GETs but cheap to recreate for safety).
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, theBeastLTKitsuIMDbURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", kitsuUserAgent)
		return m.HTTP.Do(req)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("imdb_mapping.json returned HTTP %d (%d bytes)", resp.StatusCode, len(body))
	}

	var entries []KitsuIMDbEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return err
	}

	cache := make(map[uint64]string, len(entries))
	for _, e := range entries {
		if e.IMDBID == "" || e.KitsuID == 0 {
			continue
		}
		// First-hit-wins when TheBeastLT carries multiple rows per kitsu id
		// (per-season/per-episode). Future per-episode mapping would key by
		// (kitsu_id, season, episode) instead.
		if _, exists := cache[e.KitsuID]; !exists {
			cache[e.KitsuID] = e.IMDBID
		}
	}

	m.mu.Lock()
	m.byKitsu = cache
	m.loaded = true
	m.mu.Unlock()

	slog.Info("kitsu→imdb mapping loaded", "entries", len(cache), "bytes", len(body))
	return nil
}

// LookupIMDB returns the IMDb id ("tt..." prefix) for the given Kitsu anime
// id, or nil if the mapping doesn't include it. Returns nil if Load hasn't
// been called (so callers can safely call before bootstrap completes).
func (m *KitsuIMDbMapper) LookupIMDB(kitsuID uint64) *string {
	if m == nil || kitsuID == 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.byKitsu[kitsuID]; ok && v != "" {
		return &v
	}
	return nil
}

// Loaded reports whether Load has succeeded. Used by tests + the bootstrap
// status to surface degraded mode in logs.
func (m *KitsuIMDbMapper) Loaded() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded
}

// Size returns the number of kitsu→imdb entries currently indexed. Useful
// for /api/admin/stats if we want to surface mapping coverage.
func (m *KitsuIMDbMapper) Size() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byKitsu)
}

// _ silences unused-import warning when strconv isn't referenced yet (kept
// for future per-episode parsing).
var _ = strconv.Itoa
