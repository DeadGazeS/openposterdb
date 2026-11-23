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

// ?badge_direction alone (the web UI sends it without a layout whenever the
// direction is non-default) sets the direction and keeps the stored layout.
func TestLegacyQuery_DirectionAloneKeepsLayout(t *testing.T) {
	base := services.DefaultRenderSettings()
	base.PosterLayout = services.ImageLayout{Top: services.SideSlot{PerRow: 2, Rows: 1, Start: "l"}, Order: []string{"top"}}
	s := overridesFor(t, "badge_direction=v", "poster", base)
	if !reflect.DeepEqual(s.PosterLayout, base.PosterLayout) {
		t.Errorf("badge_direction alone replaced the layout: %+v", s.PosterLayout)
	}
	if s.PosterBadgeDirection != services.ParseBadgeDirection("v") {
		t.Errorf("direction %v, want v", s.PosterBadgeDirection)
	}
	// With position it still feeds the conversion.
	s = overridesFor(t, "position=tl&badge_direction=h", "poster", base)
	if want := services.LayoutFromLegacy("poster", "tl", "h", false, 2); !reflect.DeepEqual(s.PosterLayout, want) {
		t.Errorf("layout %+v, want %+v", s.PosterLayout, want)
	}
}

// As in the Rust version: logos ignore position / split / direction, and split
// is poster-only.
func TestLegacyQuery_LogoIgnoresPlacementSplitPosterOnly(t *testing.T) {
	base := services.DefaultRenderSettings()
	s := overridesFor(t, "position=tl&split=true&badge_direction=v", "logo", base)
	if !reflect.DeepEqual(s.LogoLayout, base.LogoLayout) {
		t.Errorf("logo layout changed by legacy placement: %+v", s.LogoLayout)
	}
	b := overridesFor(t, "split=true", "backdrop", base)
	if !reflect.DeepEqual(b.BackdropLayout, base.BackdropLayout) {
		t.Errorf("split alone changed the backdrop layout: %+v", b.BackdropLayout)
	}
	b = overridesFor(t, "position=bl&split=true&ratings_limit=4", "backdrop", base)
	if want := services.LayoutFromLegacy("backdrop", "bl", "", false, 4); !reflect.DeepEqual(b.BackdropLayout, want) {
		t.Errorf("backdrop layout %+v, want unsplit %+v", b.BackdropLayout, want)
	}
}

// As in the Rust version, ?position= without ?badge_direction= keeps the
// key's stored direction instead of the kind's old default.
func TestLegacyQuery_PositionKeepsStoredDirection(t *testing.T) {
	base := services.DefaultRenderSettings()
	base.PosterBadgeDirection = services.BadgeDirectionHorizontal
	s := overridesFor(t, "position=tl", "poster", base)
	if want := services.LayoutFromLegacy("poster", "tl", "h", false, base.PosterLayout.Total()); !reflect.DeepEqual(s.PosterLayout, want) {
		t.Errorf("layout %+v, want stored direction h %+v", s.PosterLayout, want)
	}
	base.EpisodeBadgeDirection = services.BadgeDirectionHorizontal
	e := overridesFor(t, "position=bl&ratings_limit=3", "episode", base)
	if want := services.LayoutFromLegacy("episode", "bl", "h", false, 3); !reflect.DeepEqual(e.EpisodeLayout, want) {
		t.Errorf("episode layout %+v, want stored direction h %+v", e.EpisodeLayout, want)
	}
}

// ?split=false alone only turns split off (Rust): it never rebuilds, and so
// never wipes, a stored layout.
func TestLegacyQuery_SplitFalseAloneKeepsLayout(t *testing.T) {
	base := services.DefaultRenderSettings()
	base.PosterLayout = services.ImageLayout{Top: services.SideSlot{PerRow: 2, Rows: 1, Start: "l"}, Order: []string{"top"}}
	s := overridesFor(t, "split=false", "poster", base)
	if !reflect.DeepEqual(s.PosterLayout, base.PosterLayout) {
		t.Errorf("split=false alone replaced the layout: %+v", s.PosterLayout)
	}
	// With position it still feeds the conversion (unsplit).
	s = overridesFor(t, "split=false&position=tl&badge_direction=h", "poster", base)
	if want := services.LayoutFromLegacy("poster", "tl", "h", false, 2); !reflect.DeepEqual(s.PosterLayout, want) {
		t.Errorf("layout %+v, want %+v", s.PosterLayout, want)
	}
}
