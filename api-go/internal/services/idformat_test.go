package services

import "testing"

// The form of an id decides its type: tt… is IMDb, movie-/series- is TMDB,
// whatever the URL says (clients send ids under the wrong type). A plain
// number and episode-N-S1E1 keep the URL's type where it takes that form.
// An id no type takes is rejected.
func TestEffectiveIDType(t *testing.T) {
	ok := []struct{ urlType, id, want string }{
		{"imdb", "tt0137523", "imdb"},
		{"imdb", "episode-tt0903747-S1E1", "imdb"},
		{"imdb", "episode-tt0903747-s01e02", "imdb"},
		{"tmdb", "movie-550", "tmdb"},
		{"tmdb", "series-1396", "tmdb"},
		{"tmdb", "episode-1396-S5E14", "tmdb"},
		{"tvdb", "81189", "tvdb"},
		{"tvdb", "episode-81189-S3E5", "tvdb"},
		{"kitsu", "13658", "kitsu"},
		{"kitsu", "fairy-tail-2018", "kitsu"},
		{"mal", "35972", "mal"},
		// The form overrides the URL's type.
		{"tvdb", "movie-2", "tmdb"},
		{"imdb", "series-124364", "tmdb"},
		{"tmdb", "tt1234567", "imdb"},
		{"kitsu", "tt0137523", "imdb"},
		{"imdb", "episode-1396-S1E1", "tmdb"},
		// Unknown types are left to ParseIDType's own error.
		{"nope", "x", "nope"},
	}
	for _, tc := range ok {
		got, err := EffectiveIDType(tc.urlType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("%s/%s: got %q, %v; want %q", tc.urlType, tc.id, got, err, tc.want)
		}
	}
	bad := []struct{ urlType, id string }{
		{"imdb", "abc"}, {"imdb", "0137523"}, {"imdb", "tt0903747-1-1"},
		{"imdb", "81189"}, // a plain number is no IMDb id, and which other type is meant is unknown
		{"tmdb", "550"}, {"tmdb", "foo-2"}, {"tmdb", "movie-"}, {"tmdb", "episode-1396"},
		{"tvdb", "foo-2"}, {"tvdb", "81189x"}, {"tvdb", "episode-81189"},
		{"kitsu", "fairy tail"}, {"kitsu", "-fairy"}, {"kitsu", "a_b"},
		{"mal", "abc"}, {"mal", "35972x"}, {"mal", "fairy-tail"},
	}
	for _, tc := range bad {
		if got, err := EffectiveIDType(tc.urlType, tc.id); err == nil {
			t.Errorf("%s/%s accepted as %q", tc.urlType, tc.id, got)
		}
	}
}
