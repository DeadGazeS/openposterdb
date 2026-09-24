package image

import (
	"context"
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
	// Kitsu/MAL alternates: when a kitsu:N request resolves to a title whose
	// mappings include a MAL id (or vice-versa), surface the MAL value as an
	// alternate cache form so a later mal:M request hits the cache instead of
	// re-fetching from Kitsu. MAL is the cross-link Kitsu exposes natively; the
	// IMDB/TVDB cross-link doesn't exist for anime in practice (verified 2026-08-14).
	if resolved.KitsuID != nil {
		v := strconv.FormatUint(*resolved.KitsuID, 10)
		if !skip("kitsu", v) {
			out = append(out, altID{idType: "kitsu", idValue: v})
		}
	}
	if resolved.MALID != nil {
		v := strconv.FormatUint(*resolved.MALID, 10)
		if !skip("mal", v) {
			out = append(out, altID{idType: "mal", idValue: v})
		}
	}
	return out
}

// filterAnimeAlternates drops kitsu/mal alternates that would render
// different artwork than this image carries (NOTES.md #7). For a
// Kitsu/MAL-artwork render (SourceProvider set), a kitsu:/mal: request of
// type T would use: the preferred provider under Prefer Kitsu / Prefer MAL
// (both toggles on), T itself under Match the ID, or TMDB/Fanart when T's
// toggle is off. Only alternates whose expected provider matches the
// rendered one are kept — e.g. Match the ID: a kitsu: render no longer
// writes a mal/ copy. imdb/tmdb/tvdb alternates are untouched.
func (p ServeParams) filterAnimeAlternates(resolved *services.ResolvedID, alts []altID) []altID {
	if resolved == nil || resolved.SourceProvider == "" || p.Settings == nil {
		return alts
	}
	expected := func(idType string) string {
		if idType == "kitsu" && !p.Settings.UseKitsu || idType == "mal" && !p.Settings.UseMAL {
			return "tmdb"
		}
		if p.Settings.UseKitsu && p.Settings.UseMAL {
			switch services.ParseAnimeArtwork(string(p.Settings.AnimeArtwork)) {
			case services.AnimeArtworkKitsu:
				return "kitsu"
			case services.AnimeArtworkMAL:
				return "mal"
			}
		}
		return idType
	}
	out := alts[:0:0]
	for _, a := range alts {
		if (a.idType == "kitsu" || a.idType == "mal") && expected(a.idType) != resolved.SourceProvider {
			continue
		}
		out = append(out, a)
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
	alternates := p.filterAnimeAlternates(resolved, alternateKeys(resolved, p.IDType, p.IDValue))
	if len(alternates) == 0 {
		return
	}
	suffix := p.renderSuffix(badges)
	releaseDate := resolved.ReleaseDate
	titleKey := resolved.TitleKey
	title := resolved.Title
	imageTypeChar := ImageDbValue(p.Kind)
	go func() {
		// Use a background context for the writes so the cross-id cache
		// population survives a client disconnect on the originating request
		// (this is server-side cache maintenance, not part of the response).
		ctx := p.Context
		if ctx == nil {
			ctx = context.Background()
		} else {
			bg := context.Background()
			ctx = bg
		}
		for _, alt := range alternates {
			altKey, altPath, err := p.cacheKeyFor(alt.idType, alt.idValue, suffix)
			if err != nil {
				continue
			}
			if err := services.WriteCache(altPath, rendered); err != nil {
				slog.Warn("cross-id cache write failed", "cache_key", altKey, "error", err)
				continue
			}
			if err := services.UpsertImageMetaCtx(ctx, p.DB, altKey, releaseDate, imageTypeChar, titleKey, title); err != nil {
				slog.Warn("cross-id meta write failed", "cache_key", altKey, "error", err)
			}
		}
	}()
}
