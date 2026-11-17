package services

import (
	"testing"
)

func timg(path, lang, region string, vote, ratio float64) TmdbImage {
	img := TmdbImage{FilePath: path, VoteAverage: vote, AspectRatio: ratio}
	if lang != "" {
		img.Iso639_1 = &lang
	}
	if region != "" {
		img.Iso3166_1 = &region
	}
	return img
}

func TestImageLanguages(t *testing.T) {
	cases := map[[2]string]string{
		{"de", "ja"}:    "de,en,null,ja",
		{"en", ""}:      "en,null",
		{"pt-BR", "pt"}: "pt,en,null",
		{"", "ko"}:      "en,null,ko",
	}
	for in, want := range cases {
		if got := imageLanguages(in[0], in[1]); got != want {
			t.Errorf("imageLanguages(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestSelectImageRanked_Tiers(t *testing.T) {
	all := []TmdbImage{
		timg("/any", "fr", "", 9, 0.667),
		timg("/null", "", "", 5, 0.667),
		timg("/xx", "xx", "", 6, 0.667),
		timg("/ja", "ja", "", 4, 0.667),
		timg("/en-low", "en", "", 2, 0.667),
		timg("/en-high", "en", "", 7, 0.667),
		timg("/de", "de", "", 3, 0.667),
		timg("/de-AT", "de", "AT", 1, 0.667),
	}
	without := func(paths ...string) []TmdbImage {
		var out []TmdbImage
		for _, img := range all {
			keep := true
			for _, p := range paths {
				keep = keep && img.FilePath != p
			}
			if keep {
				out = append(out, img)
			}
		}
		return out
	}
	cases := []struct {
		name     string
		images   []TmdbImage
		lang     string
		orig     string
		textless bool
		want     string
	}{
		{"region first", all, "de-AT", "ja", false, "/de-AT"},
		{"language", without("/de-AT"), "de-AT", "ja", false, "/de"},
		{"english before original, best vote", without("/de-AT", "/de"), "de", "ja", false, "/en-high"},
		{"original language", without("/de-AT", "/de", "/en-low", "/en-high"), "de", "ja", false, "/ja"},
		{"untagged (null and xx), best vote", without("/de-AT", "/de", "/en-low", "/en-high", "/ja"), "de", "ja", false, "/xx"},
		{"any as last resort", []TmdbImage{timg("/fr", "fr", "", 1, 0.667)}, "de", "ja", false, "/fr"},
		{"textless takes only untagged", all, "de", "ja", true, "/xx"},
		{"textless without untagged", []TmdbImage{timg("/de", "de", "", 1, 0.667)}, "de", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectPoster(c.images, c.lang, c.orig, c.textless)
			gotPath := ""
			if got != nil {
				gotPath = got.FilePath
			}
			if gotPath != c.want {
				t.Errorf("got %q, want %q", gotPath, c.want)
			}
		})
	}
}

// A German request for a title that only has English logos gets the English
// logo (the "en" tier used to never match because /images was asked for
// "de,null" only).
func TestSelectImage_GermanFallsBackToEnglishLogo(t *testing.T) {
	logos := []TmdbImage{timg("/en-logo", "en", "", 5, 3)}
	if got := SelectImage(logos, "de", "en", false); got == nil || got.FilePath != "/en-logo" {
		t.Fatalf("got %+v, want /en-logo", got)
	}
}

// Within a tier a 2:3 poster beats a higher-voted odd shape.
func TestSelectPoster_AspectBeforeVotes(t *testing.T) {
	imgs := []TmdbImage{timg("/wide", "de", "", 9, 1.0), timg("/std", "de", "", 1, 0.667)}
	if got := SelectPoster(imgs, "de", "", false); got.FilePath != "/std" {
		t.Fatalf("got %s, want /std", got.FilePath)
	}
}

func TestSelectBackdrop(t *testing.T) {
	imgs := []TmdbImage{timg("/en", "en", "", 9, 1.78), timg("/de", "de", "", 8, 1.78), timg("/xx", "xx", "", 1, 1.78)}
	if got := SelectBackdrop(imgs, "de"); got.FilePath != "/xx" {
		t.Errorf("untagged first: got %s", got.FilePath)
	}
	if got := SelectBackdrop(imgs[:2], "de"); got.FilePath != "/de" {
		t.Errorf("then language: got %s", got.FilePath)
	}
	if got := SelectBackdrop(imgs[:1], "de"); got.FilePath != "/en" {
		t.Errorf("then any: got %s", got.FilePath)
	}
	if SelectBackdrop(nil, "de") != nil {
		t.Error("empty list must give nil")
	}
}

func TestSelectFanartImage_Tiers(t *testing.T) {
	fp := func(id, lang, likes string) FanartPoster { return FanartPoster{ID: id, Lang: lang, Likes: likes} }
	all := []FanartPoster{
		fp("pt", "pt", "1"), fp("en", "en", "3"), fp("tl", "00", "2"), fp("none", "", "4"), fp("fr", "fr", "9"),
	}
	cases := []struct {
		name     string
		posters  []FanartPoster
		lang     string
		textless bool
		want     string
	}{
		{"exact language", all, "pt", false, "pt"},
		{"base language", all, "pt-BR", false, "pt"},
		{"english", all[1:], "de", false, "en"},
		{"textless after english", all[2:], "de", false, "tl"},
		{"untagged", all[3:], "de", false, "none"},
		{"any, most likes", all[4:], "de", false, "fr"},
		{"textless asked: 00 first", all, "pt", true, "tl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, ok := SelectFanartImage(c.posters, c.lang, c.textless)
			if !ok || got.ID != c.want {
				t.Errorf("got %+v ok=%v, want %s", got, ok, c.want)
			}
		})
	}
	if _, _, ok := SelectFanartImage(nil, "en", false); ok {
		t.Error("empty list must not select")
	}
}
