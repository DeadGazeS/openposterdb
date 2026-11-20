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
	"time"
)

// Disk cache size cap (CACHE_MAX_SIZE). Settings changes orphan renders
// under their old cache keys and base/ (downloaded artwork) is never pruned,
// so without a cap the cache only grows. When it is over the cap, the least
// recently used files are deleted until it is at cacheCapTarget of it:
// rendered images by image_meta.last_accessed (else updated_at), everything
// else (base/, preview/) by modification time.

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
}

// StartCacheSizeCap checks the cache size every cacheCapInterval (first
// after a minute) until ctx is done. maxBytes <= 0 disables it.
func StartCacheSizeCap(ctx context.Context, db *sql.DB, cacheDir string, maxBytes int64) {
	if maxBytes <= 0 {
		return
	}
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
	files, total, err := listCacheFiles(ctx, cacheDir)
	if err != nil || total <= maxBytes {
		return 0, 0, err
	}
	if db != nil {
		FlushImageAccess(db) // pending touches count as use
		if err := fillLastUsed(ctx, db, files); err != nil {
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
	}
	if removed > 0 {
		notePurge()
	}
	if db != nil && len(metaRows) > 0 {
		if err := deleteImageMetaRows(context.WithoutCancel(ctx), db, metaRows); err != nil {
			slog.Warn("cache size sweep: image_meta cleanup failed", "rows", len(metaRows), "error", err)
		}
	}
	return removed, freed, nil
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
