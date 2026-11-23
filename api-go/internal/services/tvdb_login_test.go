package services

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"openposterdb/internal/httpx"
)

// A login that cannot reach TVDB keeps the network error as its cause, so
// the API answers 502 (unreachable) or 504 (timeout), not 500.
func TestTVDB_LoginNetworkErrorKeepsCause(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused from now on
	c := NewTVDBClient("key", nil)
	c.BaseURL = url
	_, err := c.SeriesCtx(t.Context(), 1)
	if err == nil {
		t.Fatal("want an error")
	}
	rec := httptest.NewRecorder()
	httpx.WriteAppError(rec, err)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("unreachable login: status = %d, want 502 (err %v)", rec.Code, err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()
	c = NewTVDBClient("key", &http.Client{Timeout: 20 * time.Millisecond})
	c.BaseURL = slow.URL
	_, err = c.SeriesCtx(t.Context(), 1)
	rec = httptest.NewRecorder()
	httpx.WriteAppError(rec, err)
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("timed-out login: status = %d, want 504 (err %v)", rec.Code, err)
	}
}

// A failed login is remembered for a while: later requests get the error
// without a new POST /login, and log in again once the window ends.
func TestTVDB_FailedLoginRemembered(t *testing.T) {
	c, fake := newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(seriesExtendedJSON))
	})
	c.apiKey = "wrong"
	_, err1 := c.SeriesCtx(t.Context(), 1)
	_, err2 := c.SeriesCtx(t.Context(), 2)
	if err1 == nil || err2 == nil || isIDNotFound(err2) {
		t.Fatalf("errs = %v / %v, want non-404 errors", err1, err2)
	}
	if n := fake.logins.Load(); n != 1 {
		t.Errorf("logins = %d, want 1 (failure remembered)", n)
	}
	// A rejected key (401) waits longer than a transient failure.
	c.mu.Lock()
	wait := time.Until(c.loginErrUntil)
	c.loginErrUntil = time.Now().Add(-time.Second) // window over
	c.mu.Unlock()
	if wait <= tvdbLoginRetryAfter {
		t.Errorf("401 backoff = %v, want more than %v", wait, tvdbLoginRetryAfter)
	}
	c.apiKey = "key"
	if _, err := c.SeriesCtx(t.Context(), 3); err != nil {
		t.Fatalf("after the window: %v", err)
	}
	if n := fake.logins.Load(); n != 2 {
		t.Errorf("logins after the window = %d, want 2", n)
	}
}

// A transient login failure (5xx) is remembered for the short window.
func TestTVDB_FailedLoginTransientWindow(t *testing.T) {
	var logins atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := NewTVDBClient("key", srv.Client())
	c.BaseURL = srv.URL
	c.backoff = func(int) time.Duration { return time.Millisecond }
	for i := range 3 {
		if _, err := c.SeriesCtx(t.Context(), uint64(i+1)); err == nil {
			t.Fatal("want an error")
		}
	}
	if n := logins.Load(); n != 1 {
		t.Errorf("logins = %d, want 1", n)
	}
	c.mu.Lock()
	wait := time.Until(c.loginErrUntil)
	c.mu.Unlock()
	if wait <= 0 || wait > tvdbLoginRetryAfter {
		t.Errorf("5xx backoff = %v, want (0, %v]", wait, tvdbLoginRetryAfter)
	}
}
