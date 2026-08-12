package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// authTestDB opens a fresh :memory: SQLite with the schema the auth handlers
// touch: admin_users (with prefs column for the new style), refresh_tokens,
// api_keys, and global_settings. The auth flow needs the prefs column because
// the schema adds it via the final migration.
func authTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	schema := `
		CREATE TABLE admin_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			prefs TEXT NOT NULL DEFAULT '{}'
		);
		CREATE TABLE refresh_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
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
		CREATE TABLE global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// testJWTSecret is the canonical 64-byte secret used across the handler tests
// (matching key_validation_test.go:51). Sharing a constant makes inter-test
// cookie/JWT compatibility trivial.
var testJWTSecret = []byte("01234567890123456789012345678901234567890123456789012345678901234567")

// seedAdmin inserts an admin user with the given credentials and returns the
// user ID. Hashes via Argon2 which is slow — callers should batch seed once.
func seedAdmin(t *testing.T, db *sql.DB, username, password string) int64 {
	t.Helper()
	hash, err := services.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES (?, ?)", username, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// authContext returns a fresh background context (the auth handlers don't
// currently honour ctx but they accept one for future DB-deadline plumbing).
func authContext() context.Context {
	return context.Background()
}

// TestAuthStatus_SetupRequired_EmptyDB guards the bootstrap signal: with no
// admin users in the DB, the auth status endpoint reports setup_required=true.
func TestAuthStatus_SetupRequired_EmptyDB(t *testing.T) {
	db := authTestDB(t)
	status, body := AuthStatus(authContext(), db, func() bool { return false }, false)
	if status != 200 {
		t.Fatalf("status: got %d, want 200", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type: %T", body)
	}
	if v, _ := m["setup_required"].(bool); !v {
		t.Errorf("setup_required=%v, want true", m["setup_required"])
	}
}

// TestAuthStatus_SetupCompleted_AfterSeed guards the inverse: once an admin
// exists, setup_required must flip to false.
func TestAuthStatus_SetupCompleted_AfterSeed(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	status, body := AuthStatus(authContext(), db, func() bool { return false }, false)
	if status != 200 {
		t.Fatalf("status: got %d, want 200", status)
	}
	m, _ := body.(map[string]any)
	if v, _ := m["setup_required"].(bool); v {
		t.Errorf("setup_required=%v, want false after seed", m["setup_required"])
	}
}

// TestSetupHandler_FirstAdminSucceeds exercises the bootstrap path: an empty
// DB, valid creds → 200, an access token, a refresh cookie, and a row in
// admin_users.
func TestSetupHandler_FirstAdminSucceeds(t *testing.T) {
	db := authTestDB(t)

	status, body, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	if status != 200 {
		t.Fatalf("status: got %d, want 200 (body=%+v)", status, body)
	}
	m, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("body type: %T", body)
	}
	if m["token"] == "" {
		t.Error("setup response missing access token")
	}
	if len(cookies) != 1 || cookies[0].Name != "refresh_token" {
		t.Errorf("expected one refresh_token cookie, got %+v", cookies)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("admin_users count: got %d, want 1", count)
	}
}

// TestSetupHandler_SecondSetupRejected guards that the bootstrap is
// one-shot: a second setup attempt against a seeded DB must fail with 403.
func TestSetupHandler_SecondSetupRejected(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	status, body, _ := SetupHandler(authContext(), db, testJWTSecret, false, "admin2", "password456")
	if status != 403 {
		t.Errorf("status: got %d, want 403", status)
	}
	m, _ := body.(map[string]string)
	if m["error"] == "" || !strings.Contains(strings.ToLower(m["error"]), "setup") {
		t.Errorf("error message should mention setup, got %q", m["error"])
	}
}

// TestSetupHandler_RejectsBadUsername exercises the username validator across
// the three rejected shapes: empty, too short (single space), and illegal
// (control character).
func TestSetupHandler_RejectsBadUsername(t *testing.T) {
	db := authTestDB(t)
	cases := []struct {
		name     string
		username string
		password string
	}{
		{"empty", "", "password123"},
		{"only-space", " ", "password123"},
		{"control-char", "adm\x01in", "password123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body, _ := SetupHandler(authContext(), db, testJWTSecret, false, c.username, c.password)
			if status != 400 {
				t.Errorf("status: got %d, want 400 (body=%+v)", status, body)
			}
		})
	}
}

// TestSetupHandler_RejectsBadPassword guards the password-length validator
// (the project's minimum is 8 chars).
func TestSetupHandler_RejectsBadPassword(t *testing.T) {
	db := authTestDB(t)
	status, body, _ := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "short")
	if status != 400 {
		t.Errorf("status: got %d, want 400 (body=%+v)", status, body)
	}
}

// TestSetupHandler_RejectsTooLongUsername guards the upper-bound validator
// (the project's max is 128 chars).
func TestSetupHandler_RejectsTooLongUsername(t *testing.T) {
	db := authTestDB(t)
	longName := strings.Repeat("a", 129)
	status, _, _ := SetupHandler(authContext(), db, testJWTSecret, false, longName, "password123")
	if status != 400 {
		t.Errorf("status: got %d, want 400", status)
	}
}

// TestLoginHandler_ValidCredentials_200_WithCookie guards the happy-path
// login: seeded admin, correct password → 200, refresh_token cookie, and a
// corresponding row in refresh_tokens.
func TestLoginHandler_ValidCredentials_200_WithCookie(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	status, body, cookies := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	if status != 200 {
		t.Fatalf("status: got %d, want 200 (body=%+v)", status, body)
	}
	m, _ := body.(map[string]string)
	if m["token"] == "" {
		t.Error("login response missing access token")
	}
	if len(cookies) != 1 || cookies[0].Name != "refresh_token" || cookies[0].Value == "" {
		t.Errorf("refresh cookie missing or empty: %+v", cookies)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("refresh_tokens rows: got %d, want 1", count)
	}
}

// TestLoginHandler_WrongPassword_401 guards the negative-path: wrong password
// must yield 401 and must NOT mint a refresh token.
func TestLoginHandler_WrongPassword_401(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	status, _, cookies := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "wrongpass")
	if status != 401 {
		t.Errorf("status: got %d, want 401", status)
	}
	if len(cookies) != 0 {
		t.Errorf("expected no cookies on failed login, got %+v", cookies)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("refresh_tokens rows after failed login: got %d, want 0", count)
	}
}

// TestLoginHandler_UnknownUser_401 guards that login attempts against an
// unknown username yield 401 without leaking whether the user exists.
func TestLoginHandler_UnknownUser_401(t *testing.T) {
	db := authTestDB(t)
	status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "ghost", "password123")
	if status != 401 {
		t.Errorf("status: got %d, want 401", status)
	}
}

// TestRefreshHandler_ValidToken_Rotates guards the refresh-token rotation
// contract: a valid cookie triggers a new token pair, and the old refresh
// row is deleted from the DB.
func TestRefreshHandler_ValidToken_Rotates(t *testing.T) {
	db := authTestDB(t)

	// Bootstrap via SetupHandler so we have a refresh cookie to send back.
	_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	if len(cookies) != 1 {
		t.Fatalf("setup cookies: %+v", cookies)
	}
	oldCookie := cookies[0].Value

	// Pre-condition: one refresh row for this user.
	var preCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&preCount); err != nil {
		t.Fatal(err)
	}
	if preCount != 1 {
		t.Fatalf("pre refresh_tokens: %d, want 1", preCount)
	}

	// Refresh with the cookie from setup.
	status, body, newCookies := RefreshHandler(authContext(), db, testJWTSecret, false, oldCookie)
	if status != 200 {
		t.Fatalf("refresh status: got %d, want 200 (body=%+v)", status, body)
	}
	m, _ := body.(map[string]string)
	if m["token"] == "" {
		t.Error("refresh response missing access token")
	}
	if len(newCookies) != 1 || newCookies[0].Value == "" || newCookies[0].Value == oldCookie {
		t.Errorf("refresh didn't rotate cookie (old=%q new=%+v)", oldCookie, newCookies)
	}

	// Post-condition: still exactly one refresh row, but a new hash.
	var postCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&postCount); err != nil {
		t.Fatal(err)
	}
	if postCount != 1 {
		t.Errorf("post refresh_tokens: %d, want 1 (rotation should delete old + insert new)", postCount)
	}
	var storedHash string
	if err := db.QueryRow("SELECT token_hash FROM refresh_tokens LIMIT 1").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == HashRefreshToken(oldCookie) {
		t.Error("refresh did not delete the old row (hash unchanged)")
	}
}

// TestRefreshHandler_AfterRotationOldCookieRejected guards the post-rotation
// security property: after a refresh rotation, the old cookie cannot be used
// again (must yield 401).
func TestRefreshHandler_AfterRotationOldCookieRejected(t *testing.T) {
	db := authTestDB(t)

	_, _, cookies := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	oldCookie := cookies[0].Value

	// Rotate.
	_, _, newCookies := RefreshHandler(authContext(), db, testJWTSecret, false, oldCookie)
	if len(newCookies) != 1 {
		t.Fatalf("refresh cookies: %+v", newCookies)
	}

	// Old cookie should now fail.
	status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, oldCookie)
	if status != 401 {
		t.Errorf("old cookie status: got %d, want 401 after rotation", status)
	}

	// But the new one should work (rotating the rotation).
	status, _, _ = RefreshHandler(authContext(), db, testJWTSecret, false, newCookies[0].Value)
	if status != 200 {
		t.Errorf("new cookie status: got %d, want 200", status)
	}
}

// TestRefreshHandler_EmptyCookie_401 guards the empty-input branch.
func TestRefreshHandler_EmptyCookie_401(t *testing.T) {
	db := authTestDB(t)
	status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, "")
	if status != 401 {
		t.Errorf("status: got %d, want 401", status)
	}
}

// TestRefreshHandler_UnknownHash_401 guards the unknown-token branch: a
// syntactically valid cookie whose hash doesn't match any row must yield 401.
func TestRefreshHandler_UnknownHash_401(t *testing.T) {
	db := authTestDB(t)
	// 64 hex chars (looks like a hash) but no row in DB.
	status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false,
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if status != 401 {
		t.Errorf("status: got %d, want 401", status)
	}
}

// TestRefreshHandler_ExpiredToken_401_DeletesRow guards the cleanup path:
// when a refresh row's expires_at is in the past, RefreshHandler must reject
// with 401 AND delete the stale row.
func TestRefreshHandler_ExpiredToken_401_DeletesRow(t *testing.T) {
	db := authTestDB(t)

	// Insert a user and an expired refresh token directly.
	hash, err := services.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', ?)", hash)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()

	token := "expired-token-value"
	tokenHash := HashRefreshToken(token)
	expiresAt := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.Exec("INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)", userID, tokenHash, expiresAt); err != nil {
		t.Fatal(err)
	}

	status, _, _ := RefreshHandler(authContext(), db, testJWTSecret, false, token)
	if status != 401 {
		t.Errorf("status: got %d, want 401", status)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = ?", tokenHash).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expired refresh row not deleted: count=%d", count)
	}
}

// TestLogoutHandler_ViaRouter_ClearsAllTokens exercises the full HTTP flow
// via a real httptest.Server: setup → login (refresh tokens exist) → logout
// → verify all refresh rows are gone.
func TestLogoutHandler_ViaRouter_ClearsAllTokens(t *testing.T) {
	db := authTestDB(t)

	mux := http.NewServeMux()
	jwtSecret := testJWTSecret

	mux.HandleFunc("/api/auth/setup", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Username, Password string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		status, resp, cookies := SetupHandler(r.Context(), db, jwtSecret, false, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Username, Password string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		status, resp, cookies := LoginHandler(r.Context(), db, jwtSecret, false, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Logout needs an authenticated user via the "token" cookie containing a
	// valid JWT.
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		ck, _ := r.Cookie("token")
		if ck != nil {
			if claims, err := ParseJWT(ck.Value, jwtSecret); err == nil && claims.Username != "" {
				_ = LogoutHandler(r.Context(), db, claims.Username)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
		http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/", MaxAge: -1})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Bootstrap: setup mints one refresh row.
	body := bytes.NewBufferString(`{"username":"admin","password":"password123"}`)
	resp, err := http.Post(srv.URL+"/api/auth/setup", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Then login mints a second refresh row and we grab the access token.
	body = bytes.NewBufferString(`{"username":"admin","password":"password123"}`)
	resp, err = http.Post(srv.URL+"/api/auth/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var loginResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatal(err)
	}
	accessToken := loginResp["token"]
	if accessToken == "" {
		t.Fatal("login response missing token")
	}

	// Pre-condition: at least two refresh rows (setup + login).
	var preCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&preCount); err != nil {
		t.Fatal(err)
	}
	if preCount < 2 {
		t.Errorf("pre-logout refresh_tokens: got %d, want >= 2", preCount)
	}

	// Logout with the token cookie.
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: accessToken})
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Post-condition: zero refresh rows.
	var postCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&postCount); err != nil {
		t.Fatal(err)
	}
	if postCount != 0 {
		t.Errorf("post-logout refresh_tokens: got %d, want 0", postCount)
	}
}

// TestKeyLoginHandler_ValidKey_200_WithToken guards the API-key login flow:
// a seeded api_keys row, POST the raw key → 200, JWT in `token`, name and
// key_prefix returned.
func TestKeyLoginHandler_ValidKey_200_WithToken(t *testing.T) {
	db := authTestDB(t)

	// Need an admin user so the api_keys.created_by FK is satisfied.
	seedAdmin(t, db, "admin", "password123")

	// Generate and seed a key.
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := db.Exec(
		"INSERT INTO api_keys (name, key_hash, key_prefix, created_by) VALUES (?, ?, ?, ?)",
		"test-key", hash, prefix, 1,
	); err != nil {
		t.Fatal(err)
	}

	status, body := KeyLoginHandler(authContext(), db, testJWTSecret, raw)
	if status != 200 {
		t.Fatalf("status: got %d, want 200 (body=%+v)", status, body)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type: %T", body)
	}
	if m["token"] == "" {
		t.Error("missing token in response")
	}
	if m["name"] != "test-key" {
		t.Errorf("name: got %v, want test-key", m["name"])
	}
	if m["key_prefix"] != prefix {
		t.Errorf("key_prefix: got %v, want %s", m["key_prefix"], prefix)
	}
}

// TestKeyLoginHandler_InvalidKey_401 guards the negative path: an unknown
// key yields 401.
func TestKeyLoginHandler_InvalidKey_401(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	status, body := KeyLoginHandler(authContext(), db, testJWTSecret, "definitely-not-a-real-key")
	if status != 401 {
		t.Errorf("status: got %d, want 401 (body=%+v)", status, body)
	}
}
