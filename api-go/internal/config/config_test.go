package config

import (
	"os"
	"testing"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	os.Setenv(key, value)
	t.Cleanup(func() { os.Unsetenv(key) })
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })
}

func TestFromEnvDefaults(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	cfg := FromEnv()
	if cfg.TMDBAPIKey != "tmdb_test" {
		t.Errorf("expected tmdb_test, got %s", cfg.TMDBAPIKey)
	}
	if cfg.CacheDir != "./cache" {
		t.Errorf("expected ./cache, got %s", cfg.CacheDir)
	}
	if cfg.ListenAddr != "0.0.0.0:3000" {
		t.Errorf("expected 0.0.0.0:3000, got %s", cfg.ListenAddr)
	}
	if cfg.RatingsMinStaleSecs != 86400 {
		t.Errorf("expected 86400, got %d", cfg.RatingsMinStaleSecs)
	}
	if cfg.ImageQuality != 85 {
		t.Errorf("expected 85, got %d", cfg.ImageQuality)
	}
	if cfg.EnableCDNRedirects {
		t.Error("expected EnableCDNRedirects to be false by default")
	}
	if cfg.ExternalCacheOnly {
		t.Error("expected ExternalCacheOnly to be false by default")
	}
}

func TestFromEnvCustomValues(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "CACHE_DIR", "/custom/cache")
	setEnv(t, "LISTEN_ADDR", "127.0.0.1:8080")
	setEnv(t, "ENABLE_CDN_REDIRECTS", "true")
	setEnv(t, "EXTERNAL_CACHE_ONLY", "1")

	cfg := FromEnv()
	if cfg.CacheDir != "/custom/cache" {
		t.Errorf("expected /custom/cache, got %s", cfg.CacheDir)
	}
	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %s", cfg.ListenAddr)
	}
	if !cfg.EnableCDNRedirects {
		t.Error("expected EnableCDNRedirects to be true")
	}
	if !cfg.ExternalCacheOnly {
		t.Error("expected ExternalCacheOnly to be true")
	}
}

func TestOptionalSecretTrimsWhitespace(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "OMDB_API_KEY", "  omdb_key  ")

	cfg := FromEnv()
	if cfg.OMDBAPIKey != "omdb_key" {
		t.Errorf("expected omdb_key, got %q", cfg.OMDBAPIKey)
	}
}

func TestOptionalSecretEmptyTreatedAsAbsent(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "MDBLIST_API_KEY", "")

	cfg := FromEnv()
	if len(cfg.MDBListAPIKeys) != 0 {
		t.Errorf("expected empty MDBList keys, got %v", cfg.MDBListAPIKeys)
	}
}

func TestCommaSeparatedKeys(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "MDBLIST_API_KEY", "key1, key2 ,key3")

	cfg := FromEnv()
	if len(cfg.MDBListAPIKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(cfg.MDBListAPIKeys))
	}
	if cfg.MDBListAPIKeys[0] != "key1" {
		t.Errorf("expected key1, got %s", cfg.MDBListAPIKeys[0])
	}
}

func TestTMDBKeyOptional(t *testing.T) {
	os.Unsetenv("TMDB_API_KEY")
	cfg := FromEnv()
	if cfg.TMDBAPIKey != "" {
		t.Errorf("expected empty TMDB key, got %s", cfg.TMDBAPIKey)
	}
}

func TestFreeKeyEnabled(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "FREE_KEY_ENABLED", "true")

	cfg := FromEnv()
	if cfg.FreeKeyEnabled == nil || !*cfg.FreeKeyEnabled {
		t.Error("expected FreeKeyEnabled to be true")
	}
}
