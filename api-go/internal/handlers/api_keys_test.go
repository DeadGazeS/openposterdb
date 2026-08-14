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
	"strings"
	"testing"

	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"

	_ "modernc.org/sqlite"
)

const apiKeySettingsTestSchema = `
CREATE TABLE IF NOT EXISTS admin_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	prefs TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
	encrypted_key TEXT,
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
	backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0,
	colors TEXT NOT NULL DEFAULT ''
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
		APIKeyID:               1,
		ImageSource:            "t",
		Lang:                   "en",
		RatingsLimit:           7,
		LogoRatingsLimit:       9,
		BackdropRatingsLimit:   2,
		EpisodeRatingsLimit:    4,
		PosterBadgeDirection:   "h",
		BackdropBadgeDirection: "v",
		EpisodeBadgeDirection:  "h",
		PosterBadgeStyle:       "lr",
		PosterLabelStyle:       "o",
		PosterBadgeShape:       "r",
		PosterBadgeAlpha:       80,
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

// TestKeySettingsUpdateMerge_BadgeSizePreservation (#10.1 regression): a partial
// payload that omits the four `*_badge_size` fields must preserve the stored
// value (which the web client always sends today, but the #10.1 fire was a
// first-save on a row with no stored value — base defaults from the effective
// settings carry the user's intended size through). Explicit sizes still win.
func TestKeySettingsUpdateMerge_BadgeSizePreservation(t *testing.T) {
	base := &services.APIKeySettings{
		PosterBadgeSize:   130,
		LogoBadgeSize:     120,
		BackdropBadgeSize: 110,
		EpisodeBadgeSize:  90,
	}

	// Payload omitting all four badge_sizes — must keep base values.
	var body keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"lang":"de"}`), &body); err != nil {
		t.Fatal(err)
	}
	merged := mergeKeySettingsUpdate(base, &body)
	if merged.PosterBadgeSize != 130 || merged.LogoBadgeSize != 120 ||
		merged.BackdropBadgeSize != 110 || merged.EpisodeBadgeSize != 90 {
		t.Errorf("omitted badge_sizes not preserved: got %d/%d/%d/%d, want 130/120/110/90",
			merged.PosterBadgeSize, merged.LogoBadgeSize, merged.BackdropBadgeSize, merged.EpisodeBadgeSize)
	}

	// Explicit badge_size override wins over base.
	var body2 keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"poster_badge_size":75,"episode_badge_size":50}`), &body2); err != nil {
		t.Fatal(err)
	}
	merged2 := mergeKeySettingsUpdate(base, &body2)
	if merged2.PosterBadgeSize != 75 {
		t.Errorf("explicit poster_badge_size=%d, want 75", merged2.PosterBadgeSize)
	}
	if merged2.LogoBadgeSize != 120 || merged2.BackdropBadgeSize != 110 {
		t.Errorf("non-overridden sizes not preserved: got %d/%d, want 120/110",
			merged2.LogoBadgeSize, merged2.BackdropBadgeSize)
	}
	if merged2.EpisodeBadgeSize != 50 {
		t.Errorf("explicit episode_badge_size=%d, want 50", merged2.EpisodeBadgeSize)
	}
}

// TestKeySettingsUpdateMerge_PointerFieldsPreserveOnOmit (#10.2 latent bug):
// every numeric/bool field not declared as a pointer in keySettingsUpdate
// would zero-out stored values when the web client omitted it. After pointer-
// ifying all numeric/bool fields, every omitted numeric/bool field must keep
// the base value. Explicit values still win.
func TestKeySettingsUpdateMerge_PointerFieldsPreserveOnOmit(t *testing.T) {
	base := &services.APIKeySettings{
		Textless:           true,
		EpisodeBlur:        true,
		PosterTextSize:     130,
		LogoTextSize:       120,
		BackdropTextSize:   110,
		EpisodeTextSize:    100,
		PosterLogoSize:     140,
		LogoLogoSize:       125,
		BackdropLogoSize:   115,
		EpisodeLogoSize:    105,
		PosterBadgeAlpha:   75,
		LogoBadgeAlpha:     70,
		BackdropBadgeAlpha: 65,
		EpisodeBadgeAlpha:  60,
		PosterBadgeWidth:   120,
		PosterBadgeHeight:  140,
		LogoBadgeWidth:     115,
		LogoBadgeHeight:    135,
		BackdropBadgeWidth: 110,
		BackdropBadgeHeight: 130,
		EpisodeBadgeWidth:  105,
		EpisodeBadgeHeight: 125,
		BackdropEdgeInsetX: 25,
		BackdropEdgeInsetY: 15,
	}

	// Payload omitting every pointer-preserved field — only lang + colors
	// supplied. Every preserved field must keep base value.
	var body keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"lang":"de","colors":"{}"}`), &body); err != nil {
		t.Fatal(err)
	}
	merged := mergeKeySettingsUpdate(base, &body)

	check := func(name string, got, want int32) {
		t.Helper()
		if got != want {
			t.Errorf("%s=%d, want %d (base not preserved on omit)", name, got, want)
		}
	}
	checkBool := func(name string, got, want bool) {
		t.Helper()
		if got != want {
			t.Errorf("%s=%v, want %v (base not preserved on omit)", name, got, want)
		}
	}

	checkBool("Textless", merged.Textless, true)
	checkBool("EpisodeBlur", merged.EpisodeBlur, true)
	check("PosterTextSize", merged.PosterTextSize, 130)
	check("LogoTextSize", merged.LogoTextSize, 120)
	check("BackdropTextSize", merged.BackdropTextSize, 110)
	check("EpisodeTextSize", merged.EpisodeTextSize, 100)
	check("PosterLogoSize", merged.PosterLogoSize, 140)
	check("LogoLogoSize", merged.LogoLogoSize, 125)
	check("BackdropLogoSize", merged.BackdropLogoSize, 115)
	check("EpisodeLogoSize", merged.EpisodeLogoSize, 105)
	check("PosterBadgeAlpha", merged.PosterBadgeAlpha, 75)
	check("LogoBadgeAlpha", merged.LogoBadgeAlpha, 70)
	check("BackdropBadgeAlpha", merged.BackdropBadgeAlpha, 65)
	check("EpisodeBadgeAlpha", merged.EpisodeBadgeAlpha, 60)
	check("PosterBadgeWidth", merged.PosterBadgeWidth, 120)
	check("PosterBadgeHeight", merged.PosterBadgeHeight, 140)
	check("LogoBadgeWidth", merged.LogoBadgeWidth, 115)
	check("LogoBadgeHeight", merged.LogoBadgeHeight, 135)
	check("BackdropBadgeWidth", merged.BackdropBadgeWidth, 110)
	check("BackdropBadgeHeight", merged.BackdropBadgeHeight, 130)
	check("EpisodeBadgeWidth", merged.EpisodeBadgeWidth, 105)
	check("EpisodeBadgeHeight", merged.EpisodeBadgeHeight, 125)
	check("BackdropEdgeInsetX", merged.BackdropEdgeInsetX, 25)
	check("BackdropEdgeInsetY", merged.BackdropEdgeInsetY, 15)

	if merged.Lang != "de" {
		t.Errorf("lang=%q, want 'de'", merged.Lang)
	}

	// Explicit override still wins (e.g. Textless explicitly false).
	var body2 keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"textless":false,"poster_text_size":50}`), &body2); err != nil {
		t.Fatal(err)
	}
	merged2 := mergeKeySettingsUpdate(base, &body2)
	if merged2.Textless != false {
		t.Errorf("explicit Textless=false, got %v", merged2.Textless)
	}
	if merged2.PosterTextSize != 50 {
		t.Errorf("explicit PosterTextSize=50, got %d", merged2.PosterTextSize)
	}
	// Non-overridden fields still preserve base.
	if merged2.LogoTextSize != 120 {
		t.Errorf("LogoTextSize not preserved when only PosterTextSize was sent: got %d, want 120", merged2.LogoTextSize)
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

// TestKeySettingsColorsRoundTrip guards per-key colours end-to-end: a PUT with
// colours stores them (normalised), and the GET returns the effective colours
// including the key's overrides plus the fanart_available flag, matching the
// global settings GET shape.
func TestKeySettingsColorsRoundTrip(t *testing.T) {
	db := newHandlersTestDB(t)

	payload := `{
		"lang": "de",
		"colors": {"imdb": {"border": "#ff0000", "text": "#00ff00"}}
	}`
	rec := putKeySettings(t, db, 1, payload)
	if rec.Code != 200 {
		t.Fatalf("PUT returned %d: %s", rec.Code, rec.Body.String())
	}

	// Stored row keeps the compact JSON overrides.
	stored, err := services.GetAPIKeySettings(db, 1)
	if err != nil || stored == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if !strings.Contains(stored.Colors, `"imdb"`) || !strings.Contains(stored.Colors, `#ff0000`) {
		t.Errorf("stored colors=%q, want imdb border override", stored.Colors)
	}

	// GET returns the effective colours (global defaults filled) + overrides.
	req := httptest.NewRequest(http.MethodGet, "/api/keys/1/settings", nil)
	req.SetPathValue("id", "1")
	rec2 := httptest.NewRecorder()
	HandleGetKeySettings(db, true)(rec2, req)
	if rec2.Code != 200 {
		t.Fatalf("GET returned %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp perKeySettingsResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Colors["imdb"].Border != "#ff0000" {
		t.Errorf("effective imdb border=%q, want #ff0000", resp.Colors["imdb"].Border)
	}
	if resp.Colors["imdb"].Text != "#00ff00" {
		t.Errorf("effective imdb text=%q, want #00ff00", resp.Colors["imdb"].Text)
	}
	// Sources without overrides keep their defaults (filled in).
	if resp.Colors["mal"].Accent == "" {
		t.Error("unoverridden source should have default colours filled in")
	}
	if !resp.FanartAvailable {
		t.Error("fanart_available should be true when passed true")
	}
}

// TestKeySettingsColorsPreservedWhenOmitted guards partial-update semantics for
// colours: a PUT without a colors field keeps the stored colours.
func TestKeySettingsColorsPreservedWhenOmitted(t *testing.T) {
	db := newHandlersTestDB(t)
	seed := &services.APIKeySettings{
		APIKeyID:     1,
		ImageSource:  "t",
		Lang:         "en",
		RatingsLimit: 3,
		Colors:       `{"imdb":{"border":"#123456"}}`,
	}
	if err := services.UpsertAPIKeySettings(db, seed); err != nil {
		t.Fatal(err)
	}

	rec := putKeySettings(t, db, 1, `{"lang":"de"}`)
	if rec.Code != 200 {
		t.Fatalf("PUT returned %d: %s", rec.Code, rec.Body.String())
	}
	got, err := services.GetAPIKeySettings(db, 1)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if !strings.Contains(got.Colors, "#123456") {
		t.Errorf("colors=%q, want stored override preserved", got.Colors)
	}
}

// TestKeySettingsInvalidColors guards colours validation: unknown sources and
// invalid hex values are rejected with 400.
func TestKeySettingsInvalidColors(t *testing.T) {
	db := newHandlersTestDB(t)
	if err := services.UpsertAPIKeySettings(db, &services.APIKeySettings{APIKeyID: 1, ImageSource: "t", Lang: "en", RatingsLimit: 3}); err != nil {
		t.Fatal(err)
	}

	rec := putKeySettings(t, db, 1, `{"colors":{"bogus":{"border":"#ff0000"}}}`)
	if rec.Code != 400 {
		t.Errorf("unknown source returned %d, want 400", rec.Code)
	}
	rec = putKeySettings(t, db, 1, `{"colors":{"imdb":{"border":"not-a-color"}}}`)
	if rec.Code != 400 {
		t.Errorf("invalid hex returned %d, want 400", rec.Code)
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
