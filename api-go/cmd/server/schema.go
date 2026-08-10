package main

import (
	"openposterdb/internal/app"
)

// schemaSQL and migrations live in internal/app so internal tests can drive
// RunSchema/RunMigrations against an in-memory SQLite (the cmd package's main
// files aren't importable from tests). The unexported aliases below keep the
// main.go call sites unchanged.
var (
	schemaSQL   = app.SchemaSQL
	migrations  = app.Migrations
)
