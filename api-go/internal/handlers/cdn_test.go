package handlers

import (
	"bytes"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

const cdnHandlerSchema = `
CREATE TABLE admin_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	prefs TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
	encrypted_key TEXT,
	created_by INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT
);
`

func newCDNTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(cdnHandlerSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// minimalImageDeps builds an ImageDeps and returns the settings whose hash
// will be registered. It also pre-seeds a JPEG at the exact cache path
// ServeImage will resolve so the CDN handler can serve it without artwork.
// Caller must use the returned settings for both Register() and the request
// URL so the suffix matches.
func minimalImageDeps(t *testing.T, db *sql.DB) (*ImageDeps, *services.RenderSettings) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE image_meta (
		cache_key TEXT PRIMARY KEY,
		release_date TEXT,
		image_type TEXT NOT NULL DEFAULT 'p',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`); err != nil {
		t.Fatalf("image_meta schema: %v", err)
	}
	cacheDir := t.TempDir()

	settings := &services.RenderSettings{
		ImageSource:       "t",
		Lang:              "en",
		PosterTextSize:    100,
		PosterBadgeSize:   100,
		PosterBadgeWidth:  100,
		PosterBadgeHeight: 100,
		PosterLogoSize:    100,
		PosterBadgeShape:  "r",
		PosterBadgeAlpha:  80,
	}
	suffix := services.SettingsCacheSuffix(settings, "poster", nil)
	cacheValue := "tt999" + suffix
	cachePath, err := services.TypedCachePath(cacheDir, services.ImageSubdir("poster"), "imdb", cacheValue, services.ImageExt("poster"))
	if err != nil {
		t.Fatalf("TypedCachePath: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	if err := services.WriteCache(cachePath, buf.Bytes()); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO image_meta(cache_key, release_date, created_at, updated_at)
		VALUES(?, NULL, 1, 1)`, "imdb/"+cacheValue); err != nil {
		t.Fatalf("image_meta seed: %v", err)
	}
	return &ImageDeps{
		DB: db,
		Config: &ImageServeConfig{
			CacheDir:       cacheDir,
			ImageQuality:   85,
			ImageStaleSecs: 0,
		},
	}, settings
}

// TestCDNHandler_HashMiss proves unknown/expired hashes produce a clean 404
// (not a mux-route 404, so the user's edge can distinguish "hash expired,
// please refetch authenticated URL" from a generic miss).
func TestCDNHandler_HashMiss(t *testing.T) {
	db := newCDNTestDB(t)
	defer db.Close()
	reg := services.NewHashRegistry(0)
	deps, _ := minimalImageDeps(t, db)

	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return *deps }, reg)(w, r)
	})
	req := httptest.NewRequest("GET", "/c/deadbeef/imdb/poster-default/tt999.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hash miss: got %d want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "settings hash") {
		t.Errorf("body should explain the hash miss, got %q", rec.Body.String())
	}
}

// TestCDNHandler_HashHit proves a registered hash renders a non-empty image.
func TestCDNHandler_HashHit(t *testing.T) {
	db := newCDNTestDB(t)
	defer db.Close()
	reg := services.NewHashRegistry(0)
	deps, settings := minimalImageDeps(t, db)
	hash := reg.Register(settings)
	if hash == "" {
		t.Fatal("Register returned empty hash")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return *deps }, reg)(w, r)
	})
	req := httptest.NewRequest("GET", "/c/"+hash+"/imdb/poster-default/tt999.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hash hit: got %d want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Fatal("rendered body must be non-empty")
	}
	// Cache-Control should advertise the long CDN cache window.
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") {
		t.Errorf("missing Cache-Control max-age: %q", got)
	}
}

// TestCDNHandler_MethodNotAllowed proves OPTIONS/PUT/etc. on the CDN route
// are rejected with 405 (CDN fetches are GET only).
func TestCDNHandler_MethodNotAllowed(t *testing.T) {
	reg := services.NewHashRegistry(0)
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return ImageDeps{} }, reg)(w, r)
	})
	req := httptest.NewRequest("POST", "/c/abc/imdb/poster-default/x.jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: got %d want 405", rec.Code)
	}
}

// TestCDNHandler_InvalidID proves the path validator rejects ".." and
// similar traversal attempts (ValidateIDValue).
func TestCDNHandler_InvalidID(t *testing.T) {
	reg := services.NewHashRegistry(0)
	settings := &services.RenderSettings{ImageSource: "t"}
	hash := reg.Register(settings)
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		HandleCDNImage(func() ImageDeps { return ImageDeps{} }, reg)(w, r)
	})
	req := httptest.NewRequest("GET", "/c/"+hash+"/imdb/poster-default/..jpg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal id: got %d want 400", rec.Code)
	}
}

// guard against accidental import creep
var _ = strings.ToUpper
