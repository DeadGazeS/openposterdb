package services

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

func slot(perRow, rows int32, start string) SideSlot {
	return SideSlot{PerRow: perRow, Rows: rows, Start: start}
}

func TestLayoutFromLegacy(t *testing.T) {
	order := func(first ...string) []string {
		out := append([]string(nil), first...)
		for _, s := range DefaultFillOrder {
			if !containsString(out, s) {
				out = append(out, s)
			}
		}
		return out
	}
	cases := []struct {
		name           string
		kind, pos, dir string
		split          bool
		limit          int32
		want           ImageLayout
	}{
		{"rust defaults keep the current default", "poster", "bc", "d", false, 3, DefaultLayout("poster")},
		{"backdrop rust defaults keep the current default", "backdrop", "tr", "d", false, 5, DefaultLayout("backdrop")},
		{"episode rust defaults keep the current default", "episode", "tr", "v", false, 1, DefaultLayout("episode")},
		{"top centre row", "poster", "tc", "d", false, 3,
			ImageLayout{Top: slot(3, 1, "c"), Order: order("top")}},
		{"default direction at a corner is a column", "poster", "tl", "d", false, 3,
			ImageLayout{Left: slot(1, 3, "t"), Order: order("left")}},
		{"horizontal row in the bottom-right corner", "poster", "br", "h", false, 2,
			ImageLayout{Bottom: slot(2, 1, "r"), Order: order("bottom")}},
		{"horizontal rows on the left edge are vertically centred", "poster", "l", "h", false, 3,
			ImageLayout{Left: slot(3, 1, "c"), Order: order("left")}},
		{"vertical column at bottom centre", "poster", "bc", "v", false, 4,
			ImageLayout{Bottom: slot(1, 4, "c"), Order: order("bottom")}},
		{"vertical column on the right edge", "episode", "r", "v", false, 2,
			ImageLayout{Right: slot(1, 2, "c"), Order: order("right")}},
		{"long rows wrap once", "poster", "bc", "h", false, 7,
			ImageLayout{Bottom: slot(4, 2, "c"), Order: order("bottom")}},
		// Rust split_anchors: rows split top/bottom keeping the horizontal
		// anchor, the first ceil(n/2) badges stay on the configured side.
		{"split rows from bottom centre", "poster", "bc", "d", true, 5,
			ImageLayout{Bottom: slot(3, 1, "c"), Top: slot(2, 1, "c"), Order: order("bottom", "top")}},
		{"split rows from top left", "poster", "tl", "h", true, 4,
			ImageLayout{Top: slot(2, 1, "l"), Bottom: slot(2, 1, "l"), Order: order("top", "bottom")}},
		// Columns split left/right keeping the vertical anchor.
		{"split column from bottom right", "poster", "br", "d", true, 3,
			ImageLayout{Right: slot(1, 2, "b"), Left: slot(1, 1, "b"), Order: order("right", "left")}},
		{"split with one badge does not split", "poster", "bc", "h", true, 1,
			ImageLayout{Bottom: slot(1, 1, "c"), Order: order("bottom")}},
		{"zero ratings shows nothing", "poster", "tl", "d", false, 0,
			ImageLayout{Order: order()}},
		{"invalid values fall back to the kind defaults", "poster", "nowhere", "x", false, 3, DefaultLayout("poster")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LayoutFromLegacy(c.kind, c.pos, c.dir, c.split, c.limit)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
			if err := ValidateLayout(&got); err != nil {
				t.Errorf("invalid layout: %v", err)
			}
			if got.Total() != c.want.Total() {
				t.Errorf("total %d, want %d", got.Total(), c.want.Total())
			}
		})
	}
}

func TestAlphaFromLegacyBackground(t *testing.T) {
	want := map[string]BadgeAlpha{"d": DefaultBadgeAlpha(), "k": 78, "t": 47, "n": 0}
	for bg, a := range want {
		if got, ok := AlphaFromLegacyBackground(bg); !ok || got != a {
			t.Errorf("%q: got %d/%v, want %d", bg, got, ok, a)
		}
	}
	if _, ok := AlphaFromLegacyBackground("x"); ok {
		t.Error("unknown background must not convert")
	}
}

func TestPercentFromLegacySize(t *testing.T) {
	cases := []struct {
		kind, size string
		want       ScalePercent
	}{
		{"poster", "xs", 58}, {"poster", "s", 79}, {"poster", "m", 100}, {"poster", "l", 121}, {"poster", "xl", 142},
		{"backdrop", "m", 100},
		{"episode", "l", 100}, {"episode", "m", 83}, {"episode", "xs", 50}, // 48 clamps to 50
	}
	for _, c := range cases {
		if got, ok := PercentFromLegacySize(c.kind, c.size); !ok || got != c.want {
			t.Errorf("%s/%s: got %d/%v, want %d", c.kind, c.size, got, ok, c.want)
		}
	}
	for _, s := range []string{"", "120", "huge"} {
		if _, ok := PercentFromLegacySize("poster", s); ok {
			t.Errorf("%q must not convert", s)
		}
	}
}

// rustSettingsDB builds api_key_settings as a Rust-era database has it after
// the alpha and layout columns were added but before the legacy columns are
// dropped (the state the conversion migrations see).
func rustSettingsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	ddl := `CREATE TABLE api_key_settings (
		api_key_id INTEGER PRIMARY KEY,
		ratings_limit INTEGER NOT NULL DEFAULT 3,
		backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
		poster_position TEXT NOT NULL DEFAULT 'bc',
		poster_badge_direction TEXT NOT NULL DEFAULT 'd',
		poster_badge_split INTEGER NOT NULL DEFAULT 0,
		backdrop_position TEXT NOT NULL DEFAULT 'tr',
		backdrop_badge_direction TEXT NOT NULL DEFAULT 'd',
		poster_badge_background TEXT NOT NULL DEFAULT 'd',
		backdrop_badge_background TEXT NOT NULL DEFAULT 'd',
		poster_badge_alpha INTEGER NOT NULL DEFAULT 80,
		backdrop_badge_alpha INTEGER NOT NULL DEFAULT 80,
		poster_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":3,"rows":1,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}',
		backdrop_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":5,"rows":1,"start":"r"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'
	)`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestConvertLegacyBadgeBackgrounds(t *testing.T) {
	db := rustSettingsDB(t)
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id, poster_badge_background, backdrop_badge_background, backdrop_badge_alpha) VALUES
		(1, 'k', 'n', 80),
		(2, 'd', 't', 30)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 2 { // idempotent: the second pass changes nothing
		if err := ConvertLegacyBadgeBackgroundsCtx(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	got := map[int64][2]int{}
	rows, err := db.Query("SELECT api_key_id, poster_badge_alpha, backdrop_badge_alpha FROM api_key_settings")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p, b int
		if err := rows.Scan(&id, &p, &b); err != nil {
			t.Fatal(err)
		}
		got[id] = [2]int{p, b}
	}
	// Row 2's backdrop alpha was already set in the new model → kept.
	want := map[int64][2]int{1: {78, 0}, 2: {80, 30}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestConvertLegacyBadgeLayouts(t *testing.T) {
	db := rustSettingsDB(t)
	custom := `{"top":{"per_row":2,"rows":1,"start":"l"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["top"]}`
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id, poster_position, poster_badge_direction, poster_badge_split, ratings_limit, backdrop_position, backdrop_badge_direction) VALUES
		(1, 'tl', 'h', 0, 3, 'tr', 'd'),
		(2, 'bc', 'd', 1, 4, 'bl', 'h'),
		(3, 'bc', 'd', 0, 3, 'tr', 'd')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id, poster_position, poster_layout) VALUES (4, 'tl', ?)`, custom); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 2 {
		if err := ConvertLegacyBadgeLayoutsCtx(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id int64, col string) ImageLayout {
		var s string
		if err := db.QueryRow("SELECT "+col+" FROM api_key_settings WHERE api_key_id = ?", id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return UnmarshalLayout(s, nil)
	}
	posterDef, backdropDef := DefaultLayout("poster"), DefaultLayout("backdrop")
	check := func(id int64, col string, want ImageLayout) {
		t.Helper()
		got := read(id, col)
		if !sameLayout(&got, &want) {
			t.Errorf("key %d %s: got %+v, want %+v", id, col, got, want)
		}
	}
	check(1, "poster_layout", LayoutFromLegacy("poster", "tl", "h", false, 3))
	check(1, "backdrop_layout", backdropDef)
	check(2, "poster_layout", LayoutFromLegacy("poster", "bc", "d", true, 4))
	check(2, "backdrop_layout", LayoutFromLegacy("backdrop", "bl", "h", false, 5))
	check(3, "poster_layout", posterDef)
	check(3, "backdrop_layout", backdropDef)
	// A layout already customised in the current model is never overwritten.
	if got := read(4, "poster_layout"); got.Top.PerRow != 2 || got.Top.Start != "l" {
		t.Errorf("custom layout overwritten: %+v", got)
	}
}

func TestConvertLegacy_NoLegacyColumnsIsNoop(t *testing.T) {
	db := upgradeTestDB(t) // api_key_settings with only api_key_id
	ctx := context.Background()
	if err := ConvertLegacyBadgeBackgroundsCtx(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ConvertLegacyBadgeLayoutsCtx(ctx, db); err != nil {
		t.Fatal(err)
	}
}

func TestParseGlobalRenderSettings_LegacyKeys(t *testing.T) {
	// Global settings as the Rust version stored them (same keys as the Rust
	// test globals_override_defaults).
	s := ParseGlobalRenderSettings(map[string]string{
		"poster_position":          "tl",
		"poster_badge_direction":   "h",
		"ratings_limit":            "4",
		"poster_badge_size":        "xl",
		"logo_badge_size":          "xs",
		"backdrop_badge_size":      "l",
		"poster_badge_background":  "k",
		"episode_badge_background": "n",
	})
	if want := LayoutFromLegacy("poster", "tl", "h", false, 4); !sameLayout(&s.PosterLayout, &want) {
		t.Errorf("poster layout: got %+v, want %+v", s.PosterLayout, want)
	}
	if s.PosterBadgeSize != 142 || s.LogoBadgeSize != 58 || s.BackdropBadgeSize != 121 {
		t.Errorf("sizes: got %d/%d/%d, want 142/58/121", s.PosterBadgeSize, s.LogoBadgeSize, s.BackdropBadgeSize)
	}
	if s.PosterBadgeAlpha != 78 || s.EpisodeBadgeAlpha != 0 || s.LogoBadgeAlpha != DefaultBadgeAlpha() {
		t.Errorf("alpha: got %d/%d/%d", s.PosterBadgeAlpha, s.EpisodeBadgeAlpha, s.LogoBadgeAlpha)
	}
}

func TestParseGlobalRenderSettings_StoredLayoutWins(t *testing.T) {
	stored := DefaultLayout("poster")
	stored.Bottom.PerRow = 2
	js, _ := MarshalLayout(&stored)
	s := ParseGlobalRenderSettings(map[string]string{
		"poster_position":         "tl",
		"poster_layout":           js,
		"poster_badge_background": "n",
		"poster_badge_alpha":      "55",
		"poster_badge_size":       "130",
	})
	if !sameLayout(&s.PosterLayout, &stored) {
		t.Errorf("stored layout replaced: %+v", s.PosterLayout)
	}
	if s.PosterBadgeAlpha != 55 || s.PosterBadgeSize != 130 {
		t.Errorf("stored values replaced: alpha %d size %d", s.PosterBadgeAlpha, s.PosterBadgeSize)
	}
}
