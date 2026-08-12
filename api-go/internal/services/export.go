package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// ExportPayload is the JSON structure used for settings backup/restore. The
// `settings` map holds every global_settings row except the encrypted service
// keys, which are exported (decrypted) separately under `service_keys` when the
// user opts in. API key values are only ever stored as hashes, so exporting an
// API key records its name + per-key settings; importing recreates it with a
// fresh key value.
type ExportPayload struct {
	Kind        string            `json:"kind"`
	Version     int               `json:"version"`
	ExportedAt  string            `json:"exported_at"`
	Settings    map[string]string `json:"settings"`
	ServiceKeys map[string]string `json:"service_keys,omitempty"`
	APIKeys     []ExportedAPIKey  `json:"api_keys,omitempty"`
}

// EncryptedExport wraps an ExportPayload with a passphrase-derived AES-256-GCM
// seal when the user opts in. The on-disk file carries `kind =
// "openposterdb/settings-encrypted"` so the import endpoint can route it
// through DecryptPayload instead of treating it as plain JSON.
type EncryptedExport struct {
	Kind        string `json:"kind"`
	Version     int    `json:"version"`
	KDF         string `json:"kdf"`
	KDFIterations int  `json:"kdf_iterations"`
	Salt        string `json:"salt"`
	Ciphertext  string `json:"ciphertext"`
}

// pbkdf2Iterations is the iteration count for the passphrase-derived KEK.
// 600k is the OWASP 2023 recommendation for PBKDF2-HMAC-SHA256.
const pbkdf2Iterations = 600_000

// EncryptPayload seals an ExportPayload with a passphrase-derived AES-256-GCM
// key. The passphrase is required to decrypt on import; the file itself carries
// only the salt + ciphertext, so leaking the file without the passphrase is
// inert. PBKDF2-HMAC-SHA256 with 600k iterations is the 2023 OWASP floor.
func EncryptPayload(payload *ExportPayload, passphrase string) (*EncryptedExport, error) {
	if passphrase == "" {
		return nil, errors.New("passphrase must not be empty")
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plain, nil)
	return &EncryptedExport{
		Kind:          "openposterdb/settings-encrypted",
		Version:       1,
		KDF:           "pbkdf2-sha256",
		KDFIterations: pbkdf2Iterations,
		Salt:          base64.StdEncoding.EncodeToString(salt),
		Ciphertext:    base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// DecryptPayload is the inverse of EncryptPayload. Returns an error on a wrong
// passphrase (GCM auth failure) so the import endpoint can surface a clear
// message instead of a generic JSON parse error.
func DecryptPayload(enc *EncryptedExport, passphrase string) (*ExportPayload, error) {
	if enc == nil {
		return nil, errors.New("nil encrypted payload")
	}
	if passphrase == "" {
		return nil, errors.New("passphrase required for encrypted export")
	}
	salt, err := base64.StdEncoding.DecodeString(enc.Salt)
	if err != nil {
		return nil, fmt.Errorf("salt base64: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(enc.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("ciphertext base64: %w", err)
	}
	iter := enc.KDFIterations
	if iter <= 0 {
		iter = pbkdf2Iterations
	}
	key := pbkdf2.Key([]byte(passphrase), salt, iter, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ct) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, sealed := ct[:nonceSize], ct[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, errors.New("decryption failed — wrong passphrase or corrupted file")
	}
	var payload ExportPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("decrypted payload is not valid JSON: %w", err)
	}
	return &payload, nil
}

// ExportedAPIKey is one API key's importable metadata + per-key settings.
type ExportedAPIKey struct {
	Name     string          `json:"name"`
	Settings *APIKeySettings `json:"settings"`
}

// RegeneratedKey reports a freshly-created API key value so the admin can
// distribute it (existing key values cannot be recovered from their hash).
type RegeneratedKey struct {
	Name      string `json:"name"`
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
}

// ImportResult summarises what ApplyImportPayload restored.
type ImportResult struct {
	RestoredSettings int              `json:"restored_settings"`
	RestoredKeys     int              `json:"restored_keys"`
	RegeneratedKeys  []RegeneratedKey `json:"regenerated_keys"`
}

// GenerateAPIKey creates a new random API key and returns its raw value, hash,
// and 8-char prefix. The raw value is only returned once (at creation).
func GenerateAPIKey() (raw, hash, prefix string) {
	b := make([]byte, 32)
	rand.Read(b)
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	prefix = raw[:8]
	return
}

// HashAPIKey returns the SHA-256 hex hash used to store API keys.
func HashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// BuildExportPayloadCtx collects the current settings into an export payload.
// Service keys are decrypted and included only when includeServiceKeys is set;
// API keys (name + per-key settings) only when includeAPIKeys is set.
func BuildExportPayloadCtx(ctx context.Context, db *sql.DB, keys *ServiceKeyManager, includeServiceKeys, includeAPIKeys bool) (*ExportPayload, error) {
	globals, err := GetGlobalSettingsCtx(ctx, db)
	if err != nil {
		return nil, err
	}
	settings := make(map[string]string, len(globals))
	for k, v := range globals {
		if strings.HasPrefix(k, "service_key_") {
			continue
		}
		settings[k] = v
	}
	p := &ExportPayload{
		Kind:       "openposterdb/settings",
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Settings:   settings,
	}
	if includeServiceKeys && keys != nil {
		p.ServiceKeys = keys.PlaintextKeys()
	}
	if includeAPIKeys {
		apiKeys, err := ListAPIKeysCtx(ctx, db)
		if err != nil {
			return nil, err
		}
		for _, k := range apiKeys {
			ek := ExportedAPIKey{Name: k.Name}
			if s, err := GetAPIKeySettingsCtx(ctx, db, k.ID); err == nil && s != nil {
				ek.Settings = s
			}
			p.APIKeys = append(p.APIKeys, ek)
		}
	}
	return p, nil
}

// BuildExportPayload collects the current settings into an export payload.
// Service keys are decrypted and included only when includeServiceKeys is set;
// API keys (name + per-key settings) only when includeAPIKeys is set.
func BuildExportPayload(db *sql.DB, keys *ServiceKeyManager, includeServiceKeys, includeAPIKeys bool) (*ExportPayload, error) {
	return BuildExportPayloadCtx(context.Background(), db, keys, includeServiceKeys, includeAPIKeys)
}

// ApplyImportPayloadCtx restores a settings export. Global settings are upserted
// (service_key_* rows are ignored — they're handled below), service keys are
// re-encrypted and stored for non-env-locked services, and each exported API
// key is matched by name (updating its settings) or recreated with a fresh key
// value. Returns a summary including any regenerated key values.
func ApplyImportPayloadCtx(ctx context.Context, db *sql.DB, keys *ServiceKeyManager, p *ExportPayload) (ImportResult, error) {
	var result ImportResult
	if p == nil {
		return result, nil
	}

	if len(p.Settings) > 0 {
		batch := make(map[string]string, len(p.Settings))
		for k, v := range p.Settings {
			if strings.HasPrefix(k, "service_key_") {
				continue
			}
			batch[k] = v
		}
		if err := SetGlobalSettingsBatchCtx(ctx, db, batch); err != nil {
			return result, err
		}
		result.RestoredSettings = len(batch)
	}

	if len(p.ServiceKeys) > 0 && keys != nil {
		update := &ServiceKeysUpdate{}
		if v, ok := p.ServiceKeys["tmdb"]; ok {
			update.TMDB = &v
		}
		if v, ok := p.ServiceKeys["mdblist"]; ok {
			update.MDBList = &v
		}
		if v, ok := p.ServiceKeys["omdb"]; ok {
			update.OMDB = &v
		}
		if v, ok := p.ServiceKeys["fanart"]; ok {
			update.Fanart = &v
		}
		if v, ok := p.ServiceKeys["trakt"]; ok {
			update.Trakt = &v
		}
		if err := keys.UpdateKeys(update); err != nil {
			return result, err
		}
		result.RestoredKeys = len(p.ServiceKeys)
	}

	for _, ek := range p.APIKeys {
		existing, err := FindAPIKeyByNameCtx(ctx, db, ek.Name)
		if err != nil {
			return result, err
		}
		var id int64
		if existing != nil {
			id = existing.ID
		} else {
			raw, hash, prefix := GenerateAPIKey()
			// Encrypt the regenerated raw key so the admin can reveal it from
			// the API Keys view after restore (same model as fresh creates).
			encrypted := ""
			if keys != nil && keys.SecretsKey != nil {
				enc, encErr := EncryptAPIKey(raw, keys.SecretsKey)
				if encErr != nil {
					slog.Error("failed to encrypt regenerated api key", "name", ek.Name, "error", encErr)
				} else {
					encrypted = enc
				}
			}
			newID, err := CreateAPIKeyCtx(ctx, db, ek.Name, hash, prefix, encrypted, 1)
			if err != nil {
				return result, err
			}
			id = newID
			result.RegeneratedKeys = append(result.RegeneratedKeys, RegeneratedKey{Name: ek.Name, Key: raw, KeyPrefix: prefix})
		}
		if ek.Settings != nil {
			ek.Settings.APIKeyID = id
			if err := UpsertAPIKeySettingsCtx(ctx, db, ek.Settings); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

// ApplyImportPayload restores a settings export. Global settings are upserted
// (service_key_* rows are ignored — they're handled below), service keys are
// re-encrypted and stored for non-env-locked services, and each exported API
// key is matched by name (updating its settings) or recreated with a fresh key
// value. Returns a summary including any regenerated key values.
func ApplyImportPayload(db *sql.DB, keys *ServiceKeyManager, p *ExportPayload) (ImportResult, error) {
	return ApplyImportPayloadCtx(context.Background(), db, keys, p)
}
