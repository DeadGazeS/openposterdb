package app

import (
	"database/sql"
	"log/slog"

	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

// SeedAdminIfNeeded creates the first admin user from the ADMIN_USERNAME /
// ADMIN_PASSWORD environment values when no admin user exists yet.
func SeedAdminIfNeeded(db *sql.DB, username, password string) {
	if username == "" || password == "" {
		return
	}
	count, err := services.CountAdminUsers(db)
	if err != nil {
		slog.Error("Failed to check admin users", "error", err)
		return
	}
	if count > 0 {
		slog.Debug("Admin user already exists, skipping seed")
		return
	}
	hash, err := handlers.HashPassword(password)
	if err != nil {
		slog.Error("Failed to hash admin password", "error", err)
		return
	}
	if _, err := services.CreateAdminUser(db, username, hash); err != nil {
		slog.Error("Failed to seed admin user", "error", err)
		return
	}
	slog.Info("Seeded admin user from environment", "username", username)
}
