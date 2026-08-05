package image

import (
	"log/slog"
	"strconv"

	"openposterdb/internal/services"
)

// altID identifies one alternate cache form of a resolved title.
type altID struct {
	idType  string
	idValue string
}

// alternateKeys returns the alternate ID forms a resolved title can be
// addressed by, in a stable order, excluding the form the current request
// already used (that key is written by the normal render path). For an
// episode-uplifted request (poster/logo/backdrop via an episode ID), the
// resolved series IDs are all valid alternates, including the same idType
// with a different value — so exclusion is by exact (idType, idValue) match,
// not by idType alone.
func alternateKeys(resolved *services.ResolvedID, requestedIDType, requestedIDValue string) []altID {
	skip := func(idType, idValue string) bool {
		return idType == requestedIDType && idValue == requestedIDValue
	}
	var out []altID
	if resolved.TMDbID != 0 {
		v := services.FormatTMDbIDValue(resolved.TMDbID, resolved.MediaType, nil)
		if !skip("tmdb", v) {
			out = append(out, altID{idType: "tmdb", idValue: v})
		}
	}
	if resolved.IMDbID != nil && *resolved.IMDbID != "" {
		if !skip("imdb", *resolved.IMDbID) {
			out = append(out, altID{idType: "imdb", idValue: *resolved.IMDbID})
		}
	}
	if resolved.TVDBID != nil {
		v := strconv.FormatUint(*resolved.TVDBID, 10)
		if !skip("tvdb", v) {
			out = append(out, altID{idType: "tvdb", idValue: v})
		}
	}
	return out
}

// writeCrossIDCache copies freshly rendered bytes under every alternate ID
// form (imdb/tmdb/tvdb) so a request via another ID type hits the cache
// instead of regenerating. Mirrors the Rust spawn_cross_id_cache.
//
// Fire-and-forget: writes run in a goroutine and failures are logged, never
// returned to the request. Skipped for episodes (stills are per-episode) and
// EXTERNAL_CACHE_ONLY (no files on disk to populate).
func (p ServeParams) writeCrossIDCache(resolved *services.ResolvedID, badges []services.RatingBadge, rendered []byte, imageSize services.ImageSize) {
	if p.ExternalCacheOnly || p.Kind == "episode" {
		return
	}
	alternates := alternateKeys(resolved, p.IDType, p.IDValue)
	if len(alternates) == 0 {
		return
	}
	suffix := p.renderSuffix(badges)
	releaseDate := resolved.ReleaseDate
	imageTypeChar := ImageDbValue(p.Kind)
	go func() {
		for _, alt := range alternates {
			altKey, altPath, err := p.cacheKeyFor(alt.idType, alt.idValue, suffix)
			if err != nil {
				continue
			}
			if err := services.WriteCache(altPath, rendered); err != nil {
				slog.Warn("cross-id cache write failed", "cache_key", altKey, "error", err)
				continue
			}
			if err := services.UpsertImageMeta(p.DB, altKey, releaseDate, imageTypeChar); err != nil {
				slog.Warn("cross-id meta write failed", "cache_key", altKey, "error", err)
			}
		}
	}()
}
