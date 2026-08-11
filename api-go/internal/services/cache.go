package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	age := time.Since(info.ModTime()).Seconds()
	isStale := staleSecs > 0 && uint64(age) > staleSecs

	return &CacheEntry{Bytes: bytes, IsStale: isStale}, nil
}

func WriteCache(path string, bytes []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
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
