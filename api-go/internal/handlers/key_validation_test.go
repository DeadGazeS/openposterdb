package handlers

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// keyCreateSchema mirrors api_keys_test.go's schema — enough for
// HandleCreateKey to insert a row.
const keyCreateSchema = `
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

func TestHandleCreateKey_ValidatesName(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(keyCreateSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO admin_users (username, password_hash) VALUES ('admin', 'x')`); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	jwtSecret := []byte("01234567890123456789012345678901234567890123456789012345678901234567")
	token, err := CreateToken("admin", jwtSecret)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	cases := []struct {
		name string
		want int
	}{
		{"", http.StatusBadRequest},                             // empty
		{"a", http.StatusCreated},                               // fine
		{"My Key Name", http.StatusCreated},                     // spaces allowed
		{"x" + strings.Repeat("y", 128), http.StatusBadRequest}, // too long
		{"bad\x00name", http.StatusBadRequest},                  // control char
	}
	for _, c := range cases {
		body := bytes.NewBufferString(`{"name":"` + strings.ReplaceAll(c.name, `"`, `\"`) + `"}`)
		req := httptest.NewRequest("POST", "/api/keys", body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		HandleCreateKey(db, testSecretsKey)(rec, req)
		if rec.Code != c.want {
			t.Errorf("name %q: got %d want %d (body %s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestValidateAPIKeyNameRules(t *testing.T) {
	valid := []string{"a", "My Key", "key-1", strings.Repeat("x", 128)}
	for _, v := range valid {
		if err := services.ValidateAPIKeyName(v); err != nil {
			t.Errorf("expected %q valid, got %v", v, err)
		}
	}
	invalid := []string{"", strings.Repeat("x", 129), "a\x01b", "a\x1fb", "a\x7fb"}
	for _, v := range invalid {
		if err := services.ValidateAPIKeyName(v); err == nil {
			t.Errorf("expected %q invalid, got no error", v)
		}
	}
}
