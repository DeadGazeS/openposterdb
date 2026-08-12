package services

import (
	"context"
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

// deriveKek returns the per-service Key Encryption Key — an AES-256 key
// derived from the master SECRETS_KEY via HKDF-SHA256, with a service-scoped
// info string so each service has an isolated KEK. Random DEKs (per-secret
// data encryption keys) are wrapped by this KEK; the wrapped DEK travels
// alongside the ciphertext in the same DB row.
func deriveKek(secretsKey []byte, service string) []byte {
	h := hkdf.New(sha256.New, secretsKey, nil, []byte("service-key:"+service+":v2"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		panic("HKDF expand should not fail for 32 bytes: " + err.Error())
	}
	return key
}

// deriveKekLegacy is the pre-tier-1 derivation: HKDF(JWT_SECRET, info="openposterdb-service-keys").
// Kept for one release so existing ciphertexts can be decrypted and re-encrypted
// in v2 format. Drop after the next version.
func deriveKekLegacy(jwtSecret []byte) []byte {
	h := hkdf.New(sha256.New, jwtSecret, nil, []byte("openposterdb-service-keys"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		panic("HKDF expand should not fail for 32 bytes: " + err.Error())
	}
	return key
}

// encryptSecret produces the v2 envelope-encrypted form: a randomly generated
// DEK encrypts the plaintext with AES-256-GCM, and the DEK itself is wrapped
// by the per-service KEK derived from SECRETS_KEY. Storage layout is
// `v2:<b64(wrappedDEK)>:<b64(nonce||ciphertext)>` so the entire secret round
// trips in a single DB row.
func encryptSecret(value []byte, secretsKey []byte, service string) (string, error) {
	// 1. Random DEK.
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return "", fmt.Errorf("read random DEK: %w", err)
	}

	// 2. Encrypt plaintext with DEK.
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	dekGcm, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return "", err
	}
	dekNonce := make([]byte, dekGcm.NonceSize())
	if _, err := rand.Read(dekNonce); err != nil {
		return "", fmt.Errorf("read DEK nonce: %w", err)
	}
	ciphertext := dekGcm.Seal(dekNonce, dekNonce, value, nil)

	// 3. Wrap DEK with per-service KEK.
	kek := deriveKek(secretsKey, service)
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return "", err
	}
	kekGcm, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return "", err
	}
	kekNonce := make([]byte, kekGcm.NonceSize())
	if _, err := rand.Read(kekNonce); err != nil {
		return "", fmt.Errorf("read KEK nonce: %w", err)
	}
	wrappedDek := kekGcm.Seal(kekNonce, kekNonce, dek, nil)

	// 4. Format: v2:<b64(wrappedDEK)>:<b64(ciphertext)>
	return "v2:" + base64.StdEncoding.EncodeToString(wrappedDek) + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptV2 is the inverse of encryptSecret for the v2 layout.
func decryptV2(encrypted string, secretsKey []byte, service string) ([]byte, error) {
	parts := strings.SplitN(encrypted[3:], ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed v2 ciphertext")
	}
	wrappedDek, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("v2 wrapped DEK base64: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("v2 ciphertext base64: %w", err)
	}

	// Unwrap DEK with the per-service KEK.
	kek := deriveKek(secretsKey, service)
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	kekGcm, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, err
	}
	nonceSize := kekGcm.NonceSize()
	if len(wrappedDek) < nonceSize {
		return nil, fmt.Errorf("v2 wrapped DEK too short")
	}
	nonce, wrappedCt := wrappedDek[:nonceSize], wrappedDek[nonceSize:]
	dek, err := kekGcm.Open(nil, nonce, wrappedCt, nil)
	if err != nil {
		return nil, fmt.Errorf("v2 DEK unwrap: %w", err)
	}

	// Decrypt ciphertext with DEK.
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	dekGcm, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("v2 ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return dekGcm.Open(nil, nonce, ct, nil)
}

// decryptLegacy attempts the pre-tier-1 format: a single AES-256-GCM seal
// using a KEK derived from JWT_SECRET. Returns an error if the ciphertext
// isn't valid v1 (no `v2:` prefix, base64-decodable, GCM-authenticating).
// Kept as a one-release migration shim.
func decryptLegacy(encrypted string, jwtSecret []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	kek := deriveKekLegacy(jwtSecret)
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("encrypted data too short")
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed — JWT_SECRET may have changed")
	}
	return plain, nil
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

// ServiceKeyManager stores source API keys (TMDB / MDBList / OMDb / Fanart.tv /
// Trakt) encrypted at rest. SECRETS_KEY derives the per-service KEKs (HKDF).
// LegacyJWTSecret is held for one release so existing v1 ciphertexts can be
// decrypted and re-encrypted as v2 on first read; drop it in the next release.
type ServiceKeyManager struct {
	DB             *sql.DB
	SecretsKey     []byte
	LegacyJWTSecret []byte
	HTTP           *http.Client
	Keys           keyCache
	mu             sync.RWMutex
	EnvTMDB        bool
	EnvMDBList     bool
	EnvOMDB        bool
	EnvFanart      bool
	EnvTrakt       bool
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

// NewServiceKeyManager builds a manager with the v2 KEK seed (SECRETS_KEY) and
// the legacy JWT secret kept only for the one-release migration shim. Pass nil
// for legacyJWTSecret on fresh installs; the manager will treat any pre-v2
// ciphertext as missing.
func NewServiceKeyManager(db *sql.DB, secretsKey []byte, legacyJWTSecret []byte, httpClient *http.Client,
	envTMDB string, envMDBList []string, envOMDB, envFanart, envTrakt string) *ServiceKeyManager {

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
		DB:              db,
		SecretsKey:      secretsKey,
		LegacyJWTSecret: legacyJWTSecret,
		HTTP:            httpClient,
		Keys:            cache,
		EnvTMDB:         envTMDB != "",
		EnvMDBList:      envMDBListLocked,
		EnvOMDB:         envOMDB != "",
		EnvFanart:       envFanart != "",
		EnvTrakt:        envTrakt != "",
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

// decryptStored picks the right path for a stored ciphertext. v2 envelopes
// are decrypted directly; pre-v2 rows are decrypted with the legacy KEK
// derived from JWT_SECRET, then re-encrypted as v2 and persisted so the
// migration is one-shot per row. The success log fires once per row (the
// next read sees v2 and skips this branch); the error logs surface any
// migration failure so it can be diagnosed.
func (m *ServiceKeyManager) decryptStored(encrypted, service string) ([]byte, error) {
	if strings.HasPrefix(encrypted, "v2:") {
		return decryptV2(encrypted, m.SecretsKey, service)
	}
	if m.LegacyJWTSecret != nil {
		plain, err := decryptLegacy(encrypted, m.LegacyJWTSecret)
		if err != nil {
			slog.Error("service key v1 decryption failed — legacy ciphertext cannot be migrated",
				"service", service, "error", err)
			return nil, err
		}
		reEncrypted, encErr := encryptSecret(plain, m.SecretsKey, service)
		if encErr != nil {
			slog.Error("service key v1→v2 re-encrypt failed", "service", service, "error", encErr)
			return plain, nil
		}
		if err := SetGlobalSetting(m.DB, "service_key_"+service, reEncrypted); err != nil {
			slog.Error("service key v1→v2 persist failed", "service", service, "error", err)
			return plain, nil
		}
		slog.Info("service key migrated v1→v2", "service", service)
		return plain, nil
	}
	return nil, fmt.Errorf("unsupported ciphertext format")
}

func (m *ServiceKeyManager) loadFromDB(service string) *[]string {
	encrypted, err := GetGlobalSetting(m.DB, "service_key_"+service)
	if err != nil || encrypted == "" {
		return nil
	}
	plain, err := m.decryptStored(encrypted, service)
	if err != nil {
		return nil
	}
	var keys []string
	for k := range strings.SplitSeq(string(plain), ",") {
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
	encrypted, err := encryptSecret([]byte(value), m.SecretsKey, service)
	if err != nil {
		return err
	}
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

// ValidateServiceKeyStringCtx validates a comma-separated service key value before
// saving it. It rejects clearly-malformed input (empty entries when a value was
// provided, duplicates, whitespace garbage) and performs a live check against
// the provider for a definitive answer. Network/provider errors and rate limits
// are tolerated (the key is accepted) — only a clear 401/403 rejects the key.
func ValidateServiceKeyStringCtx(ctx context.Context, service, value string, httpClient *http.Client) error {
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
		if err := ValidateServiceKeyCtx(ctx, service, k, httpClient); err != nil {
			return err
		}
	}
	return nil
}

// ValidateServiceKeyString validates a comma-separated service key value before
// saving it. It rejects clearly-malformed input (empty entries when a value was
// provided, duplicates, whitespace garbage) and performs a live check against
// the provider for a definitive answer. Network/provider errors and rate limits
// are tolerated (the key is accepted) — only a clear 401/403 rejects the key.
func ValidateServiceKeyString(service, value string, httpClient *http.Client) error {
	return ValidateServiceKeyStringCtx(context.Background(), service, value, httpClient)
}

// ValidateServiceKeyCtx performs a live check against the provider for a single key.
// Only a definitive 401/403 is treated as invalid; network errors, 5xx, and rate
// limits are tolerated so flaky providers don't block saving a good key.
func ValidateServiceKeyCtx(ctx context.Context, service, key string, httpClient *http.Client) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if len(key) < 6 {
		return fmt.Errorf("key is too short (%d chars)", len(key))
	}

	switch service {
	case "tmdb":
		return checkProviderURLCtx(ctx, httpClient, "https://api.themoviedb.org/3/configuration?api_key="+key, "TMDB")
	case "omdb":
		return checkProviderURLCtx(ctx, httpClient, "https://www.omdbapi.com/?apikey="+key+"&i=tt3896198", "OMDb")
	case "mdblist":
		return checkProviderURLCtx(ctx, httpClient, "https://api.mdblist.com/tmdb/movie/1?apikey="+key, "MDBList")
	case "fanart":
		return checkProviderURLCtx(ctx, httpClient, "https://webservice.fanart.tv/v3/movies/550?api_key="+key, "Fanart.tv")
	case "trakt":
		return checkTraktClientIDCtx(ctx, httpClient, key)
	}
	return nil
}

// ValidateServiceKey performs a live check against the provider for a single key.
// Only a definitive 401/403 is treated as invalid; network errors, 5xx, and rate
// limits are tolerated so flaky providers don't block saving a good key.
func ValidateServiceKey(service, key string, httpClient *http.Client) error {
	return ValidateServiceKeyCtx(context.Background(), service, key, httpClient)
}

func checkProviderURLCtx(ctx context.Context, httpClient *http.Client, url, service string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func checkProviderURL(httpClient *http.Client, url, service string) error {
	return checkProviderURLCtx(context.Background(), httpClient, url, service)
}

func checkTraktClientIDCtx(ctx context.Context, httpClient *http.Client, clientID string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.trakt.tv/movies/tt3896198", nil)
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

func checkTraktClientID(httpClient *http.Client, clientID string) error {
	return checkTraktClientIDCtx(context.Background(), httpClient, clientID)
}
