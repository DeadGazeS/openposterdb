package app

import (
	"database/sql"
	"strings"
)

// Migration describes one idempotent schema migration: the SQL to run and the
// substring of the error that means "already applied" (e.g. "duplicate column
// name"), which is skipped instead of failing. A migration with Func set runs
// that function instead of SQL; it must be idempotent, because the list
// replays in full on every boot.
type Migration struct {
	SQL           string
	ExpectedError string
	Func          func(*sql.DB) error
}

// RunSchema executes each schema statement in order.
func RunSchema(db *sql.DB, queries []string) error {
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// RunMigrations applies each migration, skipping those that fail with their
// ExpectedError substring (i.e. already applied to an existing database).
func RunMigrations(db *sql.DB, migrations []Migration) error {
	for _, m := range migrations {
		if m.Func != nil {
			if err := m.Func(db); err != nil {
				return err
			}
			continue
		}
		_, err := db.Exec(m.SQL)
		if err != nil {
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, m.ExpectedError) {
				continue
			}
			return err
		}
	}
	return nil
}
