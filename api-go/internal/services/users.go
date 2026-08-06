package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

func CountAdminUsers(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count)
	return count, err
}

func CreateAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	now := nowUTC()
	result, err := db.Exec(
		"INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateFirstAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, fmt.Errorf("Setup already completed")
	}

	now := nowUTC()
	result, err := tx.Exec(
		"INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, now,
	)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func FindAdminUserByUsername(db *sql.DB, username string) (id int64, usernameOut string, passwordHash string, err error) {
	err = db.QueryRow("SELECT id, username, password_hash FROM admin_users WHERE username = ?", username).
		Scan(&id, &usernameOut, &passwordHash)
	return
}

func FindAdminUserByID(db *sql.DB, id int64) (username string, passwordHash string, err error) {
	err = db.QueryRow("SELECT username, password_hash FROM admin_users WHERE id = ?", id).
		Scan(&username, &passwordHash)
	return
}

// --- Refresh token CRUD ---

// GetUserPrefs returns the admin user's stored UI preferences (JSON map).
// Unknown/missing prefs come back as an empty map.
func GetUserPrefs(db *sql.DB, username string) (map[string]string, error) {
	var raw string
	err := db.QueryRow("SELECT prefs FROM admin_users WHERE username = ?", username).Scan(&raw)
	if err != nil {
		return nil, err
	}
	prefs := map[string]string{}
	if raw != "" && raw != "{}" {
		if uerr := json.Unmarshal([]byte(raw), &prefs); uerr != nil {
			return nil, uerr
		}
	}
	return prefs, nil
}

// SetUserPrefs stores the admin user's UI preferences (JSON map).
func SetUserPrefs(db *sql.DB, username string, prefs map[string]string) error {
	if prefs == nil {
		prefs = map[string]string{}
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE admin_users SET prefs = ? WHERE username = ?", string(raw), username)
	return err
}

// --- Password hashing ---

// HashPassword returns an argon2id hash of password in the project's
// canonical "hex(salt):hex(hash)" format. The salt is 16 random bytes and the
// hash uses argon2id with parameters matching the Rust port: time=1,
// memory=64MiB, threads=4, keyLen=32.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("%x:%x", salt, hash), nil
}

// VerifyPassword checks password against a storedHash produced by
// HashPassword. Returns false (not an error) when the password doesn't match;
// returns an error only on malformed stored hash.
func VerifyPassword(password, storedHash string) (bool, error) {
	before, after, ok := strings.Cut(storedHash, ":")
	if !ok {
		return false, fmt.Errorf("invalid hash format")
	}
	salt, err := hex.DecodeString(before)
	if err != nil {
		return false, err
	}
	expectedHash, err := hex.DecodeString(after)
	if err != nil {
		return false, err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return hex.EncodeToString(hash) == hex.EncodeToString(expectedHash), nil
}
