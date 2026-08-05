package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TMDBAPIKey          string
	OMDBAPIKey          string
	MDBListAPIKeys      []string
	FanartAPIKey        string
	TraktClientID       string
	CacheDir            string
	DBDir               string
	ListenAddr          string
	RatingsMinStaleSecs uint64
	RatingsMaxAgeSecs   uint64
	ImageStaleSecs      uint64
	ImageQuality        uint8
	ImageMemCacheMB     uint64
	StaticDir           string
	CORSOrigin          string
	EnableCDNRedirects  bool
	ExternalCacheOnly   bool
	FreeKeyEnabled      *bool
	DisablePublicPages  bool
	LogLevel            string
	JWTSecret           []byte
	SecureCookies       bool
	AdminUsername       string
	AdminPassword       string
}

func FromEnv() (*Config, error) {
	c := &Config{
		TMDBAPIKey:          sanitizeValue(optionalSecret("TMDB_API_KEY")),
		OMDBAPIKey:          sanitizeValue(optionalSecret("OMDB_API_KEY")),
		MDBListAPIKeys:      sanitizeSecrets(optionalSecrets("MDBLIST_API_KEY")),
		FanartAPIKey:        sanitizeValue(optionalSecret("FANART_API_KEY")),
		TraktClientID:       sanitizeValue(optionalSecret("TRAKT_CLIENT_ID")),
		CacheDir:            sanitizeValue(envOrDefault("CACHE_DIR", "./cache")),
		DBDir:               sanitizeValue(envOrDefault("DB_DIR", "./db")),
		ListenAddr:          sanitizeValue(envOrDefault("LISTEN_ADDR", "0.0.0.0:3000")),
		RatingsMinStaleSecs: envUint64OrDefault("RATINGS_STALE_SECS", 86400),
		RatingsMaxAgeSecs:   envUint64OrDefault("RATINGS_MAX_AGE_SECS", 31536000),
		ImageStaleSecs:      envUint64OrDefault("IMAGE_STALE_SECS", 0),
		ImageQuality:        envUint8OrDefault("IMAGE_QUALITY", 85),
		ImageMemCacheMB:     envUint64OrDefault("IMAGE_MEM_CACHE_MB", 512),
		StaticDir:           sanitizeValue(os.Getenv("STATIC_DIR")),
		CORSOrigin:          sanitizeValue(os.Getenv("CORS_ORIGIN")),
		EnableCDNRedirects:  envBool("ENABLE_CDN_REDIRECTS"),
		ExternalCacheOnly:   envBool("EXTERNAL_CACHE_ONLY"),
		DisablePublicPages:  envBool("DISABLE_PUBLIC_PAGES"),
		LogLevel:            sanitizeValue(envOrDefault("LOG_LEVEL", "info")),
	}

	if v := os.Getenv("FREE_KEY_ENABLED"); v != "" {
		b := v == "true" || v == "1"
		c.FreeKeyEnabled = &b
	}

	jwtSecret, err := loadJWTSecret()
	if err != nil {
		return nil, err
	}
	c.JWTSecret = jwtSecret
	c.SecureCookies = loadSecureCookies()
	c.AdminUsername = os.Getenv("ADMIN_USERNAME")
	c.AdminPassword = os.Getenv("ADMIN_PASSWORD")

	return c, nil
}

// loadJWTSecret reads the required JWT_SECRET env var (64 hex chars = 32
// bytes) and returns an actionable error when it is missing or invalid.
func loadJWTSecret() ([]byte, error) {
	hexStr := os.Getenv("JWT_SECRET")
	if hexStr == "" {
		return nil, errors.New("JWT_SECRET is not set. This is required.\n" +
			"Generate one with: openssl rand -hex 32\n" +
			"Then add it to your .env file.")
	}
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("JWT_SECRET is not valid hex: %w", err)
	}
	if len(bytes) != 32 {
		return nil, fmt.Errorf("JWT_SECRET must be 32 bytes (64 hex chars), got %d", len(bytes))
	}
	return bytes, nil
}

// loadSecureCookies reports whether cookies get the Secure attribute. Defaults
// to true unless COOKIE_SECURE is explicitly "false" or "0".
func loadSecureCookies() bool {
	val := os.Getenv("COOKIE_SECURE")
	if val == "" {
		return true
	}
	return val != "false" && val != "0"
}

func optionalSecret(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func optionalSecrets(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envUint64OrDefault(key string, def uint64) uint64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envUint8OrDefault(key string, def uint8) uint8 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 8)
	if err != nil {
		return def
	}
	return uint8(n)
}

func envBool(key string) bool {
	v := os.Getenv(key)
	return v == "true" || v == "1"
}

// sanitizeValue guards against trailing annotations accidentally ending up in a
// value. Docker's env_file does not strip inline comments, so a line like
// "LOG_LEVEL=debug (optional)" would otherwise set the whole string.
func sanitizeValue(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, " \t("); i >= 0 {
		v = v[:i]
	}
	return v
}

func sanitizeSecrets(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if s := sanitizeValue(k); s != "" {
			out = append(out, s)
		}
	}
	return out
}
