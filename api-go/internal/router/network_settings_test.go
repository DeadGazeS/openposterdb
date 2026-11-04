package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openposterdb/internal/config"
)

func adminToken(t *testing.T, r *Router) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	return body.Token
}

type networkResp struct {
	Locked   bool          `json:"locked"`
	Settings proxySettings `json:"settings"`
	Detected struct {
		ClientIP string `json:"client_ip"`
	} `json:"detected"`
}

func networkCall(t *testing.T, r *Router, token, method, body, remote, xff string) (int, networkResp) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/admin/settings/network", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out networkResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// Settings → General → Config: the trusted-proxy setting is editable,
// applies on the next request (readout and rate limiter alike), rejects
// catch-all ranges.
func TestNetworkSettings_EditAndApplyLive(t *testing.T) {
	r := authTestRouter(t, 0)
	token := adminToken(t, r)
	viaCloudflare := "9.9.9.9, 162.158.1.1" // visitor, then the Cloudflare edge nginx saw

	code, got := networkCall(t, r, token, http.MethodGet, "", "172.18.0.3:5000", viaCloudflare)
	if code != 200 || got.Locked || !got.Settings.TrustPrivate || got.Settings.TrustCloudflare {
		t.Fatalf("default: %d %+v", code, got)
	}
	if got.Detected.ClientIP != "162.158.1.1" {
		t.Fatalf("default trust sees the Cloudflare edge, got %q", got.Detected.ClientIP)
	}

	code, got = networkCall(t, r, token, http.MethodPut, `{"trust_private":true,"trust_cloudflare":true,"custom":[]}`, "172.18.0.3:5000", viaCloudflare)
	if code != 200 || !got.Settings.TrustCloudflare {
		t.Fatalf("save: %d %+v", code, got)
	}
	if got.Detected.ClientIP != "9.9.9.9" {
		t.Fatalf("after trusting Cloudflare the visitor must be seen, got %q", got.Detected.ClientIP)
	}

	// The rate limiter keys on the same live setting: two visitors behind
	// one Cloudflare edge get separate buckets.
	rl := newRateLimiter(1)
	rl.clientIP = r.trust.clientIP
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for _, visitor := range []string{"9.9.9.9", "8.8.8.8"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "172.18.0.3:5000"
		req.Header.Set("X-Forwarded-For", visitor+", 162.158.1.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("visitor %s throttled (%d): buckets not keyed per visitor", visitor, rec.Code)
		}
	}

	code, _ = networkCall(t, r, token, http.MethodPut, `{"trust_private":true,"custom":["0.0.0.0/0"]}`, "172.18.0.3:5000", "")
	if code != 400 {
		t.Fatalf("catch-all range: got %d want 400", code)
	}
	code, _ = networkCall(t, r, token, http.MethodPut, `{"custom":["not-an-ip"]}`, "172.18.0.3:5000", "")
	if code != 400 {
		t.Fatalf("invalid entry: got %d want 400", code)
	}
	code, got = networkCall(t, r, token, http.MethodPut, `{"trust_private":false,"trust_cloudflare":false,"custom":[]}`, "172.18.0.3:5000", viaCloudflare)
	if code != 200 || got.Detected.ClientIP != "172.18.0.3" {
		t.Fatalf("trust nothing: %d %+v", code, got)
	}
}

// TRUSTED_PROXIES in the environment locks the setting.
func TestNetworkSettings_LockedByEnv(t *testing.T) {
	r := authTestRouter(t, 0)
	r.trust = newProxyTrust(&config.Config{TrustedProxiesEnv: "private,cloudflare"}, r.state.DB)
	token := adminToken(t, r)
	code, got := networkCall(t, r, token, http.MethodGet, "", "10.0.0.2:1", "")
	if code != 200 || !got.Locked || !got.Settings.TrustCloudflare {
		t.Fatalf("locked GET: %d %+v", code, got)
	}
	if code, _ := networkCall(t, r, token, http.MethodPut, `{"trust_private":true}`, "10.0.0.2:1", ""); code != 409 {
		t.Fatalf("locked PUT: got %d want 409", code)
	}
}

func TestProxySettings_RoundTrip(t *testing.T) {
	for raw, want := range map[string]string{
		"":                             "private",
		"none":                         "none",
		"private,cloudflare":           "private,cloudflare",
		"cloudflare, 10.1.2.0/24":      "cloudflare,10.1.2.0/24",
		"private,cloudflare,192.0.2.1": "private,cloudflare,192.0.2.1",
	} {
		if got := parseProxySettings(raw).raw(); got != want {
			t.Errorf("%q: got %q want %q", raw, got, want)
		}
	}
}
