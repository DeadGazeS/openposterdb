package main

var schemaSQL = []string{
	`CREATE TABLE IF NOT EXISTS image_meta (
		cache_key TEXT PRIMARY KEY,
		release_date TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
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
		poster_position        TEXT NOT NULL DEFAULT 'bc',
		logo_ratings_limit     INTEGER NOT NULL DEFAULT 5,
		backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5,
		poster_badge_style     TEXT NOT NULL DEFAULT 'h',
		logo_badge_style       TEXT NOT NULL DEFAULT 'v',
		backdrop_badge_style   TEXT NOT NULL DEFAULT 'v',
		poster_label_style     TEXT NOT NULL DEFAULT 'i',
		logo_label_style       TEXT NOT NULL DEFAULT 'i',
		backdrop_label_style   TEXT NOT NULL DEFAULT 'i',
		poster_badge_direction TEXT NOT NULL DEFAULT 'd'
	)`,
}

type migration struct {
	SQL           string
	ExpectedError string
}

var migrations = []migration{
	{
		"ALTER TABLE api_key_settings ADD COLUMN ratings_limit INTEGER NOT NULL DEFAULT 3",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN ratings_order TEXT NOT NULL DEFAULT 'mal,imdb,lb,rt,rta,mc,tmdb,trakt'",
		"duplicate column",
	},
	{
		"ALTER TABLE image_meta ADD COLUMN image_type TEXT NOT NULL DEFAULT 'poster'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_position TEXT NOT NULL DEFAULT 'bc'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_ratings_limit INTEGER NOT NULL DEFAULT 5",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_ratings_limit INTEGER NOT NULL DEFAULT 5",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_style TEXT NOT NULL DEFAULT 'h'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_badge_style TEXT NOT NULL DEFAULT 'v'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_style TEXT NOT NULL DEFAULT 'v'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_label_style TEXT NOT NULL DEFAULT 'i'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_label_style TEXT NOT NULL DEFAULT 'i'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_label_style TEXT NOT NULL DEFAULT 'i'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_direction TEXT NOT NULL DEFAULT 'd'",
		"duplicate column",
	},
	{
		"ALTER TABLE available_ratings ADD COLUMN release_date TEXT",
		"duplicate column",
	},
	{
		"CREATE INDEX IF NOT EXISTS idx_available_ratings_updated_at ON available_ratings(updated_at)",
		"already exists",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_size TEXT NOT NULL DEFAULT 'm'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_badge_size TEXT NOT NULL DEFAULT 'm'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_size TEXT NOT NULL DEFAULT 'm'",
		"duplicate column",
	},
	{
		"DROP INDEX IF EXISTS idx_image_meta_type",
		"no such index",
	},
	{
		"CREATE INDEX IF NOT EXISTS idx_image_meta_type_created ON image_meta(image_type, created_at DESC)",
		"already exists",
	},
	{
		"ALTER TABLE api_key_settings RENAME COLUMN poster_source TO image_source",
		"no such column",
	},
	{
		"ALTER TABLE api_key_settings RENAME COLUMN fanart_lang TO lang",
		"no such column",
	},
	{
		"ALTER TABLE api_key_settings RENAME COLUMN fanart_textless TO textless",
		"no such column",
	},
	{
		"UPDATE global_settings SET key = 'image_source' WHERE key = 'poster_source'",
		"no such table",
	},
	{
		"UPDATE global_settings SET key = 'lang' WHERE key = 'fanart_lang'",
		"no such table",
	},
	{
		"UPDATE global_settings SET key = 'textless' WHERE key = 'fanart_textless'",
		"no such table",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_ratings_limit INTEGER NOT NULL DEFAULT 1",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_badge_style TEXT NOT NULL DEFAULT 'v'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_label_style TEXT NOT NULL DEFAULT 'o'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_badge_size TEXT NOT NULL DEFAULT 'l'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_position TEXT NOT NULL DEFAULT 'tr'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_badge_direction TEXT NOT NULL DEFAULT 'v'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_blur INTEGER NOT NULL DEFAULT 0",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_position TEXT NOT NULL DEFAULT 'tr'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_direction TEXT NOT NULL DEFAULT 'v'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN ratings_exclude TEXT NOT NULL DEFAULT ''",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_shape TEXT NOT NULL DEFAULT 'r'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_badge_shape TEXT NOT NULL DEFAULT 'r'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_shape TEXT NOT NULL DEFAULT 'r'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_badge_shape TEXT NOT NULL DEFAULT 'r'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_alpha INTEGER NOT NULL DEFAULT 80",
		"duplicate column",
	},
	{
		// Migrate pre-alpha DBs: drop the old enum column (data replaced by the
		// new alpha default).
		"ALTER TABLE api_key_settings DROP COLUMN poster_badge_background",
		"no such column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN logo_badge_background TEXT NOT NULL DEFAULT 'd'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_badge_background TEXT NOT NULL DEFAULT 'd'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN episode_badge_background TEXT NOT NULL DEFAULT 'd'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_badge_split INTEGER NOT NULL DEFAULT 0",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN poster_fit TEXT NOT NULL DEFAULT 'native'",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_edge_inset_x INTEGER NOT NULL DEFAULT 0",
		"duplicate column",
	},
	{
		"ALTER TABLE api_key_settings ADD COLUMN backdrop_edge_inset_y INTEGER NOT NULL DEFAULT 0",
		"duplicate column",
	},
}
