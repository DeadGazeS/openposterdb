package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/services"
)

// A database failure during refresh is our failure (500), not a logout (401).
func TestRefreshHandler_DBErrors_500(t *testing.T) {
	t.Run("token lookup", func(t *testing.T) {
		db := authTestDB(t)
		_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
		db.Close()
		if status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, cookies[0].Value); status != 500 {
			t.Errorf("status %d, want 500", status)
		}
	})
	t.Run("user lookup", func(t *testing.T) {
		db := authTestDB(t)
		_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
		if _, err := db.Exec("DROP TABLE admin_users"); err != nil {
			t.Fatal(err)
		}
		if status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, cookies[0].Value); status != 500 {
			t.Errorf("status %d, want 500", status)
		}
	})
	t.Run("consume", func(t *testing.T) {
		db := authTestDB(t)
		_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
		if _, err := db.Exec("CREATE TRIGGER fail_consume BEFORE UPDATE ON refresh_tokens BEGIN SELECT RAISE(ABORT, 'database is locked'); END"); err != nil {
			t.Fatal(err)
		}
		if status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, cookies[0].Value); status != 500 {
			t.Errorf("status %d, want 500", status)
		}
	})
	// A deleted user stays a 401.
	t.Run("user gone", func(t *testing.T) {
		db := authTestDB(t)
		_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
		if _, err := db.Exec("DELETE FROM admin_users"); err != nil {
			t.Fatal(err)
		}
		if status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, cookies[0].Value); status != 401 {
			t.Errorf("status %d, want 401", status)
		}
	})
}

// No hashing slot within the bounded wait: login and setup answer 503 with
// Retry-After.
func TestAuth_HashBusy_503(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	origV, origH := verifyPassword, hashPassword
	t.Cleanup(func() { verifyPassword, hashPassword = origV, origH })
	verifyPassword = func(context.Context, string, string) (bool, error) { return false, services.ErrPasswordHashBusy }
	hashPassword = func(context.Context, string) (string, error) { return "", services.ErrPasswordHashBusy }

	check := func(name string, status int, resp any) {
		t.Helper()
		var ae *apperr.AppError
		if err, ok := resp.(error); status != 503 || !ok || !errors.As(err, &ae) || ae.RetryAfter <= 0 {
			t.Errorf("%s: %d %#v, want 503 busy with Retry-After", name, status, resp)
		}
		rec := httptest.NewRecorder()
		WriteAuthResponse(rec, status, resp, nil)
		if rec.Code != 503 || rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s: written %d Retry-After %q", name, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
	status, resp, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	check("login", status, resp)
	status, resp, _ = LoginHandler(authContext(), db, testJWTSecret, false, "nobody", "password123")
	check("login unknown user", status, resp)
	status, resp, _ = SetupHandler(authContext(), authTestDB(t), testJWTSecret, false, "admin", "password123")
	check("setup", status, resp)
}

// A stored hash we can't parse is our own failure, not a wrong password.
func TestLoginHandler_MalformedStoredHash_500(t *testing.T) {
	db := authTestDB(t)
	if _, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', 'not-a-hash')"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "password123"); status != 500 {
		t.Errorf("status %d, want 500", status)
	}
}

// The failed-login log carries a keyed hash of the username, not a plain
// SHA-256 one can reverse with a dictionary.
func TestLoginHandler_UsernameLogHashKeyed(t *testing.T) {
	db := authTestDB(t)
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })

	logged := func(secret []byte) string {
		t.Helper()
		buf.Reset()
		LoginHandler(authContext(), db, secret, false, "alice", "password123")
		var rec struct {
			UsernameHash string `json:"username_hash"`
		}
		for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			_ = json.Unmarshal(line, &rec)
			if rec.UsernameHash != "" {
				return rec.UsernameHash
			}
		}
		t.Fatalf("no username_hash in log: %s", buf.String())
		return ""
	}
	a := logged(testJWTSecret)
	plain := sha256.Sum256([]byte("alice"))
	if a == services.KeyHash("alice") || a == hex.EncodeToString(plain[:])[:len(a)] {
		t.Errorf("username_hash %q is the unsalted SHA-256", a)
	}
	if len(a) != len(services.KeyHash("alice")) {
		t.Errorf("username_hash %q: length %d, want %d", a, len(a), len(services.KeyHash("alice")))
	}
	if b := logged(testJWTSecret); b != a {
		t.Errorf("not stable for one secret: %q vs %q", a, b)
	}
	if c := logged([]byte("another-secret-another-secret-00")); c == a {
		t.Errorf("same hash under another secret: %q", c)
	}
}

// RequireAnyAuth admits only sessions whose admin user or API key still
// exists; a lookup failure is a 500.
func TestRequireAnyAuth_ChecksExistence(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	_, hash, prefix := services.GenerateAPIKey()
	keyID, err := services.CreateAPIKey(db, "k", hash, prefix, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	h := RequireAnyAuth(testJWTSecret, db)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	do := func(token string) int {
		req := httptest.NewRequest("GET", "/p", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	admin, _ := CreateToken("admin", testJWTSecret)
	ghost, _ := CreateToken("ghost", testJWTSecret)
	key, _ := CreateAPIKeyToken(keyID, testJWTSecret)
	goneKey, _ := CreateAPIKeyToken(keyID+100, testJWTSecret)
	for name, c := range map[string]struct {
		token string
		want  int
	}{
		"admin": {admin, 200}, "deleted admin": {ghost, 401},
		"key": {key, 200}, "deleted key": {goneKey, 401}, "garbage": {"x", 401},
	} {
		if got := do(c.token); got != c.want {
			t.Errorf("%s: %d, want %d", name, got, c.want)
		}
	}
	// Database gone: uncached lookups fail with a 500.
	db2 := authTestDB(t)
	h = RequireAnyAuth(testJWTSecret, db2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	db2.Close()
	if got := do(admin); got != 500 {
		t.Errorf("admin, db error: %d, want 500", got)
	}
	if got := do(key); got != 500 {
		t.Errorf("key, db error: %d, want 500", got)
	}
}

// Admin-only routes reject tokens of admins that no longer exist; a lookup
// failure is a 500.
func TestRequireAuth_ChecksAdminExists(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	do := func(h http.Handler, token string) int {
		req := httptest.NewRequest("GET", "/p", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	admin, _ := CreateToken("admin", testJWTSecret)
	ghost, _ := CreateToken("ghost", testJWTSecret)
	h := RequireAuth(testJWTSecret, db)(ok)
	if got := do(h, admin); got != 200 {
		t.Errorf("admin: %d, want 200", got)
	}
	if got := do(h, ghost); got != 401 {
		t.Errorf("deleted admin: %d, want 401", got)
	}
	db2 := authTestDB(t)
	h = RequireAuth(testJWTSecret, db2)(ok)
	db2.Close()
	if got := do(h, admin); got != 500 {
		t.Errorf("db error: %d, want 500", got)
	}
}
