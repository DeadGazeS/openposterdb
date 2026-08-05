package app

import (
	"database/sql"
	"path/filepath"
)

// DBPath resolves the database directory to an absolute path and returns the
// database file path inside it along with the absolute directory.
func DBPath(dbDir string) (dbPath string, dir string) {
	abs, _ := filepath.Abs(dbDir)
	return filepath.Join(abs, "openposterdb.db"), abs
}

// OpenDatabase opens the SQLite database at dbPath and applies the
// performance/integrity pragmas used by openposterdb.
func OpenDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(4)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-8000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		db.Exec(p)
	}
	return db, nil
}
