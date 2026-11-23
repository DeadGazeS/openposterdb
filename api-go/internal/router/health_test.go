package router

import (
	"net/http"
	"testing"
)

func TestHealthAndReady(t *testing.T) {
	r, s := phase5Router(t, nil)

	if rec := serve(r, http.MethodGet, "/api/health", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/health = %d, want 200", rec.Code)
	}
	if rec := serve(r, http.MethodGet, "/api/ready", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/ready = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	s.Draining.Store(true)
	if rec := serve(r, http.MethodGet, "/api/ready", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/api/ready while draining = %d, want 503", rec.Code)
	}
	if rec := serve(r, http.MethodGet, "/api/health", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/health while draining = %d, want 200 (the process still serves)", rec.Code)
	}
	s.Draining.Store(false)

	s.DB.Close()
	if rec := serve(r, http.MethodGet, "/api/ready", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/api/ready without a database = %d, want 503", rec.Code)
	}
	if rec := serve(r, http.MethodGet, "/api/health", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/health without a database = %d, want 200 (no dependencies)", rec.Code)
	}
}
