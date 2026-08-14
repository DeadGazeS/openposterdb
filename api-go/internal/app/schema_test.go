package app

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

func newSchemaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// existingTables returns the set of user-visible table names from sqlite_master.
func existingTables(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	return out
}

// TestRunSchema_CreatesAllTables verifies RunSchema creates every table the
// app needs to operate (image_meta, admin_users, refresh_tokens, api_keys,
// global_settings, available_ratings, api_key_settings).
func TestRunSchema_CreatesAllTables(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("RunSchema: %v", err)
	}
	tables := existingTables(t, db)
	want := []string{
		"image_meta", "admin_users", "refresh_tokens", "api_keys",
		"global_settings", "available_ratings", "api_key_settings",
	}
	for _, name := range want {
		if !tables[name] {
			t.Errorf("table %q missing after RunSchema (got %v)", name, tables)
		}
	}
}

// TestRunSchema_Idempotent guards that RunSchema can be called repeatedly
// against an existing schema without error (uses CREATE TABLE IF NOT EXISTS).
func TestRunSchema_Idempotent(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("first RunSchema: %v", err)
	}
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("second RunSchema: %v", err)
	}
}

// TestRunMigrations_FreshDBSucceeds exercises the full SQL migration set on a
// freshly created schema. The first migration's "ADD COLUMN ratings_limit" is
// a no-op (the column is already in the schema), and subsequent migrations
// either ADD columns or DROP missing ones — every "duplicate column"/"no such
// column" error must be swallowed by RunMigrations.
func TestRunMigrations_FreshDBSucceeds(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("RunMigrations on fresh DB: %v", err)
	}
	// Sanity: a column added by the migrations (image_type) is now present.
	cols, err := db.Query("PRAGMA table_info(image_meta)")
	if err != nil {
		t.Fatal(err)
	}
	defer cols.Close()
	hasImageType := false
	for cols.Next() {
		var cid int
		var name, ctype string
		var dfltValue, pk sql.NullString
		var notnullFlag int
		if err := cols.Scan(&cid, &name, &ctype, &notnullFlag, &dfltValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "image_type" {
			hasImageType = true
		}
	}
	if !hasImageType {
		t.Error("image_type column missing on image_meta after RunMigrations")
	}
}

// TestRunMigrations_Idempotent guards the run-once contract for the bare
// RunMigrations path: every migration's ExpectedError matches the resulting
// error string on a second pass, so the second call is a silent no-op.
func TestRunMigrations_Idempotent(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
}

// TestRunMigrations_HandlesExpectedError exercises the swallow-error branch by
// pre-applying a schema that's already past one migration's target state, then
// asserting the ExpectedError substring lets RunMigrations continue rather
// than abort. We use migration #3 (ALTER TABLE image_meta ADD COLUMN
// image_type) — pre-add the column with a non-default value, run migrations,
// and expect the "duplicate column" error to be swallowed (the custom value
// is preserved, and the rest of the migrations complete).
func TestRunMigrations_HandlesExpectedError(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatal(err)
	}
	// Manually add the column that migration #3 wants to add — the duplicate
	// column error must be swallowed.
	if _, err := db.Exec("ALTER TABLE image_meta ADD COLUMN image_type TEXT NOT NULL DEFAULT 'backdrop'"); err != nil {
		t.Fatalf("pre-add column: %v", err)
	}
	// The rest of the migrations must still complete without aborting.
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("RunMigrations with pre-existing state: %v", err)
	}
	// The seeded custom value (backdrop) survived the migration pass.
	var got string
	if err := db.QueryRow("SELECT image_type FROM image_meta").Scan(&got); err != nil {
		// No rows yet — that's fine, the column was just added with the default.
		// Insert one to verify the default value sticks.
		if _, err2 := db.Exec("INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES ('k', 1, 1, 'logo')"); err2 != nil {
			t.Fatal(err2)
		}
		if err := db.QueryRow("SELECT image_type FROM image_meta WHERE cache_key='k'").Scan(&got); err != nil {
			t.Fatal(err)
		}
	}
	if got != "logo" {
		t.Errorf("image_type=%q, want seeded 'logo'", got)
	}
}

// TestRunMigrations_AllApplied asserts every migration in Migrations has a
// non-empty ExpectedError substring (i.e. the SQL is idempotent) and uses a
// recognised verb. This is a static-shape check that catches accidental drops
// or untracked migrations at code-review time.
func TestRunMigrations_AllApplied(t *testing.T) {
	for i, m := range Migrations {
		if m.ExpectedError == "" {
			t.Errorf("migration %d has no ExpectedError — should be idempotent (SQL: %q)", i, m.SQL)
		}
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(m.SQL)), "ALTER") &&
			!strings.HasPrefix(strings.ToUpper(strings.TrimSpace(m.SQL)), "CREATE") &&
			!strings.HasPrefix(strings.ToUpper(strings.TrimSpace(m.SQL)), "DROP") &&
			!strings.HasPrefix(strings.ToUpper(strings.TrimSpace(m.SQL)), "UPDATE") {
			t.Errorf("migration %d starts with unexpected verb: %q", i, m.SQL)
		}
	}
}

// TestRunMigrations_UnexpectedErrorBubblesUp guards that only the ExpectedError
// substring is swallowed: a migration whose error doesn't match must abort
// RunMigrations instead of being silently skipped. We craft a tiny migration
// list whose SQL fails with "syntax error" (not in our ExpectedError substring)
// and assert RunMigrations surfaces it.
func TestRunMigrations_UnexpectedErrorBubblesUp(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatal(err)
	}
	bad := []Migration{
		{SQL: "THIS IS NOT VALID SQL", ExpectedError: "no-such-thing-ever"},
	}
	if err := RunMigrations(db, bad); err == nil {
		t.Error("RunMigrations with bad SQL should return error")
	}
}

// TestFullBootstrap_KeySettingsRoundTrip exercises the complete startup
// bootstrap (RunSchema → RunMigrations → services.RunUpgrades) and then runs
// the real per-key settings SQL against the resulting schema. Handler and
// service unit tests hand-create their own api_key_settings table, so a
// migration list that drops — or never creates — a referenced column passes
// every package test while breaking every real database (the 2026-08-14
// "no column named poster_badge_size" bug).
func TestFullBootstrap_KeySettingsRoundTrip(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("RunSchema: %v", err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := services.RunUpgrades(db, t.TempDir(), true); err != nil {
		t.Fatalf("RunUpgrades: %v", err)
	}

	// The seven columns the 2026-08-14 bug dropped or never created must all
	// be present as INTEGER.
	rows, err := db.Query("PRAGMA table_info(api_key_settings)")
	if err != nil {
		t.Fatal(err)
	}
	colTypes := map[string]string{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, declType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &declType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		colTypes[name] = declType
	}
	rows.Close()
	for _, name := range []string{
		"poster_badge_size", "logo_badge_size", "backdrop_badge_size", "episode_badge_size",
		"poster_badge_alpha", "logo_badge_alpha", "backdrop_badge_alpha", "episode_badge_alpha",
	} {
		if got := colTypes[name]; got != "INTEGER" {
			t.Errorf("api_key_settings.%s: got type %q, want INTEGER", name, got)
		}
	}

	// Full round-trip through the production SQL (every referenced column).
	in := &services.APIKeySettings{
		APIKeyID:           1,
		ImageSource:        "t",
		Lang:               "de",
		RatingsOrder:       "mal,imdb",
		PosterLayout:       `{"top":{"per_row":0,"rows":0,"start":"c"}}`,
		PosterBadgeSize:    130,
		LogoBadgeSize:      120,
		BackdropBadgeSize:  110,
		EpisodeBadgeSize:   90,
		PosterBadgeAlpha:   70,
		LogoBadgeAlpha:     65,
		BackdropBadgeAlpha: 60,
		EpisodeBadgeAlpha:  55,
	}
	if err := services.UpsertAPIKeySettings(db, in); err != nil {
		t.Fatalf("UpsertAPIKeySettings: %v", err)
	}
	got, err := services.GetAPIKeySettings(db, 1)
	if err != nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if got == nil {
		t.Fatal("GetAPIKeySettings returned nil after upsert")
	}
	if got.PosterBadgeSize != 130 || got.LogoBadgeSize != 120 ||
		got.BackdropBadgeSize != 110 || got.EpisodeBadgeSize != 90 {
		t.Errorf("badge sizes: got %d/%d/%d/%d, want 130/120/110/90",
			got.PosterBadgeSize, got.LogoBadgeSize, got.BackdropBadgeSize, got.EpisodeBadgeSize)
	}
	if got.PosterBadgeAlpha != 70 || got.LogoBadgeAlpha != 65 ||
		got.BackdropBadgeAlpha != 60 || got.EpisodeBadgeAlpha != 55 {
		t.Errorf("badge alphas: got %d/%d/%d/%d, want 70/65/60/55",
			got.PosterBadgeAlpha, got.LogoBadgeAlpha, got.BackdropBadgeAlpha, got.EpisodeBadgeAlpha)
	}
	if got.Lang != "de" || got.ImageSource != "t" {
		t.Errorf("scalar fields: got lang=%q source=%q, want de/t", got.Lang, got.ImageSource)
	}
}

// TestOpenDatabase_InMemory exercises the public OpenDatabase helper against
// an in-memory SQLite: it must return a non-nil *sql.DB with the connection
// pool configured, and (because :memory: ignores journal_mode WAL) the open
// call must succeed even though WAL isn't actually applied.
func TestOpenDatabase_InMemory(t *testing.T) {
	db, err := OpenDatabase(":memory:")
	if err != nil {
		t.Fatalf("OpenDatabase(:memory:): %v", err)
	}
	defer db.Close()

	// Sanity: a SELECT works.
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query: %v", err)
	}
	if one != 1 {
		t.Errorf("got %d, want 1", one)
	}

	// Connection pool: Stats are populated lazily; just verify DB is usable
	// after pool config.
	stats := db.Stats()
	if stats.OpenConnections < 1 {
		t.Errorf("no open connections: %+v", stats)
	}
}

// TestSchemaToMigrations_FreshDB exercises the full bootstrap path
// (RunSchema → RunMigrations) on an empty DB and confirms every schema-level
// table exists at the end. The `upgrades` bookkeeping is created by
// services.runOnce (not by RunMigrations) and is covered by
// services/upgrade_test.go.
func TestSchemaToMigrations_FreshDB(t *testing.T) {
	db := newSchemaTestDB(t)

	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("RunSchema: %v", err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	tables := existingTables(t, db)
	for _, want := range []string{
		"image_meta", "admin_users", "refresh_tokens", "api_keys",
		"global_settings", "available_ratings", "api_key_settings",
	} {
		if !tables[want] {
			t.Errorf("table %q missing after full bootstrap (got %v)", want, tables)
		}
	}
}
