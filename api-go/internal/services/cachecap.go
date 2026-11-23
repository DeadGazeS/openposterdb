package services

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Disk cache size cap (CACHE_MAX_SIZE). Settings changes orphan renders
// under their old cache keys and base/ (downloaded artwork) is never pruned,
// so without a cap the cache only grows. When it is over the cap, the least
// recently used files are deleted until it is at cacheCapTarget of it:
// rendered images by image_meta.last_accessed (else updated_at), base/
// artwork by its last read (base_access, else mtime), everything else
// (preview/) by modification time.

// cacheCapTarget is the fraction of the cap a sweep shrinks the cache to, so
// it doesn't run again on the next render.
const cacheCapTarget = 0.9

// cacheCapInterval is how often the cache size is checked.
var cacheCapInterval = 10 * time.Minute

var renderedSubdirs = map[string]string{"posters": "p", "logos": "l", "backdrops": "b", "episodes": "e"}

type cacheFile struct {
	path      string
	size      int64
	used      int64  // unix seconds, the LRU order
	imageType string // rendered images only: image_meta.image_type
	cacheKey  string // rendered images only
	baseKey   string // base/ files only: base_access.path ("base/…", an .etag sidecar maps to its image)
}

// Base artwork reads. A base file's mtime belongs to staleness and
// revalidation (IMAGE_STALE_SECS, If-Modified-Since, a 304 touches it), so
// reads are tracked apart from it: recorded in memory (one entry per file,
// however often it is read) and written to base_access when the cache size
// is checked, i.e. at most once per cacheCapInterval per file. Only tracked
// while the cap is on (nothing else reads it).
var (
	baseAccessTracking atomic.Bool
	baseAccessMu       sync.Mutex
	baseAccessPending  = map[string]int64{} // absolute path -> unix seconds of the latest read
)

// NoteBaseAccess records that the base artwork file at path was just read.
func NoteBaseAccess(path string) {
	if !baseAccessTracking.Load() {
		return
	}
	baseAccessMu.Lock()
	baseAccessPending[path] = nowUnix()
	baseAccessMu.Unlock()
}

// flushBaseAccess writes the pending base reads to base_access (paths
// relative to cacheDir). A failed write keeps them for the next flush.
func flushBaseAccess(ctx context.Context, db *sql.DB, cacheDir string) error {
	baseAccessMu.Lock()
	batch := baseAccessPending
	baseAccessPending = make(map[string]int64, len(batch))
	baseAccessMu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	err := func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO base_access (path, last_accessed) VALUES (?, ?)
			ON CONFLICT(path) DO UPDATE SET last_accessed = excluded.last_accessed WHERE excluded.last_accessed > base_access.last_accessed`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for path, at := range batch {
			rel, err := filepath.Rel(cacheDir, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			if _, err := stmt.ExecContext(ctx, filepath.ToSlash(rel), at); err != nil {
				return err
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		baseAccessMu.Lock()
		for k, at := range batch {
			if cur, ok := baseAccessPending[k]; !ok || at > cur {
				baseAccessPending[k] = at
			}
		}
		baseAccessMu.Unlock()
	}
	return err
}

// FlushBaseAccess writes the pending base reads now (shutdown, so up to one
// size-check interval of reads isn't lost). A failure is logged.
func FlushBaseAccess(db *sql.DB, cacheDir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := flushBaseAccess(ctx, db, cacheDir); err != nil {
		slog.Warn("failed to persist base artwork access times", "error", err)
	}
}

// StartCacheSizeCap checks the cache size every cacheCapInterval (first
// after a minute) until ctx is done. maxBytes <= 0 disables it.
func StartCacheSizeCap(ctx context.Context, db *sql.DB, cacheDir string, maxBytes int64) {
	if maxBytes <= 0 {
		return
	}
	baseAccessTracking.Store(true)
	go func() {
		wait := time.Minute
		for {
			if sleepCtx(ctx, wait) != nil {
				return
			}
			wait = cacheCapInterval
			files, freed, err := SweepCacheToSize(ctx, db, cacheDir, maxBytes)
			if err != nil {
				slog.Warn("cache size sweep failed", "error", err)
			} else if files > 0 {
				slog.Info("cache over CACHE_MAX_SIZE: removed least recently used files", "files", files, "freed_bytes", freed, "max_bytes", maxBytes)
			}
		}
	}()
}

// SweepCacheToSize deletes least-recently-used cache files while the cache
// is over maxBytes, down to cacheCapTarget × maxBytes, and the image_meta
// rows of the rendered images removed. Returns what it removed.
func SweepCacheToSize(ctx context.Context, db *sql.DB, cacheDir string, maxBytes int64) (int, int64, error) {
	if maxBytes <= 0 {
		return 0, 0, nil
	}
	if db != nil {
		if err := flushBaseAccess(ctx, db, cacheDir); err != nil {
			slog.Warn("base artwork access flush failed; will retry", "error", err)
		}
	}
	files, total, err := listCacheFiles(ctx, cacheDir)
	if err != nil || total <= maxBytes {
		return 0, 0, err
	}
	var staleBaseRows []string
	if db != nil {
		FlushImageAccess(db) // pending touches count as use
		if err := fillLastUsed(ctx, db, files); err != nil {
			return 0, 0, err
		}
		if staleBaseRows, err = fillBaseLastUsed(ctx, db, files); err != nil {
			return 0, 0, err
		}
	}
	slices.SortStableFunc(files, func(a, b cacheFile) int {
		switch {
		case a.used < b.used:
			return -1
		case a.used > b.used:
			return 1
		}
		return strings.Compare(a.path, b.path)
	})
	target := int64(float64(maxBytes) * cacheCapTarget)
	removed, freed := 0, int64(0)
	var metaRows []cacheFile
	for _, f := range files {
		if total-freed <= target {
			break
		}
		if err := ctx.Err(); err != nil {
			break
		}
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			continue // in use (Windows) or gone: skip it
		}
		removed++
		freed += f.size
		if f.cacheKey != "" {
			metaRows = append(metaRows, f)
		}
		if f.baseKey != "" && !strings.HasSuffix(f.path, ".etag") {
			staleBaseRows = append(staleBaseRows, f.baseKey)
		}
	}
	if removed > 0 {
		notePurge()
	}
	if db != nil && len(metaRows) > 0 {
		if err := deleteImageMetaRows(context.WithoutCancel(ctx), db, metaRows); err != nil {
			slog.Warn("cache size sweep: image_meta cleanup failed", "rows", len(metaRows), "error", err)
		}
	}
	if db != nil && len(staleBaseRows) > 0 {
		if err := deleteBaseAccessRows(context.WithoutCancel(ctx), db, staleBaseRows); err != nil {
			slog.Warn("cache size sweep: base_access cleanup failed", "rows", len(staleBaseRows), "error", err)
		}
	}
	return removed, freed, nil
}

// fillBaseLastUsed sets each base/ file's LRU time to its last recorded read
// when that is later than its mtime (a fresh download counts as use). It
// returns the base_access rows that match no file on disk.
func fillBaseLastUsed(ctx context.Context, db *sql.DB, files []cacheFile) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT path, last_accessed FROM base_access")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	read := make(map[string]int64)
	for rows.Next() {
		var path string
		var at int64
		if err := rows.Scan(&path, &at); err != nil {
			return nil, err
		}
		read[path] = at
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	onDisk := make(map[string]bool)
	for i := range files {
		f := &files[i]
		if f.baseKey == "" {
			continue
		}
		onDisk[f.baseKey] = true
		if at, ok := read[f.baseKey]; ok && at > f.used {
			f.used = at
		}
	}
	var stale []string
	for path := range read {
		if !onDisk[path] {
			stale = append(stale, path)
		}
	}
	return stale, nil
}

func deleteBaseAccessRows(ctx context.Context, db *sql.DB, paths []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "DELETE FROM base_access WHERE path = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range paths {
		if _, err := stmt.ExecContext(ctx, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// listCacheFiles returns every cache file (temp files and dirs staged for
// deletion excluded) with its size and modification time, and the total.
func listCacheFiles(ctx context.Context, cacheDir string) ([]cacheFile, int64, error) {
	var files []cacheFile
	var total int64
	err := filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == cacheDir && os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), purgePrefix) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f := cacheFile{path: path, size: info.Size(), used: info.ModTime().Unix()}
		// <cacheDir>/<posters|logos|…>/<idType>/<cacheValue>.<ext> is the
		// rendered image of cache key "<idType>/<cacheValue>".
		if rel, err := filepath.Rel(cacheDir, path); err == nil {
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) == 3 {
				if t, ok := renderedSubdirs[parts[0]]; ok {
					f.imageType = t
					f.cacheKey = parts[1] + "/" + strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))
				}
			}
			if parts[0] == "base" {
				f.baseKey = strings.TrimSuffix(strings.Join(parts, "/"), ".etag")
			}
		}
		files = append(files, f)
		total += f.size
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return files, total, nil
}

// fillLastUsed sets each rendered file's LRU time from image_meta:
// last_accessed when it was ever served, else updated_at (its render).
func fillLastUsed(ctx context.Context, db *sql.DB, files []cacheFile) error {
	rows, err := db.QueryContext(ctx, "SELECT cache_key, image_type, last_accessed, updated_at FROM image_meta")
	if err != nil {
		return err
	}
	defer rows.Close()
	used := make(map[string]int64)
	for rows.Next() {
		var key, typ string
		var accessed, updated int64
		if err := rows.Scan(&key, &typ, &accessed, &updated); err != nil {
			return err
		}
		used[typ+"|"+key] = max(accessed, updated)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range files {
		f := &files[i]
		if f.cacheKey == "" {
			continue
		}
		if t, ok := used[f.imageType+"|"+f.cacheKey]; ok && t > 0 {
			f.used = t
		}
	}
	return nil
}

func deleteImageMetaRows(ctx context.Context, db *sql.DB, files []cacheFile) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "DELETE FROM image_meta WHERE cache_key = ? AND image_type = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, f := range files {
		if _, err := stmt.ExecContext(ctx, f.cacheKey, f.imageType); err != nil {
			return err
		}
	}
	return tx.Commit()
}
