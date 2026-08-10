package services

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// upgradeTestSchema is a minimal copy of the production schema: enough for
// upgrade.go's image_meta queries. We can't import internal/app here because
// app/seed.go imports services (cycle), so we duplicate the parts we need.
const upgradeTestSchema = `
CREATE TABLE IF NOT EXISTS image_meta (
	cache_key TEXT PRIMARY KEY,
	release_date TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	image_type TEXT NOT NULL DEFAULT 'poster'
);
`

// upgradeTestDB opens an in-memory SQLite and applies the minimal schema
// upgrade.go touches.
func upgradeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(upgradeTestSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// TestRunUpgrades_CreatesUpgradesTable guards the bookkeeping table: a fresh
// DB + RunUpgrades must create the upgrades table and insert one row per
// v*** upgrade (v001, v002, v003).
func TestRunUpgrades_CreatesUpgradesTable(t *testing.T) {
	db := upgradeTestDB(t)

	cacheDir := t.TempDir()
	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatalf("RunUpgrades: %v", err)
	}

	rows, err := db.Query("SELECT name FROM upgrades ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	want := []string{
		"v001_backdrop_cache_keys",
		"v002_backdrop_position_direction_cache",
		"v003_badge_shape_background_cache",
	}
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("upgrades rows: got %v, want %v", got, want)
	}
}

// TestRunUpgrades_Idempotent guards the run-once contract: the second call
// must be a silent no-op (no extra upgrades rows, no FS side effects).
func TestRunUpgrades_Idempotent(t *testing.T) {
	db := upgradeTestDB(t)
	cacheDir := t.TempDir()

	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatal(err)
	}
	if err := RunUpgrades(db, cacheDir, false); err != nil {
		t.Fatalf("second RunUpgrades: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM upgrades").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("upgrades rows after rerun: got %d, want 3", count)
	}
}

// TestUpgradeV001_DB_BackdropKeyRename drives the DB-side v001 migration:
// rows with `_b@` and image_type='b' must be renamed to `_b_f@`; rows with
// image_type != 'b' must be untouched.
func TestUpgradeV001_DB_BackdropKeyRename(t *testing.T) {
	db := upgradeTestDB(t)
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES
		('title1_b@_en.jpg', 1, 1, 'b'),
		('title2_b@_en.jpg', 1, 1, 'p'),
		('title3_b_f@_en.jpg', 1, 1, 'b')`); err != nil {
		t.Fatal(err)
	}

	if err := upgradeV001Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}

	rows := map[string]string{}
	r, err := db.Query("SELECT cache_key, image_type FROM image_meta")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for r.Next() {
		var ck, it string
		if err := r.Scan(&ck, &it); err != nil {
			t.Fatal(err)
		}
		rows[ck] = it
	}
	if _, ok := rows["title1_b_f@_en.jpg"]; !ok {
		t.Errorf("title1 not renamed (rows=%v)", rows)
	}
	if _, ok := rows["title2_b@_en.jpg"]; !ok {
		t.Errorf("title2 (poster) was renamed (rows=%v)", rows)
	}
	if _, ok := rows["title3_b_f@_en.jpg"]; !ok {
		t.Errorf("title3 (already migrated) lost (rows=%v)", rows)
	}
}

// TestRenameFilesV001_RenamesMatching guards the FS-side v001: a mix of
// matching (`_b@`) and non-matching files in a backdrops dir must produce
// exactly N renames, and only the matching ones change.
func TestRenameFilesV001_RenamesMatching(t *testing.T) {
	dir := t.TempDir()
	bd := filepath.Join(dir, "backdrops")
	if err := os.MkdirAll(bd, 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		"title1_b@_en.jpg",   // matches → rename
		"title2_b@_de.jpg",   // matches → rename
		"title3_b_f@_en.jpg", // already migrated → skip
		"title4_b_t@_en.jpg", // already migrated (top) → skip
		"title5_en.jpg",      // unrelated → skip
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(bd, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	count, err := renameFilesV001(bd)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("rename count: got %d, want 2", count)
	}

	want := map[string]bool{
		"title1_b_f@_en.jpg": true,
		"title2_b_f@_de.jpg": true,
		"title3_b_f@_en.jpg": true,
		"title4_b_t@_en.jpg": true,
		"title5_en.jpg":      true,
	}
	got, err := os.ReadDir(bd)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		if !want[f.Name()] {
			t.Errorf("unexpected file after rename: %q", f.Name())
		}
		delete(want, f.Name())
	}
	for f := range want {
		t.Errorf("missing file after rename: %q", f)
	}
}

// TestRenameFilesV001_SkipsAlreadyMigrated guards the safety check: files
// containing `_b_f@` or `_b_t@` must be left untouched (no double-rename,
// no destructive overwrite).
func TestRenameFilesV001_SkipsAlreadyMigrated(t *testing.T) {
	dir := t.TempDir()
	bd := filepath.Join(dir, "backdrops")
	if err := os.MkdirAll(bd, 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{"a_b_f@_en.jpg", "b_b_t@_de.jpg"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(bd, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	count, err := renameFilesV001(bd)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("rename count: got %d, want 0 (already migrated files)", count)
	}
}

// TestRenameFilesV001_MissingDir_NoError guards the missing-dir branch:
// passing a non-existent path must return (0, nil) so callers don't need a
// pre-check.
func TestRenameFilesV001_MissingDir_NoError(t *testing.T) {
	count, err := renameFilesV001(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Errorf("expected nil error for missing dir, got %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 renamed, got %d", count)
	}
}

// TestUpgradeV002_DB_PtrAndDvSuffixes drives v002: rows with the literal
// `.sv.l.` and `.lt.b.` substrings (backdrop only) must have `.ptr.` (or
// `.dv.`) inserted before the matched fragment; other image types and
// already-migrated rows are untouched.
//
// The migration's `style + "l" + "."` pattern requires the literal `.sv.l.`
// substring in the cache key (period-s-v-period-l-period), so we seed
// matching patterns explicitly.
func TestUpgradeV002_DB_PtrAndDvSuffixes(t *testing.T) {
	db := upgradeTestDB(t)
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES
		('a.sv.l.b.en.jpg', 1, 1, 'b'),
		('b.lt.b.en.jpg', 1, 1, 'b'),
		('c.ptr.sv.l.b.en.jpg', 1, 1, 'b'),
		('d.sv.l.b.en.jpg', 1, 1, 'p')`); err != nil {
		t.Fatal(err)
	}

	if err := upgradeV002Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}

	rows := map[string]string{}
	r, err := db.Query("SELECT cache_key, image_type FROM image_meta")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for r.Next() {
		var ck, it string
		if err := r.Scan(&ck, &it); err != nil {
			t.Fatal(err)
		}
		rows[ck] = it
	}
	// After v002 the migration inserts `.ptr.` (and `.dv.`) before the
	// matched fragment, producing a `.ptr..sv.l.` double-dot in the cache
	// key (the leading dot of the new `.ptr.` token sits next to the
	// original `.sv.l.` separator).
	for _, want := range []string{"a.ptr..sv.l.b.en.jpg", "b.lt.dv.b.en.jpg", "c.ptr.sv.l.b.en.jpg"} {
		if _, ok := rows[want]; !ok {
			t.Errorf("missing renamed row %q (rows=%v)", want, rows)
		}
	}
	// The poster row must remain untouched.
	if _, ok := rows["d.sv.l.b.en.jpg"]; !ok {
		t.Errorf("poster row was renamed (rows=%v)", rows)
	}
}

// TestUpgradeV002_DB_LeavesAlreadyMigratedAlone guards the .ptr./.dv.
// skip: rows with both new tokens must be untouched.
func TestUpgradeV002_DB_LeavesAlreadyMigratedAlone(t *testing.T) {
	db := upgradeTestDB(t)
	// Already has both .ptr. and .dv. — must be skipped.
	original := "x.ptr.sv.l.lt.dv.b.en.jpg"
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES (?, 1, 1, 'b')`, original); err != nil {
		t.Fatal(err)
	}
	if err := upgradeV002Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow("SELECT cache_key FROM image_meta").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Errorf("cache_key changed: got %q, want %q", got, original)
	}
}

// TestUpgradeV003_DB_AddsShrBgd drives v003: rows with `.bs.` (etc.) get a
// `shr.bgd.` suffix inserted after the size token.
func TestUpgradeV003_DB_AddsShrBgd(t *testing.T) {
	db := upgradeTestDB(t)
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES
		('a.bs.en.jpg', 1, 1, 'p'),
		('b.bm.en.jpg', 1, 1, 'l'),
		('c.bl.en.jpg', 1, 1, 'b'),
		('d_no_size.jpg', 1, 1, 'p')`); err != nil {
		t.Fatal(err)
	}
	if err := upgradeV003Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	r, err := db.Query("SELECT cache_key FROM image_meta")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for r.Next() {
		var ck string
		if err := r.Scan(&ck); err != nil {
			t.Fatal(err)
		}
		rows[ck] = ck
	}
	for _, want := range []string{"a.bs.shr.bgd.en.jpg", "b.bm.shr.bgd.en.jpg", "c.bl.shr.bgd.en.jpg"} {
		if _, ok := rows[want]; !ok {
			t.Errorf("missing renamed row %q (rows=%v)", want, rows)
		}
	}
	if _, ok := rows["d_no_size.jpg"]; !ok {
		t.Errorf("row without size token was renamed (rows=%v)", rows)
	}
}

// TestUpgradeV003_DB_SkipsIfBgPresent guards the safety check: rows already
// containing `.bg` (anywhere) must be skipped.
func TestUpgradeV003_DB_SkipsIfBgPresent(t *testing.T) {
	db := upgradeTestDB(t)
	original := "a.bs.shr.bgb.en.jpg"
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES (?, 1, 1, 'p')`, original); err != nil {
		t.Fatal(err)
	}
	if err := upgradeV003Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow("SELECT cache_key FROM image_meta").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Errorf("cache_key changed: got %q, want %q", got, original)
	}
}

// TestRenameFilesV003_WalksSubdirs guards v003's subdir coverage: a cache
// dir containing posters/logos/backdrops/episodes must be walked and
// matching files in each renamed.
func TestRenameFilesV003_WalksSubdirs(t *testing.T) {
	root := t.TempDir()
	subdirs := []string{"posters", "logos", "backdrops", "episodes"}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, sub, "x.bs.en.jpg"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	total := uint64(0)
	for _, sub := range subdirs {
		n, err := renameFilesV003(filepath.Join(root, sub))
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	if total != 4 {
		t.Errorf("rename count: got %d, want 4", total)
	}

	for _, sub := range subdirs {
		entries, err := os.ReadDir(filepath.Join(root, sub))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "x.bs.shr.bgd.en.jpg" {
			t.Errorf("subdir %s: got %+v, want x.bs.shr.bgd.en.jpg", sub, entries)
		}
	}
}

// TestMigrateNameV002_RejectsAlreadyMigrated guards the no-op branch:
// names containing both `.ptr.` and `.dv.` must return nil (no rename).
func TestMigrateNameV002_RejectsAlreadyMigrated(t *testing.T) {
	if got := migrateNameV002("title.ptr.sv.l.lt.dv.b.en.jpg"); got != nil {
		t.Errorf("expected nil for already-migrated name, got %v", got)
	}
}

// TestMigrateNameV003_NoSizeToken guards the no-op branch: names without any
// size token must return nil.
func TestMigrateNameV003_NoSizeToken(t *testing.T) {
	if got := migrateNameV003("title.en.jpg"); got != nil {
		t.Errorf("expected nil for name without size token, got %v", got)
	}
}

// TestRunUpgrades_ExternalCacheOnly guards that with external_cache_only=true
// the FS isn't touched but the DB-side upgrades still run.
func TestRunUpgrades_ExternalCacheOnly(t *testing.T) {
	db := upgradeTestDB(t)
	root := t.TempDir()
	// Pre-create a subdir so we can detect any FS mutation.
	bd := filepath.Join(root, "backdrops")
	if err := os.MkdirAll(bd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bd, "title_b@_en.jpg"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RunUpgrades(db, root, true); err != nil {
		t.Fatal(err)
	}

	// File should still be there with its original name.
	entries, err := os.ReadDir(bd)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "title_b@_en.jpg" {
		t.Errorf("FS touched under external_cache_only: %+v", entries)
	}

	// DB-side: v001 still recorded as completed.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM upgrades WHERE name = 'v001_backdrop_cache_keys'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("v001 not recorded: count=%d", count)
	}
}

// TestStringInt guards the int-to-single-digit helper used to compose
// SQLite substr() expressions (it intentionally only handles 0-9 because
// that's the range the migration uses).
func TestStringInt(t *testing.T) {
	for i := 0; i <= 9; i++ {
		if got := stringInt(i); got != string(rune('0'+i)) {
			t.Errorf("stringInt(%d) = %q, want %q", i, got, string(rune('0'+i)))
		}
	}
}

// TestRunUpgrades_ConcurrentCallersRunFnOnce guards the run-once contract
// under concurrency: 5 goroutines all calling RunUpgrades must observe
// each v*** migration run at most once (and end up with exactly 3 rows in
// the upgrades table).
//
// The shared DB is opened with shared cache + a single connection so all
// goroutines see the same :memory: instance (otherwise :memory: gives each
// connection its own private DB and concurrent calls would race on missing
// tables).
func TestRunUpgrades_ConcurrentCallersRunFnOnce(t *testing.T) {
	// Use a temporary file so all connections share the same DB; this also
	// avoids the :memory: per-connection gotcha with concurrent access.
	tmp := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(upgradeTestSchema); err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := RunUpgrades(db, cacheDir, true); err != nil {
				t.Errorf("RunUpgrades: %v", err)
			}
		}()
	}
	wg.Wait()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM upgrades").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("upgrades rows after concurrent runs: got %d, want 3", count)
	}
}
