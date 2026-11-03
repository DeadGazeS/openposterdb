package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"openposterdb/internal/app"
	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"
)

// BenchmarkHandleImage_WarmHit measures a public image request whose render
// is already in the memory cache: API key + settings resolution, ID and
// ratings cache lookups, cache-key build and the hit itself, against a real
// (file-backed, WAL) SQLite database.
func BenchmarkHandleImage_WarmHit(b *testing.B) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := appimg.LoadFont("../../assets/fonts/Inter-Bold.ttf"); err != nil {
		b.Skip(err)
	}
	dir := b.TempDir()
	db, err := app.OpenDatabase(filepath.Join(dir, "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := app.RunSchema(db, app.SchemaSQL); err != nil {
		b.Fatal(err)
	}
	if err := app.RunMigrations(db, app.Migrations); err != nil {
		b.Fatal(err)
	}
	if err := services.RunUpgrades(db, dir, true); err != nil {
		b.Fatal(err)
	}
	app.SeedAdminIfNeeded(db, "admin", "benchpassword") // api_keys.created_by FK
	raw, hash, prefix := services.GenerateAPIKey()
	if _, err := services.CreateAPIKey(db, "bench", hash, prefix, "", 1); err != nil {
		b.Fatal(err)
	}
	deps := ImageDeps{
		DB: db,
		Config: &ImageServeConfig{
			CacheDir:     filepath.Join(dir, "cache"),
			ImageQuality: 85,
			Caches: &services.MemCacheSet{
				ImageMem: services.NewMemCache(64<<20, 0, time.Hour, 30*time.Minute),
				IDs:      services.NewMemCache(0, 50_000, 24*time.Hour, 0),
				Ratings:  services.NewMemCache(0, 50_000, 24*time.Hour, 0),
			},
			Inflight: appimg.NewInflightSet(),
		},
		TMDB: services.NewTmdbClient("test-key", &http.Client{Transport: cdnTmdbStub{}}),
	}
	h := HandleImage(func() ImageDeps { return deps }, func() bool { return false })
	path := "/" + raw + "/imdb/poster-default/tt0111161.jpg"
	do := func() int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetPathValue("apiKey", raw)
		req.SetPathValue("rest", "imdb/poster-default/tt0111161.jpg")
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code
	}
	if code := do(); code != http.StatusOK {
		b.Fatalf("warm-up: %d", code)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := do(); code != http.StatusOK {
			b.Fatalf("status %d", code)
		}
	}
}
