package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

var dummyHash = hashPasswordDummy()

// hashPassword / verifyPassword are the argon2id calls, replaceable in tests.
var (
	hashPassword   = services.HashPasswordCtx
	verifyPassword = services.VerifyPasswordCtx
)

// loginBusyRetryAfter is the Retry-After (seconds) of a login declined because
// no password-hashing slot freed up in time.
const loginBusyRetryAfter = 5

// WriteAuthResponse writes a setup/login result: its cookies, then the body as
// JSON. An error body (e.g. the busy 503) goes through httpx.WriteAppError so
// it carries its Retry-After.
func WriteAuthResponse(w http.ResponseWriter, status int, resp any, cookies []*http.Cookie) {
	for _, c := range cookies {
		http.SetCookie(w, c)
	}
	if err, ok := resp.(error); ok {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, status, resp)
}

// isHashWaitEnded reports a password check that never ran because the
// request's context ended while waiting for a hashing slot.
func isHashWaitEnded(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// loginBusy is the answer to a login whose password check never ran: the
// server is at its hashing capacity, not a wrong password.
func loginBusy(ctx context.Context, err error) (int, any, []*http.Cookie) {
	slog.Warn("Login declined: no password-hashing slot", "client_ip", ClientIPFromContext(ctx), "error", err)
	return http.StatusServiceUnavailable, apperr.NewBusy("server busy, retry shortly", loginBusyRetryAfter), nil
}

type clientIPKey struct{}

// WithClientIP stores the request's client IP (resolved through the trusted
// proxies) for handlers that log it.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIPFromContext returns the IP stored by WithClientIP, or "".
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

func hashPasswordDummy() string {
	h, _ := services.HashPassword("openposterdb-dummy-timing-pad")
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
	}, jwtParseOptions...)
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
	}, jwtParseOptions...)
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

func IssueTokenPairCtx(ctx context.Context, db *sql.DB, jwtSecret []byte, userID int64, username string) (accessToken, rawRefresh string, err error) {
	accessToken, err = CreateToken(username, jwtSecret)
	if err != nil {
		return "", "", err
	}

	rawRefresh = GenerateRefreshToken()
	tokenHash := HashRefreshToken(rawRefresh)
	expiresAt := time.Now().UTC().Add(refreshTokenExpiryDays * 24 * time.Hour).Format("2006-01-02 15:04:05")

	if _, err := services.CreateRefreshTokenCtx(ctx, db, userID, tokenHash, expiresAt); err != nil {
		return "", "", err
	}
	// Used tokens are kept until expiry for reuse detection; prune expired
	// rows here as well as at startup so the table stays small.
	if _, err := services.DeleteExpiredRefreshTokensCtx(ctx, db); err != nil {
		slog.Warn("pruning expired refresh tokens failed", "error", err)
	}

	return accessToken, rawRefresh, nil
}

func IssueTokenPair(db *sql.DB, jwtSecret []byte, userID int64, username string) (accessToken, rawRefresh string, err error) {
	return IssueTokenPairCtx(context.Background(), db, jwtSecret, userID, username)
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

func AuthStatus(ctx context.Context, db *sql.DB, isFreeAPIKeyEnabled func() bool, disablePublicPages bool) (int, any) {
	count, err := services.CountAdminUsersCtx(ctx, db)
	if err != nil {
		slog.Error("auth status: counting admin users failed", "error", err)
		return 500, map[string]string{"error": "Internal server error"}
	}
	return 200, map[string]any{
		"setup_required":       count == 0,
		"free_api_key_enabled": isFreeAPIKeyEnabled(),
		"disable_public_pages": disablePublicPages,
	}
}

func SetupHandler(ctx context.Context, db *sql.DB, jwtSecret []byte, secureCookies bool, username, password string) (int, any, []*http.Cookie) {
	if err := services.ValidateUsername(username); err != nil {
		return 400, map[string]string{"error": err.Error()}, nil
	}
	if err := services.ValidatePassword(password); err != nil {
		return 400, map[string]string{"error": err.Error()}, nil
	}

	passwordHash, err := hashPassword(ctx, password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		return 500, map[string]string{"error": "Account operation failed"}, nil
	}

	userID, err := services.CreateFirstAdminUserCtx(ctx, db, username, passwordHash)
	if errors.Is(err, services.ErrSetupCompleted) {
		return 403, map[string]string{"error": err.Error()}, nil
	}
	if err != nil {
		slog.Error("setup: creating the admin user failed", "error", err)
		return 500, map[string]string{"error": "Account operation failed"}, nil
	}

	slog.Info("Admin account setup completed", "user", username)

	accessToken, rawRefresh, err := IssueTokenPairCtx(ctx, db, jwtSecret, userID, username)
	if err != nil {
		return 500, map[string]string{"error": "Authentication failed"}, nil
	}

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

func LoginHandler(ctx context.Context, db *sql.DB, jwtSecret []byte, secureCookies bool, username, password string) (int, any, []*http.Cookie) {
	userID, returnedUsername, passwordHash, err := services.FindAdminUserByUsernameCtx(ctx, db, username)
	// Failed logins carry the (trusted-proxy resolved) client IP and a hash of
	// the attempted username: enough to spot a guessing run in the log
	// without writing typed passwords-as-usernames into it.
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("login: looking up the admin user failed", "error", err)
		return 500, map[string]string{"error": "Authentication failed"}, nil
	}
	if err != nil || returnedUsername == "" {
		if _, verr := verifyPassword(ctx, password, dummyHash); isHashWaitEnded(verr) {
			return loginBusy(ctx, verr)
		}
		slog.Warn("Login failed: unknown username", "client_ip", ClientIPFromContext(ctx), "username_hash", services.KeyHash(username))
		return 401, map[string]string{"error": "Unauthorized"}, nil
	}

	ok, err := verifyPassword(ctx, password, passwordHash)
	if isHashWaitEnded(err) {
		return loginBusy(ctx, err)
	}
	if err != nil || !ok {
		slog.Warn("Login failed: incorrect password", "client_ip", ClientIPFromContext(ctx), "username_hash", services.KeyHash(username))
		return 401, map[string]string{"error": "Unauthorized"}, nil
	}

	slog.Info("Admin login successful", "user", username)

	// Rust-era installs stored PHC argon2id strings; replace them with the
	// current format now that we know the plaintext. A failed rewrite is not
	// fatal: the PHC hash keeps working and is retried on the next login.
	if services.NeedsRehash(passwordHash) {
		if newHash, herr := services.HashPasswordCtx(ctx, password); herr != nil {
			slog.Warn("re-hashing legacy password failed", "error", herr)
		} else if uerr := services.UpdateAdminPasswordHashCtx(ctx, db, userID, newHash); uerr != nil {
			slog.Warn("storing re-hashed legacy password failed", "error", uerr)
		} else {
			slog.Info("legacy password hash upgraded", "user", username)
		}
	}

	accessToken, rawRefresh, err := IssueTokenPairCtx(ctx, db, jwtSecret, userID, username)
	if err != nil {
		return 500, map[string]string{"error": "Authentication failed"}, nil
	}

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

func RefreshHandler(ctx context.Context, db *sql.DB, jwtSecret []byte, secureCookies bool, refreshToken string) (int, any, []*http.Cookie) {
	if refreshToken == "" {
		return 401, nil, nil
	}

	tokenHash := HashRefreshToken(refreshToken)
	stored, err := services.FindRefreshTokenByHashCtx(ctx, db, tokenHash)
	if err != nil || stored == nil {
		return 401, nil, nil
	}

	// Reuse detection: a refresh token is only valid once. A second use
	// within refreshReuseGrace is a benign race (two tabs refreshing
	// together); later, it means the token was copied and replayed — revoke
	// every session of that user so the copy dies with the original.
	if stored.UsedAt != nil {
		if time.Since(time.Unix(*stored.UsedAt, 0)) > refreshReuseGrace {
			slog.Warn("refresh token reuse detected; revoking all sessions of the user", "user_id", stored.UserID)
			if err := services.DeleteRefreshTokensForUserCtx(ctx, db, stored.UserID); err != nil {
				slog.Error("revoking sessions after refresh token reuse failed", "user_id", stored.UserID, "error", err)
			}
		}
		return 401, nil, nil
	}

	expiresAt, err := time.Parse("2006-01-02 15:04:05", stored.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) {
		services.DeleteRefreshTokenCtx(ctx, db, stored.ID)
		return 401, nil, nil
	}

	username, _, err := services.FindAdminUserByIDCtx(ctx, db, stored.UserID)
	if err != nil {
		return 401, nil, nil
	}

	// Consume first: only one of several concurrent refreshes with the same
	// token wins (the old order issued a pair for each, then deleted).
	if consumed, err := services.ConsumeRefreshTokenCtx(ctx, db, stored.ID); err != nil || !consumed {
		return 401, nil, nil
	}

	accessToken, rawRefresh, err := IssueTokenPairCtx(ctx, db, jwtSecret, stored.UserID, username)
	if err != nil {
		return 500, nil, nil
	}

	return 200, map[string]string{"token": accessToken}, []*http.Cookie{
		RefreshCookie(rawRefresh, refreshTokenMaxAgeSecs, secureCookies),
	}
}

// LogoutHandler revokes every active refresh token for the given user so a
// stolen cookie becomes unusable after logout (previously a no-op: tokens
// outlived the logout request). Returns the lookup error if the user no
// longer exists; callers that ignore the return value are unaffected because
// the original behaviour was to look up the user and discard the result.
func LogoutHandler(ctx context.Context, db *sql.DB, username string) error {
	userID, _, _, err := services.FindAdminUserByUsernameCtx(ctx, db, username)
	if err != nil {
		return err
	}
	return services.DeleteRefreshTokensForUserCtx(ctx, db, userID)
}

func KeyLoginHandler(ctx context.Context, db *sql.DB, jwtSecret []byte, apiKey string) (int, any) {
	keyHash := services.HashAPIKey(apiKey)
	k, err := services.FindAPIKeyByHashCtx(ctx, db, keyHash)
	if err != nil {
		slog.Error("key login: looking up the api key failed", "error", err)
		return 500, map[string]string{"error": "Internal server error"}
	}
	if k == nil {
		return 401, map[string]string{"error": "Unauthorized"}
	}

	token, err := CreateAPIKeyToken(k.ID, jwtSecret)
	if err != nil {
		slog.Error("key login: issuing the token failed", "error", err)
		return 500, map[string]string{"error": "Internal server error"}
	}

	return 200, map[string]any{
		"token":      token,
		"name":       k.Name,
		"key_prefix": k.KeyPrefix,
	}
}

// ClearRefreshCookie expires the refresh cookie. Path must match the one it
// was set with (RefreshCookie), or the browser keeps it.
func ClearRefreshCookie(secure bool) *http.Cookie {
	c := RefreshCookie("", -1, secure)
	return c
}

// jwtParseOptions pins the signing method to HS256 (tokens are only ever
// issued with it) and requires an expiry claim.
var jwtParseOptions = []jwt.ParserOption{
	jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	jwt.WithExpirationRequired(),
}

// refreshReuseGrace: a used refresh token presented again within this window
// is treated as a concurrent-refresh race, not as theft.
var refreshReuseGrace = 30 * time.Second
