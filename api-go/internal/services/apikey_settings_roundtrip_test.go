package services

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// fillDistinct sets every field of an APIKeySettings (except APIKeyID) to a
// distinct non-zero value, so a round trip that drops or swaps any column
// is caught. Strings get "v_<json tag>", ints their index+1, bools true.
func fillDistinct(t *testing.T, s *APIKeySettings) {
	t.Helper()
	v := reflect.ValueOf(s).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		tag := v.Type().Field(i).Tag.Get("json")
		if tag == "api_key_id" {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString("v_" + tag)
		case reflect.Int32, reflect.Int64:
			f.SetInt(int64(i + 1))
		case reflect.Bool:
			f.SetBool(true)
		default:
			t.Fatalf("unhandled kind %s for %s", f.Kind(), tag)
		}
	}
}

// TestAPIKeySettings_FullRoundTrip locks the per-key SQL column lists: every
// struct field survives Upsert → Get unchanged, and an update overwrites
// every column.
func TestAPIKeySettings_FullRoundTrip(t *testing.T) {
	db := newExportTestDB(t)
	if _, err := db.Exec("INSERT INTO api_keys (id, name, key_hash, key_prefix, created_by) VALUES (7, 'k', 'h', 'p', 1)"); err != nil {
		t.Fatal(err)
	}
	want := APIKeySettings{APIKeyID: 7}
	fillDistinct(t, &want)
	if err := UpsertAPIKeySettings(db, &want); err != nil {
		t.Fatal(err)
	}
	got, err := GetAPIKeySettings(db, 7)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", *got, want)
	}

	// Update path (ON CONFLICT): every column must be overwritten.
	upd := APIKeySettings{APIKeyID: 7, UseKitsu: false}
	fillDistinct(t, &upd)
	v := reflect.ValueOf(&upd).Elem()
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.String {
			f.SetString(f.String() + "_2")
		} else if f.Kind() == reflect.Int32 {
			f.SetInt(f.Int() + 100)
		} else if f.Kind() == reflect.Bool {
			f.SetBool(false)
		}
	}
	if err := UpsertAPIKeySettings(db, &upd); err != nil {
		t.Fatal(err)
	}
	got, err = GetAPIKeySettings(db, 7)
	if err != nil || got == nil {
		t.Fatalf("get after update: %v %v", got, err)
	}
	if !reflect.DeepEqual(*got, upd) {
		t.Fatalf("update mismatch:\n got %+v\nwant %+v", *got, upd)
	}
}

// effectiveProbe returns valid RenderSettings in which every per-key field
// differs from DefaultRenderSettings. TestEffectiveSettings_CarriesEveryPerKeyField
// fails if a per-key column is left at its default here — so a new per-key
// setting has to be added to this probe, which then checks that
// GetEffectiveRenderSettingsCtx maps it.
func effectiveProbe() RenderSettings {
	s := DefaultRenderSettings()
	lay := func(kind string) ImageLayout {
		d := DefaultLayout(kind)
		return UnmarshalLayout(`{"top":{"per_row":2,"rows":1,"start":"l"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["top","bottom","left","right"]}`, &d)
	}
	s.ImageSource = ImageSourceFanart
	s.Lang = "de"
	s.Textless = true
	s.RatingsLimit, s.LogoRatingsLimit, s.BackdropRatingsLimit, s.EpisodeRatingsLimit = 2, 4, 6, 3
	s.RatingsOrder = "imdb,tmdb"
	s.RatingsExclude = "rt"
	s.PosterLayout, s.LogoLayout, s.BackdropLayout, s.EpisodeLayout = lay("poster"), lay("logo"), lay("backdrop"), lay("episode")
	s.PosterBadgeStyle, s.LogoBadgeStyle, s.BackdropBadgeStyle, s.EpisodeBadgeStyle = BadgeStyleValueLeftLogoRight, BadgeStyleValueTB, BadgeStyleLogoLeftValueRight, BadgeStyleValueTB
	s.PosterLabelStyle, s.LogoLabelStyle, s.BackdropLabelStyle, s.EpisodeLabelStyle = LabelStyleText, LabelStyleIcon, LabelStyleHighRes, LabelStyleText
	s.PosterBadgeDirection, s.BackdropBadgeDirection, s.EpisodeBadgeDirection = BadgeDirectionVertical, BadgeDirectionHorizontal, BadgeDirectionHorizontal
	s.PosterFit = PosterFitBlur
	s.PosterTextSize, s.LogoTextSize, s.BackdropTextSize, s.EpisodeTextSize = 110, 120, 130, 140
	s.PosterBadgeSize, s.LogoBadgeSize, s.BackdropBadgeSize, s.EpisodeBadgeSize = 150, 160, 170, 180
	s.PosterBadgeWidth, s.PosterBadgeHeight = 190, 200
	s.LogoBadgeWidth, s.LogoBadgeHeight = 210, 220
	s.BackdropBadgeWidth, s.BackdropBadgeHeight = 230, 240
	s.EpisodeBadgeWidth, s.EpisodeBadgeHeight = 250, 260
	s.PosterLogoSize, s.LogoLogoSize, s.BackdropLogoSize, s.EpisodeLogoSize = 270, 280, 290, 300
	s.BackdropEdgeInsetX, s.BackdropEdgeInsetY = 5, 7
	s.EpisodeBlur = true
	s.PosterBadgeShape, s.LogoBadgeShape, s.BackdropBadgeShape, s.EpisodeBadgeShape = BadgeShapePill, BadgeShapePill, BadgeShapePill, BadgeShapePill
	s.PosterBadgeAlpha, s.LogoBadgeAlpha, s.BackdropBadgeAlpha, s.EpisodeBadgeAlpha = 10, 20, 30, 40
	s.UseKitsu, s.UseMAL = false, false
	s.AnimeArtwork = AnimeArtworkMAL
	return s
}

func jsonFields(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestEffectiveSettings_CarriesEveryPerKeyField guards the one per-key
// mapping that stays hand-written (GetEffectiveRenderSettingsCtx's struct
// literal): every stored per-key column must reach the effective settings.
// A forgotten field would silently render with its zero value.
func TestEffectiveSettings_CarriesEveryPerKeyField(t *testing.T) {
	probe := effectiveProbe()
	def, want := jsonFields(t, DefaultRenderSettings()), jsonFields(t, probe)
	for _, col := range APIKeySettingsColumns() {
		if col == "api_key_id" || col == "colors" { // colors merge with globals; covered elsewhere
			continue
		}
		if _, ok := want[col]; !ok {
			t.Errorf("per-key column %q has no RenderSettings field with the same json tag", col)
			continue
		}
		if reflect.DeepEqual(def[col], want[col]) {
			t.Errorf("effectiveProbe: set a non-default value for %q", col)
		}
	}

	// Store the probe as a per-key row exactly the way the settings handler
	// derives one (RenderSettings JSON → APIKeySettings), then read it back.
	db := newExportTestDB(t)
	if _, err := db.Exec("INSERT INTO api_keys (id, name, key_hash, key_prefix, created_by) VALUES (7, 'k', 'h', 'p', 1)"); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(probe)
	var row APIKeySettings
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	row.APIKeyID = 7
	if err := UpsertAPIKeySettings(db, &row); err != nil {
		t.Fatal(err)
	}
	globals := DefaultRenderSettings()
	got := jsonFields(t, GetEffectiveRenderSettings(db, 7, &globals))
	for _, col := range APIKeySettingsColumns() {
		if col == "api_key_id" || col == "colors" {
			continue
		}
		if !reflect.DeepEqual(got[col], want[col]) {
			t.Errorf("per-key %q not carried into the effective settings: got %v, want %v", col, got[col], want[col])
		}
	}
}

// TestGlobalSettings_HandWrittenPathsCoverEveryField guards the two
// hand-written global paths — the class behind the 2026-09-24 Kitsu/MAL
// toggle bug, where ParseGlobalRenderSettings and SettingsResponseMap both
// forgot use_kitsu/use_mal: every RenderSettings field must survive
// RenderSettingsToMap → ParseGlobalRenderSettings and appear in
// SettingsResponseMap.
func TestGlobalSettings_HandWrittenPathsCoverEveryField(t *testing.T) {
	probe := effectiveProbe()
	want := jsonFields(t, probe)
	got := jsonFields(t, ParseGlobalRenderSettings(RenderSettingsToMap(&probe)))
	resp := SettingsResponseMap(&probe)
	for tag := range want {
		if tag == "is_default" || tag == "colors" { // not stored as a global key / flattened separately
			continue
		}
		if !reflect.DeepEqual(got[tag], want[tag]) {
			t.Errorf("ParseGlobalRenderSettings drops %q: got %v, want %v", tag, got[tag], want[tag])
		}
		if _, ok := resp[tag]; !ok {
			t.Errorf("SettingsResponseMap is missing %q", tag)
		}
	}
}

// apiKeySettingsTestDDL builds the api_key_settings table for services tests
// (which can't import internal/app for the real bootstrap — import cycle)
// from the same column list the per-key SQL uses, so a new field needs no
// test-schema edit. Production columns come from migrations and are checked
// against this list by app's TestFullBootstrap_AllTableColumnsPresent.
func apiKeySettingsTestDDL() string {
	t := reflect.TypeOf(APIKeySettings{})
	cols := []string{"api_key_id INTEGER PRIMARY KEY"}
	for i := 1; i < t.NumField(); i++ {
		name := apiKeySettingsColumns[i]
		switch t.Field(i).Type.Kind() {
		case reflect.String:
			cols = append(cols, name+" TEXT NOT NULL DEFAULT ''")
		default: // int32 / int64 / bool
			cols = append(cols, name+" INTEGER NOT NULL DEFAULT 0")
		}
	}
	return "\nCREATE TABLE IF NOT EXISTS api_key_settings (\n\t" + strings.Join(cols, ",\n\t") + "\n);\n"
}

// TestOverlayAPIKeySettingsNonZero_EveryField: for old-version imports, every
// non-zero imported field overwrites the stored one and every zero keeps it —
// for all fields, including ones added after the export format (the old
// hand-written overlay had silently missed use_kitsu / use_mal /
// anime_artwork).
func TestOverlayAPIKeySettingsNonZero_EveryField(t *testing.T) {
	base := APIKeySettings{APIKeyID: 7}
	fillDistinct(t, &base)
	if got := overlayAPIKeySettingsNonZero(&base, &APIKeySettings{APIKeyID: 9}); !reflect.DeepEqual(*got, base) {
		t.Errorf("all-zero import changed the stored row:\n got %+v\nwant %+v", *got, base)
	}
	over := APIKeySettings{APIKeyID: 9}
	fillDistinct(t, &over)
	v := reflect.ValueOf(&over).Elem()
	for i := 1; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.String {
			f.SetString(f.String() + "_imported")
		} else if f.Kind() == reflect.Int32 {
			f.SetInt(f.Int() + 1000)
		}
	}
	want := over
	want.APIKeyID = 7
	if got := overlayAPIKeySettingsNonZero(&base, &over); !reflect.DeepEqual(*got, want) {
		t.Errorf("non-zero import not applied to every field:\n got %+v\nwant %+v", *got, want)
	}
}
