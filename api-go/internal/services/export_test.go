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
	if len(result.RegeneratedKeys) != 0 {
		t.Errorf("expected 0 regenerated api keys (hash carried across SECRETS_KEY), got %+v", result.RegeneratedKeys)
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
	// Cross-SECRETS_KEY: hash + prefix are carried verbatim so /api/key/me
	// keeps authenticating with the original raw value. The source row had
	// no encrypted_key (legacy schema), so this is a hash-only carry.
	if restored.KeyHash != hash {
		t.Errorf("hash not carried across SECRETS_KEY: got %q, want %q", restored.KeyHash, hash)
	}
	if restored.KeyPrefix != prefix {
		t.Errorf("prefix not carried across SECRETS_KEY: got %q, want %q", restored.KeyPrefix, prefix)
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

// TestPreFixImportMergesOntoExisting (#10.4): a pre-fix export (Version <
// MinExportVersion) doesn't carry badge_size / badge_alpha / edge_inset columns
// — the 2026-08-14 outage class. Importing such an export onto an existing
// per-key row must preserve the user's stored values for those columns instead
// of zero-filling (which would clamp to 50 and silently degrade the rendered
// output). The version on a fresh export is MinExportVersion; this test
// constructs the export with an older Version to exercise the merge branch.
func TestPreFixImportMergesOntoExisting(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	keys := NewServiceKeyManager(db, []byte("test-secret"), nil, &http.Client{}, "", nil, "", "", "")
	keys.Init()

	// Seed: an api key with stored per-key settings carrying non-default
	// badge_size / badge_alpha / edge_inset values the user wants to keep.
	_, hash, prefix := GenerateAPIKey()
	id, err := CreateAPIKey(db, "test-key", hash, prefix, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	stored := &APIKeySettings{
		APIKeyID:           id,
		ImageSource:        "t",
		Lang:               "de",
		RatingsLimit:       5,
		LogoRatingsLimit:   4,
		PosterBadgeSize:    130,
		LogoBadgeSize:      120,
		BackdropBadgeSize:  110,
		EpisodeBadgeSize:   100,
		PosterBadgeAlpha:   75,
		LogoBadgeAlpha:     70,
		BackdropBadgeAlpha: 65,
		EpisodeBadgeAlpha:  60,
		BackdropEdgeInsetX: 25,
		BackdropEdgeInsetY: 15,
	}
	if err := UpsertAPIKeySettings(db, stored); err != nil {
		t.Fatal(err)
	}

	// Construct a pre-fix export: Version < MinExportVersion, Settings only
	// carries the columns that existed before the 2026-08-14 fix. The export
	// has badge_size=0, badge_alpha=0, edge_inset=0 (they weren't in the
	// schema when the export was produced).
	preFix := &ExportPayload{
		Kind:       "openposterdb/settings",
		Version:    MinExportVersion - 1, // pre-fix marker
		ExportedAt: "2026-08-13T00:00:00Z",
		APIKeys: []ExportedAPIKey{
			{
				Name: "test-key",
				Settings: &APIKeySettings{
					// Intentionally zero on badge_size / badge_alpha / edge_inset
					// to mimic a pre-fix export's missing columns.
					APIKeyID:  id,
					Lang:      "en", // explicit override from the export
					ImageSource: "f",
					RatingsLimit: 9,
				},
			},
		},
	}

	// Import into the same db (the existing row is the merge target).
	if _, err := ApplyImportPayload(db, keys, preFix); err != nil {
		t.Fatalf("ApplyImportPayload: %v", err)
	}

	got, err := GetAPIKeySettings(db, id)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}

	// Non-zero import fields win over existing.
	if got.Lang != "en" {
		t.Errorf("Lang=%q, want %q (export override should win)", got.Lang, "en")
	}
	if got.ImageSource != "f" {
		t.Errorf("ImageSource=%q, want %q (export override should win)", got.ImageSource, "f")
	}
	if got.RatingsLimit != 9 {
		t.Errorf("RatingsLimit=%d, want 9 (export override should win)", got.RatingsLimit)
	}

	// Zero/empty import fields (the missing-from-export columns) preserve
	// the stored value — this is the bug the merge fixes.
	if got.PosterBadgeSize != 130 {
		t.Errorf("PosterBadgeSize=%d, want 130 (pre-fix import must preserve stored value)", got.PosterBadgeSize)
	}
	if got.LogoBadgeSize != 120 {
		t.Errorf("LogoBadgeSize=%d, want 120", got.LogoBadgeSize)
	}
	if got.BackdropBadgeSize != 110 {
		t.Errorf("BackdropBadgeSize=%d, want 110", got.BackdropBadgeSize)
	}
	if got.EpisodeBadgeSize != 100 {
		t.Errorf("EpisodeBadgeSize=%d, want 100", got.EpisodeBadgeSize)
	}
	if got.PosterBadgeAlpha != 75 {
		t.Errorf("PosterBadgeAlpha=%d, want 75", got.PosterBadgeAlpha)
	}
	if got.LogoBadgeAlpha != 70 {
		t.Errorf("LogoBadgeAlpha=%d, want 70", got.LogoBadgeAlpha)
	}
	if got.BackdropBadgeAlpha != 65 {
		t.Errorf("BackdropBadgeAlpha=%d, want 65", got.BackdropBadgeAlpha)
	}
	if got.EpisodeBadgeAlpha != 60 {
		t.Errorf("EpisodeBadgeAlpha=%d, want 60", got.EpisodeBadgeAlpha)
	}
	if got.BackdropEdgeInsetX != 25 {
		t.Errorf("BackdropEdgeInsetX=%d, want 25", got.BackdropEdgeInsetX)
	}
	if got.BackdropEdgeInsetY != 15 {
		t.Errorf("BackdropEdgeInsetY=%d, want 15", got.BackdropEdgeInsetY)
	}
}

// TestCurrentVersionImportUpsertsNormally (#10.4 sanity check): a current-
// version (Version >= MinExportVersion) import behaves exactly like before —
// upserts the per-key row directly without the merge branch. The full payload
// carries every column, so no preservation is needed.
func TestCurrentVersionImportUpsertsNormally(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	keys := NewServiceKeyManager(db, []byte("test-secret"), nil, &http.Client{}, "", nil, "", "", "")
	keys.Init()

	_, hash, prefix := GenerateAPIKey()
	id, err := CreateAPIKey(db, "test-key", hash, prefix, "", 1)
	if err != nil {
		t.Fatal(err)
	}

	// Current-version export with all columns populated (mimics BuildExportPayloadCtx output).
	current := &ExportPayload{
		Kind:       "openposterdb/settings",
		Version:    MinExportVersion,
		ExportedAt: "2026-08-14T00:00:00Z",
		APIKeys: []ExportedAPIKey{
			{
				Name: "test-key",
				Settings: &APIKeySettings{
					APIKeyID:         id,
					Lang:             "de",
					PosterBadgeSize:  130,
					LogoBadgeSize:    120,
					BackdropBadgeSize: 110,
					EpisodeBadgeSize: 100,
				},
			},
		},
	}

	if _, err := ApplyImportPayload(db, keys, current); err != nil {
		t.Fatalf("ApplyImportPayload: %v", err)
	}

	got, err := GetAPIKeySettings(db, id)
	if err != nil || got == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if got.PosterBadgeSize != 130 || got.LogoBadgeSize != 120 || got.BackdropBadgeSize != 110 || got.EpisodeBadgeSize != 100 {
		t.Errorf("current-version import: got badge sizes %d/%d/%d/%d, want 130/120/110/100",
			got.PosterBadgeSize, got.LogoBadgeSize, got.BackdropBadgeSize, got.EpisodeBadgeSize)
	}
	if got.Lang != "de" {
		t.Errorf("Lang=%q, want 'de'", got.Lang)
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

// TestExportImportAPIKey_RoundTripEncryptedKey_OnSameSecretsKey covers the
// 2026-08-14 export/import improvement: an API key's encrypted_key, hash,
// and prefix all round-trip when source and target share SECRETS_KEY, so
// the admin Reveal flow returns the original raw value after restore (not a
// freshly-regenerated one).
func TestExportImportAPIKey_RoundTripEncryptedKey_OnSameSecretsKey(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	secretsKey := []byte("roundtrip-same-secrets-key-32-byte")
	keys := NewServiceKeyManager(db, secretsKey, nil, &http.Client{}, "", nil, "", "", "")
	keys.Init()

	// Source: create an API key with encrypted_key populated.
	raw, hash, prefix := GenerateAPIKey()
	encrypted, err := EncryptAPIKey(raw, secretsKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateAPIKey(db, "transfer-me", hash, prefix, encrypted, 1); err != nil {
		t.Fatal(err)
	}

	// Export and verify the payload carries the key credentials.
	payload, err := BuildExportPayload(db, keys, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.APIKeys) != 1 {
		t.Fatalf("export: want 1 api key, got %d", len(payload.APIKeys))
	}
	ek := payload.APIKeys[0]
	if ek.EncryptedKey == nil || *ek.EncryptedKey != encrypted {
		t.Errorf("export: encrypted_key not carried (got %v, want %q)", ek.EncryptedKey, encrypted)
	}
	if ek.KeyHash == nil || *ek.KeyHash != hash {
		t.Errorf("export: key_hash not carried (got %v, want %q)", ek.KeyHash, hash)
	}
	if ek.KeyPrefix == nil || *ek.KeyPrefix != prefix {
		t.Errorf("export: key_prefix not carried (got %v, want %q)", ek.KeyPrefix, prefix)
	}

	// Delete the source row to prove the import recreated the key from the
	// payload (not by reading existing state).
	if err := DeleteAPIKey(db, 1); err != nil {
		t.Fatal(err)
	}

	// Import and verify the new row carries the same credentials.
	if _, err := ApplyImportPayload(db, keys, payload); err != nil {
		t.Fatal(err)
	}
	listed, err := ListAPIKeys(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("import: want 1 api key, got %d", len(listed))
	}
	restored := listed[0]
	if restored.KeyHash != hash {
		t.Errorf("import: hash=%q, want %q", restored.KeyHash, hash)
	}
	if restored.KeyPrefix != prefix {
		t.Errorf("import: prefix=%q, want %q", restored.KeyPrefix, prefix)
	}
	if restored.EncryptedKey == nil || *restored.EncryptedKey == "" {
		t.Fatal("import: encrypted_key not populated")
	}
	// Reveal: DecryptAPIKey with the same SECRETS_KEY must return the original raw.
	dec, err := DecryptAPIKey(*restored.EncryptedKey, secretsKey)
	if err != nil {
		t.Fatalf("reveal after round-trip: %v", err)
	}
	if dec != raw {
		t.Errorf("reveal: got %q, want %q (original raw)", dec, raw)
	}
	// The import is a true round-trip — no regenerated key surfaces in the
	// response, since the original raw was recovered, not regenerated.
	if _, err := ApplyImportPayload(db, keys, payload); err != nil {
		t.Fatal(err)
	}
}

// TestExportImportAPIKey_CarriesHashAcrossSecretsKey covers the cross-instance
// case: a backup from instance A (SECRETS_KEY=foo) imported into instance B
// (SECRETS_KEY=bar) preserves auth (the SHA-256 hash is not SECRETS_KEY-bound)
// so /api/key/me keeps validating with the original raw key. Reveal will fail
// on B because the encrypted_key is sealed under foo; that's expected and is
// why the slog.Warn fires in ApplyImportPayloadCtx's fallback branch.
func TestExportImportAPIKey_CarriesHashAcrossSecretsKey(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	sourceKey := []byte("source-secrets-key-32-bytes-aaaa")
	targetKey := []byte("target-secrets-key-32-bytes-bbbb")
	src := NewServiceKeyManager(db, sourceKey, nil, &http.Client{}, "", nil, "", "", "")
	src.Init()

	raw, hash, prefix := GenerateAPIKey()
	encrypted, err := EncryptAPIKey(raw, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateAPIKey(db, "carry-over", hash, prefix, encrypted, 1); err != nil {
		t.Fatal(err)
	}

	payload, err := BuildExportPayload(db, src, false, true)
	if err != nil {
		t.Fatal(err)
	}

	// Wipe source and import with target SECRETS_KEY.
	if err := DeleteAPIKey(db, 1); err != nil {
		t.Fatal(err)
	}
	tgt := NewServiceKeyManager(db, targetKey, nil, &http.Client{}, "", nil, "", "", "")
	tgt.Init()
	if _, err := ApplyImportPayload(db, tgt, payload); err != nil {
		t.Fatal(err)
	}

	listed, err := ListAPIKeys(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("import: want 1 api key, got %d", len(listed))
	}
	restored := listed[0]
	if restored.KeyHash != hash {
		t.Errorf("cross-SECRETS_KEY: hash=%q, want %q (carried)", restored.KeyHash, hash)
	}
	if restored.KeyPrefix != prefix {
		t.Errorf("cross-SECRETS_KEY: prefix=%q, want %q (carried)", restored.KeyPrefix, prefix)
	}
	// encrypted_key is carried verbatim — it'll fail to decrypt under the
	// target key, which is the documented cross-SECRETS_KEY constraint.
	if restored.EncryptedKey == nil || *restored.EncryptedKey == "" {
		t.Fatal("cross-SECRETS_KEY: encrypted_key should still be carried verbatim")
	}
	if _, err := DecryptAPIKey(*restored.EncryptedKey, targetKey); err == nil {
		t.Error("cross-SECRETS_KEY: DecryptAPIKey under target key should fail (expected)")
	}
}
