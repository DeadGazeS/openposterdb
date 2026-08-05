package services

import "database/sql"

func CreateRefreshToken(db *sql.DB, userID int64, tokenHash, expiresAt string) (int64, error) {
	now := nowUTC()
	result, err := db.Exec(
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)",
		userID, tokenHash, expiresAt, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt string
	CreatedAt string
}

func FindRefreshTokenByHash(db *sql.DB, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken
	err := db.QueryRow(
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

func DeleteRefreshToken(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE id = ?", id)
	return err
}

func DeleteRefreshTokensForUser(db *sql.DB, userID int64) error {
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}

func DeleteExpiredRefreshTokens(db *sql.DB) (int64, error) {
	now := nowUTC()
	result, err := db.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- API key CRUD ---
