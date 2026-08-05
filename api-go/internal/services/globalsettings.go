package services

import (
	"database/sql"
	"strings"
)

func GetGlobalSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func GetGlobalSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM global_settings")
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

func SetGlobalSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

func SetGlobalSettingsBatch(db *sql.DB, settings map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range settings {
		if _, err := tx.Exec(
			"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
			key, value, value,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveGlobalSettings deletes the given keys from global_settings. Used to
// drop settings that are no longer set (e.g. per-source color overrides that
// were reset to their default), since SetGlobalSettingsBatch only upserts.
func RemoveGlobalSettings(db *sql.DB, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, key := range keys {
		if _, err := tx.Exec("DELETE FROM global_settings WHERE key = ?", key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneStaleColorSettings deletes per-source color rows (color_<key>_<attr>)
// that exist in the current globals but are absent from the new batch. This
// keeps reset colors / toggled-off borders from resurrecting on the next load,
// because the batch only carries non-default overrides.
func PruneStaleColorSettings(db *sql.DB, globals, batch map[string]string) error {
	var stale []string
	for key := range globals {
		if !strings.HasPrefix(key, "color_") {
			continue
		}
		if _, ok := batch[key]; !ok {
			stale = append(stale, key)
		}
	}
	return RemoveGlobalSettings(db, stale)
}

// --- Per-key settings ---
