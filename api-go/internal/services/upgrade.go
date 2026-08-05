package services

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func RunUpgrades(db *sql.DB, cacheDir string, externalCacheOnly bool) error {
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS upgrades (name TEXT PRIMARY KEY, completed_at INTEGER NOT NULL)"); err != nil {
		return err
	}

	if err := runOnce(db, "v001_backdrop_cache_keys", func() error {
		return upgradeV001(db, cacheDir, externalCacheOnly)
	}); err != nil {
		return err
	}

	if err := runOnce(db, "v002_backdrop_position_direction_cache", func() error {
		return upgradeV002(db, cacheDir, externalCacheOnly)
	}); err != nil {
		return err
	}

	if err := runOnce(db, "v003_badge_shape_background_cache", func() error {
		return upgradeV003(db, cacheDir, externalCacheOnly)
	}); err != nil {
		return err
	}

	return nil
}

// v001: Migrate backdrop cache keys from `_b@` to `_b_f@`
func upgradeV001(db *sql.DB, cacheDir string, externalCacheOnly bool) error {
	// DB step
	result, err := db.Exec(
		"UPDATE image_meta SET cache_key = replace(cache_key, '_b@', '_b_f@') " +
			"WHERE image_type = 'b' AND instr(cache_key, '_b@') > 0",
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	slog.Info("backdrop cache keys migrated in DB (_b@ → _b_f@)", "db_rows", rows)

	// Filesystem step
	if externalCacheOnly {
		slog.Info("backdrop filesystem rename skipped (external_cache_only)")
		return nil
	}

	backdropDir := filepath.Join(cacheDir, "backdrops")
	renamed, err := renameFilesV001(backdropDir)
	if err != nil {
		return err
	}
	slog.Info("backdrop cache files renamed (_b@ → _b_f@)", "fs_renamed", renamed)
	return nil
}

func renameFilesV001(dir string) (uint64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var count uint64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			n, err := renameFilesV001(path)
			if err != nil {
				return count, err
			}
			count += n
		} else {
			name := entry.Name()
			if strings.Contains(name, "_b@") && !strings.Contains(name, "_b_f@") && !strings.Contains(name, "_b_t@") {
				newName := strings.Replace(name, "_b@", "_b_f@", 1)
				newPath := filepath.Join(dir, newName)
				if err := os.Rename(path, newPath); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}

// v002: Insert `.ptr` (TopRight) and `.dv` (Vertical) into backdrop cache keys
func upgradeV002(db *sql.DB, cacheDir string, externalCacheOnly bool) error {
	// DB step
	var total uint64

	stylePatterns := []string{".sv.", ".sh.", ".sd."}
	for _, style := range stylePatterns {
		old := style + "l" + "."
		newVal := ".ptr." + style + "l" + "."
		oldLen := len(old)
		result, err := db.Exec(
			"UPDATE image_meta SET cache_key = " +
				"substr(cache_key, 1, instr(cache_key, '" + old + "') - 1) || '" + newVal + "' || " +
				"substr(cache_key, instr(cache_key, '" + old + "') + " + stringInt(oldLen) + ") " +
				"WHERE image_type = 'b' AND instr(cache_key, '" + old + "') > 0 AND instr(cache_key, '.ptr.') = 0",
		)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		total += uint64(n)
	}

	labelPrefixes := []string{".lt.", ".li.", ".lo."}
	for _, label := range labelPrefixes {
		old := label + "b" + "."
		newVal := label + "dv." + "b" + "."
		oldLen := len(old)
		result, err := db.Exec(
			"UPDATE image_meta SET cache_key = " +
				"substr(cache_key, 1, instr(cache_key, '" + old + "') - 1) || '" + newVal + "' || " +
				"substr(cache_key, instr(cache_key, '" + old + "') + " + stringInt(oldLen) + ") " +
				"WHERE image_type = 'b' AND instr(cache_key, '" + old + "') > 0 AND instr(cache_key, '.dv.') = 0",
		)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		total += uint64(n)
	}

	slog.Info("backdrop cache keys migrated (added position/direction suffixes)", "db_rows", total)

	// Filesystem step
	if externalCacheOnly {
		slog.Info("backdrop position/direction filesystem rename skipped")
		return nil
	}

	backdropDir := filepath.Join(cacheDir, "backdrops")
	renamed, err := renameFilesV002(backdropDir)
	if err != nil {
		return err
	}
	slog.Info("backdrop cache files renamed (added position/direction suffixes)", "fs_renamed", renamed)
	return nil
}

func renameFilesV002(dir string) (uint64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var count uint64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			n, err := renameFilesV002(path)
			if err != nil {
				return count, err
			}
			count += n
		} else {
			name := entry.Name()
			if newName := migrateNameV002(name); newName != nil {
				newPath := filepath.Join(dir, *newName)
				if err := os.Rename(path, newPath); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}

func migrateNameV002(name string) *string {
	if strings.Contains(name, ".ptr.") && strings.Contains(name, ".dv.") {
		return nil
	}

	result := name
	modified := false

	if !strings.Contains(result, ".ptr.") {
		for _, style := range []string{".sv.l", ".sh.l", ".sd.l"} {
			if idx := strings.Index(result, style); idx >= 0 {
				result = result[:idx] + ".ptr" + result[idx:]
				modified = true
				break
			}
		}
	}

	if !strings.Contains(result, ".dv.") {
		pairs := [][2]string{
			{".lt.b", ".lt.dv.b"},
			{".li.b", ".li.dv.b"},
			{".lo.b", ".lo.dv.b"},
		}
		for _, p := range pairs {
			if strings.Contains(result, p[0]) {
				result = strings.Replace(result, p[0], p[1], 1)
				modified = true
				break
			}
		}
	}

	if modified {
		return &result
	}
	return nil
}

// v003: Insert `.shr.bgd` (default rounded shape + default background) after badge size token
func upgradeV003(db *sql.DB, cacheDir string, externalCacheOnly bool) error {
	// DB step
	sizeTokens := []string{".bxs.", ".bs.", ".bm.", ".bl.", ".bxl."}
	defaultSuffix := "shr.bgd."
	var total uint64

	for _, token := range sizeTokens {
		newToken := token + defaultSuffix
		oldLen := len(token)
		result, err := db.Exec(
			"UPDATE image_meta SET cache_key = " +
				"substr(cache_key, 1, instr(cache_key, '" + token + "') - 1) || '" + newToken + "' || " +
				"substr(cache_key, instr(cache_key, '" + token + "') + " + stringInt(oldLen) + ") " +
				"WHERE instr(cache_key, '" + token + "') > 0 AND instr(cache_key, '.bg') = 0",
		)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		total += uint64(n)
	}

	slog.Info("cache keys migrated (added shape/background suffixes)", "db_rows", total)

	// Filesystem step
	if externalCacheOnly {
		slog.Info("shape/background filesystem rename skipped")
		return nil
	}

	subdirs := []string{"posters", "logos", "backdrops", "episodes"}
	var renamed uint64
	for _, sub := range subdirs {
		n, err := renameFilesV003(filepath.Join(cacheDir, sub))
		if err != nil {
			return err
		}
		renamed += n
	}
	slog.Info("cache files renamed (added shape/background suffixes)", "fs_renamed", renamed)
	return nil
}

func renameFilesV003(dir string) (uint64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var count uint64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			n, err := renameFilesV003(path)
			if err != nil {
				return count, err
			}
			count += n
		} else {
			name := entry.Name()
			if newName := migrateNameV003(name); newName != nil {
				newPath := filepath.Join(dir, *newName)
				if err := os.Rename(path, newPath); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}

func migrateNameV003(name string) *string {
	if strings.Contains(name, ".bg") {
		return nil
	}

	sizeTokens := []string{".bxs.", ".bs.", ".bm.", ".bl.", ".bxl."}
	defaultSuffix := "shr.bgd."
	for _, token := range sizeTokens {
		if strings.Contains(name, token) {
			newToken := token + defaultSuffix
			result := strings.Replace(name, token, newToken, 1)
			return &result
		}
	}
	return nil
}

func stringInt(n int) string {
	return string(rune('0' + n%10))
}

func runOnce(db *sql.DB, name string, f func() error) error {
	var exists int
	if err := db.QueryRow("SELECT 1 FROM upgrades WHERE name = ?", name).Scan(&exists); err == nil {
		return nil
	}
	slog.Info("running data upgrade", "name", name)
	if err := f(); err != nil {
		return err
	}
	if _, err := db.Exec("INSERT INTO upgrades (name, completed_at) VALUES (?, unixepoch())", name); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return err
		}
	}
	slog.Info("data upgrade complete", "name", name)
	return nil
}
