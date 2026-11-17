package services

import "testing"

func TestValidateUsername_UnicodeWhitespace(t *testing.T) {
	for _, u := range []string{"ad min", "ad min", "ad　min", "ad\u0085min", "ad min", "ad min", "ad\tmin"} {
		if ValidateUsername(u) == nil {
			t.Errorf("%q accepted", u)
		}
	}
	for _, u := range []string{"admin", "Ädmin", "管理者", "a.b-c_d"} {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
}

func TestValidateSettingEnum(t *testing.T) {
	ok := map[string]string{
		"poster_badge_style": "h", "logo_badge_style": "tb", "episode_label_style": "o",
		"backdrop_badge_shape": "p", "poster_badge_direction": "v", "image_source": "f",
		"poster_fit": "blur", "lang": "anything", "ratings_order": "x",
	}
	for f, v := range ok {
		if err := ValidateSettingEnum(f, v); err != nil {
			t.Errorf("%s=%q rejected: %v", f, v, err)
		}
	}
	bad := map[string]string{
		"poster_badge_style": "x", "episode_label_style": "", "logo_badge_shape": "square",
		"backdrop_badge_direction": "up", "image_source": "tmdb", "poster_fit": "fill",
	}
	for f, v := range bad {
		if ValidateSettingEnum(f, v) == nil {
			t.Errorf("%s=%q accepted", f, v)
		}
	}
}

func TestMemCacheDeleteTitle_AnchoredOnDelimiter(t *testing.T) {
	c := NewMemCache(0, 100, 0, 0)
	for _, k := range []string{"imdb/tt1", "imdb/tt1@ab", "imdb/tt1_t_de@a", "imdb/tt12", "imdb/tt12@ab"} {
		c.Set(k, 1, 1)
	}
	c.DeleteTitle("imdb/tt1")
	for k, want := range map[string]bool{"imdb/tt1": false, "imdb/tt1@ab": false, "imdb/tt1_t_de@a": false, "imdb/tt12": true, "imdb/tt12@ab": true} {
		if _, ok := c.Peek(k); ok != want {
			t.Errorf("%s present=%v, want %v", k, ok, want)
		}
	}
}
