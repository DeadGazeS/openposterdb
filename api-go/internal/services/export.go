package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
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
			newID, err := CreateAPIKeyCtx(ctx, db, ek.Name, hash, prefix, 1)
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
