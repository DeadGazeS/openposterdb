package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

// The key list never carries plaintext; POST /api/keys/{id}/reveal returns
// one key on explicit request.
func TestListKeys_NoPlaintext_RevealOnRequest(t *testing.T) {
	db := crudTestDB(t)
	crudSeedAdmin(t, db)
	id, raw, _ := crudSeedKey(t, db, "k", 1)
	enc, err := services.EncryptAPIKey(raw, testSecretsKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE api_keys SET encrypted_key = ? WHERE id = ?", enc, id); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	HandleListKeys(db, testSecretsKey)(rec, httptest.NewRequest(http.MethodGet, "/api/keys", nil))
	if strings.Contains(rec.Body.String(), raw) {
		t.Fatal("the key list must not contain the raw key")
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0]["revealable"] != true {
		t.Fatalf("list: %v %s", err, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/keys/"+strconv.FormatInt(id, 10)+"/reveal", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec = httptest.NewRecorder()
	HandleRevealKey(db, testSecretsKey)(rec, req)
	var got map[string]string
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["key"] != raw {
		t.Fatalf("reveal: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("reveal must be no-store")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/keys/999/reveal", nil)
	req.SetPathValue("id", "999")
	rec = httptest.NewRecorder()
	HandleRevealKey(db, testSecretsKey)(rec, req)
	if rec.Code != 404 {
		t.Fatalf("unknown id: got %d want 404", rec.Code)
	}
}
