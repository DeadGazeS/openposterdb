package image

import (
	"bytes"
	"context"
	stderrors "errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/services"
)

// --- concurrency limits ---

func TestSlotLimiter_SaturatedReturns503WithRetryAfter(t *testing.T) {
	l := newSlotLimiter(1, 1, 50*time.Millisecond)
	release, err := l.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Queue has room for one waiter: it times out after maxWait.
	start := time.Now()
	_, err = l.acquire(context.Background())
	var ae *apperr.AppError
	if !stderrors.As(err, &ae) || ae.Status != http.StatusServiceUnavailable || ae.RetryAfter <= 0 {
		t.Fatalf("timed-out wait: err %v, want 503 with Retry-After", err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Error("returned before maxWait")
	}

	// A full queue is declined at once.
	l.waiting.Store(1)
	start = time.Now()
	if _, err := l.acquire(context.Background()); !stderrors.As(err, &ae) || ae.Status != 503 {
		t.Fatalf("full queue: err %v, want 503", err)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Error("full queue should not wait")
	}
	l.waiting.Store(0)

	// A waiter whose context ends leaves the queue.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.acquire(ctx); !stderrors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: err %v", err)
	}
}

func TestSlotLimiter_WaiterGetsReleasedSlot(t *testing.T) {
	l := newSlotLimiter(1, 4, time.Second)
	release, _ := l.acquire(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); release() }()
	r2, err := l.acquire(context.Background())
	if err != nil {
		t.Fatalf("waiter: %v", err)
	}
	r2()
	if _, ok := l.tryAcquire(); !ok {
		t.Fatal("slot not returned")
	}
}

func TestRefreshBackoff(t *testing.T) {
	b := &refreshBackoff{m: map[string]backoffEntry{}}
	now := time.Now()
	if b.blocked("k", now) {
		t.Fatal("blocked before any failure")
	}
	b.failed("k", now)
	if !b.blocked("k", now.Add(refreshBackoffMin-time.Second)) || b.blocked("k", now.Add(refreshBackoffMin+time.Second)) {
		t.Fatal("first failure must block for refreshBackoffMin")
	}
	b.failed("k", now)
	if !b.blocked("k", now.Add(2*refreshBackoffMin-time.Second)) {
		t.Fatal("second failure must double the delay")
	}
	b.succeeded("k")
	if b.blocked("k", now) {
		t.Fatal("success must clear the back-off")
	}
}

// --- artwork fallback chain ---

// chainStub plays TMDB (API + CDN) and fanart.tv. imagesStatus is the status
// of TMDB /images; fanartCalls counts fanart.tv API calls.
type chainStub struct {
	imagesStatus int
	fanartCalls  atomic.Int32
}

func (s *chainStub) RoundTrip(req *http.Request) (*http.Response, error) {
	host, path := req.URL.Host, req.URL.Path
	switch {
	case strings.Contains(host, "image.tmdb.org"):
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: nopCloser{bytes.NewReader([]byte("tmdb:" + path))}}, nil
	case strings.Contains(host, "webservice.fanart.tv"):
		s.fanartCalls.Add(1)
		return jsonResponse(`{"movieposter":[{"id":"f1","url":"https://assets.fanart.tv/p.jpg","lang":"en","likes":"1"}],
			"tvposter":[{"id":"t1","url":"https://assets.fanart.tv/t.jpg","lang":"en","likes":"1"}]}`), nil
	case strings.Contains(host, "assets.fanart.tv"):
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: nopCloser{bytes.NewReader([]byte("fanart:" + path))}}, nil
	case strings.Contains(path, "/images"):
		if s.imagesStatus != 200 {
			return &http.Response{StatusCode: s.imagesStatus, Header: http.Header{}, Body: nopCloser{bytes.NewReader([]byte(`{}`))}}, nil
		}
		return jsonResponse(`{"posters":[{"file_path":"/de.jpg","iso_639_1":"de","aspect_ratio":0.667}]}`), nil
	}
	return jsonResponse(`{}`), nil
}

func chainParams(t *testing.T, stub *chainStub, withFanart bool) ServeParams {
	t.Helper()
	// The failing /images call would otherwise sit through TMDB's retry
	// back-off (seconds per case).
	saved := services.TMDBAPIRetry
	services.TMDBAPIRetry.MaxRetries = 0
	t.Cleanup(func() { services.TMDBAPIRetry = saved })
	client := &http.Client{Transport: stub}
	p := ServeParams{
		Context:  context.Background(),
		TMDB:     services.NewTmdbClient("k", client),
		Kind:     "poster",
		Settings: &services.RenderSettings{ImageSource: "t", Lang: "de"},
		CacheDir: t.TempDir(),
	}
	if withFanart {
		p.Fanart = services.NewFanartClient([]string{"k"}, client)
	}
	return p
}

func moviePoster() *services.ResolvedID {
	poster := "/default.jpg"
	return &services.ResolvedID{TMDbID: 278, MediaType: services.MediaTypeMovie, PosterPath: &poster}
}

func TestFetchBaseArtwork_Chain(t *testing.T) {
	t.Run("TMDB in the requested language first", func(t *testing.T) {
		p := chainParams(t, &chainStub{imagesStatus: 200}, true)
		b, err := p.fetchBaseArtwork(moviePoster(), services.ImageSizeMedium)
		if err != nil || string(b) != "tmdb:/t/p/w780/de.jpg" {
			t.Fatalf("got %q %v", b, err)
		}
	})
	t.Run("failed image list falls through to Fanart", func(t *testing.T) {
		p := chainParams(t, &chainStub{imagesStatus: 500}, true)
		b, err := p.fetchBaseArtwork(moviePoster(), services.ImageSizeMedium)
		if err != nil || string(b) != "fanart:/p.jpg" {
			t.Fatalf("got %q %v", b, err)
		}
	})
	t.Run("then the default poster", func(t *testing.T) {
		p := chainParams(t, &chainStub{imagesStatus: 500}, false)
		b, err := p.fetchBaseArtwork(moviePoster(), services.ImageSizeMedium)
		if err != nil || string(b) != "tmdb:/t/p/w780/default.jpg" {
			t.Fatalf("got %q %v", b, err)
		}
	})
	t.Run("Fanart first when it is the image source", func(t *testing.T) {
		p := chainParams(t, &chainStub{imagesStatus: 200}, true)
		p.Settings.ImageSource = "f"
		b, err := p.fetchBaseArtwork(moviePoster(), services.ImageSizeMedium)
		if err != nil || string(b) != "fanart:/p.jpg" {
			t.Fatalf("got %q %v", b, err)
		}
	})
	t.Run("backdrop falls back to backdrop_path, not the poster", func(t *testing.T) {
		p := chainParams(t, &chainStub{imagesStatus: 500}, false)
		p.Kind = "backdrop"
		r := moviePoster()
		bd := "/backdrop.jpg"
		r.BackdropPath = &bd
		b, err := p.fetchBaseArtwork(r, services.ImageSizeMedium)
		if err != nil || string(b) != "tmdb:/t/p/w780/backdrop.jpg" {
			t.Fatalf("got %q %v", b, err)
		}
	})
}

// A series without a TVDB id never reaches fanart.tv (a TMDB id would look
// up some other show there).
func TestFetchFanart_SeriesWithoutTVDBSkipped(t *testing.T) {
	stub := &chainStub{imagesStatus: 500}
	p := chainParams(t, stub, true)
	poster := "/default.jpg"
	series := &services.ResolvedID{TMDbID: 555001, MediaType: services.MediaTypeTV, PosterPath: &poster}
	b, err := p.fetchBaseArtwork(series, services.ImageSizeMedium)
	if err != nil || string(b) != "tmdb:/t/p/w780/default.jpg" {
		t.Fatalf("got %q %v", b, err)
	}
	if n := stub.fanartCalls.Load(); n != 0 {
		t.Fatalf("fanart.tv called %d times for a series without TVDB id", n)
	}
	tvdb := uint64(81189)
	series.TVDBID = &tvdb
	if b, _ := p.fetchBaseArtwork(series, services.ImageSizeMedium); string(b) != "fanart:/t.jpg" {
		t.Fatalf("with TVDB id: got %q", b)
	}
}

// --- cold-cache fast path ---

// failingTransport fails every upstream call.
type failingTransport struct{ calls atomic.Int32 }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return nil, stderrors.New("upstream unreachable")
}

// After a restart (empty in-memory caches) a cached poster is served without
// any upstream call: the key is rebuilt from available_ratings.
func TestServeImage_ColdCacheServesWithoutUpstream(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()
	settings := func() *services.RenderSettings {
		return &services.RenderSettings{ImageSource: "t", Lang: "en", PosterLayout: services.DefaultLayout("poster")}
	}

	p := serveParams(t, db, cacheDir)
	p.Settings = settings()
	p.Caches.IDs = services.NewMemCache(0, 100, 0, 0)
	first, _, err := ServeImage(p)
	if err != nil {
		t.Fatal(err)
	}
	var sources string
	if err := db.QueryRow(`SELECT sources FROM available_ratings WHERE id_key = 'imdb/tt0111161'`).Scan(&sources); err != nil {
		t.Fatalf("available_ratings not written: %v", err)
	}

	// "Restart": new in-memory caches, TMDB unreachable.
	ft := &failingTransport{}
	cold := serveParams(t, db, cacheDir)
	cold.Settings = settings()
	cold.TMDB = services.NewTmdbClient("test-key", &http.Client{Transport: ft})
	cold.Caches = &services.MemCacheSet{ImageMem: services.NewMemCache(10<<20, 100, 0, 0), IDs: services.NewMemCache(0, 100, 0, 0)}
	got, _, err := ServeImage(cold)
	if err != nil {
		t.Fatalf("cold request: %v", err)
	}
	if !bytes.Equal(got, first) {
		t.Fatal("cold request served different bytes")
	}
	// Let the background warm-up finish before the TempDir goes away.
	time.Sleep(200 * time.Millisecond)
}

// A title without an available_ratings row has no known rating sources: the
// cold path must not guess "no ratings" and serve that key's file.
func TestServeImage_ColdCacheSkippedWithoutAvailableRatingsRow(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()
	settings := func() *services.RenderSettings {
		return &services.RenderSettings{ImageSource: "t", Lang: "en", PosterLayout: services.DefaultLayout("poster")}
	}
	p := serveParams(t, db, cacheDir)
	p.Settings = settings()
	if _, _, err := ServeImage(p); err != nil {
		t.Fatal(err)
	}
	var sources string
	if err := db.QueryRow(`SELECT sources FROM available_ratings WHERE id_key = 'imdb/tt0111161'`).Scan(&sources); err != nil || sources != "" {
		t.Fatalf("want a no-ratings render, got sources %q (%v)", sources, err)
	}
	if _, err := db.Exec(`DELETE FROM available_ratings`); err != nil {
		t.Fatal(err)
	}

	ft := &failingTransport{}
	cold := serveParams(t, db, cacheDir)
	cold.Settings = settings()
	cold.TMDB = services.NewTmdbClient("test-key", &http.Client{Transport: ft})
	cold.Caches = &services.MemCacheSet{ImageMem: services.NewMemCache(10<<20, 100, 0, 0), IDs: services.NewMemCache(0, 100, 0, 0)}
	if _, _, err := ServeImage(cold); err == nil {
		t.Fatal("served a cached file without knowing the title's rating sources")
	}
}

// Cross-ID copies get their own available_ratings row, so a later cold
// request via the alternate id takes the fast path too.
func TestServeImage_CrossIDWritesAvailableRatingsRow(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()
	p := serveParams(t, db, cacheDir)
	p.IDValue = "tt0071562"
	p.Settings = &services.RenderSettings{ImageSource: "t", Lang: "en", PosterLayout: services.DefaultLayout("poster")}
	if _, _, err := ServeImage(p); err != nil {
		t.Fatal(err)
	}
	var n int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM available_ratings WHERE id_key = 'tmdb/movie-278'`).Scan(&n); err == nil && n == 1 {
			// Let the cross-ID goroutine finish before the TempDir goes away.
			time.Sleep(100 * time.Millisecond)
			return
		}
	}
	t.Fatal("no available_ratings row for the tmdb alternate")
}

// With ratings_limit=0 the cache key doesn't depend on the rating sources:
// the cold path serves it without an available_ratings row.
func TestServeImage_ColdCacheLimitZeroWithoutRow(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()
	zero := int32(0)
	settings := func() *services.RenderSettings {
		return &services.RenderSettings{ImageSource: "t", Lang: "en", PosterLayout: services.DefaultLayout("poster")}
	}
	p := serveParams(t, db, cacheDir)
	p.IDValue = "tt0050083"
	p.RatingsLimit = &zero
	p.Settings = settings()
	first, _, err := ServeImage(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM available_ratings`); err != nil {
		t.Fatal(err)
	}
	ft := &failingTransport{}
	cold := serveParams(t, db, cacheDir)
	cold.IDValue = "tt0050083"
	cold.RatingsLimit = &zero
	cold.Settings = settings()
	cold.TMDB = services.NewTmdbClient("test-key", &http.Client{Transport: ft})
	cold.Caches = &services.MemCacheSet{ImageMem: services.NewMemCache(10<<20, 100, 0, 0), IDs: services.NewMemCache(0, 100, 0, 0)}
	got, _, err := ServeImage(cold)
	if err != nil || !bytes.Equal(got, first) {
		t.Fatalf("limit-0 cold request not served from cache: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}

// status500Transport answers every call with a 500.
type status500Transport struct{ calls atomic.Int32 }

func (s *status500Transport) RoundTrip(*http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return &http.Response{StatusCode: 500, Header: http.Header{}, Body: nopCloser{bytes.NewReader([]byte(`{}`))}}, nil
}

// An upstream outage during ID resolution is remembered briefly per id: the
// next request for the same id doesn't call upstream again, another id does,
// and ResetRenderFailures forgets it.
func TestServeImage_ResolveFailureNegativeCache(t *testing.T) {
	saved := services.TMDBAPIRetry
	services.TMDBAPIRetry.MaxRetries = 0
	t.Cleanup(func() { services.TMDBAPIRetry = saved })
	t.Cleanup(ResetRenderFailures)
	st := &status500Transport{}
	params := func(id string) ServeParams {
		p := serveParams(t, nil, t.TempDir())
		p.IDValue = id
		p.TMDB = services.NewTmdbClient("test-key", &http.Client{Transport: st})
		p.Caches.IDs = services.NewMemCache(0, 100, 0, 0)
		return p
	}
	if _, _, err := ServeImage(params("tt7000001")); err == nil {
		t.Fatal("want an error from the failing upstream")
	}
	calls := st.calls.Load()
	if calls == 0 {
		t.Fatal("upstream never called")
	}
	if _, _, err := ServeImage(params("tt7000001")); err == nil {
		t.Fatal("want the remembered error")
	}
	if got := st.calls.Load(); got != calls {
		t.Fatalf("upstream called again within the window (%d → %d)", calls, got)
	}
	if _, _, err := ServeImage(params("tt7000002")); err == nil || st.calls.Load() == calls {
		t.Fatal("another id must still reach upstream")
	}
	ResetRenderFailures()
	calls = st.calls.Load()
	ServeImage(params("tt7000001"))
	if st.calls.Load() == calls {
		t.Fatal("reset did not clear the resolution failure")
	}
}

// A stale render served on a cold hit is re-rendered by the warm-up: the cold
// hit must not leave a "fresh" in-memory entry that the warm-up then mistakes
// for an up-to-date render.
func TestServeImage_ColdCacheStaleFileIsReRendered(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	cacheDir := t.TempDir()
	settings := func() *services.RenderSettings {
		return &services.RenderSettings{ImageSource: "t", Lang: "en", PosterLayout: services.DefaultLayout("poster")}
	}
	// Own id: other cold-cache tests leave a refresh back-off on theirs.
	const id = "tt0068646"
	p := serveParams(t, db, cacheDir)
	p.IDValue = id
	p.Settings = settings()
	p.Caches.IDs = services.NewMemCache(0, 100, 0, 0)
	var served services.CDNServed
	p.Served = &served
	if _, _, err := ServeImage(p); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(served.CachePath, old, old); err != nil {
		t.Fatal(err)
	}

	cold := serveParams(t, db, cacheDir)
	cold.IDValue = id
	cold.Settings = settings()
	cold.RatingsMinStaleSecs = 60
	cold.Caches = &services.MemCacheSet{ImageMem: services.NewMemCache(10<<20, 100, 0, 0), IDs: services.NewMemCache(0, 100, 0, 0)}
	if _, _, err := ServeImage(cold); err != nil {
		t.Fatalf("cold request: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(served.CachePath); err == nil && info.ModTime().After(old.Add(time.Hour)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("stale render served on a cold hit was never re-rendered")
}
