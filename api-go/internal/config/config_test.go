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

func TestMain(m *testing.M) {
	// JWT_SECRET and SECRETS_KEY are required by FromEnv; use fixed valid
	// 32-byte hex values for tests.
	os.Setenv("JWT_SECRET", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	os.Setenv("SECRETS_KEY", "101112131415161718191a1b1c1d1e1f000102030405060708090a0b0c0d0e0f")
	os.Exit(m.Run())
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })
}

func TestFromEnvDefaults(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
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
	if cfg.ExternalCacheOnly {
		t.Error("expected ExternalCacheOnly to be false by default")
	}
}

func TestFromEnvCustomValues(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "CACHE_DIR", "/custom/cache")
	setEnv(t, "LISTEN_ADDR", "127.0.0.1:8080")
	setEnv(t, "EXTERNAL_CACHE_ONLY", "1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheDir != "/custom/cache" {
		t.Errorf("expected /custom/cache, got %s", cfg.CacheDir)
	}
	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %s", cfg.ListenAddr)
	}
	if !cfg.ExternalCacheOnly {
		t.Error("expected ExternalCacheOnly to be true")
	}
}

func TestOptionalSecretTrimsWhitespace(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "OMDB_API_KEY", "  omdb_key  ")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OMDBAPIKey != "omdb_key" {
		t.Errorf("expected omdb_key, got %q", cfg.OMDBAPIKey)
	}
}

func TestOptionalSecretEmptyTreatedAsAbsent(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "MDBLIST_API_KEY", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MDBListAPIKeys) != 0 {
		t.Errorf("expected empty MDBList keys, got %v", cfg.MDBListAPIKeys)
	}
}

func TestCommaSeparatedKeys(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "MDBLIST_API_KEY", "key1, key2 ,key3")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MDBListAPIKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(cfg.MDBListAPIKeys))
	}
	if cfg.MDBListAPIKeys[0] != "key1" {
		t.Errorf("expected key1, got %s", cfg.MDBListAPIKeys[0])
	}
}

func TestTMDBKeyOptional(t *testing.T) {
	os.Unsetenv("TMDB_API_KEY")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TMDBAPIKey != "" {
		t.Errorf("expected empty TMDB key, got %s", cfg.TMDBAPIKey)
	}
}

func TestFreeKeyEnabled(t *testing.T) {
	setEnv(t, "TMDB_API_KEY", "tmdb_test")
	setEnv(t, "FREE_KEY_ENABLED", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FreeKeyEnabled == nil || !*cfg.FreeKeyEnabled {
		t.Error("expected FreeKeyEnabled to be true")
	}
}

// Near-catch-all ranges trust (almost) everyone, just like /0.
func TestParseTrustedProxies_RejectsHugeRanges(t *testing.T) {
	for _, v := range []string{"0.0.0.0/0", "0.0.0.0/1", "128.0.0.0/4", "::/0", "::/3"} {
		if _, err := ParseTrustedProxies(v); err == nil {
			t.Errorf("%s accepted", v)
		}
	}
	for _, v := range []string{"10.0.0.0/8", "172.16.0.0/12", "fc00::/7", "203.0.113.7", "2001:db8::/32"} {
		if _, err := ParseTrustedProxies(v); err != nil {
			t.Errorf("%s rejected: %v", v, err)
		}
	}
}

// A bad TRUSTED_PROXIES set only in the dotenv file (bare-metal runs) used
// to skip the startup check: it was validated before that file was loaded.
func TestFromEnv_ValidatesTrustedProxiesFromDotEnv(t *testing.T) {
	unsetEnv(t, "TRUSTED_PROXIES")
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("TRUSTED_PROXIES=not-an-ip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := FromEnv(); err == nil {
		t.Fatal("invalid TRUSTED_PROXIES from the dotenv file accepted")
	}
}

// IMAGE_QUALITY is a JPEG quality: values outside 1-100 are clamped.
func TestFromEnv_ImageQualityClamped(t *testing.T) {
	for in, want := range map[string]uint8{"0": 1, "250": 100, "70": 70} {
		setEnv(t, "IMAGE_QUALITY", in)
		cfg, err := FromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ImageQuality != want {
			t.Errorf("IMAGE_QUALITY=%s: got %d, want %d", in, cfg.ImageQuality, want)
		}
	}
}
