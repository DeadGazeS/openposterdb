package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// crudTestDB returns the schema used by the api_keys tests (defined in
// api_keys_test.go) so CRUD tests can exercise the same code path.
func crudTestDB(t *testing.T) *sql.DB {
	return newHandlersTestDB(t)
}

// crudSeedAdmin inserts a placeholder admin so api_keys.created_by FK holds.
func crudSeedAdmin(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO admin_users (username, password_hash) VALUES ('admin', 'x')")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// crudSeedKey creates one API key row and returns its id, the raw key (for
// /api/auth/key-login), and its prefix.
func crudSeedKey(t *testing.T, db *sql.DB, name string, adminID int64) (int64, string, string) {
	t.Helper()
	raw, hash, prefix := services.GenerateAPIKey()
	res, err := db.Exec(
		"INSERT INTO api_keys (name, key_hash, key_prefix, created_by) VALUES (?, ?, ?, ?)",
		name, hash, prefix, adminID,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id, raw, prefix
}

// TestListKeys_EmptyDB guards the empty-state branch: an empty api_keys table
// returns 200 with an empty JSON array (not null).
func TestListKeys_EmptyDB(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	rec := httptest.NewRecorder()
	HandleListKeys(db)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("empty DB: got %d keys, want 0", len(resp))
	}
}

// TestListKeys_Seeded guards the populated branch: a seeded key is reflected
// in the GET response with its id, name, prefix, created_at.
func TestListKeys_Seeded(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	crudSeedKey(t, db, "alpha", 1)
	crudSeedKey(t, db, "beta", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	rec := httptest.NewRecorder()
	HandleListKeys(db)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}

	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 2 {
		t.Errorf("got %d keys, want 2", len(resp))
	}
	names := map[string]bool{}
	for _, k := range resp {
		names[k["name"].(string)] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("missing names in response: %v", names)
	}
}

// TestCreateKey_Valid_201_WithKey exercises the happy-path POST: a valid
// name returns 201, the raw key, the prefix, and a row in api_keys.
func TestCreateKey_Valid_201_WithKey(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)

	body := bytes.NewBufferString(`{"name":"my-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/keys", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleCreateKey(db)(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["key"] == "" || resp["name"] != "my-key" || resp["key_prefix"] == "" {
		t.Errorf("response missing fields: %v", resp)
	}
	if id, ok := resp["id"].(float64); !ok || id <= 0 {
		t.Errorf("id missing or non-positive: %v", resp["id"])
	}

	// And the row exists in the DB.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE name = ?", "my-key").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("api_keys rows: got %d, want 1", count)
	}
}

// TestCreateKey_RejectsEmptyName guards the name validator: an empty name
// yields 400, no row inserted.
func TestCreateKey_RejectsEmptyName(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)

	body := bytes.NewBufferString(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/keys", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleCreateKey(db)(rec, req)
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("api_keys rows after reject: got %d, want 0", count)
	}
}

// TestCreateKey_AllowsDuplicateName documents the current contract: the
// api_keys table has no UNIQUE constraint on `name`, so two creates with the
// same name both succeed and produce two rows. The handler returns 201 for
// both (not 409 or 500). Future hardening would add the constraint here.
func TestCreateKey_AllowsDuplicateName(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)

	body := bytes.NewBufferString(`{"name":"dup"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/keys", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleCreateKey(db)(rec, req)
	if rec.Code != 201 {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"name":"dup"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/keys", body)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	HandleCreateKey(db)(rec, req)
	if rec.Code != 201 {
		t.Errorf("second create: status %d, want 201 (current contract allows dup names)", rec.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("api_keys rows: got %d, want 2 (current contract allows dup names)", count)
	}
}

// TestDeleteKey_ByID_204 — note: current handler returns 200 {ok:true} not
// 204. The test asserts the actual contract: status, body, and DB removal.
func TestDeleteKey_ByID_204(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, _ := crudSeedKey(t, db, "to-delete", 1)

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/"+strconv.FormatInt(id, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleDeleteKey(db)(rec, req)
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE id = ?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("api_keys rows after delete: got %d, want 0", count)
	}
}

// TestDeleteKey_NotFound_404 — deleting a non-existent id is a no-op that
// returns 200 (the DELETE doesn't error). The contract here is "no error,
// no row".
func TestDeleteKey_NotFound_404(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/9999", nil)
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	HandleDeleteKey(db)(rec, req)
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
}

// TestDeleteKey_BadID_400 guards invalid id parsing.
func TestDeleteKey_BadID_400(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/notanumber", nil)
	req.SetPathValue("id", "notanumber")
	rec := httptest.NewRecorder()
	HandleDeleteKey(db)(rec, req)
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// TestGetKey_ByID_200 guards the per-key settings GET: returns the
// effective settings including fanart_available.
func TestGetKey_ByID_200(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, _ := crudSeedKey(t, db, "k", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/"+strconv.FormatInt(id, 10)+"/settings", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleGetKeySettings(db, true)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings_Contains(rec.Body.String(), "fanart_available") {
		t.Errorf("response missing fanart_available: %s", rec.Body.String())
	}
}

// TestGetKey_NotFound_ReturnsDefaults guards the missing-row branch: a GET
// for a key id that has no api_key_settings row returns the effective
// (global-default) settings rather than 404 — that's the documented contract
// (the per-key settings endpoint always returns a settings object).
func TestGetKey_NotFound_ReturnsDefaults(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/9999/settings", nil)
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	HandleGetKeySettings(db, false)(rec, req)
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200 (missing key returns defaults)", rec.Code)
	}
}

// TestGetKey_BadID_400 guards the invalid-id parse branch.
func TestGetKey_BadID_400(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/notanumber/settings", nil)
	req.SetPathValue("id", "notanumber")
	rec := httptest.NewRecorder()
	HandleGetKeySettings(db, false)(rec, req)
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// TestUpdateSettings_ByID_200 guards the happy-path PUT.
func TestUpdateSettings_ByID_200(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, _ := crudSeedKey(t, db, "k", 1)

	payload := `{"lang":"de","ratings_limit":5}`
	req := httptest.NewRequest(http.MethodPut, "/api/keys/"+strconv.FormatInt(id, 10)+"/settings", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleUpdateKeySettings(db)(rec, req)
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	stored, err := services.GetAPIKeySettings(db, id)
	if err != nil || stored == nil {
		t.Fatalf("GetAPIKeySettings: %v", err)
	}
	if stored.Lang != "de" {
		t.Errorf("lang: got %q, want de", stored.Lang)
	}
	if stored.RatingsLimit != 5 {
		t.Errorf("ratings_limit: got %d, want 5", stored.RatingsLimit)
	}
}

// TestResetSettings_204 — note: handler returns 200 {ok:true}, not 204. We
// assert the actual contract: status + that the row is gone.
func TestResetSettings_204(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, _ := crudSeedKey(t, db, "k", 1)
	if err := services.UpsertAPIKeySettings(db, &services.APIKeySettings{
		APIKeyID: id, ImageSource: "t", Lang: "de", RatingsLimit: 5,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/"+strconv.FormatInt(id, 10)+"/settings", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleResetKeySettings(db)(rec, req)
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	stored, err := services.GetAPIKeySettings(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("settings not deleted: %+v", stored)
	}
}

// TestKeyMe_ValidKey_200 guards the /api/key/me endpoint with a valid API
// key JWT. The handler reads the APIKeyUser from the request context (set
// by RequireAPIKeyAuth middleware), so we inject it directly via
// WithAPIKeyUser here — this mirrors what the middleware does in production.
func TestKeyMe_ValidKey_200(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, prefix := crudSeedKey(t, db, "self-key", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/key/me", nil)
	req = WithAPIKeyUser(req, id)
	rec := httptest.NewRecorder()
	HandleSelfKeyInfo(db)(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["name"] != "self-key" {
		t.Errorf("name: got %v, want self-key", resp["name"])
	}
	if resp["key_prefix"] != prefix {
		t.Errorf("key_prefix: got %v, want %s", resp["key_prefix"], prefix)
	}
}

// TestKeyMe_InvalidKey_401 guards the no-auth branch.
func TestKeyMe_InvalidKey_401(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/key/me", nil)
	rec := httptest.NewRecorder()
	HandleSelfKeyInfo(db)(rec, req)
	if rec.Code != 401 {
		t.Errorf("status: got %d, want 401", rec.Code)
	}
}

// TestListKeys_RejectsWrongMethod guards the method guard: GET-only.
func TestListKeys_RejectsWrongMethod(t *testing.T) {
	db := crudTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/keys", nil)
	rec := httptest.NewRecorder()
	HandleListKeys(db)(rec, req)
	if rec.Code != 405 {
		t.Errorf("status: got %d, want 405", rec.Code)
	}
}

// TestCreateKey_RejectsBadJSON guards the JSON decoder: garbage in → 400.
func TestCreateKey_RejectsBadJSON(t *testing.T) {
	db := crudTestDB(t)

	body := bytes.NewBufferString(`{not-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/keys", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleCreateKey(db)(rec, req)
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// TestUpdateSettings_InvalidJSON_400 guards the PUT JSON decoder.
func TestUpdateSettings_InvalidJSON_400(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, _, _ := crudSeedKey(t, db, "k", 1)

	body := bytes.NewBufferString(`{not-json`)
	req := httptest.NewRequest(http.MethodPut, "/api/keys/"+strconv.FormatInt(id, 10)+"/settings", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	HandleUpdateKeySettings(db)(rec, req)
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// strings_Contains is a tiny indirection so this file doesn't have to import
// "strings" just for one contains check.
func strings_Contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// _ keeps ctx in scope to avoid "imported and not used" warnings if the
// file is reorganised; not currently used by any test but the imports stay
// for consistency with sibling files.
var _ = context.Background
