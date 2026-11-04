package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

func TestExportSettings_PassphraseRules(t *testing.T) {
	db := newHandlersTestDB(t)
	h := HandleExportSettings(db, nil)
	for _, tc := range []struct {
		name, method, url, body string
		want                    int
		encrypted               bool
	}{
		{"POST encrypted", "POST", "/", `{"passphrase":"s3cret-pass"}`, 200, true},
		{"POST plain", "POST", "/", `{}`, 200, false},
		{"provider keys need a passphrase", "POST", "/", `{"include_service_keys":true}`, 400, false},
		{"no passphrase in the URL", "GET", "/?passphrase=leaky", "", 400, false},
		{"GET provider keys refused", "GET", "/?include_service_keys=1", "", 400, false},
		{"GET plain still works", "GET", "/", "", 200, false},
	} {
		req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
			continue
		}
		if tc.want == 200 {
			isEnc := strings.Contains(rec.Body.String(), "settings-encrypted")
			if isEnc != tc.encrypted {
				t.Errorf("%s: encrypted=%v, want %v", tc.name, isEnc, tc.encrypted)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s: export must be no-store", tc.name)
			}
		}
	}
	_ = http.MethodGet
}

// Import takes the passphrase from the body only (URLs end up in logs).
func TestImportSettings_RejectsPassphraseInURL(t *testing.T) {
	db := newHandlersTestDB(t)
	h := HandleImportSettings(db, nil, nil)
	req := httptest.NewRequest("POST", "/?passphrase=leaky", strings.NewReader(`{"kind":"openposterdb/settings-encrypted"}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "request body") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

// A successful import must rebuild the provider clients (imported service
// keys used to take effect only after a restart).
func TestImportSettings_RefreshesClients(t *testing.T) {
	db := newHandlersTestDB(t)
	calls := 0
	h := HandleImportSettings(db, nil, func() { calls++ })
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"kind":"openposterdb/settings","version":1}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if calls != 1 {
		t.Errorf("onApplied called %d times, want 1", calls)
	}
}

// The import body is bounded (it was read with an unlimited io.ReadAll).
func TestImportSettings_RejectsHugeBody(t *testing.T) {
	db := newHandlersTestDB(t)
	h := HandleImportSettings(db, nil, nil)
	big := `{"kind":"openposterdb/settings","pad":"` + strings.Repeat("x", maxImportBytes) + `"}`
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/", strings.NewReader(big)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

// Imports get the same validation as the PUT endpoints: a hand-edited or
// corrupted backup used to store values those endpoints reject.
func TestImportSettings_ValidatesValues(t *testing.T) {
	for name, body := range map[string]string{
		"per-key ratings_limit": `{"kind":"openposterdb/settings","version":99,"api_keys":[{"name":"k","settings":{"ratings_limit":999}}]}`,
		"global ratings_limit":  `{"kind":"openposterdb/settings","version":99,"settings":{"ratings_limit":"999"}}`,
		"trusted_proxies":       `{"kind":"openposterdb/settings","version":99,"settings":{"trusted_proxies":"0.0.0.0/0"}}`,
	} {
		db := newHandlersTestDB(t)
		calls := 0
		h := HandleImportSettings(db, nil, func() { calls++ })
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Errorf("%s: got %d %s, want 400", name, rec.Code, rec.Body.String())
		}
		if calls != 0 {
			t.Errorf("%s: a rejected import must change nothing", name)
		}
	}
}

// A real export (default settings + a key with default settings) must pass
// the import validation unchanged.
func TestImportSettings_ExportRoundTripPassesValidation(t *testing.T) {
	db := newHandlersTestDB(t)
	_, hash, prefix := services.GenerateAPIKey()
	id, err := services.CreateAPIKey(db, "k", hash, prefix, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if rec := putKeySettings(t, db, id, fullKeySettingsPayload); rec.Code != 200 {
		t.Fatalf("seed key settings: %d %s", rec.Code, rec.Body.String())
	}
	exp := httptest.NewRecorder()
	HandleExportSettings(db, nil)(exp, httptest.NewRequest("POST", "/", strings.NewReader(`{"include_api_keys":true}`)))
	if exp.Code != 200 {
		t.Fatalf("export: %d %s", exp.Code, exp.Body.String())
	}
	imp := httptest.NewRecorder()
	HandleImportSettings(db, nil, nil)(imp, httptest.NewRequest("POST", "/", strings.NewReader(exp.Body.String())))
	if imp.Code != 200 {
		t.Fatalf("import of a fresh export: %d %s", imp.Code, imp.Body.String())
	}
}
