package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openposterdb/internal/services"
)

// "Clear posters" moved the dir aside and never deleted it.
func TestHandleClearKind_RemovesFiles(t *testing.T) {
	db := newHandlersTestDB(t)
	cacheDir := t.TempDir()
	posters := filepath.Join(cacheDir, "posters", "imdb")
	os.MkdirAll(posters, 0o755)
	os.WriteFile(filepath.Join(posters, "tt1.jpg"), []byte("x"), 0o644)

	rec := httptest.NewRecorder()
	HandleClearKind(db, cacheDir, "p", false, &services.MemCacheSet{})(rec, httptest.NewRequest(http.MethodDelete, "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		entries, _ := os.ReadDir(cacheDir)
		left := []string{}
		for _, e := range entries {
			left = append(left, e.Name())
		}
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cache dir still holds %v after the clear", left)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A failed step is reported, not hidden behind ok:true.
func TestHandleClearKind_ReportsFailure(t *testing.T) {
	db := newHandlersTestDB(t)
	db.Close() // every DB step fails
	rec := httptest.NewRecorder()
	HandleClearKind(db, t.TempDir(), "p", true, nil)(rec, httptest.NewRequest(http.MethodDelete, "/", nil))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("got %d %s, want 500 ok:false", rec.Code, rec.Body.String())
	}
}

// A title purge drops the title's cached ratings too (they are keyed by
// TMDB id, so the purge used to leave them in memory).
func TestHandlePurgeTitle_InvalidatesRatings(t *testing.T) {
	db := newHandlersTestDB(t)
	caches := &services.MemCacheSet{
		ImageMem: services.NewMemCache(1<<20, 0, time.Hour, 0),
		IDs:      services.NewMemCache(0, 100, time.Hour, 0),
		Ratings:  services.NewMemCache(0, 100, time.Hour, 0),
	}
	tmdb := services.NewTmdbClient("k", &http.Client{Transport: cdnTmdbStub{}})
	if _, err := services.ResolveIDCachedCtx(t.Context(), caches.IDs, services.IDTypeIMDB, "tt0111161", services.IDClients{TMDB: tmdb}); err != nil {
		t.Fatal(err)
	}
	caches.Ratings.Set("278/movie", "cached", 1)

	rec := httptest.NewRecorder()
	HandlePurgeTitle(db, t.TempDir(), "p", "imdb", "tt0111161", true, caches)(rec, httptest.NewRequest(http.MethodDelete, "/", nil))
	if rec.Code != 200 {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := caches.Ratings.Get("278/movie"); ok {
		t.Fatal("the purged title's ratings are still cached")
	}
}
