package services

import (
	"database/sql"
	"net/http"
	"testing"

	_ "modernc.org/sqlite"
)

const exportTestSchema = `
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
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
	poster_position TEXT NOT NULL DEFAULT 'bc',
	logo_ratings_limit INTEGER NOT NULL DEFAULT 5,
	backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
	poster_badge_style TEXT NOT NULL DEFAULT 'h',
	logo_badge_style TEXT NOT NULL DEFAULT 'v',
	backdrop_badge_style TEXT NOT NULL DEFAULT 'v',
	poster_label_style TEXT NOT NULL DEFAULT 'o',
	logo_label_style TEXT NOT NULL DEFAULT 'o',
	backdrop_label_style TEXT NOT NULL DEFAULT 'o',
	poster_badge_direction TEXT NOT NULL DEFAULT 'd',
	poster_badge_split INTEGER NOT NULL DEFAULT 0,
	poster_fit TEXT NOT NULL DEFAULT 'native',
	poster_text_size INTEGER NOT NULL DEFAULT 100,
	logo_text_size INTEGER NOT NULL DEFAULT 100,
	backdrop_text_size INTEGER NOT NULL DEFAULT 100,
	poster_badge_size INTEGER NOT NULL DEFAULT 100,
	logo_badge_size INTEGER NOT NULL DEFAULT 100,
	backdrop_badge_size INTEGER NOT NULL DEFAULT 100,
	poster_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_logo_size INTEGER NOT NULL DEFAULT 100,
	backdrop_logo_size INTEGER NOT NULL DEFAULT 100,
	logo_position TEXT NOT NULL DEFAULT 'bc',
	logo_badge_split INTEGER NOT NULL DEFAULT 0,
	backdrop_position TEXT NOT NULL DEFAULT 'tr',
	backdrop_badge_direction TEXT NOT NULL DEFAULT 'd',
	episode_ratings_limit INTEGER NOT NULL DEFAULT 1,
	episode_badge_style TEXT NOT NULL DEFAULT 'v',
	episode_label_style TEXT NOT NULL DEFAULT 'o',
	episode_text_size INTEGER NOT NULL DEFAULT 100,
	episode_badge_size INTEGER NOT NULL DEFAULT 100,
	episode_logo_size INTEGER NOT NULL DEFAULT 100,
	episode_position TEXT NOT NULL DEFAULT 'tr',
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
	backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0
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

	keys := NewServiceKeyManager(db, []byte("test-secret"), &http.Client{}, "", nil, "", "", "")
	keys.Init()
	if err := keys.UpdateKeys(&ServiceKeysUpdate{TMDB: strP("abc123def456ghi789")}); err != nil {
		t.Fatal(err)
	}

	_, hash, prefix := GenerateAPIKey()
	id, err := CreateAPIKey(db, "test-key", hash, prefix, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertAPIKeySettings(db, &APIKeySettings{APIKeyID: id, ImageSource: "t", Lang: "en", RatingsLimit: 3}); err != nil {
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
	keys2 := NewServiceKeyManager(db2, []byte("other-secret"), &http.Client{}, "", nil, "", "", "")
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

	// Export without keys should exclude them.
	noKeys, err := BuildExportPayload(db, keys, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if noKeys.ServiceKeys != nil || noKeys.APIKeys != nil {
		t.Errorf("keys should be excluded when unchecked")
	}
}
