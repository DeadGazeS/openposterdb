package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The dashboard's disk-cache tile: total size of the files under the cache
// dir and how many files, nested kinds included; a missing dir is simply empty.
func TestCacheDiskUsage(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("p/imdb/tt1.jpg", 1000)
	write("l/tmdb/2.png", 500)
	write("b/x/y/z.jpg", 24)

	got, err := CacheDiskUsage(context.Background(), dir)
	if err != nil || got.Bytes != 1524 || got.Files != 3 {
		t.Fatalf("got %+v, %v; want 1524 bytes in 3 files", got, err)
	}
	if got, err := CacheDiskUsage(context.Background(), filepath.Join(dir, "missing")); err != nil || got.Bytes != 0 || got.Files != 0 {
		t.Fatalf("missing dir: got %+v, %v", got, err)
	}
}

// Folders staged for deletion by a clear and leftover temp files are not
// counted; the result is reused for a short while (the dashboard refetches
// on every window focus) but a purge shows up immediately.
func TestCacheDiskUsage_SkipsStagedAndCachesUntilPurge(t *testing.T) {
	ResetDiskUsageCache()
	dir := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("posters/imdb/tt1.jpg", 100)
	write(".purging.logos.123/tmdb/2.png", 5000)
	write("posters/imdb/.tmp-abc", 7000)

	got, err := CachedDiskUsage(context.Background(), dir)
	if err != nil || got.Bytes != 100 || got.Files != 1 {
		t.Fatalf("got %+v, %v; want 100 bytes in 1 file", got, err)
	}

	// A new file within the reuse window is not seen yet…
	write("posters/imdb/tt2.jpg", 50)
	if got, _ := CachedDiskUsage(context.Background(), dir); got.Bytes != 100 {
		t.Fatalf("within the window: %+v, want the reused 100", got)
	}
	// …but a purge invalidates the reused value at once.
	if _, err := StageDirForClear(dir, "posters"); err != nil {
		t.Fatal(err)
	}
	if got, _ := CachedDiskUsage(context.Background(), dir); got.Bytes != 0 || got.Files != 0 {
		t.Fatalf("after a clear: %+v, want empty", got)
	}
}
