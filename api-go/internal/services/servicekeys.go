package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/crypto/hkdf"

	apperr "openposterdb/internal/errors"
)

func deriveCipherKey(jwtSecret []byte) []byte {
	h := hkdf.New(sha256.New, jwtSecret, nil, []byte("openposterdb-service-keys"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		panic("HKDF expand should not fail for 32 bytes: " + err.Error())
	}
	return key
}

func encrypt(value string, gcm cipher.AEAD) string {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic("failed to generate nonce: " + err.Error())
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func decrypt(encrypted string, gcm cipher.AEAD) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("encrypted data too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed — JWT_SECRET may have changed")
	}
	return string(plaintext), nil
}

func MaskKey(key string) string {
	n := min(len(key)/4, 4)
	if n == 0 {
		return "****"
	}
	prefix := key[:n]
	rev := []rune(key)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	suffix := string(rev[:n])
	revSuf := []rune(suffix)
	for i, j := 0, len(revSuf)-1; i < j; i, j = i+1, j-1 {
		revSuf[i], revSuf[j] = revSuf[j], revSuf[i]
	}
	return fmt.Sprintf("%s...%s", prefix, string(revSuf))
}

type keyCache struct {
	tmdb    []string
	mdblist []string
	omdb    []string
	fanart  []string
	trakt   []string
}

type ServiceKeyManager struct {
	DB         *sql.DB
	GCM        cipher.AEAD
	HTTP       *http.Client
	Keys       keyCache
	mu         sync.RWMutex
	EnvTMDB    bool
	EnvMDBList bool
	EnvOMDB    bool
	EnvFanart  bool
	EnvTrakt   bool
}

type ServiceKeyStatus struct {
	Locked bool    `json:"locked"`
	HasKey bool    `json:"has_key"`
	Masked *string `json:"masked"`
	// Keys holds the real (decrypted) keys. Only populated by the admin
	// settings endpoint, which already has plaintext access (export).
	Keys []string `json:"keys,omitempty"`
}

type ServiceKeysResponse struct {
	TMDB    ServiceKeyStatus `json:"tmdb"`
	MDBList ServiceKeyStatus `json:"mdblist"`
	OMDB    ServiceKeyStatus `json:"omdb"`
	Fanart  ServiceKeyStatus `json:"fanart"`
	Trakt   ServiceKeyStatus `json:"trakt"`
}

type ServiceKeysUpdate struct {
	TMDB    *string `json:"tmdb"`
	MDBList *string `json:"mdblist"`
	OMDB    *string `json:"omdb"`
	Fanart  *string `json:"fanart"`
	Trakt   *string `json:"trakt"`
}

func NewServiceKeyManager(db *sql.DB, jwtSecret []byte, httpClient *http.Client,
	envTMDB string, envMDBList []string, envOMDB, envFanart, envTrakt string) *ServiceKeyManager {

	cipherKey := deriveCipherKey(jwtSecret)
	aesCipher, err := aes.NewCipher(cipherKey)
	if err != nil {
		panic("AES-256-GCM: " + err.Error())
	}
	gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		panic("AES-256-GCM: " + err.Error())
	}

	cache := keyCache{
		mdblist: envMDBList,
	}

	envMDBListLocked := len(envMDBList) > 0

	if envTMDB != "" {
		cache.tmdb = []string{envTMDB}
	}
	if envOMDB != "" {
		cache.omdb = []string{envOMDB}
	}
	if envFanart != "" {
		cache.fanart = []string{envFanart}
	}
	if envTrakt != "" {
		cache.trakt = []string{envTrakt}
	}

	return &ServiceKeyManager{
		DB:         db,
		GCM:        gcm,
		HTTP:       httpClient,
		Keys:       cache,
		EnvTMDB:    envTMDB != "",
		EnvMDBList: envMDBListLocked,
		EnvOMDB:    envOMDB != "",
		EnvFanart:  envFanart != "",
		EnvTrakt:   envTrakt != "",
	}
}

func (m *ServiceKeyManager) Init() {
	if !m.EnvTMDB {
		if v := m.loadFromDB("tmdb"); v != nil && len(*v) > 0 {
			m.Keys.tmdb = *v
		}
	}
	if !m.EnvMDBList {
		if v := m.loadFromDB("mdblist"); v != nil {
			m.Keys.mdblist = *v
		}
	}
	if !m.EnvOMDB {
		if v := m.loadFromDB("omdb"); v != nil {
			m.Keys.omdb = *v
		}
	}
	if !m.EnvFanart {
		if v := m.loadFromDB("fanart"); v != nil {
			m.Keys.fanart = *v
		}
	}
	if !m.EnvTrakt {
		if v := m.loadFromDB("trakt"); v != nil {
			m.Keys.trakt = *v
		}
	}
}

func (m *ServiceKeyManager) loadFromDB(service string) *[]string {
	encrypted, err := GetGlobalSetting(m.DB, "service_key_"+service)
	if err != nil || encrypted == "" {
		return nil
	}
	plain, err := decrypt(encrypted, m.GCM)
	if err != nil {
		return nil
	}
	var keys []string
	for k := range strings.SplitSeq(plain, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return &keys
}

func (m *ServiceKeyManager) TMDBKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.Keys.tmdb) > 0 {
		return m.Keys.tmdb[0]
	}
	return ""
}

func (m *ServiceKeyManager) MDBListKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Keys.mdblist
}

func (m *ServiceKeyManager) OMDBKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Keys.omdb
}

func (m *ServiceKeyManager) FanartKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Keys.fanart
}

func (m *ServiceKeyManager) TraktClientIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Keys.trakt
}

func (m *ServiceKeyManager) EnvLocked(service string) bool {
	switch service {
	case "tmdb":
		return m.EnvTMDB
	case "mdblist":
		return m.EnvMDBList
	case "omdb":
		return m.EnvOMDB
	case "fanart":
		return m.EnvFanart
	case "trakt":
		return m.EnvTrakt
	}
	return false
}

// PlaintextKeys returns the decrypted DB-stored service keys keyed by service
// (comma-joined for multi-key services). Env-locked services are excluded since
// their keys live in the environment, not the database.
func (m *ServiceKeyManager) PlaintextKeys() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string)
	for _, svc := range []string{"tmdb", "mdblist", "omdb", "fanart", "trakt"} {
		if m.EnvLocked(svc) {
			continue
		}
		if v := m.loadFromDB(svc); v != nil && len(*v) > 0 {
			out[svc] = strings.Join(*v, ",")
		}
	}
	return out
}

func (m *ServiceKeyManager) GetStatus() ServiceKeysResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ServiceKeysResponse{
		TMDB:    m.statusFor("tmdb", m.Keys.tmdb),
		MDBList: m.statusFor("mdblist", m.Keys.mdblist),
		OMDB:    m.statusFor("omdb", m.Keys.omdb),
		Fanart:  m.statusFor("fanart", m.Keys.fanart),
		Trakt:   m.statusFor("trakt", m.Keys.trakt),
	}
}

func (m *ServiceKeyManager) statusFor(service string, keys []string) ServiceKeyStatus {
	locked := m.EnvLocked(service)
	hasKey := len(keys) > 0
	var masked *string
	if hasKey {
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = MaskKey(k)
		}
		s := strings.Join(parts, ", ")
		masked = &s
	}
	// The real keys are only exposed for unlocked services (env-locked keys
	// stay hidden); the admin UI uses them to render removable per-key chips.
	var realKeys []string
	if !locked && hasKey {
		realKeys = keys
	}
	return ServiceKeyStatus{Locked: locked, HasKey: hasKey, Masked: masked, Keys: realKeys}
}

func (m *ServiceKeyManager) UpdateKeys(update *ServiceKeysUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.EnvTMDB && update.TMDB != nil {
		keys := parseKeys(*update.TMDB)
		if err := m.storeKey("tmdb", *update.TMDB); err != nil {
			return err
		}
		m.Keys.tmdb = keys
	}
	if !m.EnvMDBList && update.MDBList != nil {
		keys := parseKeys(*update.MDBList)
		if err := m.storeKey("mdblist", *update.MDBList); err != nil {
			return err
		}
		m.Keys.mdblist = keys
	}
	if !m.EnvOMDB && update.OMDB != nil {
		keys := parseKeys(*update.OMDB)
		if err := m.storeKey("omdb", *update.OMDB); err != nil {
			return err
		}
		m.Keys.omdb = keys
	}
	if !m.EnvFanart && update.Fanart != nil {
		keys := parseKeys(*update.Fanart)
		if err := m.storeKey("fanart", *update.Fanart); err != nil {
			return err
		}
		m.Keys.fanart = keys
	}
	if !m.EnvTrakt && update.Trakt != nil {
		keys := parseKeys(*update.Trakt)
		if err := m.storeKey("trakt", *update.Trakt); err != nil {
			return err
		}
		m.Keys.trakt = keys
	}
	return nil
}

func (m *ServiceKeyManager) storeKey(service, value string) error {
	encrypted := encrypt(value, m.GCM)
	return SetGlobalSetting(m.DB, "service_key_"+service, encrypted)
}

func parseKeys(keys string) []string {
	var result []string
	for k := range strings.SplitSeq(keys, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			result = append(result, k)
		}
	}
	return result
}

// ParseServiceKeys is the exported form of parseKeys.
func ParseServiceKeys(keys string) []string {
	return parseKeys(keys)
}

// ValidateServiceKeyString validates a comma-separated service key value before
// saving it. It rejects clearly-malformed input (empty entries when a value was
// provided, duplicates, whitespace garbage) and performs a live check against
// the provider for a definitive answer. Network/provider errors and rate limits
// are tolerated (the key is accepted) — only a clear 401/403 rejects the key.
func ValidateServiceKeyString(service, value string, httpClient *http.Client) error {
	rawKeys := parseKeys(value)
	if len(rawKeys) == 0 {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return fmt.Errorf("value contained no usable keys")
	}

	seen := make(map[string]bool)
	for _, k := range rawKeys {
		if k == "" {
			return fmt.Errorf("contains an empty key")
		}
		if seen[k] {
			return fmt.Errorf("duplicate key: %s", MaskKey(k))
		}
		seen[k] = true
		if err := ValidateServiceKey(service, k, httpClient); err != nil {
			return err
		}
	}
	return nil
}

// ValidateServiceKey performs a live check against the provider for a single key.
// Only a definitive 401/403 is treated as invalid; network errors, 5xx, and rate
// limits are tolerated so flaky providers don't block saving a good key.
func ValidateServiceKey(service, key string, httpClient *http.Client) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if len(key) < 6 {
		return fmt.Errorf("key is too short (%d chars)", len(key))
	}

	switch service {
	case "tmdb":
		return checkProviderURL(httpClient, "https://api.themoviedb.org/3/configuration?api_key="+key, "TMDB")
	case "omdb":
		return checkProviderURL(httpClient, "https://www.omdbapi.com/?apikey="+key+"&i=tt3896198", "OMDb")
	case "mdblist":
		return checkProviderURL(httpClient, "https://api.mdblist.com/tmdb/movie/1?apikey="+key, "MDBList")
	case "fanart":
		return checkProviderURL(httpClient, "https://webservice.fanart.tv/v3/movies/550?api_key="+key, "Fanart.tv")
	case "trakt":
		return checkTraktClientID(httpClient, key)
	}
	return nil
}

func checkProviderURL(httpClient *http.Client, url, service string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "openposterdb/1.2.1")
	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("service key validation request failed", "service", service, "error", apperr.RedactURLSecrets(err))
		return nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid key (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func checkTraktClientID(httpClient *http.Client, clientID string) error {
	req, err := http.NewRequest("GET", "https://api.trakt.tv/movies/tt3896198", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", clientID)
	req.Header.Set("User-Agent", "openposterdb/1.2.1")
	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("service key validation request failed", "service", "trakt", "error", err)
		return nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid Client ID (HTTP %d)", resp.StatusCode)
	}
	return nil
}
