package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openposterdb/internal/services"
)

func getStats(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The dashboard shows the disk cache size next to the in-memory one; with
// EXTERNAL_CACHE_ONLY there is no disk cache, so the value is null ("—").
func TestHandleStats_DiskCacheSize(t *testing.T) {
	db := newHandlersTestDB(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "p", "imdb", "tt1.jpg")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	out := getStats(t, HandleStats(db, dir, false, nil))
	if got, ok := out["image_disk_cache_mb"].(float64); !ok || got != 2 {
		t.Fatalf("image_disk_cache_mb = %v, want 2", out["image_disk_cache_mb"])
	}
	if got, ok := out["image_disk_cache_files"].(float64); !ok || got != 1 {
		t.Fatalf("image_disk_cache_files = %v, want 1", out["image_disk_cache_files"])
	}

	out = getStats(t, HandleStats(db, dir, true, nil))
	for _, k := range []string{"image_disk_cache_mb", "image_disk_cache_files"} {
		if v, present := out[k]; !present || v != nil {
			t.Fatalf("external cache only: %s = %v (present %v), want null", k, v, present)
		}
	}
}

// Uptime, database size and the memory image cache's request counters.
func TestHandleStats_UptimeDatabaseAndHitRate(t *testing.T) {
	db := newHandlersTestDB(t)
	caches := &services.MemCacheSet{ImageMem: services.NewMemCache(0, 0, time.Hour, 0)}

	out := getStats(t, HandleStats(db, t.TempDir(), false, caches))
	if v, ok := out["uptime_seconds"].(float64); !ok || v < 0 {
		t.Fatalf("uptime_seconds = %v", out["uptime_seconds"])
	}
	if v, ok := out["database_mb"].(float64); !ok || v <= 0 {
		t.Fatalf("database_mb = %v, want > 0", out["database_mb"])
	}
	// No image requests yet: no rate to show.
	if out["image_requests"].(float64) != 0 || out["mem_cache_hit_rate"] != nil {
		t.Fatalf("before requests: %v / %v", out["image_requests"], out["mem_cache_hit_rate"])
	}

	caches.ImageMem.Set("k", []byte("x"), 1)
	caches.ImageMem.Get("k")
	caches.ImageMem.Get("k")
	caches.ImageMem.Get("k")
	caches.ImageMem.Get("other")
	out = getStats(t, HandleStats(db, t.TempDir(), false, caches))
	if out["image_requests"].(float64) != 4 || out["mem_cache_hit_rate"].(float64) != 75 {
		t.Fatalf("after requests: %v / %v, want 4 / 75", out["image_requests"], out["mem_cache_hit_rate"])
	}
}
