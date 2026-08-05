package handlers

import (
	"context"
	"net/http"
	"strings"
)

// KeyRecorder is satisfied by anything that can remember an api_keys ID for
// later persistence (e.g. services.LastUsedFlusher). Passing nil disables
// recording.
type KeyRecorder interface {
	Record(keyID int64)
}

// recordKey is a helper that safely no-ops on a nil recorder.
func recordKey(r KeyRecorder, keyID int64) {
	if r != nil {
		r.Record(keyID)
	}
}

type contextKey string

const authUserKey contextKey = "authUser"
const apiKeyUserKey contextKey = "apiKeyUser"

type AuthUser struct {
	Username string
}

type APIKeyUser struct {
	KeyID int64
}

func GetAuthUser(r *http.Request) *AuthUser {
	if u, ok := r.Context().Value(authUserKey).(*AuthUser); ok {
		return u
	}
	return nil
}

func WithAuthUser(r *http.Request, username string) *http.Request {
	user := &AuthUser{Username: username}
	ctx := context.WithValue(r.Context(), authUserKey, user)
	return r.WithContext(ctx)
}

func GetAPIKeyUser(r *http.Request) *APIKeyUser {
	if u, ok := r.Context().Value(apiKeyUserKey).(*APIKeyUser); ok {
		return u
	}
	return nil
}

func WithAPIKeyUser(r *http.Request, keyID int64) *http.Request {
	user := &APIKeyUser{KeyID: keyID}
	ctx := context.WithValue(r.Context(), apiKeyUserKey, user)
	return r.WithContext(ctx)
}

func RequireAuth(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ParseJWT(token, jwtSecret)
			if err != nil || claims.Username == "" {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, WithAuthUser(r, claims.Username))
		})
	}
}

func RequireAPIKeyAuth(jwtSecret []byte, flusher KeyRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ParseAPIKeyJWT(token, jwtSecret)
			if err != nil || claims.KeyID == 0 {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Record the key use so the admin UI can show when each key was
			// last seen. nil-safe so routes that don't pass a flusher (tests)
			// keep working.
			recordKey(flusher, claims.KeyID)

			next.ServeHTTP(w, WithAPIKeyUser(r, claims.KeyID))
		})
	}
}

func RequireAnyAuth(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if claims, err := ParseJWT(token, jwtSecret); err == nil && claims.Username != "" {
				next.ServeHTTP(w, WithAuthUser(r, claims.Username))
				return
			}
			if claims, err := ParseAPIKeyJWT(token, jwtSecret); err == nil && claims.KeyID != 0 {
				next.ServeHTTP(w, WithAPIKeyUser(r, claims.KeyID))
				return
			}

			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return token
	}
	if cookie, err := r.Cookie("token"); err == nil {
		return cookie.Value
	}
	return ""
}
