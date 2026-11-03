package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"openposterdb/internal/app"
	"openposterdb/internal/config"
	"openposterdb/internal/services"
)

func authTestRouter(t *testing.T, authRPM uint64) *Router {
	t.Helper()
	dir := t.TempDir()
	db, err := app.OpenDatabase(filepath.Join(dir, "auth.db"))
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
	return New(&AppState{
		Config:     &config.Config{CacheDir: dir, RateLimitAuthRPM: authRPM},
		DB:         db,
		HTTPClient: &http.Client{},
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
	})
}

func login(t *testing.T, r *Router) (*http.Cookie, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			return c, rec.Code
		}
	}
	return nil, rec.Code
}

func postWithCookie(r *Router, path string, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// Logout used to leave the refresh token valid for 7 days: the SPA never
// called it, the server read a cookie the SPA doesn't send, and the cookie
// was cleared on the wrong path.
func TestLogout_RevokesRefreshToken(t *testing.T) {
	r := authTestRouter(t, 0)
	cookie, code := login(t, r)
	if cookie == nil {
		t.Fatalf("login: %d, no refresh cookie", code)
	}
	rec := postWithCookie(r, "/api/auth/refresh/logout", cookie)
	if rec.Code != 200 {
		t.Fatalf("logout: %d", rec.Code)
	}
	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			cleared = c
		}
	}
	if cleared == nil || cleared.Path != "/api/auth/refresh" || cleared.MaxAge >= 0 {
		t.Errorf("refresh cookie must be cleared on its own path, got %+v", cleared)
	}
	if rec := postWithCookie(r, "/api/auth/refresh", cookie); rec.Code != 401 {
		t.Fatalf("refresh after logout: got %d want 401", rec.Code)
	}
}

// Concurrent refreshes with one token: exactly one rotation may succeed (each
// used to get a fresh pair, leaving orphaned refresh tokens).
func TestRefresh_ConcurrentRotationSingleWinner(t *testing.T) {
	r := authTestRouter(t, 0)
	cookie, _ := login(t, r)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if postWithCookie(r, "/api/auth/refresh", cookie).Code == 200 {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := ok.Load(); n != 1 {
		t.Fatalf("%d concurrent refreshes succeeded, want exactly 1", n)
	}
}

// Credential endpoints are throttled per IP (they were unlimited).
func TestLogin_Throttled(t *testing.T) {
	r := authTestRouter(t, 3)
	codes := []int{}
	for i := 0; i < 5; i++ {
		_, code := login(t, r)
		codes = append(codes, code)
	}
	if codes[3] != http.StatusTooManyRequests || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v, want 429 after 3 attempts", codes)
	}
}
