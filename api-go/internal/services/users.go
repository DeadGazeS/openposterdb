package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

func CountAdminUsersCtx(ctx context.Context, db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users").Scan(&count)
	return count, err
}

func CountAdminUsers(db *sql.DB) (int64, error) {
	return CountAdminUsersCtx(context.Background(), db)
}

func CreateAdminUserCtx(ctx context.Context, db *sql.DB, username, passwordHash string) (int64, error) {
	now := nowUTC()
	result, err := db.ExecContext(ctx,
		"INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	return CreateAdminUserCtx(context.Background(), db, username, passwordHash)
}

func CreateFirstAdminUserCtx(ctx context.Context, db *sql.DB, username, passwordHash string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, fmt.Errorf("Setup already completed")
	}

	now := nowUTC()
	result, err := tx.ExecContext(ctx,
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

func CreateFirstAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	return CreateFirstAdminUserCtx(context.Background(), db, username, passwordHash)
}

func FindAdminUserByUsernameCtx(ctx context.Context, db *sql.DB, username string) (id int64, usernameOut string, passwordHash string, err error) {
	err = db.QueryRowContext(ctx, "SELECT id, username, password_hash FROM admin_users WHERE username = ?", username).
		Scan(&id, &usernameOut, &passwordHash)
	return
}

func FindAdminUserByUsername(db *sql.DB, username string) (id int64, usernameOut string, passwordHash string, err error) {
	return FindAdminUserByUsernameCtx(context.Background(), db, username)
}

func FindAdminUserByIDCtx(ctx context.Context, db *sql.DB, id int64) (username string, passwordHash string, err error) {
	err = db.QueryRowContext(ctx, "SELECT username, password_hash FROM admin_users WHERE id = ?", id).
		Scan(&username, &passwordHash)
	return
}

func FindAdminUserByID(db *sql.DB, id int64) (username string, passwordHash string, err error) {
	return FindAdminUserByIDCtx(context.Background(), db, id)
}

// --- Refresh token CRUD ---

// GetUserPrefs returns the admin user's stored UI preferences (JSON map).
// Unknown/missing prefs come back as an empty map.
func GetUserPrefsCtx(ctx context.Context, db *sql.DB, username string) (map[string]string, error) {
	var raw string
	err := db.QueryRowContext(ctx, "SELECT prefs FROM admin_users WHERE username = ?", username).Scan(&raw)
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

func GetUserPrefs(db *sql.DB, username string) (map[string]string, error) {
	return GetUserPrefsCtx(context.Background(), db, username)
}

// SetUserPrefs stores the admin user's UI preferences (JSON map).
func SetUserPrefsCtx(ctx context.Context, db *sql.DB, username string, prefs map[string]string) error {
	if prefs == nil {
		prefs = map[string]string{}
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "UPDATE admin_users SET prefs = ? WHERE username = ?", string(raw), username)
	return err
}

func SetUserPrefs(db *sql.DB, username string, prefs map[string]string) error {
	return SetUserPrefsCtx(context.Background(), db, username, prefs)
}

// --- Password hashing ---

// HashPassword returns an argon2id hash of password in the project's
// canonical "hex(salt):hex(hash)" format. The salt is 16 random bytes and the
// hash uses argon2id with time=1, memory=64MiB, threads=4, keyLen=32. (The
// Rust version stored PHC strings with the argon2 crate defaults instead; see
// VerifyPassword.)
func HashPassword(password string) (string, error) {
	return HashPasswordCtx(context.Background(), password)
}

// HashPasswordCtx is HashPassword; waiting for a hashing slot ends with ctx.
func HashPasswordCtx(ctx context.Context, password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := argon2IDKey(ctx, []byte(password), salt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x:%x", salt, hash), nil
}

// phcPrefix marks a password hash written by the Rust version
// ("$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>").
const phcPrefix = "$argon2id$"

// NeedsRehash reports whether storedHash is in a legacy format that should be
// replaced by HashPassword after the next successful login.
func NeedsRehash(storedHash string) bool {
	return strings.HasPrefix(storedHash, phcPrefix)
}

// VerifyPassword checks password against a storedHash produced by
// HashPassword, or a PHC-format argon2id string from a Rust-era install.
// Returns false (not an error) when the password doesn't match; returns an
// error only on malformed stored hash.
func VerifyPassword(password, storedHash string) (bool, error) {
	return VerifyPasswordCtx(context.Background(), password, storedHash)
}

// VerifyPasswordCtx is VerifyPassword; waiting for a hashing slot ends with
// ctx (the error is then ctx.Err()).
func VerifyPasswordCtx(ctx context.Context, password, storedHash string) (bool, error) {
	if strings.HasPrefix(storedHash, phcPrefix) {
		return verifyPHCArgon2id(ctx, password, storedHash)
	}
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
	hash, err := argon2IDKey(ctx, []byte(password), salt)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(hash, expectedHash) == 1, nil
}

// passwordHashSlots caps concurrent argon2id runs: each allocates 64 MiB, so
// a burst of parallel logins (login is unauthenticated) could otherwise
// exhaust memory. Extra callers wait for a slot, or until their context ends
// (a client that went away no longer holds a place in the queue).
var passwordHashSlots = make(chan struct{}, 4)

func argon2IDKey(ctx context.Context, password, salt []byte) ([]byte, error) {
	return argon2IDKeyParams(ctx, password, salt, 1, 64*1024, 4, 32)
}

func argon2IDKeyParams(ctx context.Context, password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) ([]byte, error) {
	select {
	case passwordHashSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-passwordHashSlots }()
	return argon2.IDKey(password, salt, time, memory, threads, keyLen), nil
}

// Upper bounds for parameters read from a stored PHC string, so a corrupted
// row can't make a login allocate gigabytes or spin for minutes.
const (
	phcMaxMemoryKiB = 256 * 1024
	phcMaxTime      = 16
	phcMaxKeyLen    = 128
)

// verifyPHCArgon2id verifies password against a PHC string
// "$argon2id$v=19$m=<KiB>,t=<iterations>,p=<lanes>$<b64 salt>$<b64 hash>"
// (unpadded standard base64, as written by the Rust argon2 crate).
func verifyPHCArgon2id(ctx context.Context, password, phc string) (bool, error) {
	parts := strings.Split(phc, "$")
	// "", "argon2id", ["v=19",] "m=..,t=..,p=..", salt, hash
	if len(parts) == 5 {
		parts = append(parts[:2], append([]string{"v=19"}, parts[2:]...)...)
	}
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid PHC hash format")
	}
	if parts[2] != "v=19" {
		return false, fmt.Errorf("unsupported argon2 version %q", parts[2])
	}
	var m, t, p uint64
	seen := 0
	for kv := range strings.SplitSeq(parts[3], ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return false, fmt.Errorf("invalid PHC parameter %q", kv)
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return false, fmt.Errorf("invalid PHC parameter %q", kv)
		}
		switch k {
		case "m":
			m = n
		case "t":
			t = n
		case "p":
			p = n
		default:
			return false, fmt.Errorf("unknown PHC parameter %q", k)
		}
		seen++
	}
	if seen != 3 || m == 0 || t == 0 || p == 0 || p > 255 || m > phcMaxMemoryKiB || t > phcMaxTime || m < 8*p {
		return false, fmt.Errorf("invalid PHC parameters %q", parts[3])
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid PHC salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid PHC hash: %w", err)
	}
	if len(salt) < 8 || len(expected) < 4 || len(expected) > phcMaxKeyLen {
		return false, fmt.Errorf("invalid PHC salt or hash length")
	}
	hash, err := argon2IDKeyParams(ctx, []byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(expected)))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(hash, expected) == 1, nil
}

// UpdateAdminPasswordHashCtx replaces the stored password hash of a user.
func UpdateAdminPasswordHashCtx(ctx context.Context, db *sql.DB, id int64, passwordHash string) error {
	_, err := db.ExecContext(ctx, "UPDATE admin_users SET password_hash = ? WHERE id = ?", passwordHash, id)
	return err
}
