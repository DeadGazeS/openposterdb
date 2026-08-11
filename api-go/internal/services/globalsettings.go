package services

import (
	"context"
	"database/sql"
	"strings"
)

func GetGlobalSettingCtx(ctx context.Context, db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM global_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func GetGlobalSetting(db *sql.DB, key string) (string, error) {
	return GetGlobalSettingCtx(context.Background(), db, key)
}

func GetGlobalSettingsCtx(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM global_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, nil
}

func GetGlobalSettings(db *sql.DB) (map[string]string, error) {
	return GetGlobalSettingsCtx(context.Background(), db)
}

func SetGlobalSettingCtx(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

func SetGlobalSetting(db *sql.DB, key, value string) error {
	return SetGlobalSettingCtx(context.Background(), db, key, value)
}

func SetGlobalSettingsBatchCtx(ctx context.Context, db *sql.DB, settings map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range settings {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
			key, value, value,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func SetGlobalSettingsBatch(db *sql.DB, settings map[string]string) error {
	return SetGlobalSettingsBatchCtx(context.Background(), db, settings)
}

// RemoveGlobalSettings deletes the given keys from global_settings. Used to
// drop settings that are no longer set (e.g. per-source color overrides that
// were reset to their default), since SetGlobalSettingsBatch only upserts.
func RemoveGlobalSettingsCtx(ctx context.Context, db *sql.DB, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, "DELETE FROM global_settings WHERE key = ?", key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RemoveGlobalSettings(db *sql.DB, keys []string) error {
	return RemoveGlobalSettingsCtx(context.Background(), db, keys)
}

// PruneStaleColorSettings deletes per-source color rows (color_<key>_<attr>)
// that exist in the current globals but are absent from the new batch. This
// keeps reset colors / toggled-off borders from resurrecting on the next load,
// because the batch only carries non-default overrides.
func PruneStaleColorSettingsCtx(ctx context.Context, db *sql.DB, globals, batch map[string]string) error {
	var stale []string
	for key := range globals {
		if !strings.HasPrefix(key, "color_") {
			continue
		}
		if _, ok := batch[key]; !ok {
			stale = append(stale, key)
		}
	}
	return RemoveGlobalSettingsCtx(ctx, db, stale)
}

func PruneStaleColorSettings(db *sql.DB, globals, batch map[string]string) error {
	return PruneStaleColorSettingsCtx(context.Background(), db, globals, batch)
}

// --- Per-key settings ---
