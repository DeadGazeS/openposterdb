package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func CountImageMetaCtx(ctx context.Context, db *sql.DB, imageType string) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM image_meta WHERE image_type = ?", imageType).Scan(&count)
	return count, err
}

func CountImageMeta(db *sql.DB, imageType string) (int64, error) {
	return CountImageMetaCtx(context.Background(), db, imageType)
}

type ImageMetaItem struct {
	CacheKey     string  `json:"cache_key"`
	ReleaseDate  *string `json:"release_date"`
	ImageType    string  `json:"image_type"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
	LastAccessed int64   `json:"last_accessed"`
}

// allowlisted columns and directions for ListImageMetaByKindCtx sorting — the
// ORDER BY is built from these maps only, never from caller input. The
// ORDER BY appends `cache_key <dir>` as a deterministic tiebreaker that
// follows the sort direction: with tied primary values (e.g. rows created
// in the same second, or last_accessed = 0), ASC and DESC still produce
// visibly reversed orders, so direction flips always reorder the list.
var listImageMetaSortColumns = map[string]bool{
	"release_date":  true,
	"created_at":    true,
	"updated_at":    true,
	"last_accessed": true,
}

var listImageMetaSortDirs = map[string]bool{
	"ASC":  true,
	"DESC": true,
}

func ListImageMetaByKindCtx(ctx context.Context, db *sql.DB, imageType, sortBy, sortDir string, page, pageSize int64) ([]ImageMetaItem, int64, error) {
	if !listImageMetaSortColumns[sortBy] {
		sortBy = "created_at"
	}
	// Callers send the direction in either case (the web UI sends lowercase
	// "asc"/"desc") — normalize before the allowlist check, otherwise every
	// lowercase value silently fell back to DESC and direction flips were
	// no-ops at the SQL level.
	sortDir = strings.ToUpper(sortDir)
	if !listImageMetaSortDirs[sortDir] {
		sortDir = "DESC"
	}

	var total int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM image_meta WHERE image_type = ?", imageType).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := "SELECT cache_key, release_date, image_type, created_at, updated_at, last_accessed FROM image_meta WHERE image_type = ? ORDER BY " + sortBy + " " + sortDir + ", cache_key " + sortDir + " LIMIT ? OFFSET ?"
	rows, err := db.QueryContext(ctx, query, imageType, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []ImageMetaItem
	for rows.Next() {
		var item ImageMetaItem
		if err := rows.Scan(&item.CacheKey, &item.ReleaseDate, &item.ImageType, &item.CreatedAt, &item.UpdatedAt, &item.LastAccessed); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func ListImageMetaByKind(db *sql.DB, imageType, sortBy, sortDir string, page, pageSize int64) ([]ImageMetaItem, int64, error) {
	return ListImageMetaByKindCtx(context.Background(), db, imageType, sortBy, sortDir, page, pageSize)
}

// --- Global settings ---

func ReadAvailableRatingsCtx(ctx context.Context, db *sql.DB, idKey string) (sources string, updatedAt int64, releaseDate *string, err error) {
	err = db.QueryRowContext(ctx,
		"SELECT sources, updated_at, release_date FROM available_ratings WHERE id_key = ?",
		idKey,
	).Scan(&sources, &updatedAt, &releaseDate)
	if err == sql.ErrNoRows {
		return "", 0, nil, nil
	}
	return
}

func ReadAvailableRatings(db *sql.DB, idKey string) (sources string, updatedAt int64, releaseDate *string, err error) {
	return ReadAvailableRatingsCtx(context.Background(), db, idKey)
}

func UpsertAvailableRatingsCtx(ctx context.Context, db *sql.DB, idKey, sources string, releaseDate *string) error {
	now := nowUnix()
	_, err := db.ExecContext(ctx,
		`INSERT INTO available_ratings (id_key, sources, updated_at, release_date) VALUES (?, ?, ?, ?)
		ON CONFLICT(id_key) DO UPDATE SET sources = ?, updated_at = ?, release_date = ?`,
		idKey, sources, now, releaseDate, sources, now, releaseDate,
	)
	return err
}

func UpsertAvailableRatings(db *sql.DB, idKey, sources string, releaseDate *string) error {
	return UpsertAvailableRatingsCtx(context.Background(), db, idKey, sources, releaseDate)
}

func DeleteAvailableRatingsCtx(ctx context.Context, db *sql.DB, idKey string) (int64, error) {
	result, err := db.ExecContext(ctx, "DELETE FROM available_ratings WHERE id_key = ?", idKey)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteAvailableRatings(db *sql.DB, idKey string) (int64, error) {
	return DeleteAvailableRatingsCtx(context.Background(), db, idKey)
}

func DeleteAllAvailableRatingsCtx(ctx context.Context, db *sql.DB) (int64, error) {
	result, err := db.ExecContext(ctx, "DELETE FROM available_ratings")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteAllAvailableRatings(db *sql.DB) (int64, error) {
	return DeleteAllAvailableRatingsCtx(context.Background(), db)
}

// --- Image meta bulk operations ---

func DeleteImageMetaByKindCtx(ctx context.Context, db *sql.DB, imageType string) (int64, error) {
	result, err := db.ExecContext(ctx, "DELETE FROM image_meta WHERE image_type = ?", imageType)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteImageMetaByKind(db *sql.DB, imageType string) (int64, error) {
	return DeleteImageMetaByKindCtx(context.Background(), db, imageType)
}

func DeleteAllImageMetaCtx(ctx context.Context, db *sql.DB) (int64, error) {
	result, err := db.ExecContext(ctx, "DELETE FROM image_meta")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteAllImageMeta(db *sql.DB) (int64, error) {
	return DeleteAllImageMetaCtx(context.Background(), db)
}

func UpsertImageMetaCtx(ctx context.Context, db *sql.DB, cacheKey string, releaseDate *string, imageType string) error {
	now := nowUnix()
	_, err := db.ExecContext(ctx,
		`INSERT INTO image_meta (cache_key, release_date, image_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET release_date = ?, updated_at = ?`,
		cacheKey, releaseDate, imageType, now, now, releaseDate, now,
	)
	return err
}

func UpsertImageMeta(db *sql.DB, cacheKey string, releaseDate *string, imageType string) error {
	return UpsertImageMetaCtx(context.Background(), db, cacheKey, releaseDate, imageType)
}

// TouchImageAccess bumps image_meta.last_accessed for the given cache_key,
// throttled to once per 60 seconds per key. A non-existent cache_key is a no-op
// (the row is only created by UpsertImageMeta on the cache-miss/regen path;
// this helper exists to record the serve event without forcing a write).
func TouchImageAccessCtx(ctx context.Context, db *sql.DB, cacheKey string) error {
	var lastAccessed int64
	err := db.QueryRowContext(ctx, "SELECT last_accessed FROM image_meta WHERE cache_key = ?", cacheKey).Scan(&lastAccessed)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("touch image access: select: %w", err)
	}

	now := nowUnix()
	if now-lastAccessed < 60 {
		return nil
	}

	_, err = db.ExecContext(ctx, "UPDATE image_meta SET last_accessed = ? WHERE cache_key = ?", now, cacheKey)
	if err != nil {
		return fmt.Errorf("touch image access: update: %w", err)
	}
	return nil
}

func TouchImageAccess(db *sql.DB, cacheKey string) error {
	return TouchImageAccessCtx(context.Background(), db, cacheKey)
}

func ReadImageMetaCtx(ctx context.Context, db *sql.DB, cacheKey string) (*string, error) {
	var releaseDate *string
	err := db.QueryRowContext(ctx, "SELECT release_date FROM image_meta WHERE cache_key = ?", cacheKey).Scan(&releaseDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return releaseDate, err
}

func ReadImageMeta(db *sql.DB, cacheKey string) (*string, error) {
	return ReadImageMetaCtx(context.Background(), db, cacheKey)
}

// --- Rating key validation ---

// ImageSubdir resolves the cache subdirectory for an image kind, accepting
// either the full kind name ("poster") or its single-letter form ("p").
func ImageSubdir(key string) string {
	switch key {
	case "poster", "p":
		return "posters"
	case "logo", "l":
		return "logos"
	case "backdrop", "b":
		return "backdrops"
	case "episode", "e":
		return "episodes"
	}
	return ""
}

// ImageExt resolves the file extension for an image kind, accepting either
// the full kind name ("logo") or its single-letter form ("l").
func ImageExt(key string) string {
	switch key {
	case "logo", "l":
		return "png"
	default:
		return "jpg"
	}
}

// --- Single variant exact delete ---

func DeleteImageMetaExactCtx(ctx context.Context, db *sql.DB, imageType, cacheKey string) (int64, error) {
	result, err := db.ExecContext(ctx,
		"DELETE FROM image_meta WHERE cache_key = ? AND image_type = ?",
		cacheKey, imageType,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteImageMetaExact(db *sql.DB, imageType, cacheKey string) (int64, error) {
	return DeleteImageMetaExactCtx(context.Background(), db, imageType, cacheKey)
}

// --- Title-scoped delete ---

func DeleteImageMetaForTitleCtx(ctx context.Context, db *sql.DB, imageType, idType, idValue string) (int64, error) {
	prefix := idType + "/" + idValue

	rows, err := db.QueryContext(ctx,
		"SELECT cache_key FROM image_meta WHERE image_type = ? AND cache_key LIKE ?",
		imageType, prefix+"%",
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			continue
		}
		if TitleFileMatch(k, prefix) {
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		placeholders[i] = "?"
		args[i] = k
	}

	query := fmt.Sprintf(
		"DELETE FROM image_meta WHERE cache_key IN (%s)",
		strings.Join(placeholders, ","),
	)
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteImageMetaForTitle(db *sql.DB, imageType, idType, idValue string) (int64, error) {
	return DeleteImageMetaForTitleCtx(context.Background(), db, imageType, idType, idValue)
}
