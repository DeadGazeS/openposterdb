package services

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const exportTestSchema = `
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
	encrypted_key TEXT,
	created_by INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT
);
CREATE TABLE IF NOT EXISTS global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS api_key_settings (
	api_key_id INTEGER PRIMARY KEY,
	image_source TEXT NOT NULL DEFAULT 't',
	lang TEXT NOT NULL DEFAULT 'en',
	textless INTEGER NOT NULL DEFAULT 0,
	ratings_limit INTEGER NOT NULL DEFAULT 3,
	ratings_order TEXT NOT NULL DEFAULT '',
	ratings_exclude TEXT NOT NULL DEFAULT '',
	poster_layout TEXT NOT NULL DEFAULT '{"bottom":{"per_row":3,"rows":1,"start":"c"}}',
	logo_ratings_limit INTEGER NOT NULL DEFAULT 5,
	backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
	poster_badge_style TEXT NOT NULL DEFAULT 'h',
	logo_badge_style TEXT NOT NULL DEFAULT 'v',
	backdrop_badge_style TEXT NOT NULL DEFAULT 'v',
	poster_label_style TEXT NOT NULL DEFAULT 'o',
	logo_label_style TEXT NOT NULL DEFAULT 'o',
	backdrop_label_style TEXT NOT NULL DEFAULT 'o',
	poster_badge_direction TEXT NOT NULL DEFAULT 'd',
	poster_fit TEXT NOT NULL DEFAULT 'native',
	poster_text_size INTEGER NOT NULL DEFAULT 100,
	logo_text_size INTEGER NOT NULL DEFAULT 100,
	backdrop_text_size INTEGER NOT NULL DEFAULT 100,
	poster_badge_size INTEGER NOT NULL DEFAULT 100,
	logo_badge_size INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_size INTEGER NOT NULL DEFAULT 100,
	poster_badge_width INTEGER NOT NULL DEFAULT 100,
	poster_badge_height INTEGER NOT NULL DEFAULT 100,
	logo_badge_width INTEGER NOT NULL DEFAULT 100,
	logo_badge_height INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_width INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_height INTEGER NOT NULL DEFAULT 100,
	episode_badge_width INTEGER NOT NULL DEFAULT 100,
	episode_badge_height INTEGER NOT NULL DEFAULT 100,
	poster_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_logo_size INTEGER NOT NULL DEFAULT 100,
	backdrop_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_layout TEXT NOT NULL DEFAULT '{"bottom":{"per_row":5,"rows":1,"start":"c"}}',
	backdrop_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":5,"rows":1,"start":"r"}}',
	backdrop_badge_direction TEXT NOT NULL DEFAULT 'd',
	episode_ratings_limit INTEGER NOT NULL DEFAULT 1,
	episode_badge_style TEXT NOT NULL DEFAULT 'v',
	episode_label_style TEXT NOT NULL DEFAULT 'o',
	episode_text_size INTEGER NOT NULL DEFAULT 100,
	episode_badge_size INTEGER NOT NULL DEFAULT 100,
	episode_logo_size INTEGER NOT NULL DEFAULT 100,
	episode_layout TEXT NOT NULL DEFAULT '{"right":{"per_row":1,"rows":1,"start":"t"}}',
	episode_badge_direction TEXT NOT NULL DEFAULT 'v',
	episode_blur INTEGER NOT NULL DEFAULT 0,
	poster_badge_shape TEXT NOT NULL DEFAULT 'r',
	logo_badge_shape TEXT NOT NULL DEFAULT 'r',
	backdrop_badge_shape TEXT NOT NULL DEFAULT 'r',
	episode_badge_shape TEXT NOT NULL DEFAULT 'r',
	poster_badge_alpha INTEGER NOT NULL DEFAULT 80,
	logo_badge_alpha INTEGER NOT NULL DEFAULT 80,
	backdrop_badge_alpha INTEGER NOT NULL DEFAULT 80,
	episode_badge_alpha INTEGER NOT NULL DEFAULT 80,
	backdrop_edge_inset_x INTEGER NOT NULL DEFAULT 0,
	backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0,
	colors TEXT NOT NULL DEFAULT ''
);
`

func newExportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(exportTestSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

func strP(s string) *string { return &s }

func TestExportImportRoundTrip(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	if err := SetGlobalSetting(db, "lang", "en"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalSetting(db, "ratings_limit", "5"); err != nil {
		t.Fatal(err)
	}

	keys := NewServiceKeyManager(db, []byte("test-secret"), nil, &http.Client{}, "", nil, "", "", "")
	keys.Init()
	if err := keys.UpdateKeys(&ServiceKeysUpdate{TMDB: strP("abc123def456ghi789")}); err != nil {
		t.Fatal(err)
	}

	_, hash, prefix := GenerateAPIKey()
	id, err := CreateAPIKey(db, "test-key", hash, prefix, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertAPIKeySettings(db, &APIKeySettings{
		APIKeyID: id, ImageSource: "t", Lang: "en", RatingsLimit: 3,
		Colors: `{"imdb":{"border":"#ff0000"}}`,
	}); err != nil {
		t.Fatal(err)
	}

	payload, err := BuildExportPayload(db, keys, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Settings["lang"] != "en" || payload.Settings["ratings_limit"] != "5" {
		t.Errorf("global settings not exported: %v", payload.Settings)
	}
	if payload.ServiceKeys["tmdb"] != "abc123def456ghi789" {
		t.Errorf("service key not decrypted in export: %q", payload.ServiceKeys["tmdb"])
	}
	if len(payload.APIKeys) != 1 || payload.APIKeys[0].Name != "test-key" {
		t.Errorf("api key not exported: %+v", payload.APIKeys)
	}

	// Import into a fresh instance.
	db2 := newExportTestDB(t)
	defer db2.Close()
	keys2 := NewServiceKeyManager(db2, []byte("other-secret"), nil, &http.Client{}, "", nil, "", "", "")
	keys2.Init()

	result, err := ApplyImportPayload(db2, keys2, payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoredSettings != 2 {
		t.Errorf("expected 2 settings restored, got %d", result.RestoredSettings)
	}
	if result.RestoredKeys != 1 {
		t.Errorf("expected 1 service key restored, got %d", result.RestoredKeys)
	}
	if len(result.RegeneratedKeys) != 1 || result.RegeneratedKeys[0].Name != "test-key" {
		t.Errorf("expected 1 regenerated api key, got %+v", result.RegeneratedKeys)
	}

	globals, _ := GetGlobalSettings(db2)
	if globals["lang"] != "en" {
		t.Errorf("lang not restored")
	}
	if got := keys2.PlaintextKeys()["tmdb"]; got != "abc123def456ghi789" {
		t.Errorf("service key not restored: %q", got)
	}
	restored, err := FindAPIKeyByName(db2, "test-key")
	if err != nil || restored == nil {
		t.Fatalf("api key not recreated: %v", err)
	}
	ks, err := GetAPIKeySettings(db2, restored.ID)
	if err != nil || ks == nil || ks.RatingsLimit != 3 {
		t.Errorf("per-key settings not restored: %+v (err %v)", ks, err)
	}
	if ks == nil || !strings.Contains(ks.Colors, `"imdb"`) || !strings.Contains(ks.Colors, "#ff0000") {
		t.Errorf("per-key colors not restored: %+v (err %v)", ks, err)
	}

	// Export without keys should exclude them.
	noKeys, err := BuildExportPayload(db, keys, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if noKeys.ServiceKeys != nil || noKeys.APIKeys != nil {
		t.Errorf("keys should be excluded when unchecked")
	}
}

// TestEncryptDecryptPayload_RoundTrip guards the passphrase-encrypted export
// path: encrypt then decrypt with the same passphrase returns the original
// payload, and a wrong passphrase fails closed.
func TestEncryptDecryptPayload_RoundTrip(t *testing.T) {
	payload := &ExportPayload{
		Kind:        "openposterdb/settings",
		Version:     1,
		ExportedAt:  "2026-08-11T00:00:00Z",
		Settings:    map[string]string{"lang": "en", "ratings_limit": "5"},
		ServiceKeys: map[string]string{"tmdb": "abc123"},
	}

	enc, err := EncryptPayload(payload, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc.Kind != "openposterdb/settings-encrypted" {
		t.Errorf("enc.Kind: got %q, want openposterdb/settings-encrypted", enc.Kind)
	}
	if enc.KDF != "pbkdf2-sha256" || enc.KDFIterations != pbkdf2Iterations {
		t.Errorf("unexpected KDF settings: %+v", enc)
	}

	dec, err := DecryptPayload(enc, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec.Kind != "openposterdb/settings" {
		t.Errorf("dec.Kind: got %q, want openposterdb/settings", dec.Kind)
	}
	if dec.Settings["lang"] != "en" || dec.ServiceKeys["tmdb"] != "abc123" {
		t.Errorf("decrypted payload mismatch: %+v", dec)
	}
}

// TestDecryptPayload_WrongPassphrase_Fails guards the GCM auth failure path:
// a wrong passphrase produces a clear error, not a panic or partial decode.
func TestDecryptPayload_WrongPassphrase_Fails(t *testing.T) {
	payload := &ExportPayload{Kind: "openposterdb/settings", Version: 1}
	enc, err := EncryptPayload(payload, "right")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptPayload(enc, "wrong"); err == nil {
		t.Error("decrypt with wrong passphrase should fail")
	}
}

// TestEncryptPayload_EmptyPassphrase_Rejected guards the no-passphrase
// branch: bypassing the passphrase should not produce a weakly-sealed file.
func TestEncryptPayload_EmptyPassphrase_Rejected(t *testing.T) {
	if _, err := EncryptPayload(&ExportPayload{}, ""); err == nil {
		t.Error("EncryptPayload with empty passphrase should fail")
	}
}

// TestEncryptAPIKey_RoundTrip guards the api_key envelope: encrypt then
// decrypt must return the original raw key. Same v2 scheme as source keys,
// different KEK domain (api-key info string).
func TestEncryptAPIKey_RoundTrip(t *testing.T) {
	secretsKey := []byte("api-key-encrypt-test-32-bytes-aa")
	raw := "abc123def4567890abcdef1234567890abcdef1234567890abcdef1234567890"
	encrypted, err := EncryptAPIKey(raw, secretsKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(encrypted, "v2:") {
		t.Errorf("expected v2: prefix, got %q", encrypted[:8])
	}
	dec, err := DecryptAPIKey(encrypted, secretsKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != raw {
		t.Errorf("decrypted=%q, want %q", dec, raw)
	}
}

// TestEncryptAPIKey_WrongSecretsKey_Fails guards the key-binding contract:
// a v2 envelope sealed with one SECRETS_KEY cannot decrypt under another.
func TestEncryptAPIKey_WrongSecretsKey_Fails(t *testing.T) {
	encKey := []byte("right-secrets-key-32-bytes-aaaaa")
	decKey := []byte("wrong-secrets-key-32-bytes-aaaaa")
	encrypted, err := EncryptAPIKey("the-raw-api-key", encKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptAPIKey(encrypted, decKey); err == nil {
		t.Error("decrypt with wrong SECRETS_KEY should fail")
	}
}

// TestCreateAPIKey_StoresEncryptedKey_ReadableOnList guards the end-to-end
// happy path: create encrypts, list decrypts. Mirrors the admin UI flow.
func TestCreateAPIKey_StoresEncryptedKey_ReadableOnList(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	secretsKey := []byte("end-to-end-encrypt-test-32-bytes")
	keys := NewServiceKeyManager(db, secretsKey, nil, &http.Client{}, "", nil, "", "", "")
	keys.Init()

	raw, hash, prefix := GenerateAPIKey()
	encrypted, err := EncryptAPIKey(raw, secretsKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateAPIKey(db, "e2e-key", hash, prefix, encrypted, 1); err != nil {
		t.Fatal(err)
	}

	listed, err := ListAPIKeys(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 key, got %d", len(listed))
	}
	if listed[0].EncryptedKey == nil || *listed[0].EncryptedKey == "" {
		t.Fatal("encrypted_key not stored on create")
	}
	dec, err := DecryptAPIKey(*listed[0].EncryptedKey, secretsKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != raw {
		t.Errorf("list-then-decrypt: got %q, want %q", dec, raw)
	}
}
