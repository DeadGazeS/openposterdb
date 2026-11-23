package services

import (
	"context"
	"testing"
)

// The error-returning cached getters report a failed read instead of
// defaults, and the failure is not cached: once the table is back the next
// call reads it.
func TestSettingsCache_ReadErrorsReturned(t *testing.T) {
	ctx := context.Background()
	db := newSettingsCacheTestDB(t)
	id, err := CreateAPIKeyCtx(ctx, db, "k", "hash1", "pfx", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalSettingsBatchCtx(ctx, db, map[string]string{"lang": "it", "free_api_key_enabled": "true"}); err != nil {
		t.Fatal(err)
	}

	// A key without stored settings is no error: it gets the globals.
	if s, err := EffectiveRenderSettingsCachedErrCtx(ctx, db, id); err != nil || s.Lang != "it" {
		t.Fatalf("no per-key row: %q, %v; want it, nil", s.Lang, err)
	}

	if err := UpsertAPIKeySettingsCtx(ctx, db, &APIKeySettings{APIKeyID: id, Lang: "de"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE api_key_settings RENAME TO aks_gone`); err != nil {
		t.Fatal(err)
	}
	bumpSettingsGen(db)
	if _, err := EffectiveRenderSettingsCachedErrCtx(ctx, db, id); err == nil {
		t.Fatal("per-key read failed: want an error")
	}
	if _, err := db.Exec(`ALTER TABLE aks_gone RENAME TO api_key_settings`); err != nil {
		t.Fatal(err)
	}
	if s, err := EffectiveRenderSettingsCachedErrCtx(ctx, db, id); err != nil || s.Lang != "de" {
		t.Fatalf("after repair: %q, %v; want de, nil (failure must not be cached)", s.Lang, err)
	}

	if _, err := db.Exec(`ALTER TABLE global_settings RENAME TO gs_gone`); err != nil {
		t.Fatal(err)
	}
	bumpSettingsGen(db)
	if _, err := GlobalRenderSettingsCachedErrCtx(ctx, db); err == nil {
		t.Error("globals read failed: GlobalRenderSettingsCachedErrCtx wants an error")
	}
	if _, err := GlobalSettingCachedErrCtx(ctx, db, "free_api_key_enabled"); err == nil {
		t.Error("globals read failed: GlobalSettingCachedErrCtx wants an error")
	}
	if _, err := EffectiveRenderSettingsCachedErrCtx(ctx, db, id); err == nil {
		t.Error("globals read failed: EffectiveRenderSettingsCachedErrCtx wants an error")
	}
	if _, err := db.Exec(`ALTER TABLE gs_gone RENAME TO global_settings`); err != nil {
		t.Fatal(err)
	}
	if v, err := GlobalSettingCachedErrCtx(ctx, db, "free_api_key_enabled"); err != nil || v != "true" {
		t.Fatalf("after repair: %q, %v; want true, nil", v, err)
	}
	if s, err := GlobalRenderSettingsCachedErrCtx(ctx, db); err != nil || s.Lang != "it" {
		t.Fatalf("after repair: %q, %v; want it, nil", s.Lang, err)
	}
}
