package router

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/config"
)

// testRouter builds a Router backed by an in-memory database with the minimal
// tables the unauthenticated handlers touch.
func testRouter(t *testing.T) *Router {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	tables := []string{
		`CREATE TABLE admin_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE global_settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL,
			created_by INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT '',
			last_used_at TEXT
		)`,
	}
	for _, q := range tables {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	freeKey := true
	cfg := &config.Config{
		CacheDir:       t.TempDir(),
		FreeKeyEnabled: &freeKey,
	}
	return New(&AppState{
		Config:      cfg,
		DB:          db,
		HTTPClient:  &http.Client{},
		JWTSecret:   []byte("0123456789abcdef0123456789abcdef"),
		Caches:      nil,
		ServiceKeys: nil,
	})
}

// probe describes one route that must exist, with the request that proves it
// and the expected status. want == -1 means "any status except the mux's
// plain-text 404".
type probe struct {
	method string
	path   string
	want   int
}

var routeProbes = []probe{
	// auth (open)
	{"GET", "/api/auth/status", 200},
	{"GET", "/api/auth/setup", 405},
	{"GET", "/api/auth/login", 405},
	{"GET", "/api/auth/refresh", 405},
	{"GET", "/api/auth/logout", 405},
	{"GET", "/api/auth/key-login", 405},
	// key management (admin-auth gated)
	{"GET", "/api/keys", 401},
	{"DELETE", "/api/keys/1", 401},
	{"GET", "/api/keys/1/settings", 401},
	{"PUT", "/api/keys/1/settings", 401},
	{"DELETE", "/api/keys/1/settings", 401},
	{"GET", "/api/key/me", 401},
	{"GET", "/api/key/me/settings", 401},
	{"PUT", "/api/key/me/settings", 401},
	// admin
	{"GET", "/api/admin/stats", 401},
	{"GET", "/api/admin/prefs", 401},
	{"GET", "/api/admin/settings", 401},
	{"PUT", "/api/admin/settings", 401},
	{"POST", "/api/admin/cache/purge", 401},
	{"GET", "/api/admin/settings/services", 401},
	{"PUT", "/api/admin/settings/services", 401},
	{"GET", "/api/admin/settings/export", 401},
	{"POST", "/api/admin/settings/import", 401},
	// admin previews
	{"GET", "/api/admin/preview/poster", 401},
	{"GET", "/api/admin/preview/logo", 401},
	{"GET", "/api/admin/preview/backdrop", 401},
	{"GET", "/api/admin/preview/episode", 401},
	// key self-service previews
	{"GET", "/api/key/me/preview/poster", 401},
	{"GET", "/api/key/me/preview/logo", 401},
	{"GET", "/api/key/me/preview/backdrop", 401},
	{"GET", "/api/key/me/preview/episode", 401},
	// kind routes: list/clear
	{"GET", "/api/admin/posters", 401},
	{"DELETE", "/api/admin/posters", 401},
	{"GET", "/api/admin/logos", 401},
	{"DELETE", "/api/admin/logos", 401},
	{"GET", "/api/admin/backdrops", 401},
	{"DELETE", "/api/admin/backdrops", 401},
	{"GET", "/api/admin/episodes", 401},
	{"DELETE", "/api/admin/episodes", 401},
	// kind routes: item image/purge (posters + episodes: /image subroute)
	{"GET", "/api/admin/posters/tmdb/1/image", 401},
	{"DELETE", "/api/admin/posters/tmdb/1", 401},
	{"POST", "/api/admin/posters/tmdb/1/fetch", 401},
	{"GET", "/api/admin/episodes/tmdb/1/image", 401},
	{"DELETE", "/api/admin/episodes/tmdb/1", 401},
	{"POST", "/api/admin/episodes/tmdb/1/fetch", 401},
	// kind routes: item image/purge (logos + backdrops: image on base path)
	{"GET", "/api/admin/logos/tmdb/1", 401},
	{"DELETE", "/api/admin/logos/tmdb/1", 401},
	{"POST", "/api/admin/logos/tmdb/1/fetch", 401},
	{"GET", "/api/admin/backdrops/tmdb/1", 401},
	{"DELETE", "/api/admin/backdrops/tmdb/1", 401},
	{"POST", "/api/admin/backdrops/tmdb/1/fetch", 401},
	// free key + icons (open)
	{"GET", "/api/free-key/settings", 200},
	{"GET", "/api/icons/bogus/key", -1}, // handler JSON 404 proves route exists
	{"POST", "/api/icons/badge", 405},
	{"GET", "/api/icons/badge", 400}, // missing source/value params
	// image catch-all (unknown API key -> JSON error, not mux 404)
	{"GET", "/" + strings.Repeat("a", 64) + "/p/tmdb/1", -1},
	{"GET", "/" + strings.Repeat("a", 64) + "/isValid", -1},
	// CDN content-addressed route — unknown hash must yield the handler's JSON
	// 404, not the mux's plain-text 404 (so the user's edge can distinguish
	// "hash expired" from a generic miss).
	{"GET", "/c/deadbeefdeadbeefdeadbeefdeadbeef/imdb/poster-default/tt999.jpg", -1},
	// OpenAPI spec (PUBLIC by default — must yield application/json, not mux 404)
	{"GET", "/api/openapi.json", 200},
	// unknown single-segment path: mux redirects to /x/ which the catch-all
	// matches, so it must never yield the mux's plain-text 404
	{"GET", "/definitely-not-a-route", -1},
}

func TestRegisteredRoutes(t *testing.T) {
	r := testRouter(t)
	for _, p := range routeProbes {
		req := httptest.NewRequest(p.method, p.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if p.want == -1 {
			body := rec.Body.String()
			if rec.Code == 404 && strings.Contains(body, "404 page not found") {
				t.Errorf("%s %s: route missing (mux 404)", p.method, p.path)
			}
			continue
		}
		if rec.Code != p.want {
			t.Errorf("%s %s: got %d, want %d (body: %s)", p.method, p.path, rec.Code, p.want, rec.Body.String())
		}
	}
}
