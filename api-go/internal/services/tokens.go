package services

import (
	"context"
	"database/sql"
)

func CreateRefreshTokenCtx(ctx context.Context, db *sql.DB, userID int64, tokenHash, expiresAt string) (int64, error) {
	now := nowUTC()
	result, err := db.ExecContext(ctx,
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)",
		userID, tokenHash, expiresAt, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateRefreshToken(db *sql.DB, userID int64, tokenHash, expiresAt string) (int64, error) {
	return CreateRefreshTokenCtx(context.Background(), db, userID, tokenHash, expiresAt)
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt string
	CreatedAt string
}

func FindRefreshTokenByHashCtx(ctx context.Context, db *sql.DB, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken
	err := db.QueryRowContext(ctx,
		"SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func FindRefreshTokenByHash(db *sql.DB, tokenHash string) (*RefreshToken, error) {
	return FindRefreshTokenByHashCtx(context.Background(), db, tokenHash)
}

func DeleteRefreshTokenCtx(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE id = ?", id)
	return err
}

func DeleteRefreshToken(db *sql.DB, id int64) error {
	return DeleteRefreshTokenCtx(context.Background(), db, id)
}

func DeleteRefreshTokensForUserCtx(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}

func DeleteRefreshTokensForUser(db *sql.DB, userID int64) error {
	return DeleteRefreshTokensForUserCtx(context.Background(), db, userID)
}

func DeleteExpiredRefreshTokensCtx(ctx context.Context, db *sql.DB) (int64, error) {
	now := nowUTC()
	result, err := db.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteExpiredRefreshTokens(db *sql.DB) (int64, error) {
	return DeleteExpiredRefreshTokensCtx(context.Background(), db)
}

// --- API key CRUD ---
