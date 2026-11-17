package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"openposterdb/internal/app"
	"openposterdb/internal/config"
	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

var phase5Secret = []byte("0123456789abcdef0123456789abcdef")

// phase5Router is authTestRouter with a configurable Config.
func phase5Router(t *testing.T, edit func(*config.Config)) (*Router, *AppState) {
	t.Helper()
	dir := t.TempDir()
	db, err := app.OpenDatabase(filepath.Join(dir, "p5.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := app.RunSchema(db, app.SchemaSQL); err != nil {
		t.Fatal(err)
	}
	if err := app.RunMigrations(db, app.Migrations); err != nil {
		t.Fatal(err)
	}
	if err := services.RunUpgrades(db, dir, true); err != nil {
		t.Fatal(err)
	}
	app.SeedAdminIfNeeded(db, "admin", "correct-horse-battery")
	cfg := &config.Config{CacheDir: dir}
	if edit != nil {
		edit(cfg)
	}
	s := &AppState{Config: cfg, DB: db, HTTPClient: &http.Client{}, JWTSecret: phase5Secret}
	return New(s), s
}

func serve(r *Router, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// Unknown /api paths are a JSON 404 instead of reaching the image
// catch-all ("invalid api key").
func TestUnknownAPIPath_JSON404(t *testing.T) {
	r, _ := phase5Router(t, nil)
	for _, p := range []string{"/api/nope", "/api/admin/nope/deeper", "/api/"} {
		rec := serve(r, "GET", p, "", nil)
		if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "json") || !strings.Contains(rec.Body.String(), "Not found") {
			t.Errorf("%s: %d %q %s", p, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
	}
	// Known routes are unaffected.
	if rec := serve(r, "GET", "/api/auth/status", "", nil); rec.Code != 200 {
		t.Errorf("/api/auth/status: %d", rec.Code)
	}
}

// With public pages disabled the OpenAPI document needs a login, and an
// API-key session counts (it used to take an admin login only).
func TestOpenAPI_DisablePublicPages_AnyLogin(t *testing.T) {
	r, _ := phase5Router(t, func(c *config.Config) { c.DisablePublicPages = true })
	if rec := serve(r, "GET", "/api/openapi.json", "", nil); rec.Code != 401 {
		t.Errorf("anonymous: %d, want 401", rec.Code)
	}
	keyToken, _ := handlers.CreateAPIKeyToken(7, phase5Secret)
	if rec := serve(r, "GET", "/api/openapi.json", "", map[string]string{"Authorization": "Bearer " + keyToken}); rec.Code != 200 {
		t.Errorf("API-key session: %d, want 200", rec.Code)
	}
	adminToken, _ := handlers.CreateToken("admin", phase5Secret)
	if rec := serve(r, "GET", "/api/openapi.json", "", map[string]string{"Authorization": "Bearer " + adminToken}); rec.Code != 200 {
		t.Errorf("admin: %d, want 200", rec.Code)
	}
}

// The refresh-token exchange shares the credential endpoints' throttle.
func TestRefresh_Throttled(t *testing.T) {
	r, _ := phase5Router(t, func(c *config.Config) { c.RateLimitAuthRPM = 2 })
	var codes []int
	for range 3 {
		codes = append(codes, serve(r, "POST", "/api/auth/refresh", "", nil).Code)
	}
	if codes[0] == 429 || codes[2] != 429 {
		t.Errorf("statuses %v, want the third throttled", codes)
	}
}

// isValid answers HEAD like GET.
func TestIsValid_HEAD(t *testing.T) {
	r, s := phase5Router(t, nil)
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKeyCtx(context.Background(), s.DB, "k", hash, prefix, "", 1); err != nil {
		t.Fatal(err)
	}
	if rec := serve(r, "HEAD", "/"+raw+"/isValid", "", nil); rec.Code != 200 {
		t.Errorf("HEAD valid key: %d", rec.Code)
	}
	if rec := serve(r, "HEAD", "/not-a-key/isValid", "", nil); rec.Code != 401 {
		t.Errorf("HEAD invalid key: %d", rec.Code)
	}
}

// Rust image limits: a library scan's burst of 240 requests goes through
// (61 parallel requests used to hit the 60/min limit), and each API key has
// its own bucket behind the same IP.
func TestImageRateLimit_RustDefaults(t *testing.T) {
	r, _ := phase5Router(t, func(c *config.Config) { c.RateLimitRPM = 300; c.RateLimitCDNRPM = 300 })
	count429 := func(key string, n int) int {
		got := 0
		for range n {
			if serve(r, "GET", "/"+key+"/imdb/poster-default/tt0111161.jpg", "", nil).Code == 429 {
				got++
			}
		}
		return got
	}
	if n := count429("key-a", 240); n != 0 {
		t.Fatalf("%d of the first 240 requests limited", n)
	}
	if n := count429("key-a", 1); n != 1 {
		t.Fatalf("request 241 for the same key not limited")
	}
	if n := count429("key-b", 10); n != 0 {
		t.Fatalf("another key behind the same IP shares the bucket (%d limited)", n)
	}
}

// isValid has its own 10/s bucket (burst 30) and doesn't eat the image quota.
func TestIsValidRateLimit_Separate(t *testing.T) {
	r, _ := phase5Router(t, func(c *config.Config) { c.RateLimitRPM = 300 })
	limited := 0
	for range 31 {
		if serve(r, "GET", "/key-c/isValid", "", nil).Code == 429 {
			limited++
		}
	}
	if limited != 1 {
		t.Errorf("isValid: %d of 31 limited, want 1 (burst 30)", limited)
	}
	if rec := serve(r, "GET", "/key-c/imdb/poster-default/tt1.jpg", "", nil); rec.Code == 429 {
		t.Error("image request limited by the isValid bucket")
	}
}

func TestServiceKeysPut_MalformedBody400(t *testing.T) {
	r, _ := phase5Router(t, nil)
	tok, _ := handlers.CreateToken("admin", phase5Secret)
	rec := serve(r, "PUT", "/api/admin/settings/services", "{not json", map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != 400 {
		t.Errorf("malformed body: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKeyPrefix(t *testing.T) {
	if apiKeyPrefix("0123456789abcdefXYZ") != "0123456789abcdef" || apiKeyPrefix("short") != "short" {
		t.Error("prefix")
	}
}
