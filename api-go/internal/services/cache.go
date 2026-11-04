package services

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func ValidateIDValue(idValue string) error {
	if idValue == "" || idValue == "." || idValue == ".." {
		return fmt.Errorf("invalid id value")
	}
	if strings.ContainsAny(idValue, "/\\\x00") {
		return fmt.Errorf("invalid id value")
	}
	return nil
}

func TypedCachePath(cacheDir string, imageType, idType, idValue, ext string) (string, error) {
	if err := ValidateIDValue(idValue); err != nil {
		return "", err
	}
	if err := ValidateIDValue(idType); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, imageType, idType, idValue+"."+ext), nil
}

func BasePosterPath(cacheDir, posterPath, tmdbSize string) (string, error) {
	filename := strings.TrimPrefix(posterPath, "/")
	if err := ValidateIDValue(filename); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "base", "posters", tmdbSize, filename), nil
}

func BaseFanartPath(cacheDir, fanartID, ext string) (string, error) {
	if err := ValidateIDValue(fanartID); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "base", "fanart", fanartID+"."+ext), nil
}

func PreviewPath(cacheDir, imageType, suffix, ext string) (string, error) {
	if err := ValidateIDValue(suffix); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "preview", imageType, suffix+"."+ext), nil
}

type CacheEntry struct {
	Bytes   []byte
	IsStale bool
}

func ReadCache(path string, staleSecs uint64) (*CacheEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	bytes := make([]byte, info.Size())
	if _, err := io.ReadFull(f, bytes); err != nil {
		return nil, err
	}
	return &CacheEntry{Bytes: bytes, IsStale: isStaleAge(info.ModTime(), staleSecs)}, nil
}

// CacheIsStale reports the staleness of a cache file from its mtime alone,
// without reading it (for revalidating entries already held in memory).
func CacheIsStale(path string, staleSecs uint64) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return isStaleAge(info.ModTime(), staleSecs), nil
}

func isStaleAge(modTime time.Time, staleSecs uint64) bool {
	return staleSecs > 0 && uint64(time.Since(modTime).Seconds()) > staleSecs
}

// WriteCache writes atomically (temp file + rename in the same directory), so
// a concurrent ReadCache — e.g. a cache hit during a background refresh —
// never sees a half-written image.
func WriteCache(path string, bytes []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(bytes); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func WriteCacheIf(path string, data io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, data)
	return err
}

func TitleFileMatch(cacheValue, idValue string) bool {
	rest := strings.TrimPrefix(cacheValue, idValue)
	if len(rest) == 0 {
		return len(cacheValue) == len(idValue)
	}
	return strings.HasPrefix(rest, "_") || strings.HasPrefix(rest, "@")
}

// PurgeTitleFiles removes all rendered files for a logical title.
func PurgeTitleFiles(cacheDir, imageType, idType, idValue string) (int64, error) {
	defer notePurge()
	if err := ValidateIDValue(idType); err != nil {
		return 0, err
	}

	dir := filepath.Join(cacheDir, imageType, idType)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var removed int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if TitleFileMatch(base, idValue) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				if !os.IsNotExist(err) {
					return removed, err
				}
			} else {
				removed++
			}
		}
	}
	return removed, nil
}

// PurgeVariantFile removes a single rendered variant.
func PurgeVariantFile(cacheDir, imageType, idType, cacheValue, ext string) (int64, error) {
	defer notePurge()
	path, err := TypedCachePath(cacheDir, imageType, idType, cacheValue, ext)
	if err != nil {
		return 0, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return 1, nil
}

const purgePrefix = ".purging."

func StageDirForClear(cacheDir, subdir string) (*string, error) {
	defer notePurge()
	dir := filepath.Join(cacheDir, subdir)
	tmp := filepath.Join(cacheDir, fmt.Sprintf("%s%s.%d", purgePrefix, subdir, time.Now().UnixNano()))

	if err := os.Rename(dir, tmp); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &tmp, nil
}

func StageCacheForClear(cacheDir string) ([]string, error) {
	subdirs := []string{"posters", "logos", "backdrops", "episodes", "base", "preview"}
	var staged []string
	for _, sub := range subdirs {
		tmp, err := StageDirForClear(cacheDir, sub)
		if err != nil {
			return staged, err
		}
		if tmp != nil {
			staged = append(staged, *tmp)
		}
	}
	return staged, nil
}

func RemoveStagedDirs(dirs []string) {
	for _, dir := range dirs {
		os.RemoveAll(dir)
	}
}

func CleanupStagedDirs(cacheDir string) error {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), purgePrefix) {
			os.RemoveAll(filepath.Join(cacheDir, entry.Name()))
		}
	}
	return nil
}

func ComputeStaleSecs(releaseDateStr string, minStale, maxAge uint64) uint64 {
	now := uint64(time.Now().Unix())

	epoch := dateStrToEpoch(releaseDateStr)
	if epoch == 0 {
		return minStale
	}

	if epoch > now {
		return minStale
	}

	filmAge := now - epoch
	if filmAge >= maxAge {
		return 0
	}

	return minStale + filmAge*(maxAge-minStale)/maxAge
}

// dateStrToEpoch parses a "YYYY-MM-DD" release date and returns the Unix
// timestamp (seconds since 1970-01-01 UTC), or 0 if the string is missing,
// malformed, or before 1970. Replaces an earlier hand-rolled leap-year /
// days-since-epoch implementation — time.Parse handles all the edge cases.
func dateStrToEpoch(s string) uint64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0
	}
	u := t.Unix()
	if u < 0 {
		return 0
	}
	return uint64(u)
}

func ComputeCDNMaxAge(releaseDate *string, minStale, maxAge uint64) uint64 {
	rs := ""
	if releaseDate != nil {
		rs = *releaseDate
	}
	stale := ComputeStaleSecs(rs, minStale, maxAge)
	if stale == 0 {
		return 365 * 24 * 3600
	}
	return stale
}

// SweepTempFiles removes WriteCache temp files (".tmp-*") older than an hour
// anywhere under cacheDir — left behind only when the process died between
// create and rename. Meant to run once in the background at startup.
func SweepTempFiles(cacheDir string) int {
	cutoff := time.Now().Add(-time.Hour)
	removed := 0
	filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip it, keep walking
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), purgePrefix) {
				return filepath.SkipDir // being deleted already
			}
			return nil
		}
		if !strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	return removed
}

// DiskUsage is the size of the rendered-image disk cache.
type DiskUsage struct {
	Bytes int64
	Files int64
}

// CacheDiskUsage returns the total size and number of the files under
// cacheDir (the rendered-image disk cache). A missing directory counts as
// empty; unreadable entries are skipped. Stops early when ctx is done.
func CacheDiskUsage(ctx context.Context, cacheDir string) (DiskUsage, error) {
	var u DiskUsage
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
				return filepath.SkipDir // staged by a clear, being deleted
			}
			return nil
		}
		if d.Type().IsRegular() && !strings.HasPrefix(d.Name(), ".tmp-") {
			if info, err := d.Info(); err == nil {
				u.Bytes += info.Size()
				u.Files++
			}
		}
		return nil
	})
	if err != nil {
		return DiskUsage{}, err
	}
	return u, nil
}

// diskUsageReuse is how long CachedDiskUsage reuses a measurement. The
// dashboard refetches on every window focus; walking a large cache each
// time is wasted I/O. New renders show up within this window; purges
// (cachePurgeGen) show up immediately.
const diskUsageReuse = 30 * time.Second

var (
	cachePurgeGen atomic.Uint64
	diskUsageMu   sync.Mutex // held for the walk: concurrent callers share one
	diskUsageLast struct {
		dir   string
		gen   uint64
		at    time.Time
		usage DiskUsage
	}
)

// notePurge invalidates the reused disk usage after files were removed.
func notePurge() { cachePurgeGen.Add(1) }

// ResetDiskUsageCache forgets the reused measurement (tests).
func ResetDiskUsageCache() {
	diskUsageMu.Lock()
	diskUsageLast.at = time.Time{}
	diskUsageMu.Unlock()
}

// CachedDiskUsage is CacheDiskUsage, reusing a recent measurement of the
// same dir unless a purge happened since.
func CachedDiskUsage(ctx context.Context, cacheDir string) (DiskUsage, error) {
	diskUsageMu.Lock()
	defer diskUsageMu.Unlock()
	gen := cachePurgeGen.Load()
	l := &diskUsageLast
	if l.dir == cacheDir && l.gen == gen && !l.at.IsZero() && time.Since(l.at) < diskUsageReuse {
		return l.usage, nil
	}
	u, err := CacheDiskUsage(ctx, cacheDir)
	if err != nil {
		return DiskUsage{}, err
	}
	l.dir, l.gen, l.at, l.usage = cacheDir, gen, time.Now(), u
	return u, nil
}
