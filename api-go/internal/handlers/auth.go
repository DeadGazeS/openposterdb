package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"

	"openposterdb/internal/services"
)

var dummyHash = hashPasswordDummy()

func hashPasswordDummy() string {
	h, _ := HashPassword("openposterdb-dummy-timing-pad")
	return h
}

const accessTokenExpiryMinutes = 15
const refreshTokenExpiryDays = 7
const refreshTokenMaxAgeSecs = refreshTokenExpiryDays * 24 * 60 * 60
const apiKeySessionExpiryHours = 24

type Claims struct {
	Username string `json:"sub"`
	jwt.RegisteredClaims
}

type APIKeyClaims struct {
	KeyID int64 `json:"key_id"`
	jwt.RegisteredClaims
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("%x:%x", salt, hash), nil
}

func VerifyPassword(password, storedHash string) (bool, error) {
	before, after, ok := strings.Cut(storedHash, ":")
	if !ok {
		return false, fmt.Errorf("invalid hash format")
	}
	saltHex := before
	hashHex := after
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false, err
	}
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false, err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return hex.EncodeToString(hash) == hex.EncodeToString(expectedHash), nil
}

func CreateToken(username string, secret []byte) (string, error) {
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenExpiryMinutes * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func GenerateRefreshToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func ParseJWT(tokenString string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func ParseAPIKeyJWT(tokenString string, secret []byte) (*APIKeyClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &APIKeyClaims{}, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*APIKeyClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func CreateAPIKeyToken(keyID int64, secret []byte) (string, error) {
	claims := APIKeyClaims{
		KeyID: keyID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(apiKeySessionExpiryHours * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func IssueTokenPair(db *sql.DB, jwtSecret []byte, userID int64, username string) (accessToken, rawRefresh string, err error) {
	accessToken, err = CreateToken(username, jwtSecret)
	if err != nil {
		return "", "", err
	}

	rawRefresh = GenerateRefreshToken()
	tokenHash := HashRefreshToken(rawRefresh)
	expiresAt := time.Now().Add(refreshTokenExpiryDays * 24 * time.Hour).Format("2006-01-02 15:04:05")

	if _, err := services.CreateRefreshToken(db, userID, tokenHash, expiresAt); err != nil {
		return "", "", err
	}

	return accessToken, rawRefresh, nil
}

func RefreshCookie(token string, maxAgeSecs int, secure bool) *http.Cookie {
	c := &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/api/auth/refresh",
		MaxAge:   maxAgeSecs,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
	return c
}

func AuthStatus(db *sql.DB, isFreeAPIKeyEnabled func() bool, disablePublicPages bool) (int, any) {
	count, err := services.CountAdminUsers(db)
	if err != nil {
		return 500, map[string]string{"error": err.Error()}
	}
	return 200, map[string]any{
		"setup_required":       count == 0,
		"free_api_key_enabled": isFreeAPIKeyEnabled(),
		"disable_public_pages": disablePublicPages,
	}
}

func SetupHandler(db *sql.DB, jwtSecret []byte, secureCookies bool, username, password string) (int, any, []*http.Cookie) {
	if err := services.ValidateUsername(username); err != nil {
		return 400, map[string]string{"error": err.Error()}, nil
	}
	if err := services.ValidatePassword(password); err != nil {
		return 400, map[string]string{"error": err.Error()}, nil
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		return 400, map[string]string{"error": "Account operation failed"}, nil
	}

	userID, err := services.CreateFirstAdminUser(db, username, passwordHash)
	if err != nil {
		return 403, map[string]string{"error": err.Error()}, nil
	}

	slog.Info("Admin account setup completed", "user", username)

	accessToken, rawRefresh, err := IssueTokenPair(db, jwtSecret, userID, username)
	if err != nil {
		return 500, map[string]string{"error": "Authentication failed"}, nil
	}

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

func LoginHandler(db *sql.DB, jwtSecret []byte, secureCookies bool, username, password string) (int, any, []*http.Cookie) {
	userID, returnedUsername, passwordHash, err := services.FindAdminUserByUsername(db, username)
	if err != nil || returnedUsername == "" {
		VerifyPassword(password, dummyHash)
		slog.Warn("Login failed: unknown username")
		return 401, map[string]string{"error": "Unauthorized"}, nil
	}

	ok, err := VerifyPassword(password, passwordHash)
	if err != nil || !ok {
		slog.Warn("Login failed: incorrect password")
		return 401, map[string]string{"error": "Unauthorized"}, nil
	}

	slog.Info("Admin login successful", "user", username)

	accessToken, rawRefresh, err := IssueTokenPair(db, jwtSecret, userID, username)
	if err != nil {
		return 500, map[string]string{"error": "Authentication failed"}, nil
	}

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

func RefreshHandler(db *sql.DB, jwtSecret []byte, secureCookies bool, refreshToken string) (int, any, []*http.Cookie) {
	if refreshToken == "" {
		return 401, nil, nil
	}

	tokenHash := HashRefreshToken(refreshToken)
	stored, err := services.FindRefreshTokenByHash(db, tokenHash)
	if err != nil || stored == nil {
		return 401, nil, nil
	}

	expiresAt, err := time.Parse("2006-01-02 15:04:05", stored.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) {
		services.DeleteRefreshToken(db, stored.ID)
		return 401, nil, nil
	}

	username, _, err := services.FindAdminUserByID(db, stored.UserID)
	if err != nil {
		return 401, nil, nil
	}

	accessToken, rawRefresh, err := IssueTokenPair(db, jwtSecret, stored.UserID, username)
	if err != nil {
		return 500, nil, nil
	}

	services.DeleteRefreshToken(db, stored.ID)

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

// LogoutHandler revokes every active refresh token for the given user so a
// stolen cookie becomes unusable after logout (previously a no-op: tokens
// outlived the logout request). Returns the lookup error if the user no
// longer exists; callers that ignore the return value are unaffected because
// the original behaviour was to look up the user and discard the result.
func LogoutHandler(db *sql.DB, username string) error {
	userID, _, _, err := services.FindAdminUserByUsername(db, username)
	if err != nil {
		return err
	}
	return services.DeleteRefreshTokensForUser(db, userID)
}

func KeyLoginHandler(db *sql.DB, jwtSecret []byte, apiKey string) (int, any) {
	keyHash := services.HashAPIKey(apiKey)
	k, err := services.FindAPIKeyByHash(db, keyHash)
	if err != nil || k == nil {
		return 401, map[string]string{"error": "Unauthorized"}
	}

	token, err := CreateAPIKeyToken(k.ID, jwtSecret)
	if err != nil {
		return 400, map[string]string{"error": "Authentication failed"}
	}

	return 200, map[string]any{
		"token":      token,
		"name":       k.Name,
		"key_prefix": k.KeyPrefix,
	}
}
