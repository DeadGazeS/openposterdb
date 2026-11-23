package router

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A failed read of the free-key flag is our failure (500), not "disabled"
// (403 on images, 401/403 on isValid).
func TestFreeKeyFlag_DBError_500(t *testing.T) {
	r, s := phase5Router(t, nil)
	s.DB.Close()
	for _, p := range []string{"/t0-free-rpdb/imdb/poster-default/tt0111161.jpg", "/t0-free-rpdb/isValid"} {
		if rec := serve(r, "GET", p, "", nil); rec.Code != 500 {
			t.Errorf("%s: got %d %s, want 500", p, rec.Code, rec.Body.String())
		}
	}
}

// Re-saving the same TVDB key keeps the client (and its token) but forgets a
// remembered login failure, so the next request logs in again.
func TestSetTVDB_SameKeyResetsLoginFailure(t *testing.T) {
	var logins atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			logins.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	s := &AppState{HTTPClient: srv.Client()}
	s.SetupTVDB("key")
	c := s.TVDB
	c.BaseURL = srv.URL
	for range 2 {
		if _, err := c.SeriesCtx(t.Context(), 1); err == nil {
			t.Fatal("want a login error")
		}
	}
	if n := logins.Load(); n != 1 {
		t.Fatalf("logins = %d, want 1 (failure remembered)", n)
	}
	s.SetupTVDB("key")
	if s.TVDB != c {
		t.Fatal("same key: the client should be kept")
	}
	c.SeriesCtx(t.Context(), 1)
	if n := logins.Load(); n != 2 {
		t.Errorf("logins after re-saving the key = %d, want 2", n)
	}
}
