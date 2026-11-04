package handlers

import (
	"testing"

	"openposterdb/internal/services"
)

// Unknown style values in the query must not reach the cache key: each
// distinct garbage value used to be a new render, disk file and image_meta
// row (cache flood through the public free key).
func TestApplyQueryOverrides_GarbageStylesShareCacheKey(t *testing.T) {
	base := services.RenderSettings{}
	key := func(v string) map[string]string {
		out := map[string]string{}
		for _, kind := range []string{"poster", "logo", "backdrop", "episode"} {
			q := &ImageQuery{LabelStyle: &v, BadgeShape: &v, BadgeDirection: &v, Fit: &v, ImageSource: &v, Layout: &v}
			s := applyQueryOverrides(&base, q, kind)
			out[kind] = services.SettingsCacheSuffix(s, kind, nil) + "|" + string(s.ImageSource)
		}
		return out
	}
	a, b := key("zzz-random-1"), key("zzz-random-2")
	for kind := range a {
		if a[kind] != b[kind] {
			t.Errorf("%s: garbage overrides gave different cache keys %q vs %q", kind, a[kind], b[kind])
		}
	}
}

// An invalid layout override keeps the stored layout.
func TestApplyQueryOverrides_InvalidLayoutIgnored(t *testing.T) {
	base := services.RenderSettings{PosterLayout: services.DefaultLayout("poster")}
	bad := `{"top":{"per_row":999,"rows":999,"start":"zz"}}`
	s := applyQueryOverrides(&base, &ImageQuery{Layout: &bad}, "poster")
	if got, want := services.SettingsCacheSuffix(s, "poster", nil), services.SettingsCacheSuffix(&base, "poster", nil); got != want {
		t.Errorf("invalid layout changed the cache key: %q vs %q", got, want)
	}
}

// Valid values still apply.
func TestApplyQueryOverrides_ValidStylesApply(t *testing.T) {
	base := services.RenderSettings{}
	ls, sh, dir, fit, src := "t", "p", "v", "cover", "f"
	s := applyQueryOverrides(&base, &ImageQuery{LabelStyle: &ls, BadgeShape: &sh, BadgeDirection: &dir, Fit: &fit, ImageSource: &src}, "poster")
	if s.PosterLabelStyle != services.LabelStyleText || s.PosterBadgeShape != services.BadgeShapePill ||
		s.PosterBadgeDirection != services.BadgeDirectionVertical || s.PosterFit != services.PosterFitCover ||
		s.ImageSource != services.ImageSourceFanart {
		t.Errorf("valid overrides not applied: %+v", s)
	}
}
