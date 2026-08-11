package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func CreateAPIKeyCtx(ctx context.Context, db *sql.DB, name, keyHash, keyPrefix string, createdBy int64) (int64, error) {
	now := nowUTC()
	result, err := db.ExecContext(ctx,
		"INSERT INTO api_keys (name, key_hash, key_prefix, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		name, keyHash, keyPrefix, createdBy, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateAPIKey(db *sql.DB, name, keyHash, keyPrefix string, createdBy int64) (int64, error) {
	return CreateAPIKeyCtx(context.Background(), db, name, keyHash, keyPrefix, createdBy)
}

type APIKey struct {
	ID         int64
	Name       string
	KeyHash    string
	KeyPrefix  string
	CreatedBy  int64
	CreatedAt  string
	LastUsedAt *string
}

func FindAPIKeyByHashCtx(ctx context.Context, db *sql.DB, keyHash string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRowContext(ctx,
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE key_hash = ?",
		keyHash,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
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
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE id = ?",
		id,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
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
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE name = ?",
		name,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
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
	rows, err := db.QueryContext(ctx, "SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt); err != nil {
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
