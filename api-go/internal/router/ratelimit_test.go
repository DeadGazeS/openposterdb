package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"openposterdb/internal/config"
	"testing"
	"time"
)

func TestRateLimit_AllowsUnderQuota(t *testing.T) {
	rl := newRateLimiter(60) // 1 per second; burst 60
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("req %d: got status %d want 200", i, rec.Code)
		}
	}
}

func TestRateLimit_RejectsOverBurst(t *testing.T) {
	rl := newRateLimiter(60)
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Burst is 60; the 61st request from the same IP must be denied.
	var last int
	for i := 0; i < 60; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.2:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusOK {
		t.Fatalf("60th request should still pass: got %d", last)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("61st request: got %d want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Error("Retry-After header required on 429")
	}
}

func TestRateLimit_PerIPIsolation(t *testing.T) {
	rl := newRateLimiter(2) // tiny budget
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, ip := range []string{"10.0.0.10:1", "10.0.0.11:1", "10.0.0.12:1"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("ip %s: got %d want 200", ip, rec.Code)
		}
	}
	// Burn the budget for 10.0.0.10.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.10:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.10:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("exhausted IP should be denied: got %d", rec.Code)
	}
}

func TestRateLimit_DisabledOnZero(t *testing.T) {
	if RateLimit(newRateLimiter(0)) == nil {
		t.Fatal("RateLimit(0) must still return a non-nil middleware")
	}
}

func TestClientIP(t *testing.T) {
	trusted := config.DefaultTrustedProxies()
	ip := func(remote string, xff ...string) string {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for _, v := range xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		return clientIPFor(r, trusted)
	}
	// Default: only loopback/private peers may set X-Forwarded-For.
	trusted = config.DefaultTrustedProxies()
	if got := ip("1.2.3.4:5555"); got != "1.2.3.4" {
		t.Errorf("no XFF: got %q", got)
	}
	if got := ip("1.2.3.4:5555", "9.9.9.9"); got != "1.2.3.4" {
		t.Errorf("a public peer must not choose its identity via XFF: got %q", got)
	}
	if got := ip("172.18.0.2:5555", "9.9.9.9"); got != "9.9.9.9" {
		t.Errorf("trusted proxy: got %q want 9.9.9.9", got)
	}
	// Behind a proxy, the client-supplied left part is ignored: the rightmost
	// untrusted hop is the client.
	if got := ip("127.0.0.1:5555", "6.6.6.6, 9.9.9.9"); got != "9.9.9.9" {
		t.Errorf("spoofed chain: got %q want 9.9.9.9", got)
	}
	if got := ip("10.0.0.5:5555", "9.9.9.9, 10.0.0.7", "  "); got != "9.9.9.9" {
		t.Errorf("trusted hops skipped: got %q want 9.9.9.9", got)
	}
	if got := ip("10.0.0.5:5555", "not-an-ip"); got != "10.0.0.5" {
		t.Errorf("malformed XFF falls back to the peer: got %q", got)
	}

	// TRUSTED_PROXIES=none: always the connection address.
	none, _ := config.ParseTrustedProxies("none")
	trusted = none
	if got := ip("172.18.0.2:5555", "9.9.9.9"); got != "172.18.0.2" {
		t.Errorf("none: got %q", got)
	}
	// Custom list (e.g. Cloudflare ranges in front of Caddy).
	custom, err := config.ParseTrustedProxies("private, 173.245.48.0/20")
	if err != nil {
		t.Fatal(err)
	}
	trusted = custom
	if got := ip("10.0.0.5:5555", "9.9.9.9, 173.245.48.10"); got != "9.9.9.9" {
		t.Errorf("custom: got %q want 9.9.9.9", got)
	}
	// "private,cloudflare": visitor → Cloudflare → nginx on the same host,
	// with nginx appending the Cloudflare edge ($proxy_add_x_forwarded_for).
	cf, err := config.ParseTrustedProxies("private,cloudflare")
	if err != nil {
		t.Fatal(err)
	}
	trusted = cf
	if got := ip("172.18.0.3:5555", "6.6.6.6, 9.9.9.9, 162.158.10.20"); got != "9.9.9.9" {
		t.Errorf("cloudflare keyword: got %q want the visitor 9.9.9.9", got)
	}
	if got := ip("172.18.0.3:5555", "9.9.9.9, 2606:4700::1"); got != "9.9.9.9" {
		t.Errorf("cloudflare keyword (IPv6 edge): got %q", got)
	}
	trusted = config.DefaultTrustedProxies()

	if _, err := config.ParseTrustedProxies("10.0.0.0/33"); err == nil {
		t.Error("invalid CIDR must be rejected")
	}
}

// New IPs beyond the cap share one bucket instead of growing the map.
func TestRateLimiter_EntryCap(t *testing.T) {
	rl := newRateLimiter(60)
	rl.count.Store(maxLimiterEntries)
	if rl.limiter("203.0.113.9") != rl.overflow {
		t.Fatal("past the cap a new IP must get the shared overflow bucket")
	}
}

// A 429 is a JSON error like every other API error, with Retry-After.
func TestRateLimit_429IsJSON(t *testing.T) {
	rl := newRateLimiter(1)
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	var rec *httptest.ResponseRecorder
	for range 2 {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.50:1"
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Errorf("body %q is not a JSON error", rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
}

// One IP sending random keys can't fill the shared table: past
// maxKeyBucketsPerIP its further keys share that IP's own bucket, so other
// IPs still get buckets of their own instead of the overflow one.
func TestRateLimiter_PerIPKeyCap(t *testing.T) {
	rl := newRateLimiterBurst(60, 1)
	rl.subKey = func(r *http.Request) string { return r.URL.Query().Get("k") }
	rl.count.Store(maxLimiterEntries - 50)
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	do := func(ip, key string) int {
		req := httptest.NewRequest("GET", "/?k="+key, nil)
		req.RemoteAddr = ip + ":1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := range 1000 {
		do("198.51.100.7", fmt.Sprintf("random-%d", i))
	}
	if got := rl.count.Load() - (maxLimiterEntries - 50); got > maxKeyBucketsPerIP+1 {
		t.Errorf("one IP created %d buckets, want at most %d", got, maxKeyBucketsPerIP+1)
	}
	if code := do("203.0.113.20", "victim"); code != http.StatusOK {
		t.Errorf("another IP's first request: %d, want 200 (pushed into the overflow bucket)", code)
	}
	// Keys seen before the cap keep their own buckets.
	if code := do("198.51.100.8", "a"); code != http.StatusOK {
		t.Fatalf("fresh key: %d", code)
	}
	if code := do("198.51.100.8", "b"); code != http.StatusOK {
		t.Errorf("second key behind the same IP shares the first one's bucket: %d", code)
	}
	// Idle sweeping releases the per-IP slots.
	rl.idleTimeout = -time.Second
	rl.SweepIdle()
	rl.mu.Lock()
	left := len(rl.keysPerIP)
	rl.mu.Unlock()
	if left != 0 {
		t.Errorf("%d IPs still counted after sweeping everything", left)
	}
}
