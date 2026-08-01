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
	"net/http"
	"strings"
	"sync"

	"golang.org/x/crypto/hkdf"
)

func deriveCipherKey(jwtSecret []byte) []byte {
	hkdf := hkdf.New(sha256.New, jwtSecret, nil, []byte("openposterdb-service-keys"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, key); err != nil {
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
	n := len(key) / 4
	if n > 4 {
		n = 4
	}
	if n == 0 {
		return "****"
	}
	prefix := key[:n]
	suffixRunes := []rune(key)
	for i, j := 0, len(suffixRunes)-1; i < j; i, j = i+1, j-1 {
		suffixRunes[i], suffixRunes[j] = suffixRunes[j], suffixRunes[i]
	}
	suffix := string(suffixRunes[:n])
	suffixRunes = []rune(suffix)
	for i, j := 0, len(suffixRunes)-1; i < j; i, j = i+1, j-1 {
		suffixRunes[i], suffixRunes[j] = suffixRunes[j], suffixRunes[i]
	}
	return fmt.Sprintf("%s...%s", prefix, string(suffixRunes))
}

type keyCache struct {
	mdblist []string
	omdb    []string
	fanart  []string
	trakt   []string
}

type ServiceKeyManager struct {
	DB          *sql.DB
	GCM         cipher.AEAD
	HTTP        *http.Client
	Keys        keyCache
	mu          sync.RWMutex
	EnvMDBList  bool
	EnvOMDB     bool
	EnvFanart   bool
	EnvTrakt    bool
}

type ServiceKeyStatus struct {
	Locked  bool    `json:"locked"`
	HasKey  bool    `json:"has_key"`
	Masked  *string `json:"masked"`
}

type ServiceKeysResponse struct {
	MDBList ServiceKeyStatus `json:"mdblist"`
	OMDB    ServiceKeyStatus `json:"omdb"`
	Fanart  ServiceKeyStatus `json:"fanart"`
	Trakt   ServiceKeyStatus `json:"trakt"`
}

type ServiceKeysUpdate struct {
	MDBList *string `json:"mdblist"`
	OMDB    *string `json:"omdb"`
	Fanart  *string `json:"fanart"`
	Trakt   *string `json:"trakt"`
}

func NewServiceKeyManager(db *sql.DB, jwtSecret []byte, httpClient *http.Client,
	envMDBList []string, envOMDB, envFanart, envTrakt string) *ServiceKeyManager {

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
	envOMDBLocked := envOMDB != ""
	envFanartLocked := envFanart != ""
	envTraktLocked := envTrakt != ""

	if envOMDBLocked {
		cache.omdb = []string{envOMDB}
	}
	if envFanartLocked {
		cache.fanart = []string{envFanart}
	}
	if envTraktLocked {
		cache.trakt = []string{envTrakt}
	}

	return &ServiceKeyManager{
		DB:         db,
		GCM:        gcm,
		HTTP:       httpClient,
		Keys:       cache,
		EnvMDBList: envMDBListLocked,
		EnvOMDB:    envOMDBLocked,
		EnvFanart:  envFanartLocked,
		EnvTrakt:   envTraktLocked,
	}
}

func (m *ServiceKeyManager) Init() {
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
	for _, k := range strings.Split(plain, ",") {
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

func (m *ServiceKeyManager) GetStatus() ServiceKeysResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ServiceKeysResponse{
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
	return ServiceKeyStatus{Locked: locked, HasKey: hasKey, Masked: masked}
}

func (m *ServiceKeyManager) UpdateKeys(update *ServiceKeysUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	for _, k := range strings.Split(keys, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			result = append(result, k)
		}
	}
	return result
}
