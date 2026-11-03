package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"
)

// cdnTmdbStub answers the TMDB API + image CDN calls the serving pipeline
// makes for tt0111161, so ServeImage can render without a real provider.
type cdnTmdbStub struct{}

func (cdnTmdbStub) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "image.tmdb.org") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(appimg.SamplePosterPNG)),
		}, nil
	}
	body := `{"status_code":34}`
	switch {
	case strings.Contains(req.URL.Path, "/find/"):
		body = `{"movie_results":[{"id":278,"poster_path":"/abc.jpg","release_date":null,"popularity":100.0}],"tv_results":[]}`
	case strings.Contains(req.URL.Path, "/movie/278/images"):
		body = `{"posters":[{"file_path":"/abc.jpg","aspect_ratio":0.6667,"iso_639_1":null,"vote_average":1.0,"width":800,"height":1200}]}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func newCDNFlowDeps(t *testing.T) (ImageDeps, string) {
	t.Helper()
	if err := appimg.LoadFont("../../assets/fonts/Inter-Bold.ttf"); err != nil {
		t.Skipf("font asset not available: %v", err)
	}
	db := newHandlersTestDB(t)
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKey(db, "test", hash, prefix, "", 1); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return ImageDeps{
		DB: db,
		Config: &ImageServeConfig{
			CacheDir:           t.TempDir(),
			ImageQuality:       85,
			EnableCDNRedirects: true,
			Caches:             &services.MemCacheSet{},
			Inflight:           appimg.NewInflightSet(),
		},
		TMDB:      services.NewTmdbClient("test-key", &http.Client{Transport: cdnTmdbStub{}}),
		CDNHashes: services.NewHashRegistry(0),
	}, raw
}

// TestCDNRedirect_EndToEnd follows the 302 HandleImage issues with
// ENABLE_CDN_REDIRECTS on. The redirect fires before any render, so the /c/
// route must be able to render on its own — it used to only read a cache
// entry nobody wrote (and under a key ServeImage never uses), so every CDN
// URL was a 404.
func TestCDNRedirect_EndToEnd(t *testing.T) {
	deps, apiKey := newCDNFlowDeps(t)
	get := func() ImageDeps { return deps }

	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", HandleCDNImage(get, deps.CDNHashes))
	mux.HandleFunc("/{apiKey}/{rest...}", HandleImage(get, func() bool { return false }))

	req := httptest.NewRequest(http.MethodGet, "/"+apiKey+"/imdb/poster-default/tt0111161.jpg?imageSize=large", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("authed URL: got %d want 302 (body %q)", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(loc.Path, "/c/") {
		t.Fatalf("bad Location %q", rec.Header().Get("Location"))
	}
	if got := loc.Query().Get("imageSize"); got != "large" {
		t.Errorf("redirect must carry imageSize=large, got %q (Location %s)", got, loc)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, loc.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("CDN URL: got %d want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("CDN body is empty")
	}
}

// TestHandleIsValid_UnknownKeyRejected: FindAPIKeyByHashCtx returns (nil, nil)
// for an unknown key; isValid used to check only err and answer valid:true
// for any string.
func TestHandleIsValid_UnknownKeyRejected(t *testing.T) {
	deps, apiKey := newCDNFlowDeps(t)
	h := HandleIsValid(deps.DB, func() bool { return false })
	for key, want := range map[string]int{
		apiKey:              http.StatusOK,
		"not-a-real-key-42": http.StatusUnauthorized,
	} {
		req := httptest.NewRequest(http.MethodGet, "/"+key+"/isValid", nil)
		req.SetPathValue("apiKey", key)
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != want {
			t.Errorf("key %q: got %d want %d (%s)", key, rec.Code, want, rec.Body.String())
		}
	}
}

// countingCDNStub counts every upstream request.
type countingCDNStub struct{ calls *atomic.Int32 }

func (c countingCDNStub) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return cdnTmdbStub{}.RoundTrip(r)
}

// A settings hash is deterministic (the default-settings hash can be computed
// from the code) and leaks in Location headers. /c/ must never make the
// server do upstream work for an id no authenticated request rendered.
func TestCDNRoute_NoUpstreamWorkWithoutKey(t *testing.T) {
	deps, apiKey := newCDNFlowDeps(t)
	var calls atomic.Int32
	deps.TMDB = services.NewTmdbClient("test-key", &http.Client{Transport: countingCDNStub{&calls}})
	get := func() ImageDeps { return deps }
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", HandleCDNImage(get, deps.CDNHashes))
	mux.HandleFunc("/{apiKey}/{rest...}", HandleImage(get, func() bool { return false }))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+apiKey+"/imdb/poster-default/tt0111161.jpg", nil))
	loc := rec.Header().Get("Location")
	hash := strings.Split(strings.TrimPrefix(loc, "/c/"), "/")[0]
	before := calls.Load()

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/c/"+hash+"/imdb/poster-default/tt0068646.jpg", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unrendered id via /c/: got %d want 404", rec.Code)
	}
	if n := calls.Load() - before; n != 0 {
		t.Fatalf("/c/ made %d upstream calls without an API key", n)
	}
}

// /c/ serves exactly what the authenticated URL rendered, overrides included
// (it used to re-render and drop ?ratings_limit).
func TestCDNRoute_ServesTheAuthedRender(t *testing.T) {
	deps, apiKey := newCDNFlowDeps(t)
	get := func() ImageDeps { return deps }
	mux := http.NewServeMux()
	mux.HandleFunc("/c/{hash}/{rest...}", HandleCDNImage(get, deps.CDNHashes))
	mux.HandleFunc("/{apiKey}/{rest...}", HandleImage(get, func() bool { return false }))
	path := "/" + apiKey + "/imdb/poster-default/tt0111161.jpg?ratings_limit=0"

	deps.Config.EnableCDNRedirects = false
	direct := httptest.NewRecorder()
	mux.ServeHTTP(direct, httptest.NewRequest(http.MethodGet, path, nil))
	if direct.Code != http.StatusOK {
		t.Fatalf("direct: %d", direct.Code)
	}

	deps.Config.EnableCDNRedirects = true
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	cdn := httptest.NewRecorder()
	mux.ServeHTTP(cdn, httptest.NewRequest(http.MethodGet, rec.Header().Get("Location"), nil))
	if cdn.Code != http.StatusOK || !bytes.Equal(cdn.Body.Bytes(), direct.Body.Bytes()) {
		t.Fatalf("/c/ must serve the authed render: code %d, equal=%v", cdn.Code, bytes.Equal(cdn.Body.Bytes(), direct.Body.Bytes()))
	}
}
