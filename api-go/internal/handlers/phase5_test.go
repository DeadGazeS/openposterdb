package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

// A new key belongs to the admin who created it (created_by was always 1);
// a session whose admin no longer exists can't create keys.
func TestCreateKey_CreatedByIsTheLoggedInAdmin(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db) // id 1, "admin"
	res, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('second', 'x')")
	if err != nil {
		t.Fatal(err)
	}
	secondID, _ := res.LastInsertId()

	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":"k"}`))
	rec := httptest.NewRecorder()
	HandleCreateKey(db, testSecretsKey)(rec, WithAuthUser(req, "second"))
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var createdBy int64
	if err := db.QueryRow("SELECT created_by FROM api_keys").Scan(&createdBy); err != nil {
		t.Fatal(err)
	}
	if createdBy != secondID {
		t.Errorf("created_by = %d, want %d", createdBy, secondID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":"k2"}`))
	rec = httptest.NewRecorder()
	HandleCreateKey(db, testSecretsKey)(rec, WithAuthUser(req, "deleted-user"))
	if rec.Code != 401 {
		t.Errorf("gone user: %d, want 401", rec.Code)
	}
}

func TestPurgeTitle_ValidatesIDAndReportsRatings(t *testing.T) {
	db := crudTestDB(t)
	if err := services.UpsertAvailableRatings(db, "imdb/tt123", "it", nil); err != nil {
		t.Fatal(err)
	}
	caches := &services.MemCacheSet{
		ImageMem: services.NewMemCache(0, 100, 0, 0),
		IDs:      services.NewMemCache(0, 100, 0, 0),
	}
	caches.ImageMem.Set("imdb/tt123@it.sd.zm", []byte("a"), 1)
	caches.ImageMem.Set("imdb/tt123_t_de@it.sd.zm", []byte("b"), 1)
	caches.ImageMem.Set("imdb/tt1234567@it.sd.zm", []byte("c"), 1)
	caches.IDs.Set("imdb/tt1234567", "other title", 1)

	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	rec := httptest.NewRecorder()
	HandlePurgeTitle(db, t.TempDir(), "p", "imdb", "tt123", true, caches)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["ratings_deleted"] != float64(1) {
		t.Errorf("ratings_deleted = %v, want 1", body["ratings_deleted"])
	}
	for _, k := range []string{"imdb/tt123@it.sd.zm", "imdb/tt123_t_de@it.sd.zm"} {
		if _, ok := caches.ImageMem.Peek(k); ok {
			t.Errorf("%s not purged", k)
		}
	}
	if _, ok := caches.ImageMem.Peek("imdb/tt1234567@it.sd.zm"); !ok {
		t.Error("purging tt123 evicted tt1234567's image")
	}
	if _, ok := caches.IDs.Peek("imdb/tt1234567"); !ok {
		t.Error("purging tt123 evicted tt1234567's id")
	}

	rec = httptest.NewRecorder()
	HandlePurgeTitle(db, t.TempDir(), "p", "bogus", "tt123", true, caches)(rec, httptest.NewRequest(http.MethodDelete, "/x", nil))
	if rec.Code != 400 {
		t.Errorf("invalid id_type: %d, want 400", rec.Code)
	}
}

// A failed purge step is reported by name; its error (paths, SQL) stays in
// the log.
func TestWritePurgeResult_HidesErrorDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	writePurgeResult(rec, []string{"cache files: open /srv/cache/posters/imdb: permission denied"}, map[string]any{})
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "/srv/cache") || !strings.Contains(rec.Body.String(), "cache files failed") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// Failed logins log the client IP and a hash of the username, never the
// username itself (often a mistyped password).
func TestLoginFailed_LogsIPAndHashedUsername(t *testing.T) {
	db := authTestDB(t)
	seedAdmin(t, db, "admin", "right-password")
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx := WithClientIP(authContext(), "203.0.113.9")
	LoginHandler(ctx, db, testJWTSecret, false, "my-secret-typo", "x")
	LoginHandler(ctx, db, testJWTSecret, false, "admin", "wrong")
	out := buf.String()
	if strings.Count(out, "client_ip=203.0.113.9") != 2 {
		t.Errorf("client IP missing: %s", out)
	}
	if !strings.Contains(out, "username_hash="+usernameLogHash(testJWTSecret, "my-secret-typo")) || strings.Contains(out, "my-secret-typo") {
		t.Errorf("username not hashed: %s", out)
	}
}

func TestAuthStatus_DBErrorIsGeneric(t *testing.T) {
	db := authTestDB(t)
	db.Close()
	status, body := AuthStatus(authContext(), db, func() bool { return false }, false)
	if status != 500 || body.(map[string]string)["error"] != "Internal server error" {
		t.Errorf("%d %v", status, body)
	}
}
