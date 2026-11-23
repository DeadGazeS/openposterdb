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

// Base artwork is ordered by its last read (base_access), not its mtime
// (which staleness and revalidation own): an old file read recently
// survives, a newer unread one goes. Its .etag sidecar follows the image,
// and rows of files no longer on disk are dropped.
func TestSweepCacheToSize_BaseByLastAccess(t *testing.T) {
	dir := t.TempDir()
	db := newImageMetaTestDB(t)
	db.SetMaxOpenConns(1)
	baseAccessTracking.Store(true)
	t.Cleanup(func() { baseAccessTracking.Store(false) })
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
	oldRead := write("base/posters/w780/old.jpg", 100, now.Add(-96*time.Hour))
	oldETag := write("base/posters/w780/old.jpg.etag", 10, now.Add(-96*time.Hour))
	newUnread := write("base/fanart/new.jpg", 100, now.Add(-time.Hour))
	NoteBaseAccess(oldRead)
	if _, err := db.Exec("INSERT INTO base_access (path, last_accessed) VALUES ('base/posters/w780/gone.jpg', 1)"); err != nil {
		t.Fatal(err)
	}

	// 210 bytes against a 150 cap → shrink to 135: one 100-byte file goes.
	files, _, err := SweepCacheToSize(t.Context(), db, dir, 150)
	if err != nil {
		t.Fatal(err)
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	if files != 1 || exists(newUnread) {
		t.Errorf("removed %d files; the newer unread base file should have gone", files)
	}
	if !exists(oldRead) || !exists(oldETag) {
		t.Error("the recently read old base file (or its etag) was removed")
	}
	if fi, _ := os.Stat(oldRead); fi != nil && fi.ModTime().After(now.Add(-95*time.Hour)) {
		t.Error("the read changed the base file's mtime (staleness depends on it)")
	}
	var paths []string
	rows, err := db.Query("SELECT path FROM base_access ORDER BY path")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		rows.Scan(&p)
		paths = append(paths, p)
	}
	if len(paths) != 1 || paths[0] != "base/posters/w780/old.jpg" {
		t.Errorf("base_access rows = %v, want only the surviving read file", paths)
	}
}

// Reads are only recorded while the cap is on, at most once per file
// between flushes.
func TestNoteBaseAccess_OnlyWhileTracking(t *testing.T) {
	baseAccessMu.Lock()
	baseAccessPending = map[string]int64{}
	baseAccessMu.Unlock()
	NoteBaseAccess("x")
	if len(baseAccessPending) != 0 {
		t.Error("recorded a read with tracking off")
	}
	baseAccessTracking.Store(true)
	t.Cleanup(func() { baseAccessTracking.Store(false) })
	NoteBaseAccess("x")
	NoteBaseAccess("x")
	if len(baseAccessPending) != 1 {
		t.Errorf("pending = %d, want one entry per file", len(baseAccessPending))
	}
}

// Shutdown writes the pending base reads (up to one size-check interval of
// them would otherwise be lost).
func TestFlushBaseAccess_WritesPending(t *testing.T) {
	dir := t.TempDir()
	db := newImageMetaTestDB(t)
	baseAccessMu.Lock()
	baseAccessPending = map[string]int64{}
	baseAccessMu.Unlock()
	baseAccessTracking.Store(true)
	t.Cleanup(func() { baseAccessTracking.Store(false) })
	NoteBaseAccess(filepath.Join(dir, "base", "posters", "w780", "a.jpg"))

	FlushBaseAccess(db, dir)

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM base_access WHERE path = 'base/posters/w780/a.jpg'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("base_access row: n=%d err=%v", n, err)
	}
}
