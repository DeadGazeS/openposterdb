package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"openposterdb/internal/services"
)

func TestHashRefreshTokenDeterministic(t *testing.T) {
	a := HashRefreshToken("token123")
	b := HashRefreshToken("token123")
	if a != b {
		t.Error("same input should produce same hash")
	}
}

func TestHashRefreshTokenDifferent(t *testing.T) {
	a := HashRefreshToken("token_a")
	b := HashRefreshToken("token_b")
	if a == b {
		t.Error("different inputs should produce different hashes")
	}
}

func TestHashAPIKeyDeterministic(t *testing.T) {
	a := services.HashAPIKey("testkey123")
	b := services.HashAPIKey("testkey123")
	if a != b {
		t.Error("same input should produce same hash")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	raw, hash, prefix := services.GenerateAPIKey()
	if len(raw) != 64 {
		t.Errorf("raw key should be 64 chars, got %d", len(raw))
	}
	if len(prefix) != 8 {
		t.Errorf("prefix should be 8 chars, got %d", len(prefix))
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if services.HashAPIKey(raw) != hash {
		t.Error("hash should match")
	}
	if raw[:8] != prefix {
		t.Error("prefix should match first 8 chars")
	}
}

func TestHashPassword(t *testing.T) {
	h1, err := services.HashPassword("testpassword")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := services.HashPassword("testpassword")
	// Passwords should hash differently due to random salt
	if h1 == h2 {
		t.Error("same password should produce different hashes due to salt")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, _ := services.HashPassword("correctpass")
	ok, _ := services.VerifyPassword("correctpass", hash)
	if !ok {
		t.Error("correct password should verify")
	}
	ok, _ = services.VerifyPassword("wrongpass", hash)
	if ok {
		t.Error("wrong password should not verify")
	}
}

func TestRefreshCookieSecure(t *testing.T) {
	c := RefreshCookie("abc123", 604800, true)
	if c.Value != "abc123" {
		t.Error("wrong cookie value")
	}
	if c.Path != "/api/auth/refresh" {
		t.Error("wrong path")
	}
	if c.MaxAge != 604800 {
		t.Error("wrong maxAge")
	}
	if !c.Secure {
		t.Error("should be secure")
	}
	if !c.HttpOnly {
		t.Error("should be HttpOnly")
	}
}

func TestRefreshCookieInsecure(t *testing.T) {
	c := RefreshCookie("abc123", 604800, false)
	if c.Secure {
		t.Error("should not be secure")
	}
}

func TestImageQueryHasOverrides(t *testing.T) {
	q := &ImageQuery{}
	if q.HasOverrides() {
		t.Error("empty query should not have overrides")
	}

	q.RatingsLimit = new(int32)
	*q.RatingsLimit = 5
	if !q.HasOverrides() {
		t.Error("query with ratings_limit should have overrides")
	}
}

func TestApplyQueryOverridesNoOverridesReturnsSame(t *testing.T) {
	s := services.DefaultRenderSettings()
	result := applyQueryOverrides(&s, &ImageQuery{}, "poster")
	if result.RatingsLimit != s.RatingsLimit {
		t.Error("no overrides should preserve settings")
	}
}

func TestApplyQueryOverridesPosterMapping(t *testing.T) {
	s := services.DefaultRenderSettings()
	q := &ImageQuery{}
	limit := int32(3)
	style := "h"
	label := "i"
	layout := `{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":2,"rows":2,"start":"l"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom"]}`
	q.RatingsLimit = &limit
	q.BadgeStyle = &style
	q.LabelStyle = &label
	q.Layout = &layout

	result := applyQueryOverrides(&s, q, "poster")
	if result.RatingsLimit != 3 {
		t.Error("ratings_limit should be 3")
	}
	if result.PosterBadgeStyle != services.BadgeStyleLogoLeftValueRight {
		t.Error("should be LogoLeftValueRight")
	}
	if result.PosterLabelStyle != services.LabelStyleIcon {
		t.Error("should be Icon")
	}
	if result.PosterLayout.Bottom.PerRow != 2 || result.PosterLayout.Bottom.Rows != 2 || result.PosterLayout.Bottom.Start != "l" {
		t.Error("layout override not applied")
	}
}

func TestApplyQueryOverridesLogoIgnoresPosterOnly(t *testing.T) {
	s := services.DefaultRenderSettings()
	q := &ImageQuery{}
	layout := `{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":1,"rows":1,"start":"t"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["right"]}`
	q.Layout = &layout

	result := applyQueryOverrides(&s, q, "logo")
	if result.LogoLayout.Right.PerRow != 1 || result.LogoLayout.Right.Start != "t" {
		t.Error("layout should apply for logo")
	}
	if result.PosterLayout.Bottom.PerRow != s.PosterLayout.Bottom.PerRow {
		t.Error("poster layout should be untouched for logo")
	}
}

func TestParseImageQueryBadgeWidthHeight(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/preview/poster?badge_width=200&badge_height=50", nil)
	q := parseImageQuery(req)
	if q.BadgeWidth == nil || *q.BadgeWidth != 200 {
		t.Errorf("badge_width not parsed: got %v", q.BadgeWidth)
	}
	if q.BadgeHeight == nil || *q.BadgeHeight != 50 {
		t.Errorf("badge_height not parsed: got %v", q.BadgeHeight)
	}
	if !q.HasOverrides() {
		t.Error("badge_width/badge_height should count as overrides")
	}
}

func TestParseImageQueryBadgeWidthHeightAbsent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/preview/poster", nil)
	q := parseImageQuery(req)
	if q.BadgeWidth != nil || q.BadgeHeight != nil {
		t.Error("badge_width/badge_height should be nil when absent")
	}
}

func TestApplyQueryOverridesBadgeWidthHeight(t *testing.T) {
	s := services.DefaultRenderSettings()
	q := &ImageQuery{}
	w := int32(150)
	h := int32(80)
	q.BadgeWidth = &w
	q.BadgeHeight = &h

	result := applyQueryOverrides(&s, q, "poster")
	if result.PosterBadgeWidth != 150 || result.PosterBadgeHeight != 80 {
		t.Errorf("poster badge width/height: got %d/%d want 150/80", result.PosterBadgeWidth, result.PosterBadgeHeight)
	}

	other := applyQueryOverrides(&s, q, "logo")
	if other.LogoBadgeWidth != 150 || other.LogoBadgeHeight != 80 {
		t.Errorf("logo badge width/height: got %d/%d want 150/80", other.LogoBadgeWidth, other.LogoBadgeHeight)
	}
	if other.PosterBadgeWidth != s.PosterBadgeWidth {
		t.Error("poster settings should be untouched for logo kind")
	}
}

func TestSettingsResponseMap(t *testing.T) {
	s := services.DefaultRenderSettings()
	resp := services.SettingsResponseMap(&s)
	if resp["image_source"] != "t" {
		t.Errorf("wrong image_source: %v", resp["image_source"])
	}
	if resp["lang"] != "en" {
		t.Errorf("wrong lang: %v", resp["lang"])
	}
	if resp["ratings_limit"] != int32(3) {
		t.Errorf("wrong ratings_limit: %v", resp["ratings_limit"])
	}
}

// TestGlobalSettingsBadgeWidthHeightRoundTrip guards the global settings
// save/load path for the per-kind badge width/height fields: a PUT must persist
// them and a subsequent GET must return the saved values (regression: the
// admin handler dropped the fields, so slider changes never survived reload).
func TestGlobalSettingsBadgeWidthHeightRoundTrip(t *testing.T) {
	db := newHandlersTestDB(t)

	payload := `{
		"poster_badge_width": 150,
		"poster_badge_height": 80,
		"logo_badge_width": 120,
		"logo_badge_height": 90,
		"backdrop_badge_width": 110,
		"backdrop_badge_height": 70,
		"episode_badge_width": 130,
		"episode_badge_height": 60
	}`
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleUpdateSettings(db, false)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("PUT failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	HandleGetSettings(db, false, false, false)(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 {
		t.Fatalf("GET failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	checks := map[string]float64{
		"poster_badge_width":    150,
		"poster_badge_height":   80,
		"logo_badge_width":      120,
		"logo_badge_height":     90,
		"backdrop_badge_width":  110,
		"backdrop_badge_height": 70,
		"episode_badge_width":   130,
		"episode_badge_height":  60,
	}
	for key, want := range checks {
		got, ok := resp[key].(float64)
		if !ok {
			t.Errorf("%s missing from GET response", key)
			continue
		}
		if got != want {
			t.Errorf("%s: got %v want %v", key, got, want)
		}
	}
}

// TestUserPrefsRoundTrip guards per-user UI preference persistence: a PUT with
// an authenticated admin user stores prefs (merging with existing), a GET
// returns them, and the value survives across requests (login sessions).
func TestUserPrefsRoundTrip(t *testing.T) {
	db := newHandlersTestDB(t)

	// Seed an admin user (the prefs row is keyed by username).
	if _, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', 'x')"); err != nil {
		t.Fatal(err)
	}

	withUser := func(method, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, "/", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, "/", nil)
		}
		req = WithAuthUser(req, "admin")
		rec := httptest.NewRecorder()
		HandleUserPrefs(db)(rec, req)
		return rec
	}

	// Initial GET: empty prefs.
	rec := withUser(http.MethodGet, "")
	if rec.Code != 200 {
		t.Fatalf("initial GET failed: %d %s", rec.Code, rec.Body.String())
	}
	var prefs map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
		t.Fatal(err)
	}
	if _, ok := prefs["disclaimer_minimised"]; ok {
		t.Error("prefs should start empty")
	}

	// PUT stores the disclaimer state.
	rec = withUser(http.MethodPut, `{"disclaimer_minimised": "minimised"}`)
	if rec.Code != 200 {
		t.Fatalf("PUT failed: %d %s", rec.Code, rec.Body.String())
	}

	// GET returns it.
	rec = withUser(http.MethodGet, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["disclaimer_minimised"] != "minimised" {
		t.Errorf("disclaimer_minimised = %q, want minimised", prefs["disclaimer_minimised"])
	}

	// PUT merges: a second key doesn't clobber the first.
	rec = withUser(http.MethodPut, `{"other": "x"}`)
	if rec.Code != 200 {
		t.Fatalf("second PUT failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = withUser(http.MethodGet, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs["disclaimer_minimised"] != "minimised" || prefs["other"] != "x" {
		t.Errorf("merge failed: %v", prefs)
	}
}

// TestLogoutRevokesRefreshTokens guards the logout-revocation contract:
// before the fix, LogoutHandler was a no-op that looked up the user and
// returned, so refresh tokens outlived logout. The fix calls
// DeleteRefreshTokensForUser, so every refresh row for the user disappears
// after LogoutHandler returns.
func TestLogoutRevokesRefreshTokens(t *testing.T) {
	db := newHandlersTestDB(t)

	// refresh_tokens isn't in apiKeySettingsTestSchema; create a minimal copy.
	if _, err := db.Exec(`CREATE TABLE refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatal(err)
	}

	// Seed an admin user.
	res, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', 'x')")
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()

	// Issue two refresh tokens for this user.
	if _, err := services.CreateRefreshToken(db, userID, HashRefreshToken("tok-a"), "2099-01-01 00:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := services.CreateRefreshToken(db, userID, HashRefreshToken("tok-b"), "2099-01-01 00:00:00"); err != nil {
		t.Fatal(err)
	}

	// Pre-condition: both rows present.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ?", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("setup: want 2 tokens, got %d", count)
	}

	// Logout.
	if err := LogoutHandler(db, "admin"); err != nil {
		t.Fatalf("LogoutHandler: %v", err)
	}

	// Post-condition: every refresh row for this user is gone.
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ?", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("after logout: want 0 tokens, got %d", count)
	}
}
