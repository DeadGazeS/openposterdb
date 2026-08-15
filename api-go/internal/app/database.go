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
// performance/integrity pragmas used by openposterdb. The pragmas live
// in the DSN (via modernc.org/sqlite's `_pragma=` query param) so they
// are applied to every connection the pool opens — the previous
// post-open `db.Exec(p)` loop only ran on the first connection, so
// subsequent pooled connections could lose WAL / busy_timeout and
// surface SQLITE_BUSY on concurrent writes (docker logs 2026-08-15).
func OpenDatabase(dbPath string) (*sql.DB, error) {
	// _pragma=... is applied to every connection the pool opens. Keep the
	// busy_timeout long enough that preview upserts racing against each
	// other (the SQLITE_BUSY 5/6 from the docker logs) wait it out instead
	// of erroring immediately.
	dsn := dbPath + "?_pragma=busy_timeout(30000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=cache_size(-8000)" +
		"&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(4)
	return db, nil
}
