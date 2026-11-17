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
// upgrade.go's image_meta queries and v004's api_key_settings column work.
// We can't import internal/app here because app/seed.go imports services
// (cycle), so we duplicate the parts we need.
const upgradeTestSchema = `
CREATE TABLE IF NOT EXISTS image_meta (
	cache_key TEXT PRIMARY KEY,
	release_date TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	image_type TEXT NOT NULL DEFAULT 'poster'
);
CREATE TABLE IF NOT EXISTS api_key_settings (
	api_key_id INTEGER PRIMARY KEY
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
// v*** upgrade (v001 … v005).
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
		"v004_badge_size_int_columns",
		"v005_rust_render_cache",
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
	if count != 5 {
		t.Errorf("upgrades rows after rerun: got %d, want 5", count)
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

// TestUpgradeV002_DB_PtrAndDvSuffixes drives v002 with real Rust-era backdrop
// keys (`{ratings}.s{style}.l{label}.b{size}.z{size}`): `.ptr` goes before the
// badge style and `.dv` after the label style, exactly as the Rust upgrade
// (v002_backdrop_position_direction_cache.rs) did. Other image types and
// already-migrated rows are untouched.
func TestUpgradeV002_DB_PtrAndDvSuffixes(t *testing.T) {
	db := upgradeTestDB(t)
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES
		('imdb/tt1234567_b_f@mil.sv.lt.bm.zm', 1, 1, 'b'),
		('imdb/tt7654321_b_t@ir.sh.li.bl.zl', 1, 1, 'b'),
		('imdb/tt1111111_b_f@m.ptr.sd.lo.dv.bxs.zs', 1, 1, 'b'),
		('imdb/tt2222222@mil.sv.lt.bm.zm', 1, 1, 'p')`); err != nil {
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
	for _, want := range []string{
		"imdb/tt1234567_b_f@mil.ptr.sv.lt.dv.bm.zm",
		"imdb/tt7654321_b_t@ir.ptr.sh.li.dv.bl.zl",
		"imdb/tt1111111_b_f@m.ptr.sd.lo.dv.bxs.zs",
	} {
		if _, ok := rows[want]; !ok {
			t.Errorf("missing renamed row %q (rows=%v)", want, rows)
		}
	}
	for k := range rows {
		if strings.Contains(k, "..") {
			t.Errorf("double dot in migrated key %q", k)
		}
	}
	// The poster row must remain untouched.
	if _, ok := rows["imdb/tt2222222@mil.sv.lt.bm.zm"]; !ok {
		t.Errorf("poster row was renamed (rows=%v)", rows)
	}
}

// TestUpgradeV002_DBMatchesFilesystem: the DB step and the file rename must
// produce the same name for the same key.
func TestUpgradeV002_DBMatchesFilesystem(t *testing.T) {
	db := upgradeTestDB(t)
	name := "tt1234567_b_f@mil.sv.lt.bxl.zvl"
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES (?, 1, 1, 'b')`, name); err != nil {
		t.Fatal(err)
	}
	if err := upgradeV002Ctx(context.Background(), db, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow("SELECT cache_key FROM image_meta").Scan(&got); err != nil {
		t.Fatal(err)
	}
	fsName := migrateNameV002(name + ".jpg")
	if fsName == nil || *fsName != got+".jpg" {
		t.Errorf("DB %q vs file %v", got, fsName)
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

// TestRunUpgrades_ConcurrentCallersRunFnOnce guards the run-once contract
// under concurrency: 5 goroutines all calling RunUpgrades must observe
// each v*** migration run at most once (and end up with exactly 5 rows in
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
	if count != 5 {
		t.Errorf("upgrades rows after concurrent runs: got %d, want 5", count)
	}
}

// v004TestDB opens an in-memory SQLite with the given api_key_settings DDL —
// each v004 test drives a different historical database state.
func v004TestDB(t *testing.T, ddl string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// TestUpgradeV004_CreatesMissingColumns reproduces the 2026-08-14 production
// state: databases bootstrapped while the DROP entries followed the INTEGER
// ADDs in the flat migration list have no *_badge_size columns at all, which
// broke every per-key settings read/write ("table api_key_settings has no
// column named poster_badge_size"). v004 must create them as INTEGER NOT NULL
// DEFAULT 100.
func TestUpgradeV004_CreatesMissingColumns(t *testing.T) {
	db := v004TestDB(t, `CREATE TABLE api_key_settings (api_key_id INTEGER PRIMARY KEY)`)
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	if err := upgradeV004Ctx(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols, err := tableColumnTypes(context.Background(), db, "api_key_settings")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"poster", "logo", "backdrop", "episode"} {
		if got := cols[kind+"_badge_size"]; got != "INTEGER" {
			t.Errorf("%s_badge_size: got type %q, want INTEGER", kind, got)
		}
	}
	// The pre-existing row picks up the column default.
	var v int
	if err := db.QueryRow(`SELECT poster_badge_size FROM api_key_settings WHERE api_key_id = 1`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 100 {
		t.Errorf("poster_badge_size default: got %d, want 100", v)
	}
}

// TestUpgradeV004_ConvertsLegacyTextColumns covers databases from before the
// percent-scale rework: TEXT enum columns ('s'/'m'/'l') are dropped and
// recreated as INTEGER, and the Rust size names are carried over as percents
// of the kind's default scale (PercentFromLegacySize).
func TestUpgradeV004_ConvertsLegacyTextColumns(t *testing.T) {
	db := v004TestDB(t, `CREATE TABLE api_key_settings (
		api_key_id INTEGER PRIMARY KEY,
		poster_badge_size TEXT NOT NULL DEFAULT 'm',
		logo_badge_size TEXT NOT NULL DEFAULT 'm',
		backdrop_badge_size TEXT NOT NULL DEFAULT 'm',
		episode_badge_size TEXT NOT NULL DEFAULT 'l'
	)`)
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id, poster_badge_size, episode_badge_size) VALUES (1, 'xl', 's')`); err != nil {
		t.Fatal(err)
	}

	if err := upgradeV004Ctx(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	cols, err := tableColumnTypes(context.Background(), db, "api_key_settings")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"poster", "logo", "backdrop", "episode"} {
		if got := cols[kind+"_badge_size"]; got != "INTEGER" {
			t.Errorf("%s_badge_size: got type %q, want INTEGER", kind, got)
		}
	}
	// The values scan into a Go int (TEXT 'm' would fail the services
	// SELECT) and keep the size the user picked in the Rust UI:
	// poster xl = 1.7/1.2 → 142 %, episode s = 0.95/1.45 → 66 %, the
	// untouched defaults (poster m, episode l) → 100 %.
	var poster, logo, episode int
	if err := db.QueryRow(`SELECT poster_badge_size, logo_badge_size, episode_badge_size FROM api_key_settings WHERE api_key_id = 1`).Scan(&poster, &logo, &episode); err != nil {
		t.Fatal(err)
	}
	if poster != 142 || logo != 100 || episode != 66 {
		t.Errorf("converted values: got poster=%d logo=%d episode=%d, want 142/100/66", poster, logo, episode)
	}
}

// TestUpgradeV004_PreservesIntegerValues guards the steady state after the
// one-shot upgrade has run: already-INTEGER columns are a no-op, so stored
// percent values survive every subsequent boot.
func TestUpgradeV004_PreservesIntegerValues(t *testing.T) {
	db := v004TestDB(t, `CREATE TABLE api_key_settings (
		api_key_id INTEGER PRIMARY KEY,
		poster_badge_size INTEGER NOT NULL DEFAULT 100,
		logo_badge_size INTEGER NOT NULL DEFAULT 100,
		backdrop_badge_size INTEGER NOT NULL DEFAULT 100,
		episode_badge_size INTEGER NOT NULL DEFAULT 100
	)`)
	if _, err := db.Exec(`INSERT INTO api_key_settings (api_key_id, poster_badge_size, logo_badge_size, backdrop_badge_size, episode_badge_size) VALUES (1, 130, 120, 110, 90)`); err != nil {
		t.Fatal(err)
	}

	if err := upgradeV004Ctx(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	var poster, logo, backdrop, episode int
	if err := db.QueryRow(`SELECT poster_badge_size, logo_badge_size, backdrop_badge_size, episode_badge_size FROM api_key_settings WHERE api_key_id = 1`).Scan(&poster, &logo, &backdrop, &episode); err != nil {
		t.Fatal(err)
	}
	if poster != 130 || logo != 120 || backdrop != 110 || episode != 90 {
		t.Errorf("values changed: got %d/%d/%d/%d, want 130/120/110/90", poster, logo, backdrop, episode)
	}
}
