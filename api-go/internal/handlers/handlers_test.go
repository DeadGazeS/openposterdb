package handlers

import (
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
	a := HashAPIKey("testkey123")
	b := HashAPIKey("testkey123")
	if a != b {
		t.Error("same input should produce same hash")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	raw, hash, prefix := GenerateAPIKey()
	if len(raw) != 64 {
		t.Errorf("raw key should be 64 chars, got %d", len(raw))
	}
	if len(prefix) != 8 {
		t.Errorf("prefix should be 8 chars, got %d", len(prefix))
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if HashAPIKey(raw) != hash {
		t.Error("hash should match")
	}
	if raw[:8] != prefix {
		t.Error("prefix should match first 8 chars")
	}
}

func TestHashPassword(t *testing.T) {
	h1, err := HashPassword("testpassword")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := HashPassword("testpassword")
	// Passwords should hash differently due to random salt
	if h1 == h2 {
		t.Error("same password should produce different hashes due to salt")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, _ := HashPassword("correctpass")
	ok, _ := VerifyPassword("correctpass", hash)
	if !ok {
		t.Error("correct password should verify")
	}
	ok, _ = VerifyPassword("wrongpass", hash)
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
	q.RatingsLimit = &limit
	q.BadgeStyle = &style
	q.LabelStyle = &label
	q.Position = (*string)(&[]string{"tl"}[0])

	result := applyQueryOverrides(&s, q, "poster")
	if result.RatingsLimit != 3 {
		t.Error("ratings_limit should be 3")
	}
	if result.PosterBadgeStyle != services.BadgeStyleHorizontal {
		t.Error("should be Horizontal")
	}
	if result.PosterLabelStyle != services.LabelStyleIcon {
		t.Error("should be Icon")
	}
}

func TestApplyQueryOverridesLogoIgnoresPosterOnly(t *testing.T) {
	s := services.DefaultRenderSettings()
	q := &ImageQuery{}
	pos := "tl"
	q.Position = &pos

	result := applyQueryOverrides(&s, q, "logo")
	if result.PosterPosition != s.PosterPosition {
		t.Error("position should be ignored for logo")
	}
}

func TestFreeKeySettingsFromRender(t *testing.T) {
	s := services.DefaultRenderSettings()
	resp := freeKeySettingsFromRender(&s)
	if resp.ImageSource != "t" {
		t.Error("wrong image_source")
	}
	if resp.Lang != "en" {
		t.Error("wrong lang")
	}
	if resp.RatingsLimit != 3 {
		t.Error("wrong ratings_limit")
	}
}
