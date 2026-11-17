package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperr "openposterdb/internal/errors"
)

// /c/ URLs change with the settings, so their lifetime follows the title's
// age (Rust compute_cdn_max_age).
func TestCDNCacheControl_FollowsReleaseDate(t *testing.T) {
	cfg := &ImageServeConfig{RatingsMinStaleSecs: 86400, RatingsMaxAgeSecs: 31536000}
	old := "1994-09-23"
	recent := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	cases := []struct {
		name    string
		release *string
		want    string
	}{
		{"old title: a year", &old, "max-age=31536000, stale-while-revalidate=220752000"},
		{"unknown release: the minimum", nil, "max-age=86400, stale-while-revalidate=604800"},
	}
	for _, c := range cases {
		if got := cdnCacheControl(c.release, cfg); got != "public, "+c.want {
			t.Errorf("%s: Cache-Control %q", c.name, got)
		}
	}
	// A day-old release: between the minimum and a few days (it grows with
	// the title's age).
	var age int
	fmt.Sscanf(cdnCacheControl(&recent, cfg), "public, max-age=%d", &age)
	if age < 86400 || age > 3*86400 {
		t.Errorf("recent release: max-age %d, want 1–3 days", age)
	}
}

// The direct URL stays the same when the settings change, so it keeps the
// short Rust lifetime whatever the release date.
func TestDirectImageCacheControl_StaysShort(t *testing.T) {
	if directImageCacheControl != "public, max-age=3600, stale-while-revalidate=86400" {
		t.Errorf("direct route Cache-Control %q", directImageCacheControl)
	}
}

func TestWriteImage_ETagAnd304(t *testing.T) {
	body := []byte("poster-bytes")
	rec := httptest.NewRecorder()
	writeImage(rec, httptest.NewRequest("GET", "/x", nil), body, "image/jpeg", directImageCacheControl)
	etag := rec.Header().Get("ETag")
	if rec.Code != 200 || etag == "" || rec.Body.String() != "poster-bytes" {
		t.Fatalf("first response: %d etag %q body %q", rec.Code, etag, rec.Body.String())
	}

	for _, inm := range []string{etag, "W/" + etag, `"other", ` + etag, "*"} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("If-None-Match", inm)
		rec := httptest.NewRecorder()
		writeImage(rec, req, body, "image/jpeg", directImageCacheControl)
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
			t.Errorf("If-None-Match %q: %d, body %d bytes", inm, rec.Code, rec.Body.Len())
		}
	}

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("If-None-Match", `"stale"`)
	rec = httptest.NewRecorder()
	writeImage(rec, req, body, "image/jpeg", directImageCacheControl)
	if rec.Code != 200 {
		t.Errorf("non-matching ETag: %d", rec.Code)
	}
}

// A declined render answers 503 with Retry-After and a readable message.
func TestWriteServeError_BusySetsRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServeError(rec, apperr.NewBusy("server busy rendering images, retry shortly", 5))
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "5" {
		t.Fatalf("got %d Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), "busy") {
		t.Errorf("body %q", rec.Body.String())
	}
}
