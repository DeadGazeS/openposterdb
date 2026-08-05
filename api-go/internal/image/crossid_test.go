package image

import (
	"testing"

	"openposterdb/internal/services"
)

func TestAlternateKeys_Movie(t *testing.T) {
	imdb := "tt0111161"
	tvdb := uint64(81189)
	resolved := &services.ResolvedID{
		IMDbID:    &imdb,
		TMDbID:    278,
		TVDBID:    &tvdb,
		MediaType: services.MediaTypeMovie,
	}
	// Requested via imdb → alternates are tmdb + tvdb.
	got := alternateKeys(resolved, "imdb", "tt0111161")
	want := []altID{{idType: "tmdb", idValue: "movie-278"}, {idType: "tvdb", idValue: "81189"}}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("alt[%d] = %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestAlternateKeys_RequestedIDExcluded(t *testing.T) {
	imdb := "tt0111161"
	resolved := &services.ResolvedID{
		IMDbID:    &imdb,
		TMDbID:    278,
		MediaType: services.MediaTypeMovie,
	}
	// Requested via tmdb → tmdb excluded, imdb included.
	got := alternateKeys(resolved, "tmdb", "movie-278")
	want := []altID{{idType: "imdb", idValue: "tt0111161"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAlternateKeys_Series(t *testing.T) {
	imdb := "tt14786934"
	resolved := &services.ResolvedID{
		IMDbID:    &imdb,
		TMDbID:    1396,
		MediaType: services.MediaTypeTV,
	}
	got := alternateKeys(resolved, "imdb", "tt14786934")
	want := []altID{{idType: "tmdb", idValue: "series-1396"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAlternateKeys_EpisodeUpliftKeepsSeriesImdb(t *testing.T) {
	// Poster requested via an episode IMDb id; after uplift the resolved
	// series' own imdb id is a DIFFERENT key than the request, so it must be
	// included despite both being idType "imdb".
	seriesIMDB := "tt14786934"
	resolved := &services.ResolvedID{
		IMDbID:    &seriesIMDB,
		TMDbID:    1396,
		TVDBID:    nil,
		MediaType: services.MediaTypeTV,
	}
	got := alternateKeys(resolved, "imdb", "episode-tt14786934-S1E1")
	found := false
	for _, a := range got {
		if a.idType == "imdb" && a.idValue == "tt14786934" {
			found = true
		}
	}
	if !found {
		t.Fatalf("series imdb alternate missing from %v", got)
	}
}

func TestAlternateKeys_NoDuplicates(t *testing.T) {
	// A request whose id matches exactly one alternate form must not produce
	// duplicate entries.
	imdb := "tt0111161"
	resolved := &services.ResolvedID{
		IMDbID:    &imdb,
		TMDbID:    278,
		MediaType: services.MediaTypeMovie,
	}
	got := alternateKeys(resolved, "tvdb", "81189")
	seen := map[altID]bool{}
	for _, a := range got {
		if seen[a] {
			t.Errorf("duplicate alternate %+v", a)
		}
		seen[a] = true
	}
}

func TestAlternateKeys_NoIDs(t *testing.T) {
	resolved := &services.ResolvedID{TMDbID: 0, MediaType: services.MediaTypeMovie}
	if got := alternateKeys(resolved, "imdb", "tt0"); len(got) != 0 {
		t.Fatalf("expected no alternates, got %v", got)
	}
}
