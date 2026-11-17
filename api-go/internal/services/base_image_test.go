package services

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cdnStub plays the TMDB image CDN: it records the conditional headers and
// answers with status/body/etag.
type cdnStub struct {
	mu          sync.Mutex
	calls       atomic.Int32
	status      int
	body        []byte
	etag        string
	delay       time.Duration
	ifNoneMatch string
	ifModified  string
	userAgent   string
}

func (s *cdnStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.ifNoneMatch = req.Header.Get("If-None-Match")
	s.ifModified = req.Header.Get("If-Modified-Since")
	s.userAgent = req.Header.Get("User-Agent")
	status, body, etag, delay := s.status, s.body, s.etag, s.delay
	s.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	h := http.Header{}
	if etag != "" {
		h.Set("ETag", etag)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func ageFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestFetchBaseImage_DownloadsAndStoresETag(t *testing.T) {
	dir := t.TempDir()
	cdn := &cdnStub{status: 200, body: []byte("v1"), etag: `"e1"`}
	c := NewTmdbClient("k", &http.Client{Transport: cdn})
	b, err := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false)
	if err != nil || string(b) != "v1" {
		t.Fatalf("got %q %v", b, err)
	}
	if cdn.userAgent == "" {
		t.Error("no User-Agent sent")
	}
	etag, _ := os.ReadFile(filepath.Join(dir, "base", "posters", "w780", "p.jpg.etag"))
	if string(etag) != `"e1"` {
		t.Errorf("stored etag %q", etag)
	}
	// Fresh copy: no request.
	if _, err := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false); err != nil || cdn.calls.Load() != 1 {
		t.Fatalf("fresh copy re-fetched: calls=%d err=%v", cdn.calls.Load(), err)
	}
}

func TestFetchBaseImage_StaleRevalidates304(t *testing.T) {
	dir := t.TempDir()
	cdn := &cdnStub{status: 200, body: []byte("v1"), etag: `"e1"`}
	c := NewTmdbClient("k", &http.Client{Transport: cdn})
	if _, err := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "base", "posters", "w780", "p.jpg")
	ageFile(t, path, time.Hour)

	cdn.mu.Lock()
	cdn.status, cdn.body = http.StatusNotModified, nil
	cdn.mu.Unlock()
	b, err := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false)
	if err != nil || string(b) != "v1" {
		t.Fatalf("got %q %v", b, err)
	}
	if cdn.ifNoneMatch != `"e1"` || cdn.ifModified == "" {
		t.Errorf("conditional headers: If-None-Match %q If-Modified-Since %q", cdn.ifNoneMatch, cdn.ifModified)
	}
	if info, _ := os.Stat(path); time.Since(info.ModTime()) > time.Minute {
		t.Error("304 did not refresh the mtime")
	}
}

func TestFetchBaseImage_StaleReplacedOn200(t *testing.T) {
	dir := t.TempDir()
	cdn := &cdnStub{status: 200, body: []byte("v1")}
	c := NewTmdbClient("k", &http.Client{Transport: cdn})
	c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false)
	ageFile(t, filepath.Join(dir, "base", "posters", "w780", "p.jpg"), time.Hour)
	cdn.mu.Lock()
	cdn.body = []byte("v2")
	cdn.mu.Unlock()
	if b, _ := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false); string(b) != "v2" {
		t.Fatalf("got %q, want v2", b)
	}
}

// A failing revalidation serves the stale copy instead of an error.
func TestFetchBaseImage_StaleServedWhenCDNFails(t *testing.T) {
	dir := t.TempDir()
	cdn := &cdnStub{status: 200, body: []byte("v1")}
	c := NewTmdbClient("k", &http.Client{Transport: cdn})
	c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false)
	ageFile(t, filepath.Join(dir, "base", "posters", "w780", "p.jpg"), time.Hour)
	cdn.mu.Lock()
	cdn.status = http.StatusNotFound
	cdn.mu.Unlock()
	b, err := c.FetchBaseImageCtx(t.Context(), dir, "/p.jpg", "w780", 60, false)
	if err != nil || string(b) != "v1" {
		t.Fatalf("got %q %v, want the stale v1", b, err)
	}
}

// Renders with different settings share the same base file: concurrent
// misses make one download.
func TestFetchBaseImage_ConcurrentMissesShareOneDownload(t *testing.T) {
	dir := t.TempDir()
	cdn := &cdnStub{status: 200, body: []byte("v1"), delay: 50 * time.Millisecond}
	c := NewTmdbClient("k", &http.Client{Transport: cdn})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b, err := c.FetchBaseImageCtx(t.Context(), dir, "/shared.jpg", "w780", 60, false); err != nil || string(b) != "v1" {
				t.Errorf("got %q %v", b, err)
			}
		}()
	}
	wg.Wait()
	if n := cdn.calls.Load(); n != 1 {
		t.Errorf("CDN calls = %d, want 1", n)
	}
}
