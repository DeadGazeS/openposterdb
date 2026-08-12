package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// apiKeyCipherService is the domain label used as the per-service KEK input
// for envelope encryption. Keeping it separate from the source-key services
// (<tmdb>, <omdb>, etc.) means the api_keys KEK is isolated from every other
// at-rest secret — a compromised source key ciphertext can't be retried under
// the api-key KEK.
const apiKeyCipherService = "api-key"

// EncryptAPIKey seals the raw key with the same v2 envelope scheme used for
// source keys. The result is a string suitable for the api_keys.encrypted_key
// column; the plaintext is never persisted.
func EncryptAPIKey(raw string, secretsKey []byte) (string, error) {
	return encryptSecret([]byte(raw), secretsKey, apiKeyCipherService)
}

// DecryptAPIKey is the inverse of EncryptAPIKey. Returns an error if the
// ciphertext is missing the v2: prefix, has a bad base64, or fails the GCM
// auth check (which catches both tampering and a wrong SECRETS_KEY).
func DecryptAPIKey(encrypted string, secretsKey []byte) (string, error) {
	plain, err := decryptV2(encrypted, secretsKey, apiKeyCipherService)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func CreateAPIKeyCtx(ctx context.Context, db *sql.DB, name, keyHash, keyPrefix, encryptedKey string, createdBy int64) (int64, error) {
	now := nowUTC()
	result, err := db.ExecContext(ctx,
		"INSERT INTO api_keys (name, key_hash, key_prefix, encrypted_key, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, keyHash, keyPrefix, encryptedKey, createdBy, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateAPIKey(db *sql.DB, name, keyHash, keyPrefix, encryptedKey string, createdBy int64) (int64, error) {
	return CreateAPIKeyCtx(context.Background(), db, name, keyHash, keyPrefix, encryptedKey, createdBy)
}

// APIKey models a row in api_keys. EncryptedKey holds the v2 envelope form of
// the raw key (or nil for legacy rows that pre-date the encrypted_key column
// and cannot be revealed — their raw was never persisted).
type APIKey struct {
	ID           int64
	Name         string
	KeyHash      string
	KeyPrefix    string
	EncryptedKey *string
	CreatedBy    int64
	CreatedAt    string
	LastUsedAt   *string
}

const apiKeySelectColumns = "id, name, key_hash, key_prefix, encrypted_key, created_by, created_at, last_used_at"

func FindAPIKeyByHashCtx(ctx context.Context, db *sql.DB, keyHash string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRowContext(ctx,
		"SELECT "+apiKeySelectColumns+" FROM api_keys WHERE key_hash = ?",
		keyHash,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.EncryptedKey, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func FindAPIKeyByHash(db *sql.DB, keyHash string) (*APIKey, error) {
	return FindAPIKeyByHashCtx(context.Background(), db, keyHash)
}

func FindAPIKeyByIDCtx(ctx context.Context, db *sql.DB, id int64) (*APIKey, error) {
	var k APIKey
	err := db.QueryRowContext(ctx,
		"SELECT "+apiKeySelectColumns+" FROM api_keys WHERE id = ?",
		id,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.EncryptedKey, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func FindAPIKeyByID(db *sql.DB, id int64) (*APIKey, error) {
	return FindAPIKeyByIDCtx(context.Background(), db, id)
}

// FindAPIKeyByName returns the API key with the given name, or nil. Used by the
// settings importer to update an existing key instead of recreating it.
func FindAPIKeyByNameCtx(ctx context.Context, db *sql.DB, name string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRowContext(ctx,
		"SELECT "+apiKeySelectColumns+" FROM api_keys WHERE name = ?",
		name,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.EncryptedKey, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func FindAPIKeyByName(db *sql.DB, name string) (*APIKey, error) {
	return FindAPIKeyByNameCtx(context.Background(), db, name)
}

func ListAPIKeysCtx(ctx context.Context, db *sql.DB) ([]APIKey, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+apiKeySelectColumns+" FROM api_keys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.EncryptedKey, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func ListAPIKeys(db *sql.DB) ([]APIKey, error) {
	return ListAPIKeysCtx(context.Background(), db)
}

func DeleteAPIKeyCtx(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM api_keys WHERE id = ?", id)
	return err
}

func DeleteAPIKey(db *sql.DB, id int64) error {
	return DeleteAPIKeyCtx(context.Background(), db, id)
}

func CountAPIKeysCtx(ctx context.Context, db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys").Scan(&count)
	return count, err
}

func CountAPIKeys(db *sql.DB) (int64, error) {
	return CountAPIKeysCtx(context.Background(), db)
}

func BatchUpdateLastUsedCtx(ctx context.Context, db *sql.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := nowUTC()
	batchSize := 100
	for i := 0; i < len(ids); i += batchSize {
		end := min(i+batchSize, len(ids))
		chunk := ids[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk)+1)
		args[0] = now
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j+1] = id
		}
		query := fmt.Sprintf("UPDATE api_keys SET last_used_at = ? WHERE id IN (%s)", strings.Join(placeholders, ","))
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func BatchUpdateLastUsed(db *sql.DB, ids []int64) error {
	return BatchUpdateLastUsedCtx(context.Background(), db, ids)
}

// --- Image meta queries ---
