package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

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
	RateLimitAuthRPM    uint64
	// TrustedProxiesEnv is the raw TRUSTED_PROXIES value. When set it locks
	// the setting (the admin UI shows it read-only); when empty the value
	// stored in global settings (Settings → General → Config) applies.
	TrustedProxiesEnv  string
	ExternalCacheOnly  bool
	EnableCDNRedirects bool
	FreeKeyEnabled     *bool
	DisablePublicPages bool
	LogLevel           string
	JWTSecret          []byte
	SecretsKey         []byte
	SecureCookies      bool
	AdminUsername      string
	AdminPassword      string
}

func FromEnv() (*Config, error) {
	// Load .env into the process env if present. Already-set env vars take
	// precedence (godotenv.Load does not overwrite); quotes/inline comments
	// are handled by godotenv so the sanitizeValue workaround below is gone.
	_ = godotenv.Load()
	// Validated after .env is loaded, so a value set only there is checked
	// too (it used to be validated before, i.e. only from the real env).
	if _, err := ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES")); err != nil {
		return nil, err
	}

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
		ImageQuality:        clampQuality(envUint8OrDefault("IMAGE_QUALITY", 85)),
		ImageMemCacheMB:     envUint64OrDefault("IMAGE_MEM_CACHE_MB", 512),
		StaticDir:           os.Getenv("STATIC_DIR"),
		CORSOrigin:          os.Getenv("CORS_ORIGIN"),
		RateLimitRPM:        envUint64OrDefault("RATE_LIMIT_RPM", 60),
		RateLimitCDNRPM:     envUint64OrDefault("RATE_LIMIT_CDN_RPM", 240),
		RateLimitAuthRPM:    envUint64OrDefault("RATE_LIMIT_AUTH_RPM", 10),
		TrustedProxiesEnv:   strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")),
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

// DefaultTrustedProxies: loopback and private ranges (a reverse proxy on the
// same host or Docker network).
func DefaultTrustedProxies() []netip.Prefix {
	var out []netip.Prefix
	for _, c := range []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7", "fe80::/10"} {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// ParseTrustedProxies parses TRUSTED_PROXIES: "" or "private" = the default
// ranges, "none" = trust no proxy (always use the connection address),
// otherwise a comma list of IPs/CIDRs and the keywords "private" (the
// defaults) and "cloudflare" (Cloudflare's edge ranges), e.g.
// "private,cloudflare" for Cloudflare → a reverse proxy on the same host.
func ParseTrustedProxies(v string) ([]netip.Prefix, error) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "private":
		return DefaultTrustedProxies(), nil
	case "none":
		return []netip.Prefix{}, nil
	}
	var out []netip.Prefix
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.EqualFold(part, "private"):
			out = append(out, DefaultTrustedProxies()...)
		case strings.EqualFold(part, "cloudflare"):
			out = append(out, CloudflareProxies()...)
		case strings.Contains(part, "/"):
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q: %w", part, err)
			}
			if p.Bits() < minTrustedPrefixBits(p.Addr()) {
				// Trusting (nearly) every address would let any client
				// choose its own rate-limit identity again.
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q is too wide to trust", part)
			}
			out = append(out, p.Masked())
		default:
			a, err := netip.ParseAddr(part)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q: %w", part, err)
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out, nil
}

// cloudflareRanges are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips-v4 and /ips-v6, checked 2026-11-03).
// Cloudflare changes them rarely; update this list when it does.
var cloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

// Current Cloudflare ranges: the built-in list until a newer one is fetched
// from cloudflare.com (SetCloudflareRanges). cloudflareGen changes with
// every update so parsed TRUSTED_PROXIES values can be re-expanded.
var (
	cloudflareCurrent atomic.Pointer[[]netip.Prefix]
	cloudflareGen     atomic.Uint64
)

// BuiltinCloudflareProxies returns the ranges compiled into the binary.
func BuiltinCloudflareProxies() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cloudflareRanges))
	for _, c := range cloudflareRanges {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// CloudflareProxies returns Cloudflare's edge ranges (TRUSTED_PROXIES
// keyword "cloudflare"): the latest fetched list, else the built-in one.
func CloudflareProxies() []netip.Prefix {
	if p := cloudflareCurrent.Load(); p != nil {
		return append([]netip.Prefix(nil), *p...)
	}
	return BuiltinCloudflareProxies()
}

// SetCloudflareRanges replaces the current Cloudflare ranges.
func SetCloudflareRanges(p []netip.Prefix) {
	cp := append([]netip.Prefix(nil), p...)
	cloudflareCurrent.Store(&cp)
	cloudflareGen.Add(1)
}

// CloudflareGeneration changes whenever SetCloudflareRanges is called.
func CloudflareGeneration() uint64 { return cloudflareGen.Load() }

// minTrustedPrefixBits is the widest prefix accepted as a trusted proxy range:
// /8 for IPv4 (10.0.0.0/8 is the widest private range) and /7 for IPv6
// (fc00::/7, unique local).
func minTrustedPrefixBits(a netip.Addr) int {
	if a.Is4() || a.Is4In6() {
		return 8
	}
	return 7
}

// clampQuality keeps IMAGE_QUALITY within JPEG's 1-100.
func clampQuality(q uint8) uint8 {
	return max(1, min(q, 100))
}
