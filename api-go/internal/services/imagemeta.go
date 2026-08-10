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
	CacheKey    string  `json:"cache_key"`
	ReleaseDate *string `json:"release_date"`
	ImageType   string  `json:"image_type"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

func ListImageMetaByKindCtx(ctx context.Context, db *sql.DB, imageType string, page, pageSize int64) ([]ImageMetaItem, int64, error) {
	var total int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM image_meta WHERE image_type = ?", imageType).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := db.QueryContext(ctx,
		"SELECT cache_key, release_date, image_type, created_at, updated_at FROM image_meta WHERE image_type = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		imageType, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []ImageMetaItem
	for rows.Next() {
		var item ImageMetaItem
		if err := rows.Scan(&item.CacheKey, &item.ReleaseDate, &item.ImageType, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func ListImageMetaByKind(db *sql.DB, imageType string, page, pageSize int64) ([]ImageMetaItem, int64, error) {
	return ListImageMetaByKindCtx(context.Background(), db, imageType, page, pageSize)
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
