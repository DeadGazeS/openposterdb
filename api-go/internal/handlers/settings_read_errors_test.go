package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

// A failed settings read is our failure: 500, not a render with default
// settings served with a one-hour max-age.
func TestHandleImage_SettingsReadError_500(t *testing.T) {
	// Free key: the globals read fails.
	db := newHandlersTestDB(t)
	db.Close()
	deps := ImageDeps{DB: db, Config: &ImageServeConfig{}}
	mux := http.NewServeMux()
	mux.Handle("/{apiKey}/{rest...}", HandleImage(func() ImageDeps { return deps }, func() (bool, error) { return true, nil }))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+freeAPIKey+"/imdb/poster-default/tt0111161.jpg", nil))
	if rec.Code != 500 || strings.Contains(rec.Header().Get("Cache-Control"), "max-age") {
		t.Errorf("free key: got %d (Cache-Control %q) %s, want 500", rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
	}

	// Per-key: the key is found, its settings read fails.
	db = newHandlersTestDB(t)
	db.SetMaxOpenConns(1)
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKeyCtx(context.Background(), db, "k", hash, prefix, "", 1); err != nil {
		t.Fatal(err)
	}
	deps = ImageDeps{DB: db, Config: &ImageServeConfig{}}
	mux = http.NewServeMux()
	mux.Handle("/{apiKey}/{rest...}", HandleImage(func() ImageDeps { return deps }, func() (bool, error) { return false, nil }))
	path := "/" + raw + "/imdb/poster-default/tt0111161.jpg"

	// No stored settings is not an error: on to the TMDB check (503 here).
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != 503 {
		t.Fatalf("no stored settings: got %d %s, want 503 (TMDB not configured)", rec.Code, rec.Body.String())
	}

	if _, err := db.Exec(`ALTER TABLE api_key_settings RENAME TO aks_gone`); err != nil {
		t.Fatal(err)
	}
	if err := services.SetGlobalSettingCtx(context.Background(), db, "lang", "en"); err != nil { // bump the cache
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != 500 || !bytes.Contains(rec.Body.Bytes(), []byte("Internal server error")) {
		t.Errorf("per-key read failed: got %d %s, want 500 Internal server error", rec.Code, rec.Body.String())
	}
}

// A disabled free key is a 403 with the same message on isValid as on the
// image routes.
func TestHandleIsValid_FreeKeyDisabled_403(t *testing.T) {
	db := newHandlersTestDB(t)
	mux := http.NewServeMux()
	mux.Handle("/{apiKey}/isValid", HandleIsValid(db, func() (bool, error) { return false, nil }, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+freeAPIKey+"/isValid", nil))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "free api key is disabled") {
		t.Errorf("got %d %s, want 403 free api key is disabled", rec.Code, rec.Body.String())
	}
}

// The global settings PUT rejects unknown enum values like the per-key route
// instead of storing them and normalising on read.
func TestHandleUpdateSettings_RejectsUnknownEnums(t *testing.T) {
	db := newHandlersTestDB(t)
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		HandleUpdateSettings(db, false)(rec, req)
		return rec
	}
	for _, field := range []string{
		"image_source", "poster_fit", "anime_artwork",
		"poster_badge_style", "episode_badge_style", "logo_label_style",
		"backdrop_badge_shape", "poster_badge_direction", "episode_badge_direction",
	} {
		rec := put(`{"` + field + `":"bogus"}`)
		want := `invalid ` + field + ` \"bogus\": must be one of`
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: got %d %s, want 400 %s", field, rec.Code, rec.Body.String(), want)
		}
	}
	globals, err := services.GetGlobalSettingsCtx(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if v := globals["poster_fit"]; v == "bogus" {
		t.Error("a rejected value was stored")
	}
	// Valid values (aliases included) still pass.
	if rec := put(`{"image_source":"f","poster_fit":"blur","anime_artwork":"kitsu","poster_badge_style":"h","logo_label_style":"o","backdrop_badge_shape":"p","poster_badge_direction":"v"}`); rec.Code != 200 {
		t.Errorf("valid values: got %d %s, want 200", rec.Code, rec.Body.String())
	}
}
