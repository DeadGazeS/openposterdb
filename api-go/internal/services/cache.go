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
	Bytes    []byte
	IsStale  bool
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

func dateStrToEpoch(s string) uint64 {
	if len(s) != 10 {
		return 0
	}
	var year, month, day uint64
	if n, _ := fmt.Sscanf(s, "%d-%d-%d", &year, &month, &day); n != 3 {
		return 0
	}
	if month < 1 || month > 12 || year < 1970 {
		return 0
	}
	maxDay := maxDaysInMonth(year, month)
	if day < 1 || day > maxDay {
		return 0
	}

	return daysFromEpoch(year, month, day) * 86400
}

func maxDaysInMonth(year, month uint64) uint64 {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeap(year) {
			return 29
		}
		return 28
	}
	return 0
}

func daysFromEpoch(year, month, day uint64) uint64 {
	leapsBefore := func(y uint64) uint64 {
		return y/4 - y/100 + y/400
	}
	prev := year - 1
	daysToYear := 365*(year-1970) + leapsBefore(prev) - leapsBefore(1969)

	daysInMonth := []uint64{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	var monthDays uint64
	for m := uint64(1); m < month; m++ {
		monthDays += daysInMonth[m]
	}
	if month > 2 && isLeap(year) {
		monthDays++
	}

	return daysToYear + monthDays + day - 1
}

func isLeap(y uint64) bool {
	return (y%4 == 0 && y%100 != 0) || y%400 == 0
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
