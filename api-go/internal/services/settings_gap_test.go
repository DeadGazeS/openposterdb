package services

import (
	"context"
	"testing"
)

// Several images in the region tier: within it the 2:3 one wins over a
// higher-voted odd shape, and among 2:3 ones the higher vote wins. Images of
// the language without the region, or of another region, never compete.
func TestSelectImageRanked_RegionalTierRanksByAspectThenVotes(t *testing.T) {
	imgs := []TmdbImage{
		timg("/de-wide-top", "de", "AT", 9.9, 1.0), // region, odd shape
		timg("/de-std-low", "de", "AT", 2, 0.667),  // region, 2:3, low vote
		timg("/de-std-high", "de", "at", 6, 0.68),  // region (case-insensitive), 2:3, best vote
		timg("/de-std-mid", "de", "AT", 4, 0.65),   // region, 2:3
		timg("/de-plain", "de", "", 9.8, 0.667),    // language only
		timg("/de-DE", "de", "DE", 9.7, 0.667),     // other region
		timg("/en-AT", "en", "AT", 9.6, 0.667),     // region, other language
	}
	if got := SelectPoster(imgs, "de-AT", "", false); got == nil || got.FilePath != "/de-std-high" {
		t.Fatalf("got %+v, want /de-std-high", got)
	}

	// Only odd shapes in the region: the vote decides among them, and the
	// region still beats a 2:3 image of the plain language.
	odd := []TmdbImage{
		timg("/de-AT-a", "de", "AT", 3, 1.2),
		timg("/de-AT-b", "de", "AT", 7, 0.9),
		timg("/de-plain", "de", "", 9, 0.667),
	}
	if got := SelectPoster(odd, "de-AT", "", false); got == nil || got.FilePath != "/de-AT-b" {
		t.Fatalf("odd shapes: got %+v, want /de-AT-b", got)
	}

	// Without a target ratio (logos) only the votes count inside the tier.
	logos := []TmdbImage{
		timg("/logo-AT-std", "de", "AT", 1, 0.667),
		timg("/logo-AT-wide", "de", "AT", 5, 3.5),
		timg("/logo-de", "de", "", 9, 3.5),
	}
	if got := SelectImage(logos, "de-AT", "", false); got == nil || got.FilePath != "/logo-AT-wide" {
		t.Fatalf("logos: got %+v, want /logo-AT-wide", got)
	}
}

// Stored per-key enum values that are not (or no longer) valid normalise
// through the Parse* functions to their fallback instead of reaching the
// renderer as an unknown value. One row per enum field of APIKeySettings.
func TestEffectiveFromPerKey_InvalidStoredEnumsNormalise(t *testing.T) {
	defaults := DefaultRenderSettings()
	cases := []struct {
		name string
		set  func(*APIKeySettings, string)
		get  func(RenderSettings) string
		want string
	}{
		{"image_source", func(k *APIKeySettings, v string) { k.ImageSource = v }, func(s RenderSettings) string { return string(s.ImageSource) }, string(ImageSourceTMDB)},
		{"poster_badge_style", func(k *APIKeySettings, v string) { k.PosterBadgeStyle = v }, func(s RenderSettings) string { return string(s.PosterBadgeStyle) }, string(BadgeStyleDefault)},
		{"logo_badge_style", func(k *APIKeySettings, v string) { k.LogoBadgeStyle = v }, func(s RenderSettings) string { return string(s.LogoBadgeStyle) }, string(BadgeStyleLogoTB)},
		{"backdrop_badge_style", func(k *APIKeySettings, v string) { k.BackdropBadgeStyle = v }, func(s RenderSettings) string { return string(s.BackdropBadgeStyle) }, string(BadgeStyleLogoTB)},
		{"episode_badge_style", func(k *APIKeySettings, v string) { k.EpisodeBadgeStyle = v }, func(s RenderSettings) string { return string(s.EpisodeBadgeStyle) }, string(BadgeStyleLogoTB)},
		{"poster_label_style", func(k *APIKeySettings, v string) { k.PosterLabelStyle = v }, func(s RenderSettings) string { return string(s.PosterLabelStyle) }, string(LabelStyleOfficial)},
		{"logo_label_style", func(k *APIKeySettings, v string) { k.LogoLabelStyle = v }, func(s RenderSettings) string { return string(s.LogoLabelStyle) }, string(LabelStyleOfficial)},
		{"backdrop_label_style", func(k *APIKeySettings, v string) { k.BackdropLabelStyle = v }, func(s RenderSettings) string { return string(s.BackdropLabelStyle) }, string(LabelStyleOfficial)},
		{"episode_label_style", func(k *APIKeySettings, v string) { k.EpisodeLabelStyle = v }, func(s RenderSettings) string { return string(s.EpisodeLabelStyle) }, string(LabelStyleOfficial)},
		{"poster_badge_direction", func(k *APIKeySettings, v string) { k.PosterBadgeDirection = v }, func(s RenderSettings) string { return string(s.PosterBadgeDirection) }, string(BadgeDirectionDefault)},
		{"backdrop_badge_direction", func(k *APIKeySettings, v string) { k.BackdropBadgeDirection = v }, func(s RenderSettings) string { return string(s.BackdropBadgeDirection) }, string(BadgeDirectionDefault)},
		{"episode_badge_direction", func(k *APIKeySettings, v string) { k.EpisodeBadgeDirection = v }, func(s RenderSettings) string { return string(s.EpisodeBadgeDirection) }, string(BadgeDirectionVertical)},
		{"poster_fit", func(k *APIKeySettings, v string) { k.PosterFit = v }, func(s RenderSettings) string { return string(s.PosterFit) }, string(PosterFitNative)},
		{"anime_artwork", func(k *APIKeySettings, v string) { k.AnimeArtwork = v }, func(s RenderSettings) string { return string(s.AnimeArtwork) }, string(AnimeArtworkMatchID)},
		{"poster_badge_shape", func(k *APIKeySettings, v string) { k.PosterBadgeShape = v }, func(s RenderSettings) string { return string(s.PosterBadgeShape) }, string(BadgeShapeRounded)},
		{"logo_badge_shape", func(k *APIKeySettings, v string) { k.LogoBadgeShape = v }, func(s RenderSettings) string { return string(s.LogoBadgeShape) }, string(BadgeShapeRounded)},
		{"backdrop_badge_shape", func(k *APIKeySettings, v string) { k.BackdropBadgeShape = v }, func(s RenderSettings) string { return string(s.BackdropBadgeShape) }, string(BadgeShapeRounded)},
		{"episode_badge_shape", func(k *APIKeySettings, v string) { k.EpisodeBadgeShape = v }, func(s RenderSettings) string { return string(s.EpisodeBadgeShape) }, string(BadgeShapeRounded)},
	}
	for _, c := range cases {
		for _, stored := range []string{"bogus", "", "TB", " h"} {
			t.Run(c.name+"/"+stored, func(t *testing.T) {
				var perKey APIKeySettings
				c.set(&perKey, stored)
				got := effectiveFromPerKey(context.Background(), nil, 1, &perKey, nil, &defaults)
				if v := c.get(got); v != c.want {
					t.Errorf("stored %q: got %q, want %q", stored, v, c.want)
				}
			})
		}
	}
}
