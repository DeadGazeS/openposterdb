package handlers

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"openposterdb/internal/services"
)

func overridesFor(t *testing.T, rawQuery, kind string, base services.RenderSettings) *services.RenderSettings {
	t.Helper()
	q := parseImageQuery(httptest.NewRequest("GET", "/x?"+rawQuery, nil))
	return applyQueryOverrides(&base, q, kind)
}

// The legacy URL from the plan: ?position=tl&badge_size=xl&badge_background=k.
func TestLegacyQuery_PositionSizeBackground(t *testing.T) {
	s := overridesFor(t, "position=tl&badge_size=xl&badge_background=k", "poster", services.DefaultRenderSettings())
	if want := services.LayoutFromLegacy("poster", "tl", "", false, 3); !reflect.DeepEqual(s.PosterLayout, want) {
		t.Errorf("layout %+v, want %+v", s.PosterLayout, want)
	}
	if s.PosterBadgeSize != 142 {
		t.Errorf("badge_size=xl → %d, want 142", s.PosterBadgeSize)
	}
	if s.PosterBadgeAlpha != 78 {
		t.Errorf("badge_background=k → %d, want 78", s.PosterBadgeAlpha)
	}
}

func TestLegacyQuery_SplitAndDirection(t *testing.T) {
	s := overridesFor(t, "split=true&badge_direction=h&ratings_limit=4", "poster", services.DefaultRenderSettings())
	if want := services.LayoutFromLegacy("poster", "bc", "h", true, 4); !reflect.DeepEqual(s.PosterLayout, want) {
		t.Errorf("layout %+v, want %+v", s.PosterLayout, want)
	}
	// Episodes take position too; the count is the stored layout's.
	e := overridesFor(t, "position=bl", "episode", services.DefaultRenderSettings())
	if want := services.LayoutFromLegacy("episode", "bl", "", false, 1); !reflect.DeepEqual(e.EpisodeLayout, want) {
		t.Errorf("episode layout %+v, want %+v", e.EpisodeLayout, want)
	}
}

// An explicit layout (or badge_alpha / numeric badge_size) wins over the
// legacy alias in the same URL.
func TestLegacyQuery_CurrentParametersWin(t *testing.T) {
	layout := `{"top":{"per_row":2,"rows":1,"start":"l"},"order":["top"]}`
	s := overridesFor(t, "position=br&layout="+layout+"&badge_alpha=30&badge_background=n&badge_size=120", "poster", services.DefaultRenderSettings())
	if s.PosterLayout.Top.PerRow != 2 || s.PosterLayout.Bottom.PerRow != 0 {
		t.Errorf("layout param lost to position: %+v", s.PosterLayout)
	}
	if s.PosterBadgeAlpha != 30 || s.PosterBadgeSize != 120 {
		t.Errorf("alpha %d size %d, want 30/120", s.PosterBadgeAlpha, s.PosterBadgeSize)
	}
}

// Invalid legacy values keep falling back (no new cache variants).
func TestLegacyQuery_InvalidValuesFallBack(t *testing.T) {
	base := services.DefaultRenderSettings()
	s := overridesFor(t, "position=nowhere&badge_size=huge&badge_background=x", "poster", base)
	if !reflect.DeepEqual(s.PosterLayout, base.PosterLayout) || s.PosterBadgeSize != base.PosterBadgeSize || s.PosterBadgeAlpha != base.PosterBadgeAlpha {
		t.Errorf("invalid legacy values changed settings: layout %+v size %d alpha %d", s.PosterLayout, s.PosterBadgeSize, s.PosterBadgeAlpha)
	}
}

// ?ratings_order= / ?ratings_exclude= (empty) clear the stored value.
func TestQuery_EmptyRatingsOrderAndExcludeClear(t *testing.T) {
	base := services.DefaultRenderSettings()
	base.RatingsOrder = "imdb,tmdb"
	base.RatingsExclude = "rt"
	s := overridesFor(t, "ratings_order=&ratings_exclude=", "poster", base)
	if s.RatingsOrder != "" || s.RatingsExclude != "" {
		t.Errorf("order %q exclude %q, want both cleared", s.RatingsOrder, s.RatingsExclude)
	}
	// Absent parameters keep them.
	s = overridesFor(t, "lang=de", "poster", base)
	if s.RatingsOrder != "imdb,tmdb" || s.RatingsExclude != "rt" {
		t.Errorf("absent params changed order %q exclude %q", s.RatingsOrder, s.RatingsExclude)
	}
}
