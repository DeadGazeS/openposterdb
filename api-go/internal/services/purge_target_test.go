package services

import (
	"testing"
	"time"
)

// A kitsu:/mal: title's badges come from its cross-referenced TMDB/IMDb
// title (RatingsTarget). That target is only known per request, so purge
// used to miss those ratings; it is now remembered next to the ID.
func TestInvalidateTitleRatings_KitsuTarget(t *testing.T) {
	ids := NewMemCache(0, 100, time.Hour, 0)
	ratings := NewMemCache(0, 100, time.Hour, 0)
	imdb := "tt1528406"
	resolved := &ResolvedID{SourceProvider: "kitsu"}
	ids.Set("kitsu/4676", idEntry{resolved: resolved}, 1)
	target := &ResolvedID{TMDbID: 61459, MediaType: MediaTypeTV, IMDbID: &imdb}
	RememberRatingsTarget(ids, "kitsu", "4676", target)

	key := ratingsCacheKey(RatingsQuery{ResolvedTMDbID: 61459, MediaType: "tv", IMDbID: &imdb})
	ratings.Set(key, "cached", 1)
	InvalidateTitleRatings(ids, ratings, "kitsu", "4676")
	if _, ok := ratings.Get(key); ok {
		t.Fatal("target's ratings survived the purge")
	}
}
