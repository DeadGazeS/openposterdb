package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:5555"
	if got := clientIP(r); got != "1.2.3.4" {
		t.Errorf("RemoteAddr only: got %q want 1.2.3.4", got)
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:5555"
	r.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
	if got := clientIP(r); got != "9.9.9.9" {
		t.Errorf("XFF first hop: got %q want 9.9.9.9", got)
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:5555"
	r.Header.Set("X-Forwarded-For", "  9.9.9.9  ")
	if got := clientIP(r); got != "9.9.9.9" {
		t.Errorf("XFF with whitespace: got %q want 9.9.9.9", got)
	}
}
