package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
