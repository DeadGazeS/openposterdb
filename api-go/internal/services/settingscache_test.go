package services

import (
	"context"
	"database/sql"
	"testing"
)

func newSettingsCacheTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := newExportTestDB(t)
	db.SetMaxOpenConns(1) // :memory: is per connection
	t.Cleanup(func() { db.Close() })
	return db
}

// The cached lookups serve from memory (a write that bypasses the services
// layer is not seen), and every services-layer write invalidates them at once.
func TestSettingsCache_EffectiveSettings(t *testing.T) {
	ctx := context.Background()
	db := newSettingsCacheTestDB(t)
	id, err := CreateAPIKeyCtx(ctx, db, "k", "hash1", "pfx", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertAPIKeySettingsCtx(ctx, db, &APIKeySettings{APIKeyID: id, Lang: "de"}); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveRenderSettingsCachedCtx(ctx, db, id).Lang; got != "de" {
		t.Fatalf("Lang = %q, want de", got)
	}

	// Out-of-band write: still cached.
	if _, err := db.Exec(`UPDATE api_key_settings SET lang = 'fr' WHERE api_key_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveRenderSettingsCachedCtx(ctx, db, id).Lang; got != "de" {
		t.Fatalf("expected the cached value, got %q", got)
	}

	// Services-layer writes invalidate immediately.
	if err := UpsertAPIKeySettingsCtx(ctx, db, &APIKeySettings{APIKeyID: id, Lang: "es"}); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveRenderSettingsCachedCtx(ctx, db, id).Lang; got != "es" {
		t.Fatalf("after upsert Lang = %q, want es", got)
	}
	if err := DeleteAPIKeySettingsCtx(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalSettingsBatchCtx(ctx, db, map[string]string{"lang": "it"}); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveRenderSettingsCachedCtx(ctx, db, id).Lang; got != "it" {
		t.Fatalf("after delete + global change Lang = %q, want it (globals)", got)
	}
	if got := GlobalRenderSettingsCachedCtx(ctx, db).Lang; got != "it" {
		t.Fatalf("globals Lang = %q, want it", got)
	}
	if err := RemoveGlobalSettingsCtx(ctx, db, []string{"lang"}); err != nil {
		t.Fatal(err)
	}
	if got := GlobalRenderSettingsCachedCtx(ctx, db).Lang; got == "it" {
		t.Fatal("RemoveGlobalSettings must invalidate the globals cache")
	}
	if err := SetGlobalSettingCtx(ctx, db, "free_api_key_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if got := GlobalSettingCachedCtx(ctx, db, "free_api_key_enabled"); got != "true" {
		t.Fatalf("GlobalSettingCached = %q", got)
	}
}

// Callers mutate the returned settings (lang/query overrides); that must not
// leak into the cache.
func TestSettingsCache_ReturnsCopies(t *testing.T) {
	ctx := context.Background()
	db := newSettingsCacheTestDB(t)
	s := GlobalRenderSettingsCachedCtx(ctx, db)
	s.Lang = "xx"
	if GlobalRenderSettingsCachedCtx(ctx, db).Lang == "xx" {
		t.Fatal("mutating a returned value changed the cache")
	}
	id, _ := CreateAPIKeyCtx(ctx, db, "k", "h", "p", "", 1)
	e := EffectiveRenderSettingsCachedCtx(ctx, db, id)
	e.Lang = "yy"
	if EffectiveRenderSettingsCachedCtx(ctx, db, id).Lang == "yy" {
		t.Fatal("mutating a returned effective value changed the cache")
	}
	k, _ := FindAPIKeyByHashCachedCtx(ctx, db, "h")
	k.Name = "changed"
	if k2, _ := FindAPIKeyByHashCachedCtx(ctx, db, "h"); k2.Name != "k" {
		t.Fatal("mutating a returned key changed the cache")
	}
}

func TestSettingsCache_KeyLookup(t *testing.T) {
	ctx := context.Background()
	db := newSettingsCacheTestDB(t)
	if k, err := FindAPIKeyByHashCachedCtx(ctx, db, "nope"); err != nil || k != nil {
		t.Fatalf("unknown key: %v %v", k, err)
	}
	id, err := CreateAPIKeyCtx(ctx, db, "k", "hash2", "pfx", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	// A cached miss must not hide a key created afterwards.
	if k, _ := FindAPIKeyByHashCachedCtx(ctx, db, "hash2"); k == nil || k.ID != id {
		t.Fatalf("created key not found: %v", k)
	}
	if err := DeleteAPIKeyCtx(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if k, _ := FindAPIKeyByHashCachedCtx(ctx, db, "hash2"); k != nil {
		t.Fatal("deleted key still served from cache")
	}
}

// A failing read must not be cached.
func TestSettingsCache_ErrorsNotCached(t *testing.T) {
	ctx := context.Background()
	db := newSettingsCacheTestDB(t)
	if _, err := db.Exec(`ALTER TABLE api_keys RENAME TO api_keys_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := FindAPIKeyByHashCachedCtx(ctx, db, "h"); err == nil {
		t.Fatal("want an error with the table missing")
	}
	if _, err := db.Exec(`ALTER TABLE api_keys_gone RENAME TO api_keys`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO api_keys (name, key_hash, key_prefix, created_by) VALUES ('k','h','p',1)`); err != nil {
		t.Fatal(err)
	}
	if k, err := FindAPIKeyByHashCachedCtx(ctx, db, "h"); err != nil || k == nil {
		t.Fatalf("the earlier error was cached: %v %v", k, err)
	}
}

// Caches are per *sql.DB, so separate databases never see each other's rows.
func TestSettingsCache_PerDB(t *testing.T) {
	ctx := context.Background()
	a, b := newSettingsCacheTestDB(t), newSettingsCacheTestDB(t)
	SetGlobalSettingCtx(ctx, a, "lang", "de")
	SetGlobalSettingCtx(ctx, b, "lang", "fr")
	if GlobalRenderSettingsCachedCtx(ctx, a).Lang != "de" || GlobalRenderSettingsCachedCtx(ctx, b).Lang != "fr" {
		t.Fatal("caches leaked across databases")
	}
}
