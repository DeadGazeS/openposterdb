package services

import (
	"encoding/json"
	"time"
)

func nowUTC() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func strOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

// --- Setting value constants ---

var validRatingKeys = map[string]bool{
	"imdb": true, "tmdb": true, "rt": true, "rta": true,
	"mc": true, "trakt": true, "lb": true, "mal": true,
	"mdblist": true, "ebert": true,
}

func isValidRatingKey(key string) bool {
	return validRatingKeys[key]
}

func allRatingKeys() string {
	return "imdb, tmdb, rt, rta, mc, trakt, lb, mal, mdblist, ebert"
}

// --- Marshal helpers (used by admin endpoints) ---

func MarshalStruct(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// --- Image type helpers ---
