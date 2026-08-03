package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"

	_ "modernc.org/sqlite"
)

const apiKeySettingsTestSchema = `
CREATE TABLE IF NOT EXISTS global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
	created_by INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT
);
CREATE TABLE IF NOT EXISTS api_key_settings (
	api_key_id INTEGER PRIMARY KEY,
	image_source TEXT NOT NULL DEFAULT 't',
	lang TEXT NOT NULL DEFAULT 'en',
	textless INTEGER NOT NULL DEFAULT 0,
	ratings_limit INTEGER NOT NULL DEFAULT 3,
	ratings_order TEXT NOT NULL DEFAULT 'mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert',
	ratings_exclude TEXT NOT NULL DEFAULT '',
	poster_layout TEXT NOT NULL DEFAULT '{"bottom":{"per_row":3,"rows":1,"start":"c"}}',
	logo_ratings_limit INTEGER NOT NULL DEFAULT 5,
	backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
	poster_badge_style TEXT NOT NULL DEFAULT 'h',
	logo_badge_style TEXT NOT NULL DEFAULT 'v',
	backdrop_badge_style TEXT NOT NULL DEFAULT 'v',
	poster_label_style TEXT NOT NULL DEFAULT 'i',
	logo_label_style TEXT NOT NULL DEFAULT 'i',
	backdrop_label_style TEXT NOT NULL DEFAULT 'i',
	poster_badge_direction TEXT NOT NULL DEFAULT 'd',
	poster_fit TEXT NOT NULL DEFAULT 'native',
	poster_text_size INTEGER NOT NULL DEFAULT 100,
	logo_text_size INTEGER NOT NULL DEFAULT 100,
	backdrop_text_size INTEGER NOT NULL DEFAULT 100,
	poster_badge_size INTEGER NOT NULL DEFAULT 100,
	logo_badge_size INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_size INTEGER NOT NULL DEFAULT 100,
	poster_badge_width INTEGER NOT NULL DEFAULT 100,
	poster_badge_height INTEGER NOT NULL DEFAULT 100,
	logo_badge_width INTEGER NOT NULL DEFAULT 100,
	logo_badge_height INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_width INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_height INTEGER NOT NULL DEFAULT 100,
	episode_badge_width INTEGER NOT NULL DEFAULT 100,
	episode_badge_height INTEGER NOT NULL DEFAULT 100,
	poster_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_logo_size INTEGER NOT NULL DEFAULT 100,
	backdrop_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_layout TEXT NOT NULL DEFAULT '{"bottom":{"per_row":5,"rows":1,"start":"c"}}',
	backdrop_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":5,"rows":1,"start":"r"}}',
	backdrop_badge_direction TEXT NOT NULL DEFAULT 'd',
	episode_ratings_limit INTEGER NOT NULL DEFAULT 1,
	episode_badge_style TEXT NOT NULL DEFAULT 'v',
	episode_label_style TEXT NOT NULL DEFAULT 'o',
	episode_text_size INTEGER NOT NULL DEFAULT 100,
	episode_badge_size INTEGER NOT NULL DEFAULT 100,
	episode_logo_size INTEGER NOT NULL DEFAULT 100,
	episode_layout TEXT NOT NULL DEFAULT '{"right":{"per_row":1,"rows":1,"start":"t"}}',
	episode_badge_direction TEXT NOT NULL DEFAULT 'v',
	episode_blur INTEGER NOT NULL DEFAULT 0,
	poster_badge_shape TEXT NOT NULL DEFAULT 'r',
	logo_badge_shape TEXT NOT NULL DEFAULT 'r',
	backdrop_badge_shape TEXT NOT NULL DEFAULT 'r',
	episode_badge_shape TEXT NOT NULL DEFAULT 'r',
	poster_badge_alpha INTEGER NOT NULL DEFAULT 80,
	logo_badge_alpha INTEGER NOT NULL DEFAULT 80,
	backdrop_badge_alpha INTEGER NOT NULL DEFAULT 80,
	episode_badge_alpha INTEGER NOT NULL DEFAULT 80,
	backdrop_edge_inset_x INTEGER NOT NULL DEFAULT 0,
	backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0
);
`

func newHandlersTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(apiKeySettingsTestSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// fullKeySettingsPayload is a realistic key-settings PUT body mirroring what
// the web client sends: every required field, but the seven optional fields
// (four ratings limits and three badge directions) omitted.
const fullKeySettingsPayload = `{
	"image_source": "t",
	"lang": "de",
	"textless": false,
	"ratings_order": "imdb,tmdb,rt",
	"ratings_exclude": "",
	"poster_layout": {"bottom":{"per_row":2,"rows":1,"start":"c"},"order":["bottom"]},
	"poster_badge_style": "tb",
	"logo_badge_style": "v",
	"backdrop_badge_style": "v",
	"poster_label_style": "t",
	"logo_label_style": "t",
	"backdrop_label_style": "t",
	"poster_fit": "native",
	"poster_text_size": 100,
	"logo_text_size": 100,
	"backdrop_text_size": 100,
	"poster_badge_size": 100,
	"logo_badge_size": 100,
	"backdrop_badge_size": 100,
	"poster_logo_size": 100,
	"logo_logo_size": 100,
	"backdrop_logo_size": 100,
	"logo_layout": {"bottom":{"per_row":5,"rows":1,"start":"c"},"order":["bottom"]},
	"backdrop_layout": {"top":{"per_row":5,"rows":1,"start":"r"},"order":["top"]},
	"backdrop_edge_inset_x": 0,
	"backdrop_edge_inset_y": 0,
	"episode_badge_style": "v",
	"episode_label_style": "t",
	"episode_text_size": 100,
	"episode_badge_size": 100,
	"episode_logo_size": 100,
	"episode_layout": {"right":{"per_row":1,"rows":1,"start":"t"},"order":["right"]},
	"episode_blur": false,
	"poster_badge_shape": "r",
	"logo_badge_shape": "r",
	"backdrop_badge_shape": "r",
	"episode_badge_shape": "r",
	"poster_badge_alpha": 80,
	"logo_badge_alpha": 80,
	"backdrop_badge_alpha": 80,
	"episode_badge_alpha": 80
}`

func putKeySettings(t *testing.T, db *sql.DB, id int64, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleUpdateKeySettings(db)(rec, req)
	return rec
}

// TestUpdateKeySettingsPreservesOmittedFields guards the partial-update
// semantics: a PUT that omits the optional ratings_limit and badge_direction
// fields must preserve the stored values instead of zeroing them.
func TestUpdateKeySettingsPreservesOmittedFields(t *testing.T) {
	db := newHandlersTestDB(t)

	seed := &services.APIKeySettings{
		APIKeyID:             1,
		ImageSource:          "t",
		Lang:                 "en",
		RatingsLimit:         7,
		LogoRatingsLimit:     9,
		BackdropRatingsLimit: 2,
		EpisodeRatingsLimit:  4,
		PosterBadgeDirection: "h",
		BackdropBadgeDirection: "v",
		EpisodeBadgeDirection:  "h",
		PosterBadgeStyle:      "lr",
		PosterLabelStyle:      "o",
		PosterBadgeShape:      "r",
		PosterBadgeAlpha:      80,
	}
	if err := services.UpsertAPIKeySettings(db, seed); err != nil {
		t.Fatal(err)
	}

	rec := putKeySettings(t, db, 1, fullKeySettingsPayload)
	if rec.Code != 200 {
		t.Fatalf("PUT returned %d: %s", rec.Code, rec.Body.String())
	}

	got, err := services.GetAPIKeySettings(db, 1)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	// Omitted optional fields keep their stored values.
	if got.RatingsLimit != 7 {
		t.Errorf("ratings_limit=%d, want stored 7 (omitted field must be preserved)", got.RatingsLimit)
	}
	if got.LogoRatingsLimit != 9 {
		t.Errorf("logo_ratings_limit=%d, want stored 9", got.LogoRatingsLimit)
	}
	if got.BackdropRatingsLimit != 2 {
		t.Errorf("backdrop_ratings_limit=%d, want stored 2", got.BackdropRatingsLimit)
	}
	if got.EpisodeRatingsLimit != 4 {
		t.Errorf("episode_ratings_limit=%d, want stored 4", got.EpisodeRatingsLimit)
	}
	if got.PosterBadgeDirection != "h" {
		t.Errorf("poster_badge_direction=%q, want stored 'h'", got.PosterBadgeDirection)
	}
	if got.BackdropBadgeDirection != "v" {
		t.Errorf("backdrop_badge_direction=%q, want stored 'v'", got.BackdropBadgeDirection)
	}
	if got.EpisodeBadgeDirection != "h" {
		t.Errorf("episode_badge_direction=%q, want stored 'h'", got.EpisodeBadgeDirection)
	}
	// Provided fields are applied (and enums normalised).
	if got.Lang != "de" {
		t.Errorf("lang=%q, want 'de'", got.Lang)
	}
	if got.PosterBadgeStyle != "tb" {
		t.Errorf("poster_badge_style=%q, want 'tb'", got.PosterBadgeStyle)
	}
}

// TestUpdateKeySettingsHonorsExplicitZero guards that an explicit 0 for a
// ratings limit is a legitimate value and must be stored (the serve path uses
// layout totals anyway, so 0 just means "no badges").
func TestUpdateKeySettingsHonorsExplicitZero(t *testing.T) {
	db := newHandlersTestDB(t)
	seed := &services.APIKeySettings{APIKeyID: 1, ImageSource: "t", Lang: "en", RatingsLimit: 7}
	if err := services.UpsertAPIKeySettings(db, seed); err != nil {
		t.Fatal(err)
	}

	payload := `{"lang":"de","ratings_limit":0}`
	rec := putKeySettings(t, db, 1, payload)
	if rec.Code != 200 {
		t.Fatalf("PUT returned %d: %s", rec.Code, rec.Body.String())
	}
	got, err := services.GetAPIKeySettings(db, 1)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if got.RatingsLimit != 0 {
		t.Errorf("ratings_limit=%d, want explicit 0", got.RatingsLimit)
	}
}

// TestUpdateKeySettingsValidation guards the input validation: garbage
// ratings_order is rejected, enum values are normalised, and numeric ranges
// are clamped like the global path.
func TestUpdateKeySettingsValidation(t *testing.T) {
	db := newHandlersTestDB(t)
	seed := &services.APIKeySettings{APIKeyID: 1, ImageSource: "t", Lang: "en", RatingsLimit: 3}
	if err := services.UpsertAPIKeySettings(db, seed); err != nil {
		t.Fatal(err)
	}

	// Unknown rating source key -> 400.
	rec := putKeySettings(t, db, 1, `{"lang":"de","ratings_order":"imdb,bogus"}`)
	if rec.Code != 400 {
		t.Errorf("invalid ratings_order returned %d, want 400", rec.Code)
	}

	// Invalid lang -> 400.
	rec = putKeySettings(t, db, 1, `{"lang":"e"}`)
	if rec.Code != 400 {
		t.Errorf("invalid lang returned %d, want 400", rec.Code)
	}

	// Garbage enums are normalised (not rejected) and numeric ranges clamped.
	rec = putKeySettings(t, db, 1, `{"lang":"de","poster_badge_style":"garbage","poster_badge_shape":"garbage","poster_badge_alpha":999,"backdrop_edge_inset_x":999}`)
	if rec.Code != 200 {
		t.Fatalf("normalising PUT returned %d: %s", rec.Code, rec.Body.String())
	}
	got, err := services.GetAPIKeySettings(db, 1)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if got.PosterBadgeStyle != "d" {
		t.Errorf("poster_badge_style=%q, want normalised 'd'", got.PosterBadgeStyle)
	}
	if got.PosterBadgeShape != "r" {
		t.Errorf("poster_badge_shape=%q, want normalised 'r'", got.PosterBadgeShape)
	}
	if got.PosterBadgeAlpha != 100 {
		t.Errorf("poster_badge_alpha=%d, want clamped 100", got.PosterBadgeAlpha)
	}
	if got.BackdropEdgeInsetX != 50 {
		t.Errorf("backdrop_edge_inset_x=%d, want clamped 50", got.BackdropEdgeInsetX)
	}
}

// TestKeySettingsUpdateMerge unit-tests the overlay logic: omitted optional
// fields keep the stored value, explicit 0 for a limit is honoured, and a
// provided badge direction wins.
func TestKeySettingsUpdateMerge(t *testing.T) {
	base := &services.APIKeySettings{
		RatingsLimit:           7,
		LogoRatingsLimit:       9,
		BackdropRatingsLimit:   2,
		EpisodeRatingsLimit:    4,
		PosterBadgeDirection:   "h",
		BackdropBadgeDirection: "v",
		EpisodeBadgeDirection:  "h",
		Lang:                   "en",
	}

	var body keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"lang":"de"}`), &body); err != nil {
		t.Fatal(err)
	}
	merged := mergeKeySettingsUpdate(base, &body)
	if merged.RatingsLimit != 7 || merged.LogoRatingsLimit != 9 || merged.BackdropRatingsLimit != 2 || merged.EpisodeRatingsLimit != 4 {
		t.Errorf("omitted limits not preserved: %+v", merged)
	}
	if merged.PosterBadgeDirection != "h" || merged.BackdropBadgeDirection != "v" || merged.EpisodeBadgeDirection != "h" {
		t.Errorf("omitted directions not preserved: %+v", merged)
	}
	if merged.Lang != "de" {
		t.Errorf("lang=%q, want 'de'", merged.Lang)
	}

	var body2 keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"ratings_limit":0}`), &body2); err != nil {
		t.Fatal(err)
	}
	merged2 := mergeKeySettingsUpdate(base, &body2)
	if merged2.RatingsLimit != 0 {
		t.Errorf("explicit zero limit=%d, want 0", merged2.RatingsLimit)
	}

	var body3 keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"poster_badge_direction":"v"}`), &body3); err != nil {
		t.Fatal(err)
	}
	merged3 := mergeKeySettingsUpdate(base, &body3)
	if merged3.PosterBadgeDirection != "v" {
		t.Errorf("provided direction=%q, want 'v'", merged3.PosterBadgeDirection)
	}
}

// TestValidateAndNormalizeKeySettings unit-tests the validation/normalisation
// pass: valid rows pass, garbage enums are normalised, ranges are clamped, and
// invalid ratings_order/lang are rejected.
func TestValidateAndNormalizeKeySettings(t *testing.T) {
	valid := &services.APIKeySettings{
		Lang:                 "de",
		RatingsOrder:         "imdb,tmdb",
		RatingsLimit:         5,
		LogoRatingsLimit:     5,
		BackdropRatingsLimit: 5,
		EpisodeRatingsLimit:  1,
		PosterBadgeStyle:     "garbage",
		PosterBadgeShape:     "garbage",
		PosterBadgeAlpha:     999,
		BackdropEdgeInsetX:   999,
		PosterLayout:         `{"bottom":{"per_row":3,"rows":1,"start":"c"},"order":["bottom"]}`,
	}
	if err := validateAndNormalizeKeySettings(valid); err != nil {
		t.Fatalf("valid row rejected: %v", err)
	}
	if valid.PosterBadgeStyle != "d" {
		t.Errorf("poster_badge_style=%q, want normalised 'd'", valid.PosterBadgeStyle)
	}
	if valid.PosterBadgeShape != "r" {
		t.Errorf("poster_badge_shape=%q, want normalised 'r'", valid.PosterBadgeShape)
	}
	if valid.PosterBadgeAlpha != 100 {
		t.Errorf("poster_badge_alpha=%d, want clamped 100", valid.PosterBadgeAlpha)
	}
	if valid.BackdropEdgeInsetX != 50 {
		t.Errorf("backdrop_edge_inset_x=%d, want clamped 50", valid.BackdropEdgeInsetX)
	}

	if err := validateAndNormalizeKeySettings(&services.APIKeySettings{Lang: "en", RatingsLimit: 5, RatingsOrder: "imdb,bogus"}); err == nil {
		t.Error("invalid ratings_order should be rejected")
	}
	if err := validateAndNormalizeKeySettings(&services.APIKeySettings{Lang: "e", RatingsLimit: 5}); err == nil {
		t.Error("invalid lang should be rejected")
	}
	if err := validateAndNormalizeKeySettings(&services.APIKeySettings{Lang: "en", RatingsLimit: 11}); err == nil {
		t.Error("ratings_limit > 10 should be rejected")
	}
}

// TestGalleryBadgeUsesPosterStyle guards the badge gallery (Rating-Colours):
// the gallery badge must mirror the poster preview, so a vertical poster badge
// style (tb) renders a vertical badge, not the horizontal default.
func TestGalleryBadgeUsesPosterStyle(t *testing.T) {
	if err := appimg.LoadFont("../../assets/fonts/Inter-Bold.ttf"); err != nil {
		t.Skipf("font asset not available: %v", err)
	}

	render := func(style string) (int, int) {
		db := newHandlersTestDB(t)
		if err := services.SetGlobalSetting(db, "poster_badge_style", style); err != nil {
			t.Fatal(err)
		}
		if err := services.SetGlobalSetting(db, "poster_label_style", "t"); err != nil {
			t.Fatal(err)
		}
		if err := services.SetGlobalSetting(db, "poster_badge_alpha", "100"); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/?source=imdb&value=10.0", nil)
		rec := httptest.NewRecorder()
		HandleBadgePreview(db)(rec, req)
		if rec.Code != 200 {
			t.Fatalf("badge preview for style %q returned %d: %s", style, rec.Code, rec.Body.String())
		}
		img, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("decode badge PNG for style %q: %v", style, err)
		}
		b := img.Bounds()
		return b.Dx(), b.Dy()
	}

	w, h := render("tb")
	if h <= w {
		t.Errorf("tb gallery badge %dx%d: want a vertical (taller) badge", w, h)
	}
	w, h = render("lr")
	if w <= h {
		t.Errorf("lr gallery badge %dx%d: want a horizontal (wider) badge", w, h)
	}
}
