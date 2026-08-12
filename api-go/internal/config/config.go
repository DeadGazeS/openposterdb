package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
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
	RateLimitRPM        uint64
	RateLimitCDNRPM     uint64
	ExternalCacheOnly   bool
	EnableCDNRedirects  bool
	FreeKeyEnabled      *bool
	DisablePublicPages  bool
	LogLevel            string
	JWTSecret           []byte
	SecretsKey          []byte
	SecureCookies       bool
	AdminUsername       string
	AdminPassword       string
}

func FromEnv() (*Config, error) {
	// Load .env into the process env if present. Already-set env vars take
	// precedence (godotenv.Load does not overwrite); quotes/inline comments
	// are handled by godotenv so the sanitizeValue workaround below is gone.
	_ = godotenv.Load()

	c := &Config{
		TMDBAPIKey:          optionalSecret("TMDB_API_KEY"),
		OMDBAPIKey:          optionalSecret("OMDB_API_KEY"),
		MDBListAPIKeys:      optionalSecrets("MDBLIST_API_KEY"),
		FanartAPIKey:        optionalSecret("FANART_API_KEY"),
		TraktClientID:       optionalSecret("TRAKT_CLIENT_ID"),
		CacheDir:            envOrDefault("CACHE_DIR", "./cache"),
		DBDir:               envOrDefault("DB_DIR", "./db"),
		ListenAddr:          envOrDefault("LISTEN_ADDR", "0.0.0.0:3000"),
		RatingsMinStaleSecs: envUint64OrDefault("RATINGS_STALE_SECS", 86400),
		RatingsMaxAgeSecs:   envUint64OrDefault("RATINGS_MAX_AGE_SECS", 31536000),
		ImageStaleSecs:      envUint64OrDefault("IMAGE_STALE_SECS", 0),
		ImageQuality:        envUint8OrDefault("IMAGE_QUALITY", 85),
		ImageMemCacheMB:     envUint64OrDefault("IMAGE_MEM_CACHE_MB", 512),
		StaticDir:           os.Getenv("STATIC_DIR"),
		CORSOrigin:          os.Getenv("CORS_ORIGIN"),
		RateLimitRPM:        envUint64OrDefault("RATE_LIMIT_RPM", 60),
		RateLimitCDNRPM:     envUint64OrDefault("RATE_LIMIT_CDN_RPM", 240),
		ExternalCacheOnly:   envBool("EXTERNAL_CACHE_ONLY"),
		EnableCDNRedirects:  envBool("ENABLE_CDN_REDIRECTS"),
		DisablePublicPages:  envBool("DISABLE_PUBLIC_PAGES"),
		LogLevel:            envOrDefault("LOG_LEVEL", "info"),
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

	secretsKey, err := loadSecretsKey()
	if err != nil {
		return nil, err
	}
	c.SecretsKey = secretsKey

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

// loadSecretsKey reads the required SECRETS_KEY env var (64 hex chars = 32
// bytes). It is the KEK for the encrypted source API keys at rest (TMDB,
// MDBList, OMDb, Fanart.tv, Trakt) — separate from JWT_SECRET so that
// JWT_SECRET rotation does not invalidate every stored secret, and so that
// JWT_SECRET compromise (the secret most likely to leak in error logs /
// debug dumps) does not directly expose the at-rest ciphertexts.
func loadSecretsKey() ([]byte, error) {
	hexStr := os.Getenv("SECRETS_KEY")
	if hexStr == "" {
		return nil, errors.New("SECRETS_KEY is not set. This is required for encrypting source API keys at rest.\n" +
			"Generate one with: openssl rand -hex 32\n" +
			"Then add it to your .env file.")
	}
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("SECRETS_KEY is not valid hex: %w", err)
	}
	if len(bytes) != 32 {
		return nil, fmt.Errorf("SECRETS_KEY must be 32 bytes (64 hex chars), got %d", len(bytes))
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
