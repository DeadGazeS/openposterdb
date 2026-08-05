package image

import (
	"bytes"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"openposterdb/internal/services"
)

// tmdbStub is a RoundTripper that answers the TMDB API + image CDN calls the
// serving pipeline makes, so ServeImage can run without a real provider.
type tmdbStub struct {
	poster []byte
}

func (s *tmdbStub) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if strings.Contains(host, "image.tmdb.org") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       ioNopCloser(bytes.NewReader(s.poster)),
		}, nil
	}
	path := req.URL.Path
	switch {
	case strings.Contains(path, "/find/"):
		body := `{"movie_results":[{"id":278,"poster_path":"/abc.jpg","release_date":null,"popularity":100.0}],"tv_results":[]}`
		return jsonResponse(body), nil
	case strings.Contains(path, "/movie/278/images"):
		body := `{"posters":[{"file_path":"/abc.jpg","aspect_ratio":0.6667,"iso_639_1":null,"vote_average":1.0,"width":800,"height":1200}]}`
		return jsonResponse(body), nil
	}
	return jsonResponse(`{"status_code":34,"status_message":"stub miss: ` + path + `"}`), nil
}

func ioNopCloser(r *bytes.Reader) interface {
	Read([]byte) (int, error)
	Close() error
} {
	return nopCloser{r}
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       nopCloser{bytes.NewReader([]byte(body))},
	}
}

func newServeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// :memory: databases are per-connection; pin one connection so the table
	// created here is visible to the cross-ID goroutine's writes too.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE image_meta (
		cache_key TEXT PRIMARY KEY,
		release_date TEXT,
		image_type TEXT NOT NULL DEFAULT 'p',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`); err != nil {
		t.Fatalf("image_meta: %v", err)
	}
	return db
}

func serveParams(t *testing.T, db *sql.DB, cacheDir string) ServeParams {
	t.Helper()
	client := &http.Client{Transport: &tmdbStub{poster: SamplePosterPNG}}
	return ServeParams{
		DB:       db,
		TMDB:     services.NewTmdbClient("test-key", client),
		IDType:   "imdb",
		IDValue:  "tt0111161",
		Kind:     "poster",
		Settings: &services.RenderSettings{ImageSource: "t", Lang: "en"},
		CacheDir: cacheDir,
		Quality:  85,
		Caches:   &services.MemCacheSet{ImageMem: services.NewMemCache(10<<20, 100, 0, 0)},
		Inflight: NewInflightSet(),
	}
}

func TestServeImagePipelineCoalescesAndWritesCrossID(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()

	// All workers share one inflight set (real coalescing) but need their own
	// settings struct: ServeImage resolves badge defaults in place.
	inflight := NewInflightSet()
	newParams := func() ServeParams {
		p := serveParams(t, db, cacheDir)
		p.Inflight = inflight
		p.Settings = &services.RenderSettings{ImageSource: "t", Lang: "en"}
		return p
	}

	// 4 concurrent requests for the same key must all succeed with identical
	// bytes (the inflight set dedups the render to a single run).
	const workers = 4
	results := make([][]byte, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], _, errs[idx] = ServeImage(newParams())
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if len(results[i]) == 0 {
			t.Fatalf("worker %d: empty render", i)
		}
		if !bytes.Equal(results[i], results[0]) {
			t.Fatalf("worker %d produced different bytes than worker 0", i)
		}
	}
	if inflight.Len() != 0 {
		t.Fatalf("inflight set not drained, Len=%d", inflight.Len())
	}

	// Primary cache file must exist on disk (written exactly once).
	primary, err := filepath.Glob(filepath.Join(cacheDir, "posters", "imdb", "*"))
	if err != nil || len(primary) != 1 {
		t.Fatalf("expected 1 primary cache file, got %v (err %v)", primary, err)
	}
	want, _ := os.ReadFile(primary[0])

	// Cross-ID write must eventually appear under the tmdb alternate form
	// with the same bytes. Polling also absorbs the fire-and-forget write
	// latency (the file exists empty between Create and Write).
	var alt []string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		alt, _ = filepath.Glob(filepath.Join(cacheDir, "posters", "tmdb", "*"))
		if len(alt) == 1 {
			if got, err := os.ReadFile(alt[0]); err == nil && bytes.Equal(want, got) {
				break
			}
		}
		alt = nil
		time.Sleep(20 * time.Millisecond)
	}
	if len(alt) != 1 {
		t.Fatalf("expected a byte-equal cross-ID cache file under posters/tmdb, got %v", alt)
	}

	// Both cache keys must have image_meta rows. The cross-ID goroutine
	// finishes with the meta upsert, so waiting for 2 rows also guarantees it
	// is done before the TempDir is cleaned up.
	var n int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err := db.QueryRow(`SELECT COUNT(*) FROM image_meta`).Scan(&n); err == nil && n == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n != 2 {
		t.Fatalf("expected 2 image_meta rows (primary + cross-ID), got %d", n)
	}
}

func TestServeImageMemHitRevalidatesStaleness(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()

	newParams := func() ServeParams {
		p := serveParams(t, db, cacheDir)
		p.Inflight = NewInflightSet()
		// 1h TTL (never expires during the test), but revalidation due after
		// 50ms so the stale check runs on a hot mem hit.
		p.Caches.ImageMem = services.NewMemCache(10<<20, 100, time.Hour, 0)
		p.Caches.ImageMem.SetRevalidateAfter(50 * time.Millisecond)
		p.RatingsMinStaleSecs = 1
		p.Settings = &services.RenderSettings{ImageSource: "t", Lang: "en"}
		return p
	}

	// First request renders and populates the mem + fs caches.
	if _, _, err := ServeImage(newParams()); err != nil {
		t.Fatalf("initial render: %v", err)
	}
	primary, _ := filepath.Glob(filepath.Join(cacheDir, "posters", "imdb", "*"))
	if len(primary) != 1 {
		t.Fatalf("expected 1 cache file, got %v", primary)
	}

	// Backdate the fs entry so the release-date staleness says "stale".
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(primary[0], old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// Wait past the revalidation window, then serve again: the mem cache must
	// HIT (serving the mem bytes) AND spawn the background refresh.
	time.Sleep(100 * time.Millisecond)
	second, _, err := ServeImage(newParams())
	if err != nil {
		t.Fatalf("mem-hit request: %v", err)
	}
	if len(second) == 0 {
		t.Fatal("mem hit must serve bytes")
	}

	// The background refresh must update the fs entry's mtime (fresh render).
	var fresh bool
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		fi, err := os.Stat(primary[0])
		if err == nil && fi.ModTime().After(time.Now().Add(-5*time.Second)) {
			fresh = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !fresh {
		t.Fatal("background refresh from a stale mem hit did not update the entry")
	}

	// The refresh's cross-ID goroutine must finish before TempDir cleanup.
	var n int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err := db.QueryRow(`SELECT COUNT(*) FROM image_meta`).Scan(&n); err == nil && n == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n != 2 {
		t.Fatalf("expected 2 image_meta rows after refresh, got %d", n)
	}
}

func TestServeImageStaleServesAndRefreshes(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()

	// Each ServeImage call needs its own settings struct: ServeImage resolves
	// badge defaults in place, and a shared pointer would shift the cache key
	// between calls (pre-existing mutation semantics; production requests get
	// a fresh struct per request anyway).
	newParams := func() ServeParams {
		p := serveParams(t, db, cacheDir)
		p.Caches.ImageMem = services.NewMemCache(10<<20, 100, 100*time.Millisecond, 0)
		p.RatingsMinStaleSecs = 1
		p.Settings = &services.RenderSettings{ImageSource: "t", Lang: "en"}
		return p
	}

	// First render populates the cache.
	first, _, err := ServeImage(newParams())
	if err != nil {
		t.Fatalf("initial render: %v", err)
	}

	primary, _ := filepath.Glob(filepath.Join(cacheDir, "posters", "imdb", "*"))
	if len(primary) != 1 {
		t.Fatalf("expected 1 cache file, got %v", primary)
	}
	// Backdate the file so it is stale by mtime, and let the mem entry lapse.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(primary[0], old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// A stale request serves the stale bytes immediately (not blocking) and
	// triggers a background refresh.
	second, _, err := ServeImage(newParams())
	if err != nil {
		t.Fatalf("stale request: %v", err)
	}
	if !bytes.Equal(second, first) {
		t.Fatal("stale request must serve the cached bytes, not a fresh render")
	}

	// Wait for the background refresh to re-render (same key, mtime updated).
	var fresh bool
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		fi, err := os.Stat(primary[0])
		if err == nil && fi.ModTime().After(time.Now().Add(-5*time.Second)) {
			fresh = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !fresh {
		t.Fatal("background refresh did not update the stale entry")
	}

	// The refresh spawns a cross-ID goroutine; wait for its meta upsert so
	// the TempDir cleanup doesn't race it (see coalescing test).
	var n int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err := db.QueryRow(`SELECT COUNT(*) FROM image_meta`).Scan(&n); err == nil && n == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n != 2 {
		t.Fatalf("expected 2 image_meta rows after refresh, got %d", n)
	}
}
