package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// keyAuthSchema is the minimal subset of tables needed to exercise the
// last_used_at recording path: an admin user + an api_keys row.
const keyAuthSchema = `
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

func newKeyAuthDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(keyAuthSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// TestRequireAPIKeyAuth_RecordsKeyUse proves that RequireAPIKeyAuth records
// the parsed key ID on a successfully-authenticated request and that a
// subsequent Flush() persists it to api_keys.last_used_at.
func TestRequireAPIKeyAuth_RecordsKeyUse(t *testing.T) {
	db := newKeyAuthDB(t)

	// Create a key + a JWT for it.
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKey(db, "test", hash, prefix, "", 1); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	// Re-derive the key_id the same way auth.go does: hash → lookup.
	row := db.QueryRow(`SELECT id FROM api_keys WHERE key_hash = ?`, services.HashAPIKey(raw))
	var keyID int64
	if err := row.Scan(&keyID); err != nil {
		t.Fatalf("lookup key: %v", err)
	}
	jwtSecret := []byte("01234567890123456789012345678901234567890123456789012345678901234567")
	token, err := CreateAPIKeyToken(keyID, jwtSecret)
	if err != nil {
		t.Fatalf("CreateAPIKeyToken: %v", err)
	}

	flusher := services.NewLastUsedFlusher(db, 1) // 1ns interval; we flush manually

	mux := http.NewServeMux()
	mux.Handle("/protected", RequireAPIKeyAuth(jwtSecret, flusher)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	))

	// Sanity: a request without a token is rejected and does not record.
	{
		req := httptest.NewRequest("GET", "/protected", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthed request: got status %d want 401", rec.Code)
		}
	}
	if _, recorded := flusher.Pending.Load(keyID); recorded {
		t.Fatal("unauthed request must not record the key")
	}

	// A valid token authenticates AND records.
	{
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("authed request: got status %d want 200", rec.Code)
		}
	}
	if _, recorded := flusher.Pending.Load(keyID); !recorded {
		t.Fatal("authed request must record the key in the flusher")
	}

	// last_used_at is still NULL in the DB until flush.
	var lastUsed *string
	if err := db.QueryRow(`SELECT last_used_at FROM api_keys WHERE id = ?`, keyID).Scan(&lastUsed); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	if lastUsed != nil {
		t.Fatalf("last_used_at must be NULL before flush, got %q", *lastUsed)
	}

	// Flush, then the column should be populated.
	flusher.Flush()
	if err := db.QueryRow(`SELECT last_used_at FROM api_keys WHERE id = ?`, keyID).Scan(&lastUsed); err != nil {
		t.Fatalf("read last_used_at post-flush: %v", err)
	}
	if lastUsed == nil || *lastUsed == "" {
		t.Fatal("last_used_at must be populated after Flush")
	}
}

// TestRequireAPIKeyAuth_NilFlusherSafe ensures the middleware doesn't panic
// when no flusher is wired (tests, alternate deployments).
func TestRequireAPIKeyAuth_NilFlusherSafe(t *testing.T) {
	jwtSecret := []byte("01234567890123456789012345678901234567890123456789012345678901234567")
	_, hash, prefix := services.GenerateAPIKey()
	// We don't need the DB to test this; just craft a JWT against a known id.
	token, err := CreateAPIKeyToken(42, jwtSecret)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	_ = hash
	_ = prefix

	mux := http.NewServeMux()
	mux.Handle("/p", RequireAPIKeyAuth(jwtSecret, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	))
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil flusher must not break auth: got %d", rec.Code)
	}
}
