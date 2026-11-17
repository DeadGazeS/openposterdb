package app

import (
	"database/sql"
	"testing"

	"openposterdb/internal/services"
)

// rustEraSchema is the api_key_settings / image_meta shape of the last Rust
// version (entity/api_key_settings.rs at e5f64f5^): named badge sizes,
// positions, split and background enums, no layout or alpha columns.
const rustEraSchema = `
CREATE TABLE image_meta (
	cache_key TEXT PRIMARY KEY,
	release_date TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	image_type TEXT NOT NULL DEFAULT 'poster'
);
CREATE TABLE admin_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	key_prefix TEXT NOT NULL,
	created_by INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT
);
CREATE TABLE api_key_settings (
	api_key_id INTEGER PRIMARY KEY REFERENCES api_keys(id) ON DELETE CASCADE,
	image_source TEXT NOT NULL DEFAULT 't',
	lang TEXT NOT NULL DEFAULT 'en',
	textless INTEGER NOT NULL DEFAULT 0,
	ratings_limit INTEGER NOT NULL DEFAULT 3,
	ratings_order TEXT NOT NULL DEFAULT 'mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert',
	poster_position TEXT NOT NULL DEFAULT 'bc',
	logo_ratings_limit INTEGER NOT NULL DEFAULT 5,
	backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
	poster_badge_style TEXT NOT NULL DEFAULT 'h',
	logo_badge_style TEXT NOT NULL DEFAULT 'v',
	backdrop_badge_style TEXT NOT NULL DEFAULT 'v',
	poster_label_style TEXT NOT NULL DEFAULT 'i',
	logo_label_style TEXT NOT NULL DEFAULT 'i',
	backdrop_label_style TEXT NOT NULL DEFAULT 'i',
	poster_badge_direction TEXT NOT NULL DEFAULT 'd',
	poster_badge_split INTEGER NOT NULL DEFAULT 0,
	poster_fit TEXT NOT NULL DEFAULT 'native',
	poster_badge_size TEXT NOT NULL DEFAULT 'm',
	logo_badge_size TEXT NOT NULL DEFAULT 'm',
	backdrop_badge_size TEXT NOT NULL DEFAULT 'm',
	backdrop_position TEXT NOT NULL DEFAULT 'tr',
	backdrop_badge_direction TEXT NOT NULL DEFAULT 'd',
	episode_ratings_limit INTEGER NOT NULL DEFAULT 1,
	episode_badge_style TEXT NOT NULL DEFAULT 'v',
	episode_label_style TEXT NOT NULL DEFAULT 'o',
	episode_badge_size TEXT NOT NULL DEFAULT 'l',
	episode_position TEXT NOT NULL DEFAULT 'tr',
	episode_badge_direction TEXT NOT NULL DEFAULT 'v',
	episode_blur INTEGER NOT NULL DEFAULT 0,
	ratings_exclude TEXT NOT NULL DEFAULT '',
	poster_badge_shape TEXT NOT NULL DEFAULT 'r',
	logo_badge_shape TEXT NOT NULL DEFAULT 'r',
	backdrop_badge_shape TEXT NOT NULL DEFAULT 'r',
	episode_badge_shape TEXT NOT NULL DEFAULT 'r',
	poster_badge_background TEXT NOT NULL DEFAULT 'd',
	logo_badge_background TEXT NOT NULL DEFAULT 'd',
	backdrop_badge_background TEXT NOT NULL DEFAULT 'd',
	episode_badge_background TEXT NOT NULL DEFAULT 'd',
	backdrop_edge_inset_x INTEGER NOT NULL DEFAULT 0,
	backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0
);
`

// TestBootstrap_RustEraDatabase boots the Go schema/migration/upgrade chain
// on a Rust-era database and checks that the per-key badge options survive
// as layout / alpha / percent, the legacy columns are gone, and Rust-only
// cache rows are removed.
func TestBootstrap_RustEraDatabase(t *testing.T) {
	db := newSchemaTestDB(t)
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(rustEraSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admin_users (id, username, password_hash) VALUES (1, 'admin', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO api_keys (id, name, key_hash, key_prefix, created_by) VALUES (1, 'k', 'h1', 'p1', 1), (2, 'd', 'h2', 'p2', 1)`); err != nil {
		t.Fatal(err)
	}
	// Key 1: customised in the Rust UI. Key 2: all Rust defaults.
	if _, err := db.Exec(`INSERT INTO api_key_settings
		(api_key_id, poster_position, poster_badge_direction, poster_badge_split, ratings_limit,
		 poster_badge_size, episode_badge_size, poster_badge_background, backdrop_badge_background,
		 backdrop_position, backdrop_badge_direction)
		VALUES (1, 'tl', 'h', 1, 4, 'xl', 's', 'k', 'n', 'bl', 'h'), (2, 'bc', 'd', 0, 3, 'm', 'l', 'd', 'd', 'tr', 'd')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO image_meta (cache_key, created_at, updated_at, image_type) VALUES
		('imdb/tt1@mil.pbc.sh.li.bm.shr.bgd.zm', 1, 1, 'p')`); err != nil {
		t.Fatal(err)
	}

	if err := RunSchema(db, SchemaSQL); err != nil {
		t.Fatalf("RunSchema: %v", err)
	}
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := services.RunUpgrades(db, t.TempDir(), true); err != nil {
		t.Fatalf("RunUpgrades: %v", err)
	}
	// A second boot must not change anything.
	if err := RunMigrations(db, Migrations); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	cols := tableColumnSetOrFail(t, db, "api_key_settings")
	for _, gone := range []string{"poster_position", "poster_badge_split", "poster_badge_background", "episode_position"} {
		if cols[gone] {
			t.Errorf("legacy column %s still present", gone)
		}
	}

	k1, err := services.GetAPIKeySettings(db, 1)
	if err != nil || k1 == nil {
		t.Fatalf("GetAPIKeySettings(1): %v", err)
	}
	wantPoster := services.LayoutFromLegacy("poster", "tl", "h", true, 4)
	gotPoster := services.UnmarshalLayout(k1.PosterLayout, nil)
	if gotPoster.Top != wantPoster.Top || gotPoster.Bottom != wantPoster.Bottom {
		t.Errorf("poster layout: got %+v, want %+v", gotPoster, wantPoster)
	}
	wantBackdrop := services.LayoutFromLegacy("backdrop", "bl", "h", false, 5)
	if got := services.UnmarshalLayout(k1.BackdropLayout, nil); got.Bottom != wantBackdrop.Bottom {
		t.Errorf("backdrop layout: got %+v, want %+v", got, wantBackdrop)
	}
	if k1.PosterBadgeSize != 142 || k1.EpisodeBadgeSize != 66 || k1.LogoBadgeSize != 100 {
		t.Errorf("sizes: got %d/%d/%d, want 142/66/100", k1.PosterBadgeSize, k1.EpisodeBadgeSize, k1.LogoBadgeSize)
	}
	if k1.PosterBadgeAlpha != 78 || k1.BackdropBadgeAlpha != 0 || k1.LogoBadgeAlpha != 80 {
		t.Errorf("alphas: got %d/%d/%d, want 78/0/80", k1.PosterBadgeAlpha, k1.BackdropBadgeAlpha, k1.LogoBadgeAlpha)
	}

	k2, err := services.GetAPIKeySettings(db, 2)
	if err != nil || k2 == nil {
		t.Fatalf("GetAPIKeySettings(2): %v", err)
	}
	for kind, js := range map[string]string{"poster": k2.PosterLayout, "backdrop": k2.BackdropLayout, "episode": k2.EpisodeLayout} {
		def := services.DefaultLayout(kind)
		got := services.UnmarshalLayout(js, nil)
		if got.Top.Count() != def.Top.Count() || got.Right.Count() != def.Right.Count() ||
			got.Bottom.Count() != def.Bottom.Count() || got.Left.Count() != def.Left.Count() {
			t.Errorf("%s layout of an untouched key changed: %s", kind, js)
		}
	}
	if k2.PosterBadgeSize != 100 || k2.EpisodeBadgeSize != 100 || k2.PosterBadgeAlpha != 80 {
		t.Errorf("defaults changed: size %d/%d alpha %d", k2.PosterBadgeSize, k2.EpisodeBadgeSize, k2.PosterBadgeAlpha)
	}

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM image_meta").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("Rust-era image_meta rows left: %d", n)
	}
}

func tableColumnSetOrFail(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, declType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &declType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	return out
}
