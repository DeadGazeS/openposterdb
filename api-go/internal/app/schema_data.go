package app

// SchemaSQL and Migrations are the canonical, version-controlled bootstrap for
// the database. They were lifted out of cmd/server/schema.go so internal tests
// can drive RunSchema/RunMigrations against an in-memory SQLite (the cmd
// package's main-only files aren't importable from tests).
//
// Keep these two variables in sync with the on-disk schema — RunSchema is
// idempotent, but adding a new table or column means appending to this file.

var SchemaSQL = []string{
	`CREATE TABLE IF NOT EXISTS image_meta (
		cache_key TEXT PRIMARY KEY,
		release_date TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		last_accessed INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS admin_users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at    TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id     INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
		token_hash  TEXT NOT NULL UNIQUE,
		expires_at  TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS api_keys (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL,
		key_hash     TEXT NOT NULL UNIQUE,
		key_prefix   TEXT NOT NULL,
		created_by   INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
		created_at   TEXT NOT NULL DEFAULT (datetime('now')),
		last_used_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS global_settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS available_ratings (
		id_key       TEXT PRIMARY KEY,
		sources      TEXT NOT NULL,
		updated_at   INTEGER NOT NULL,
		release_date TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS api_key_settings (
		api_key_id             INTEGER PRIMARY KEY REFERENCES api_keys(id) ON DELETE CASCADE,
		image_source           TEXT NOT NULL DEFAULT 't',
		lang                   TEXT NOT NULL DEFAULT 'en',
		textless               INTEGER NOT NULL DEFAULT 0,
		ratings_limit          INTEGER NOT NULL DEFAULT 3,
		ratings_order          TEXT NOT NULL DEFAULT 'mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert',
		logo_ratings_limit     INTEGER NOT NULL DEFAULT 5,
		backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
		poster_badge_style     TEXT NOT NULL DEFAULT 'h',
		logo_badge_style       TEXT NOT NULL DEFAULT 'v',
		backdrop_badge_style   TEXT NOT NULL DEFAULT 'v',
		poster_label_style     TEXT NOT NULL DEFAULT 'i',
		logo_label_style       TEXT NOT NULL DEFAULT 'i',
		backdrop_label_style   TEXT NOT NULL DEFAULT 'i',
		poster_badge_direction TEXT NOT NULL DEFAULT 'd',
		poster_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":3,"rows":1,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}',
		logo_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":5,"rows":1,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}',
		backdrop_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":5,"rows":1,"start":"r"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}',
		episode_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":1,"rows":1,"start":"t"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'
	)`,
}

var Migrations = []Migration{
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN ratings_limit INTEGER NOT NULL DEFAULT 3",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN ratings_order TEXT NOT NULL DEFAULT 'mal,imdb,lb,rt,rta,mc,tmdb,trakt'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE image_meta ADD COLUMN image_type TEXT NOT NULL DEFAULT 'poster'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_position TEXT NOT NULL DEFAULT 'bc'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_ratings_limit INTEGER NOT NULL DEFAULT 5",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_style TEXT NOT NULL DEFAULT 'h'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_style TEXT NOT NULL DEFAULT 'v'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_style TEXT NOT NULL DEFAULT 'v'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_label_style TEXT NOT NULL DEFAULT 'i'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_label_style TEXT NOT NULL DEFAULT 'i'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_label_style TEXT NOT NULL DEFAULT 'i'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_direction TEXT NOT NULL DEFAULT 'd'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE available_ratings ADD COLUMN release_date TEXT",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "CREATE INDEX IF NOT EXISTS idx_available_ratings_updated_at ON available_ratings(updated_at)",
		ExpectedError: "already exists",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_size TEXT NOT NULL DEFAULT 'm'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_size TEXT NOT NULL DEFAULT 'm'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_size TEXT NOT NULL DEFAULT 'm'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "DROP INDEX IF EXISTS idx_image_meta_type",
		ExpectedError: "no such index",
	},
	{
		SQL:           "CREATE INDEX IF NOT EXISTS idx_image_meta_type_created ON image_meta(image_type, created_at DESC)",
		ExpectedError: "already exists",
	},
	{
		SQL:           "ALTER TABLE api_key_settings RENAME COLUMN poster_source TO image_source",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings RENAME COLUMN fanart_lang TO lang",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings RENAME COLUMN fanart_textless TO textless",
		ExpectedError: "no such column",
	},
	{
		SQL:           "UPDATE global_settings SET key = 'image_source' WHERE key = 'poster_source'",
		ExpectedError: "no such table",
	},
	{
		SQL:           "UPDATE global_settings SET key = 'lang' WHERE key = 'fanart_lang'",
		ExpectedError: "no such table",
	},
	{
		SQL:           "UPDATE global_settings SET key = 'textless' WHERE key = 'fanart_textless'",
		ExpectedError: "no such table",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_ratings_limit INTEGER NOT NULL DEFAULT 1",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_style TEXT NOT NULL DEFAULT 'v'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_label_style TEXT NOT NULL DEFAULT 'o'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_size TEXT NOT NULL DEFAULT 'l'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_position TEXT NOT NULL DEFAULT 'tr'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_direction TEXT NOT NULL DEFAULT 'v'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_blur INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_position TEXT NOT NULL DEFAULT 'tr'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_direction TEXT NOT NULL DEFAULT 'v'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN ratings_exclude TEXT NOT NULL DEFAULT ''",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_shape TEXT NOT NULL DEFAULT 'r'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_shape TEXT NOT NULL DEFAULT 'r'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_shape TEXT NOT NULL DEFAULT 'r'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_shape TEXT NOT NULL DEFAULT 'r'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_alpha INTEGER NOT NULL DEFAULT 80",
		ExpectedError: "duplicate column",
	},
	{
		// Migrate pre-alpha DBs: drop the old enum column (data replaced by the
		// new alpha default).
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN poster_badge_background",
		ExpectedError: "no such column",
	},
	{
		// The logo/backdrop/episode_badge_background columns were added by a
		// later migration but nothing ever reads or writes them (the alpha
		// replaced the background enum). Drop them for DBs that already ran the
		// ADD; fresh DBs never had them and tolerate the failed DROP.
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN logo_badge_background",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN backdrop_badge_background",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN episode_badge_background",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_split INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_fit TEXT NOT NULL DEFAULT 'native'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_edge_inset_x INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_text_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_text_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_text_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_text_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_width INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_badge_height INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_width INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_height INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_width INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_height INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_width INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_badge_height INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN poster_logo_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_logo_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN backdrop_logo_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN episode_logo_size INTEGER NOT NULL DEFAULT 100",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_position TEXT NOT NULL DEFAULT 'bc'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN logo_badge_split INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN poster_badge_size",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN logo_badge_size",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN backdrop_badge_size",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN episode_badge_size",
		ExpectedError: "no such column",
	},
	{
		SQL:           `ALTER TABLE api_key_settings ADD COLUMN poster_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":3,"rows":1,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'`,
		ExpectedError: "duplicate column",
	},
	{
		SQL:           `ALTER TABLE api_key_settings ADD COLUMN logo_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":5,"rows":1,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'`,
		ExpectedError: "duplicate column",
	},
	{
		SQL:           `ALTER TABLE api_key_settings ADD COLUMN backdrop_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":5,"rows":1,"start":"r"},"right":{"per_row":0,"rows":0,"start":"c"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'`,
		ExpectedError: "duplicate column",
	},
	{
		SQL:           `ALTER TABLE api_key_settings ADD COLUMN episode_layout TEXT NOT NULL DEFAULT '{"top":{"per_row":0,"rows":0,"start":"c"},"right":{"per_row":1,"rows":1,"start":"t"},"bottom":{"per_row":0,"rows":0,"start":"c"},"left":{"per_row":0,"rows":0,"start":"c"},"order":["bottom","top","left","right"]}'`,
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN poster_position",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN logo_position",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN backdrop_position",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN episode_position",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN poster_badge_split",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN logo_badge_split",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN poster_badges_per_row",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN logo_badges_per_row",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN backdrop_badges_per_row",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings DROP COLUMN episode_badges_per_row",
		ExpectedError: "no such column",
	},
	{
		SQL:           "ALTER TABLE api_key_settings ADD COLUMN colors TEXT NOT NULL DEFAULT ''",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE admin_users ADD COLUMN prefs TEXT NOT NULL DEFAULT '{}'",
		ExpectedError: "duplicate column",
	},
	{
		SQL:           "ALTER TABLE image_meta ADD COLUMN last_accessed INTEGER NOT NULL DEFAULT 0",
		ExpectedError: "duplicate column",
	},
}
