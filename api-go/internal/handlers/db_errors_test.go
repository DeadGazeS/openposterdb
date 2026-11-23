package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/services"
)

// A failed key lookup (here: the database is gone) is our own failure, a 500
// with a generic message, not a 401 "invalid api key".
func TestHandleImage_KeyLookupDBError_500(t *testing.T) {
	db := newHandlersTestDB(t)
	raw, _, _ := services.GenerateAPIKey()
	db.Close()
	deps := ImageDeps{DB: db, Config: &ImageServeConfig{}}
	mux := http.NewServeMux()
	mux.Handle("/{apiKey}/{rest...}", HandleImage(func() ImageDeps { return deps }, func() bool { return false }))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+raw+"/imdb/poster-default/tt0111161.jpg", nil))
	if rec.Code != 500 || !bytes.Contains(rec.Body.Bytes(), []byte("Internal server error")) {
		t.Errorf("got %d %s, want 500 Internal server error", rec.Code, rec.Body.String())
	}
}

func TestHandleIsValid_KeyLookupDBError_500(t *testing.T) {
	db := newHandlersTestDB(t)
	raw, _, _ := services.GenerateAPIKey()
	db.Close()
	mux := http.NewServeMux()
	mux.Handle("/{apiKey}/isValid", HandleIsValid(db, func() bool { return false }, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+raw+"/isValid", nil))
	if rec.Code != 500 {
		t.Errorf("got %d %s, want 500", rec.Code, rec.Body.String())
	}
	// An unknown key on a working database stays 401.
	db2 := newHandlersTestDB(t)
	mux = http.NewServeMux()
	mux.Handle("/{apiKey}/isValid", HandleIsValid(db2, func() bool { return false }, nil))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+raw+"/isValid", nil))
	if rec.Code != 401 {
		t.Errorf("unknown key: got %d, want 401", rec.Code)
	}
}

func TestLoginHandler_DBError_500(t *testing.T) {
	db := authTestDB(t)
	db.Close()
	status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	if status != 500 {
		t.Errorf("status: got %d, want 500", status)
	}
}

func TestHandleCreateKey_UserLookupDBError_500(t *testing.T) {
	db := authTestDB(t)
	db.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":"k"}`))
	rec := httptest.NewRecorder()
	HandleCreateKey(db, testSecretsKey)(rec, WithAuthUser(req, "admin"))
	if rec.Code != 500 {
		t.Errorf("got %d %s, want 500", rec.Code, rec.Body.String())
	}
}

// A failed password hash during setup is our failure (500), not a bad request.
func TestSetupHandler_HashFailure_500(t *testing.T) {
	db := authTestDB(t)
	orig := hashPassword
	hashPassword = func(context.Context, string) (string, error) { return "", errors.New("rng broke") }
	t.Cleanup(func() { hashPassword = orig })
	status, _, _ := SetupHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
	if status != 500 {
		t.Errorf("status: got %d, want 500", status)
	}
}

// Giving up on the argon2 slot (context cancelled / timed out) is a capacity
// refusal: 503 + Retry-After, not a 401 for a wrong password.
func TestLoginHandler_HashSlotWaitEnded_503(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	orig := verifyPassword
	t.Cleanup(func() { verifyPassword = orig })
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		verifyPassword = func(context.Context, string, string) (bool, error) { return false, cause }
		status, resp, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "password123")
		if status != 503 {
			t.Errorf("%v: status %d, want 503", cause, status)
		}
		var ae *apperr.AppError
		if err, ok := resp.(error); !ok || !errors.As(err, &ae) || ae.RetryAfter <= 0 {
			t.Errorf("%v: resp %#v, want a busy AppError with Retry-After", cause, resp)
		}
		rec := httptest.NewRecorder()
		WriteAuthResponse(rec, status, resp, nil)
		if rec.Code != 503 || rec.Header().Get("Retry-After") == "" {
			t.Errorf("%v: written %d Retry-After %q", cause, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
	// A real mismatch stays 401.
	verifyPassword = orig
	if status, _, _ := LoginHandler(authContext(), db, testJWTSecret, false, "admin", "wrong-pass"); status != 401 {
		t.Errorf("wrong password: %d, want 401", status)
	}
}

// Key login (web UI with an API key): a failed lookup is a 500, an unknown
// key stays 401.
func TestKeyLoginHandler_DBError_500(t *testing.T) {
	db := authTestDB(t)
	raw, _, _ := services.GenerateAPIKey()
	if status, _ := KeyLoginHandler(authContext(), db, testJWTSecret, raw); status != 401 {
		t.Errorf("unknown key: got %d, want 401", status)
	}
	db.Close()
	if status, _ := KeyLoginHandler(authContext(), db, testJWTSecret, raw); status != 500 {
		t.Errorf("db error: got %d, want 500", status)
	}
}
