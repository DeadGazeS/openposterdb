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
//
// Version is the export-format version: bump whenever the per-key settings
// shape changes in a way that would lose data on import. The import path uses
// MinExportVersion to detect pre-fix exports (e.g. v1 didn't carry badge_size
// /badge_alpha — the 2026-08-14 outage class — so v1 imports merge with the
// existing row instead of zero-filling).
type ExportPayload struct {
	Kind        string            `json:"kind"`
	Version     int               `json:"version"`
	ExportedAt  string            `json:"exported_at"`
	Settings    map[string]string `json:"settings"`
	ServiceKeys map[string]string `json:"service_keys,omitempty"`
	APIKeys     []ExportedAPIKey  `json:"api_keys,omitempty"`
}

// MinExportVersion is the lowest export-format version the current import path
// can read without losing data. Exports with Version < MinExportVersion trigger
// a non-zero overlay merge (preserve existing settings on any field absent
// from the export) instead of a zero-fill upsert.
const MinExportVersion = 2

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
		Version:    MinExportVersion,
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
			// #10.4: a pre-fix (Version < MinExportVersion) export was produced
			// before badge_size / badge_alpha / edge_inset columns existed in
			// api_key_settings; zero-filling the upsert with those fields at 0
			// would silently clobber the user's stored values (the
			// 2026-08-14 outage class). Merge onto the existing row instead:
			// only fields the import carries a non-zero/non-empty value for
			// are overlaid; everything else preserves the stored value.
			if p.Version < MinExportVersion {
				if existingSettings, gerr := GetAPIKeySettingsCtx(ctx, db, id); gerr == nil && existingSettings != nil {
					slog.Warn("ApplyImportPayloadCtx: pre-fix export version detected, merging per-key settings onto existing row to avoid zero-fill",
						"api_key_id", id, "name", ek.Name, "export_version", p.Version, "min_version", MinExportVersion)
					ek.Settings = overlayAPIKeySettingsNonZero(existingSettings, ek.Settings)
				}
			}
			if err := UpsertAPIKeySettingsCtx(ctx, db, ek.Settings); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

// overlayAPIKeySettingsNonZero overlays `over` onto `base`, treating zero /
// empty / false in `over` as "preserve the base value" rather than "set to
// zero". Used by ApplyImportPayloadCtx for pre-fix (#10.4) imports where the
// export doesn't carry every schema column (notably badge_size, badge_alpha,
// edge_inset — the 2026-08-14 outage class) so the user doesn't lose stored
// values on a round-trip. Strings use `""` as the absent sentinel; enums that
// intentionally have an empty-string meaning (none in this struct) would
// collide, so the helper is opt-in only on the old-version import path.
func overlayAPIKeySettingsNonZero(base, over *APIKeySettings) *APIKeySettings {
	if base == nil {
		return over
	}
	if over == nil {
		return base
	}
	merged := *base
	if over.ImageSource != "" {
		merged.ImageSource = over.ImageSource
	}
	if over.Lang != "" {
		merged.Lang = over.Lang
	}
	if over.Textless {
		merged.Textless = over.Textless
	}
	if over.RatingsLimit != 0 {
		merged.RatingsLimit = over.RatingsLimit
	}
	if over.RatingsOrder != "" {
		merged.RatingsOrder = over.RatingsOrder
	}
	if over.RatingsExclude != "" {
		merged.RatingsExclude = over.RatingsExclude
	}
	if over.PosterLayout != "" {
		merged.PosterLayout = over.PosterLayout
	}
	if over.LogoRatingsLimit != 0 {
		merged.LogoRatingsLimit = over.LogoRatingsLimit
	}
	if over.BackdropRatingsLimit != 0 {
		merged.BackdropRatingsLimit = over.BackdropRatingsLimit
	}
	if over.PosterBadgeStyle != "" {
		merged.PosterBadgeStyle = over.PosterBadgeStyle
	}
	if over.LogoBadgeStyle != "" {
		merged.LogoBadgeStyle = over.LogoBadgeStyle
	}
	if over.BackdropBadgeStyle != "" {
		merged.BackdropBadgeStyle = over.BackdropBadgeStyle
	}
	if over.PosterLabelStyle != "" {
		merged.PosterLabelStyle = over.PosterLabelStyle
	}
	if over.LogoLabelStyle != "" {
		merged.LogoLabelStyle = over.LogoLabelStyle
	}
	if over.BackdropLabelStyle != "" {
		merged.BackdropLabelStyle = over.BackdropLabelStyle
	}
	if over.PosterBadgeDirection != "" {
		merged.PosterBadgeDirection = over.PosterBadgeDirection
	}
	if over.PosterFit != "" {
		merged.PosterFit = over.PosterFit
	}
	if over.PosterTextSize != 0 {
		merged.PosterTextSize = over.PosterTextSize
	}
	if over.LogoTextSize != 0 {
		merged.LogoTextSize = over.LogoTextSize
	}
	if over.BackdropTextSize != 0 {
		merged.BackdropTextSize = over.BackdropTextSize
	}
	if over.PosterBadgeSize != 0 {
		merged.PosterBadgeSize = over.PosterBadgeSize
	}
	if over.LogoBadgeSize != 0 {
		merged.LogoBadgeSize = over.LogoBadgeSize
	}
	if over.BackdropBadgeSize != 0 {
		merged.BackdropBadgeSize = over.BackdropBadgeSize
	}
	if over.PosterBadgeWidth != 0 {
		merged.PosterBadgeWidth = over.PosterBadgeWidth
	}
	if over.PosterBadgeHeight != 0 {
		merged.PosterBadgeHeight = over.PosterBadgeHeight
	}
	if over.LogoBadgeWidth != 0 {
		merged.LogoBadgeWidth = over.LogoBadgeWidth
	}
	if over.LogoBadgeHeight != 0 {
		merged.LogoBadgeHeight = over.LogoBadgeHeight
	}
	if over.BackdropBadgeWidth != 0 {
		merged.BackdropBadgeWidth = over.BackdropBadgeWidth
	}
	if over.BackdropBadgeHeight != 0 {
		merged.BackdropBadgeHeight = over.BackdropBadgeHeight
	}
	if over.EpisodeBadgeWidth != 0 {
		merged.EpisodeBadgeWidth = over.EpisodeBadgeWidth
	}
	if over.EpisodeBadgeHeight != 0 {
		merged.EpisodeBadgeHeight = over.EpisodeBadgeHeight
	}
	if over.PosterLogoSize != 0 {
		merged.PosterLogoSize = over.PosterLogoSize
	}
	if over.LogoLogoSize != 0 {
		merged.LogoLogoSize = over.LogoLogoSize
	}
	if over.BackdropLogoSize != 0 {
		merged.BackdropLogoSize = over.BackdropLogoSize
	}
	if over.LogoLayout != "" {
		merged.LogoLayout = over.LogoLayout
	}
	if over.BackdropLayout != "" {
		merged.BackdropLayout = over.BackdropLayout
	}
	if over.BackdropBadgeDirection != "" {
		merged.BackdropBadgeDirection = over.BackdropBadgeDirection
	}
	if over.BackdropEdgeInsetX != 0 {
		merged.BackdropEdgeInsetX = over.BackdropEdgeInsetX
	}
	if over.BackdropEdgeInsetY != 0 {
		merged.BackdropEdgeInsetY = over.BackdropEdgeInsetY
	}
	if over.EpisodeRatingsLimit != 0 {
		merged.EpisodeRatingsLimit = over.EpisodeRatingsLimit
	}
	if over.EpisodeBadgeStyle != "" {
		merged.EpisodeBadgeStyle = over.EpisodeBadgeStyle
	}
	if over.EpisodeLabelStyle != "" {
		merged.EpisodeLabelStyle = over.EpisodeLabelStyle
	}
	if over.EpisodeTextSize != 0 {
		merged.EpisodeTextSize = over.EpisodeTextSize
	}
	if over.EpisodeBadgeSize != 0 {
		merged.EpisodeBadgeSize = over.EpisodeBadgeSize
	}
	if over.EpisodeLogoSize != 0 {
		merged.EpisodeLogoSize = over.EpisodeLogoSize
	}
	if over.EpisodeLayout != "" {
		merged.EpisodeLayout = over.EpisodeLayout
	}
	if over.EpisodeBadgeDirection != "" {
		merged.EpisodeBadgeDirection = over.EpisodeBadgeDirection
	}
	if over.EpisodeBlur {
		merged.EpisodeBlur = over.EpisodeBlur
	}
	if over.PosterBadgeShape != "" {
		merged.PosterBadgeShape = over.PosterBadgeShape
	}
	if over.LogoBadgeShape != "" {
		merged.LogoBadgeShape = over.LogoBadgeShape
	}
	if over.BackdropBadgeShape != "" {
		merged.BackdropBadgeShape = over.BackdropBadgeShape
	}
	if over.EpisodeBadgeShape != "" {
		merged.EpisodeBadgeShape = over.EpisodeBadgeShape
	}
	if over.PosterBadgeAlpha != 0 {
		merged.PosterBadgeAlpha = over.PosterBadgeAlpha
	}
	if over.LogoBadgeAlpha != 0 {
		merged.LogoBadgeAlpha = over.LogoBadgeAlpha
	}
	if over.BackdropBadgeAlpha != 0 {
		merged.BackdropBadgeAlpha = over.BackdropBadgeAlpha
	}
	if over.EpisodeBadgeAlpha != 0 {
		merged.EpisodeBadgeAlpha = over.EpisodeBadgeAlpha
	}
	if over.Colors != "" {
		merged.Colors = over.Colors
	}
	return &merged
}

// ApplyImportPayload restores a settings export. Global settings are upserted
// (service_key_* rows are ignored — they're handled below), service keys are
// re-encrypted and stored for non-env-locked services, and each exported API
// key is matched by name (updating its settings) or recreated with a fresh key
// value. Returns a summary including any regenerated key values.
func ApplyImportPayload(db *sql.DB, keys *ServiceKeyManager, p *ExportPayload) (ImportResult, error) {
	return ApplyImportPayloadCtx(context.Background(), db, keys, p)
}
