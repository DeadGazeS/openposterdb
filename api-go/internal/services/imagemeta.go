package services

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	// TitleKey groups every cached image of one title in the admin list
	// (NOTES.md #8); "" for rows written before the column existed.
	TitleKey string `json:"title_key"`
	// Title is the display name (NOTES.md #9); nil for older rows.
	Title *string `json:"title"`
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
	query := "SELECT cache_key, release_date, image_type, created_at, updated_at, last_accessed, title_key, title FROM image_meta WHERE image_type = ? ORDER BY " + sortBy + " " + sortDir + ", cache_key " + sortDir + " LIMIT ? OFFSET ?"
	rows, err := db.QueryContext(ctx, query, imageType, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// Non-nil so an empty page encodes as [] (not null) — the admin list
	// view iterates items directly.
	items := make([]ImageMetaItem, 0)
	for rows.Next() {
		var item ImageMetaItem
		if err := rows.Scan(&item.CacheKey, &item.ReleaseDate, &item.ImageType, &item.CreatedAt, &item.UpdatedAt, &item.LastAccessed, &item.TitleKey, &item.Title); err != nil {
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

func UpsertImageMetaCtx(ctx context.Context, db *sql.DB, cacheKey string, releaseDate *string, imageType, titleKey string, title *string) error {
	now := nowUnix()
	_, err := db.ExecContext(ctx,
		`INSERT INTO image_meta (cache_key, release_date, image_type, created_at, updated_at, title_key, title) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET release_date = ?, updated_at = ?, title_key = ?, title = ?`,
		cacheKey, releaseDate, imageType, now, now, titleKey, title, releaseDate, now, titleKey, title,
	)
	return err
}

func UpsertImageMeta(db *sql.DB, cacheKey string, releaseDate *string, imageType, titleKey string, title *string) error {
	return UpsertImageMetaCtx(context.Background(), db, cacheKey, releaseDate, imageType, titleKey, title)
}

// Image access tracking. Serving a cached image used to cost a SELECT (plus
// an UPDATE at most once a minute per key) on the request path. Touches are
// now recorded in memory and written in one transaction at most once per
// imageAccessFlushInterval (triggered by a later touch), plus on shutdown via
// FlushImageAccess. last_accessed therefore lags by up to the interval — the
// same resolution the old per-key throttle had.

var imageAccessFlushInterval = 60 * time.Second

type accessRecorder struct {
	flushMu   sync.Mutex // serializes flushes (the shutdown flush waits for a running one)
	mu        sync.Mutex
	pending   map[string]int64 // cache_key -> unix seconds of the latest serve
	lastFlush atomic.Int64     // unix nanos
	flushing  atomic.Bool
}

var accessRecorders sync.Map // *sql.DB -> *accessRecorder

func accessRecorderFor(db *sql.DB) *accessRecorder {
	if r, ok := accessRecorders.Load(db); ok {
		return r.(*accessRecorder)
	}
	r := &accessRecorder{pending: make(map[string]int64)}
	r.lastFlush.Store(time.Now().UnixNano())
	actual, _ := accessRecorders.LoadOrStore(db, r)
	return actual.(*accessRecorder)
}

// TouchImageAccessCtx records that cacheKey was just served. It never touches
// the database on the calling goroutine; a missing image_meta row is simply
// not updated at flush time. Always returns nil (kept for call-site shape).
func TouchImageAccessCtx(ctx context.Context, db *sql.DB, cacheKey string) error {
	r := accessRecorderFor(db)
	r.mu.Lock()
	r.pending[cacheKey] = nowUnix()
	r.mu.Unlock()
	if time.Since(time.Unix(0, r.lastFlush.Load())) >= imageAccessFlushInterval && r.flushing.CompareAndSwap(false, true) {
		go func() {
			defer r.flushing.Store(false)
			r.flush(db)
		}()
	}
	return nil
}

// FlushImageAccess writes all pending access times now (shutdown, tests).
func FlushImageAccess(db *sql.DB) {
	if r, ok := accessRecorders.Load(db); ok {
		r.(*accessRecorder).flush(db)
	}
}

func (r *accessRecorder) flush(db *sql.DB) {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	r.lastFlush.Store(time.Now().UnixNano())
	r.mu.Lock()
	batch := r.pending
	r.pending = make(map[string]int64, len(batch))
	r.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := writeImageAccess(ctx, db, batch); err != nil {
		slog.Warn("image access flush failed; will retry", "keys", len(batch), "error", err)
		// Put the batch back for the next flush, keeping newer touches.
		r.mu.Lock()
		for k, at := range batch {
			if cur, ok := r.pending[k]; !ok || at > cur {
				r.pending[k] = at
			}
		}
		r.mu.Unlock()
	}
}

func writeImageAccess(ctx context.Context, db *sql.DB, batch map[string]int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "UPDATE image_meta SET last_accessed = ? WHERE cache_key = ? AND last_accessed < ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for key, at := range batch {
		if _, err := stmt.ExecContext(ctx, at, key, at); err != nil {
			return err
		}
	}
	return tx.Commit()
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

	// A byte range on the primary key instead of LIKE: SQLite's LIKE is
	// case-insensitive (no index use, so it scanned every row of the kind)
	// and treats "_" in an id as a wildcard. TitleFileMatch below still
	// filters out keys that merely share the prefix. "+image_type" keeps the
	// planner on the primary-key range instead of scanning the whole kind.
	rows, err := db.QueryContext(ctx,
		"SELECT cache_key FROM image_meta WHERE +image_type = ? AND cache_key >= ? AND cache_key < ?",
		imageType, prefix, prefixUpperBound(prefix),
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

// prefixUpperBound returns the smallest string greater than every string
// starting with prefix (for `>= prefix AND < bound` range scans).
func prefixUpperBound(prefix string) string {
	b := []byte(prefix)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xff {
			b[i]++
			return string(b[:i+1])
		}
	}
	return string(b) + "\xff"
}
