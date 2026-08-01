package config

import (
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
}

func FromEnv() *Config {
	c := &Config{
		TMDBAPIKey:          requireEnv("TMDB_API_KEY"),
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
		EnableCDNRedirects:  envBool("ENABLE_CDN_REDIRECTS"),
		ExternalCacheOnly:   envBool("EXTERNAL_CACHE_ONLY"),
		DisablePublicPages:  envBool("DISABLE_PUBLIC_PAGES"),
	}

	if v := os.Getenv("FREE_KEY_ENABLED"); v != "" {
		b := v == "true" || v == "1"
		c.FreeKeyEnabled = &b
	}

	return c
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(key + " must be set")
	}
	return v
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
