package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Over the cap, the least recently used files go first (rendered images by
// image_meta.last_accessed, base artwork by mtime) until the cache is at 90 %
// of the cap; removed renders lose their image_meta row.
func TestSweepCacheToSize_LRU(t *testing.T) {
	dir := t.TempDir()
	db := newImageMetaTestDB(t)
	db.SetMaxOpenConns(1) // :memory: is per connection
	now := time.Now()

	write := func(rel string, size int, mtime time.Time) string {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Rendered: the file mtimes are all recent; image_meta decides the order.
	oldRender := write("posters/imdb/tt1_x.jpg", 100, now)
	hotRender := write("posters/imdb/tt2_x.jpg", 100, now)
	neverServed := write("logos/tmdb/movie-3_x.png", 100, now)
	seedImageMetaRow(t, db, "imdb/tt1_x", "p", nil, now.Add(-72*time.Hour).Unix(), now.Add(-72*time.Hour).Unix(), now.Add(-48*time.Hour).Unix())
	seedImageMetaRow(t, db, "imdb/tt2_x", "p", nil, now.Add(-72*time.Hour).Unix(), now.Add(-72*time.Hour).Unix(), now.Unix())
	seedImageMetaRow(t, db, "tmdb/movie-3_x", "l", nil, now.Add(-time.Hour).Unix(), now.Add(-time.Hour).Unix(), 0)
	// Base artwork: mtime only.
	oldBase := write("base/posters/w780/a.jpg", 100, now.Add(-96*time.Hour))
	newBase := write("base/posters/w780/b.jpg", 100, now.Add(-time.Minute))
	write("posters/imdb/.tmp-123", 1000, now.Add(-200*time.Hour)) // never counted or removed

	// 500 bytes against a 400 cap → shrink to 360: the two oldest go.
	files, freed, err := SweepCacheToSize(t.Context(), db, dir, 400)
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 || freed != 200 {
		t.Fatalf("removed %d files / %d bytes, want 2 / 200", files, freed)
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	if exists(oldBase) || exists(oldRender) {
		t.Error("the two least recently used files survived")
	}
	for _, p := range []string{hotRender, neverServed, newBase} {
		if !exists(p) {
			t.Errorf("%s was removed", p)
		}
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM image_meta WHERE cache_key = 'imdb/tt1_x'").Scan(&n)
	if n != 0 {
		t.Error("image_meta row of the removed render was kept")
	}
	db.QueryRow("SELECT COUNT(*) FROM image_meta").Scan(&n)
	if n != 2 {
		t.Errorf("image_meta rows = %d, want 2", n)
	}

	// Under the cap: nothing to do.
	if files, _, _ := SweepCacheToSize(t.Context(), db, dir, 400); files != 0 {
		t.Errorf("second sweep removed %d files", files)
	}
}
