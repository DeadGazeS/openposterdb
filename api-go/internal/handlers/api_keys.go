package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

func HandleListKeys(db *sql.DB, secretsKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		keys, err := services.ListAPIKeysCtx(r.Context(), db)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to list keys")
			return
		}

		type keyResp struct {
			ID         int64   `json:"id"`
			Name       string  `json:"name"`
			KeyPrefix  string  `json:"key_prefix"`
			// Key holds the raw key, decrypted from the api_keys.encrypted_key
			// column. Empty when the row pre-dates the encrypted_key column
			// (raw is gone with the dismissed create banner) — the frontend
			// shows the prefix-only fallback for those rows.
			Key        string  `json:"key,omitempty"`
			CreatedAt  string  `json:"created_at"`
			LastUsedAt *string `json:"last_used_at"`
		}

		result := make([]keyResp, len(keys))
		for i, k := range keys {
			row := keyResp{
				ID:         k.ID,
				Name:       k.Name,
				KeyPrefix:  k.KeyPrefix,
				CreatedAt:  k.CreatedAt,
				LastUsedAt: k.LastUsedAt,
			}
			if k.EncryptedKey != nil && *k.EncryptedKey != "" {
				raw, decErr := services.DecryptAPIKey(*k.EncryptedKey, secretsKey)
				if decErr != nil {
					slog.Error("failed to decrypt api key for list response", "id", k.ID, "error", decErr)
				} else {
					row.Key = raw
				}
			}
			result[i] = row
		}

		httpx.WriteJSON(w, 200, result)
	}
}

func HandleCreateKey(db *sql.DB, secretsKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, 400, "invalid request body")
			return
		}
		if err := services.ValidateAPIKeyName(body.Name); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}

		raw, hash, prefix := services.GenerateAPIKey()
		encrypted, err := services.EncryptAPIKey(raw, secretsKey)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to encrypt key")
			return
		}

		var createdBy int64 = 1
		id, err := services.CreateAPIKeyCtx(r.Context(), db, body.Name, hash, prefix, encrypted, createdBy)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to create key")
			return
		}

		httpx.WriteJSON(w, 201, map[string]any{
			"id":         id,
			"name":       body.Name,
			"key":        raw,
			"key_prefix": prefix,
		})
	}
}

func HandleDeleteKey(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			httpx.WriteError(w, 400, "invalid id")
			return
		}

		if err := services.DeleteAPIKeyCtx(r.Context(), db, id); err != nil {
			httpx.WriteError(w, 500, "Failed to delete key")
			return
		}

		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}

// perKeySettingsResponse wraps the effective render settings with the
// fanart_available flag the shared settings form needs, matching the global
// settings GET response shape (admin.go HandleGetSettings).
type perKeySettingsResponse struct {
	services.RenderSettings
	FanartAvailable bool `json:"fanart_available"`
}

func HandleGetKeySettings(db *sql.DB, fanartAvailable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			httpx.WriteError(w, 400, "invalid id")
			return
		}

		settings := services.GetEffectiveRenderSettingsCtx(r.Context(), db, id, nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perKeySettingsResponse{settings, fanartAvailable})
	}
}

// validateKeySettingsLayouts validates the four per-key layout fields. Each
// field is a JSON string (the DB stores layouts as text). ValidateLayout runs
// on the parsed ImageLayout, but UnmarshalLayout silently falls back to an
// empty layout on malformed JSON, so the raw string is checked with json.Valid
// first to reject garbage that would otherwise sail through validation and
// corrupt rendering.
func validateKeySettingsLayouts(s *services.APIKeySettings) error {
	fields := []struct {
		name string
		raw  string
	}{
		{"poster_layout", s.PosterLayout},
		{"logo_layout", s.LogoLayout},
		{"backdrop_layout", s.BackdropLayout},
		{"episode_layout", s.EpisodeLayout},
	}
	for _, f := range fields {
		if f.raw == "" {
			continue
		}
		if !json.Valid([]byte(f.raw)) {
			return fmt.Errorf("invalid %s: not valid JSON", f.name)
		}
		l := services.UnmarshalLayout(f.raw, nil)
		if err := services.ValidateLayout(&l); err != nil {
			return fmt.Errorf("invalid %s: %w", f.name, err)
		}
	}
	return nil
}

// keySettingsUpdate is the partial-update payload for per-key settings. Every
// numeric/bool field the web client might omit is a pointer so an omitted
// field preserves the stored value while an explicit 0 (e.g. a ratings limit
// of 0 meaning "no ratings") is honoured. Everything else decodes into Body
// via APIKeySettings' layout-normalising UnmarshalJSON.
//
// The pointer fields here mirror the int32/bool columns of APIKeySettings; the
// pattern closes the #10.2 latent bug where Body's non-pointer fields zeroed
// out any omitted column on upsert (badge_size fires today, the rest are
// "loaded weapons" against any future partial-payload path).
type keySettingsUpdate struct {
	// Ratings + badge direction (existing pointer preservation).
	RatingsLimit           *int32                   `json:"ratings_limit"`
	LogoRatingsLimit       *int32                   `json:"logo_ratings_limit"`
	BackdropRatingsLimit   *int32                   `json:"backdrop_ratings_limit"`
	EpisodeRatingsLimit    *int32                   `json:"episode_ratings_limit"`
	PosterBadgeDirection   *services.BadgeDirection `json:"poster_badge_direction"`
	BackdropBadgeDirection *services.BadgeDirection `json:"backdrop_badge_direction"`
	EpisodeBadgeDirection  *services.BadgeDirection `json:"episode_badge_direction"`

	// Bool columns (#10.2).
	Textless    *bool `json:"textless"`
	EpisodeBlur *bool `json:"episode_blur"`

	// Text/badge/logo/badge-alpha sizes (#10.1 badge_size + #10.2 the rest).
	PosterTextSize    *int32 `json:"poster_text_size"`
	LogoTextSize      *int32 `json:"logo_text_size"`
	BackdropTextSize  *int32 `json:"backdrop_text_size"`
	EpisodeTextSize   *int32 `json:"episode_text_size"`
	PosterBadgeSize   *int32 `json:"poster_badge_size"`
	LogoBadgeSize     *int32 `json:"logo_badge_size"`
	BackdropBadgeSize *int32 `json:"backdrop_badge_size"`
	EpisodeBadgeSize  *int32 `json:"episode_badge_size"`
	PosterLogoSize    *int32 `json:"poster_logo_size"`
	LogoLogoSize      *int32 `json:"logo_logo_size"`
	BackdropLogoSize  *int32 `json:"backdrop_logo_size"`
	EpisodeLogoSize   *int32 `json:"episode_logo_size"`
	PosterBadgeAlpha  *int32 `json:"poster_badge_alpha"`
	LogoBadgeAlpha    *int32 `json:"logo_badge_alpha"`
	BackdropBadgeAlpha *int32 `json:"backdrop_badge_alpha"`
	EpisodeBadgeAlpha *int32 `json:"episode_badge_alpha"`

	// Badge per-axis dimensions (#10.2).
	PosterBadgeWidth    *int32 `json:"poster_badge_width"`
	PosterBadgeHeight   *int32 `json:"poster_badge_height"`
	LogoBadgeWidth      *int32 `json:"logo_badge_width"`
	LogoBadgeHeight     *int32 `json:"logo_badge_height"`
	BackdropBadgeWidth  *int32 `json:"backdrop_badge_width"`
	BackdropBadgeHeight *int32 `json:"backdrop_badge_height"`
	EpisodeBadgeWidth   *int32 `json:"episode_badge_width"`
	EpisodeBadgeHeight  *int32 `json:"episode_badge_height"`

	// Edge insets (#10.2).
	BackdropEdgeInsetX *int32 `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY *int32 `json:"backdrop_edge_inset_y"`

	Body services.APIKeySettings `json:"-"`
}

func (u *keySettingsUpdate) UnmarshalJSON(data []byte) error {
	// First pass: capture every preservable field as a pointer so an absent
	// JSON key leaves the pointer nil and the merge can fall back to base.
	// Second pass: decode the full payload into Body (custom UnmarshalJSON
	// normalises the four layouts + colors into their stored JSON-string form).
	var p struct {
		RatingsLimit           *int32                   `json:"ratings_limit"`
		LogoRatingsLimit       *int32                   `json:"logo_ratings_limit"`
		BackdropRatingsLimit   *int32                   `json:"backdrop_ratings_limit"`
		EpisodeRatingsLimit    *int32                   `json:"episode_ratings_limit"`
		PosterBadgeDirection   *services.BadgeDirection `json:"poster_badge_direction"`
		BackdropBadgeDirection *services.BadgeDirection `json:"backdrop_badge_direction"`
		EpisodeBadgeDirection  *services.BadgeDirection `json:"episode_badge_direction"`
		Textless               *bool                    `json:"textless"`
		EpisodeBlur            *bool                    `json:"episode_blur"`
		PosterTextSize         *int32                   `json:"poster_text_size"`
		LogoTextSize           *int32                   `json:"logo_text_size"`
		BackdropTextSize       *int32                   `json:"backdrop_text_size"`
		EpisodeTextSize        *int32                   `json:"episode_text_size"`
		PosterBadgeSize        *int32                   `json:"poster_badge_size"`
		LogoBadgeSize          *int32                   `json:"logo_badge_size"`
		BackdropBadgeSize      *int32                   `json:"backdrop_badge_size"`
		EpisodeBadgeSize       *int32                   `json:"episode_badge_size"`
		PosterLogoSize         *int32                   `json:"poster_logo_size"`
		LogoLogoSize           *int32                   `json:"logo_logo_size"`
		BackdropLogoSize       *int32                   `json:"backdrop_logo_size"`
		EpisodeLogoSize        *int32                   `json:"episode_logo_size"`
		PosterBadgeAlpha       *int32                   `json:"poster_badge_alpha"`
		LogoBadgeAlpha         *int32                   `json:"logo_badge_alpha"`
		BackdropBadgeAlpha     *int32                   `json:"backdrop_badge_alpha"`
		EpisodeBadgeAlpha      *int32                   `json:"episode_badge_alpha"`
		PosterBadgeWidth       *int32                   `json:"poster_badge_width"`
		PosterBadgeHeight      *int32                   `json:"poster_badge_height"`
		LogoBadgeWidth         *int32                   `json:"logo_badge_width"`
		LogoBadgeHeight        *int32                   `json:"logo_badge_height"`
		BackdropBadgeWidth     *int32                   `json:"backdrop_badge_width"`
		BackdropBadgeHeight    *int32                   `json:"backdrop_badge_height"`
		EpisodeBadgeWidth      *int32                   `json:"episode_badge_width"`
		EpisodeBadgeHeight     *int32                   `json:"episode_badge_height"`
		BackdropEdgeInsetX     *int32                   `json:"backdrop_edge_inset_x"`
		BackdropEdgeInsetY     *int32                   `json:"backdrop_edge_inset_y"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*u = keySettingsUpdate{
		RatingsLimit:           p.RatingsLimit,
		LogoRatingsLimit:       p.LogoRatingsLimit,
		BackdropRatingsLimit:   p.BackdropRatingsLimit,
		EpisodeRatingsLimit:    p.EpisodeRatingsLimit,
		PosterBadgeDirection:   p.PosterBadgeDirection,
		BackdropBadgeDirection: p.BackdropBadgeDirection,
		EpisodeBadgeDirection:  p.EpisodeBadgeDirection,
		Textless:               p.Textless,
		EpisodeBlur:            p.EpisodeBlur,
		PosterTextSize:         p.PosterTextSize,
		LogoTextSize:           p.LogoTextSize,
		BackdropTextSize:       p.BackdropTextSize,
		EpisodeTextSize:        p.EpisodeTextSize,
		PosterBadgeSize:        p.PosterBadgeSize,
		LogoBadgeSize:          p.LogoBadgeSize,
		BackdropBadgeSize:      p.BackdropBadgeSize,
		EpisodeBadgeSize:       p.EpisodeBadgeSize,
		PosterLogoSize:         p.PosterLogoSize,
		LogoLogoSize:           p.LogoLogoSize,
		BackdropLogoSize:       p.BackdropLogoSize,
		EpisodeLogoSize:        p.EpisodeLogoSize,
		PosterBadgeAlpha:       p.PosterBadgeAlpha,
		LogoBadgeAlpha:         p.LogoBadgeAlpha,
		BackdropBadgeAlpha:     p.BackdropBadgeAlpha,
		EpisodeBadgeAlpha:      p.EpisodeBadgeAlpha,
		PosterBadgeWidth:       p.PosterBadgeWidth,
		PosterBadgeHeight:      p.PosterBadgeHeight,
		LogoBadgeWidth:         p.LogoBadgeWidth,
		LogoBadgeHeight:        p.LogoBadgeHeight,
		BackdropBadgeWidth:     p.BackdropBadgeWidth,
		BackdropBadgeHeight:    p.BackdropBadgeHeight,
		EpisodeBadgeWidth:      p.EpisodeBadgeWidth,
		EpisodeBadgeHeight:     p.EpisodeBadgeHeight,
		BackdropEdgeInsetX:     p.BackdropEdgeInsetX,
		BackdropEdgeInsetY:     p.BackdropEdgeInsetY,
	}
	return json.Unmarshal(data, &u.Body)
}

// apiKeySettingsFromEffective maps the effective render settings (for a key
// with no stored row these are the globals-derived defaults) onto a per-key
// settings row. The JSON tags align between the two structs, and
// APIKeySettings' UnmarshalJSON normalises the layout objects to their stored
// JSON-string form.
func apiKeySettingsFromEffective(ctx context.Context, db *sql.DB, apiKeyID int64) (*services.APIKeySettings, error) {
  eff := services.GetEffectiveRenderSettingsCtx(ctx, db, apiKeyID, nil)
  data, err := json.Marshal(&eff)
  if err != nil {
    // Marshal+Unmarshal converts RenderSettings → APIKeySettings so the merge
    // step has zero-valued defaults for fields the client omitted. If either
    // step fails (e.g. an unmarshallable shape slipped into the effective
    // settings, or a regression in the custom UnmarshalJSON) we'd rather
    // persist only the client's payload than 500 the whole PUT and leave the
    // user locked out of saving. Log and return a zero-valued row so
    // mergeKeySettingsUpdate starts from empty defaults; the frontend's
    // Body still carries every field it sent and lands in merged.
    slog.Warn("apiKeySettingsFromEffective: marshal failed, falling back to empty defaults", "api_key_id", apiKeyID, "err", err)
    return &services.APIKeySettings{}, nil
  }
  var s services.APIKeySettings
  if err := json.Unmarshal(data, &s); err != nil {
    slog.Warn("apiKeySettingsFromEffective: convert failed, falling back to empty defaults", "api_key_id", apiKeyID, "err", err)
    return &services.APIKeySettings{}, nil
  }
  return &s, nil
}

// loadKeySettingsBase returns the stored settings row for a key, or the
// effective (globals-derived) defaults when no row exists yet, so omitted
// optional fields on a first save get meaningful values instead of zeroes.
func loadKeySettingsBase(ctx context.Context, db *sql.DB, apiKeyID int64) (*services.APIKeySettings, error) {
  base, err := services.GetAPIKeySettingsCtx(ctx, db, apiKeyID)
  if err != nil {
    // The DB row read failed (transient connection issue, schema drift, or a
    // row with corrupt stored values that failed to scan). Rather than 500
    // the PUT and lock the user out of saving, log and fall through to
    // apiKeySettingsFromEffective, which will also fall back to empty defaults
    // if its own conversion trips. The merge then layers the client's full
    // payload over whatever base comes back, so the user's intended values
    // still land in the row.
    slog.Warn("loadKeySettingsBase: row read failed, treating as no row", "api_key_id", apiKeyID, "err", err)
    return apiKeySettingsFromEffective(ctx, db, apiKeyID)
  }
  if base == nil {
    return apiKeySettingsFromEffective(ctx, db, apiKeyID)
  }
  return base, nil
}

// mergeKeySettingsUpdate overlays the client payload onto the stored (or
// effective-default) settings: omitted optional fields keep the stored value,
// an explicit 0 for a numeric field or `false` for a bool is honoured, and all
// other fields take the payload's values (the web client always sends them).
// String/enum fields not declared as pointers in keySettingsUpdate decode via
// Body and use the payload's value directly — those fields are always sent by
// the web client.
func mergeKeySettingsUpdate(base *services.APIKeySettings, body *keySettingsUpdate) services.APIKeySettings {
	merged := body.Body

	// Ratings limits (#10.2 already-pointer + explicit-0 honoured).
	if body.RatingsLimit != nil {
		merged.RatingsLimit = *body.RatingsLimit
	} else {
		merged.RatingsLimit = base.RatingsLimit
	}
	if body.LogoRatingsLimit != nil {
		merged.LogoRatingsLimit = *body.LogoRatingsLimit
	} else {
		merged.LogoRatingsLimit = base.LogoRatingsLimit
	}
	if body.BackdropRatingsLimit != nil {
		merged.BackdropRatingsLimit = *body.BackdropRatingsLimit
	} else {
		merged.BackdropRatingsLimit = base.BackdropRatingsLimit
	}
	if body.EpisodeRatingsLimit != nil {
		merged.EpisodeRatingsLimit = *body.EpisodeRatingsLimit
	} else {
		merged.EpisodeRatingsLimit = base.EpisodeRatingsLimit
	}

	// Badge directions (#10.2 already-pointer).
	if body.PosterBadgeDirection != nil {
		merged.PosterBadgeDirection = string(*body.PosterBadgeDirection)
	} else {
		merged.PosterBadgeDirection = base.PosterBadgeDirection
	}
	if body.BackdropBadgeDirection != nil {
		merged.BackdropBadgeDirection = string(*body.BackdropBadgeDirection)
	} else {
		merged.BackdropBadgeDirection = base.BackdropBadgeDirection
	}
	if body.EpisodeBadgeDirection != nil {
		merged.EpisodeBadgeDirection = string(*body.EpisodeBadgeDirection)
	} else {
		merged.EpisodeBadgeDirection = base.EpisodeBadgeDirection
	}

	// Bools (#10.2).
	if body.Textless != nil {
		merged.Textless = *body.Textless
	} else {
		merged.Textless = base.Textless
	}
	if body.EpisodeBlur != nil {
		merged.EpisodeBlur = *body.EpisodeBlur
	} else {
		merged.EpisodeBlur = base.EpisodeBlur
	}

	// Text sizes (#10.2).
	if body.PosterTextSize != nil {
		merged.PosterTextSize = *body.PosterTextSize
	} else {
		merged.PosterTextSize = base.PosterTextSize
	}
	if body.LogoTextSize != nil {
		merged.LogoTextSize = *body.LogoTextSize
	} else {
		merged.LogoTextSize = base.LogoTextSize
	}
	if body.BackdropTextSize != nil {
		merged.BackdropTextSize = *body.BackdropTextSize
	} else {
		merged.BackdropTextSize = base.BackdropTextSize
	}
	if body.EpisodeTextSize != nil {
		merged.EpisodeTextSize = *body.EpisodeTextSize
	} else {
		merged.EpisodeTextSize = base.EpisodeTextSize
	}

	// Badge sizes (#10.1 — the field that fires today: first save on any
	// per-key surface previously zeroed all four badge_sizes → clamped to 50).
	if body.PosterBadgeSize != nil {
		merged.PosterBadgeSize = *body.PosterBadgeSize
	} else {
		merged.PosterBadgeSize = base.PosterBadgeSize
	}
	if body.LogoBadgeSize != nil {
		merged.LogoBadgeSize = *body.LogoBadgeSize
	} else {
		merged.LogoBadgeSize = base.LogoBadgeSize
	}
	if body.BackdropBadgeSize != nil {
		merged.BackdropBadgeSize = *body.BackdropBadgeSize
	} else {
		merged.BackdropBadgeSize = base.BackdropBadgeSize
	}
	if body.EpisodeBadgeSize != nil {
		merged.EpisodeBadgeSize = *body.EpisodeBadgeSize
	} else {
		merged.EpisodeBadgeSize = base.EpisodeBadgeSize
	}

	// Logo sizes (#10.2).
	if body.PosterLogoSize != nil {
		merged.PosterLogoSize = *body.PosterLogoSize
	} else {
		merged.PosterLogoSize = base.PosterLogoSize
	}
	if body.LogoLogoSize != nil {
		merged.LogoLogoSize = *body.LogoLogoSize
	} else {
		merged.LogoLogoSize = base.LogoLogoSize
	}
	if body.BackdropLogoSize != nil {
		merged.BackdropLogoSize = *body.BackdropLogoSize
	} else {
		merged.BackdropLogoSize = base.BackdropLogoSize
	}
	if body.EpisodeLogoSize != nil {
		merged.EpisodeLogoSize = *body.EpisodeLogoSize
	} else {
		merged.EpisodeLogoSize = base.EpisodeLogoSize
	}

	// Badge alphas (#10.2).
	if body.PosterBadgeAlpha != nil {
		merged.PosterBadgeAlpha = *body.PosterBadgeAlpha
	} else {
		merged.PosterBadgeAlpha = base.PosterBadgeAlpha
	}
	if body.LogoBadgeAlpha != nil {
		merged.LogoBadgeAlpha = *body.LogoBadgeAlpha
	} else {
		merged.LogoBadgeAlpha = base.LogoBadgeAlpha
	}
	if body.BackdropBadgeAlpha != nil {
		merged.BackdropBadgeAlpha = *body.BackdropBadgeAlpha
	} else {
		merged.BackdropBadgeAlpha = base.BackdropBadgeAlpha
	}
	if body.EpisodeBadgeAlpha != nil {
		merged.EpisodeBadgeAlpha = *body.EpisodeBadgeAlpha
	} else {
		merged.EpisodeBadgeAlpha = base.EpisodeBadgeAlpha
	}

	// Badge per-axis dimensions (#10.2).
	if body.PosterBadgeWidth != nil {
		merged.PosterBadgeWidth = *body.PosterBadgeWidth
	} else {
		merged.PosterBadgeWidth = base.PosterBadgeWidth
	}
	if body.PosterBadgeHeight != nil {
		merged.PosterBadgeHeight = *body.PosterBadgeHeight
	} else {
		merged.PosterBadgeHeight = base.PosterBadgeHeight
	}
	if body.LogoBadgeWidth != nil {
		merged.LogoBadgeWidth = *body.LogoBadgeWidth
	} else {
		merged.LogoBadgeWidth = base.LogoBadgeWidth
	}
	if body.LogoBadgeHeight != nil {
		merged.LogoBadgeHeight = *body.LogoBadgeHeight
	} else {
		merged.LogoBadgeHeight = base.LogoBadgeHeight
	}
	if body.BackdropBadgeWidth != nil {
		merged.BackdropBadgeWidth = *body.BackdropBadgeWidth
	} else {
		merged.BackdropBadgeWidth = base.BackdropBadgeWidth
	}
	if body.BackdropBadgeHeight != nil {
		merged.BackdropBadgeHeight = *body.BackdropBadgeHeight
	} else {
		merged.BackdropBadgeHeight = base.BackdropBadgeHeight
	}
	if body.EpisodeBadgeWidth != nil {
		merged.EpisodeBadgeWidth = *body.EpisodeBadgeWidth
	} else {
		merged.EpisodeBadgeWidth = base.EpisodeBadgeWidth
	}
	if body.EpisodeBadgeHeight != nil {
		merged.EpisodeBadgeHeight = *body.EpisodeBadgeHeight
	} else {
		merged.EpisodeBadgeHeight = base.EpisodeBadgeHeight
	}

	// Edge insets (#10.2).
	if body.BackdropEdgeInsetX != nil {
		merged.BackdropEdgeInsetX = *body.BackdropEdgeInsetX
	} else {
		merged.BackdropEdgeInsetX = base.BackdropEdgeInsetX
	}
	if body.BackdropEdgeInsetY != nil {
		merged.BackdropEdgeInsetY = *body.BackdropEdgeInsetY
	} else {
		merged.BackdropEdgeInsetY = base.BackdropEdgeInsetY
	}

	if merged.Colors == "" {
		// Omitted colours keep the stored value (the web client always sends
		// them, so this only affects partial/legacy payloads).
		merged.Colors = base.Colors
	}
	return merged
}

// validateAndNormalizeKeySettings validates the merged per-key settings like
// the global path does (lang, ratings order/exclude, limits, layouts) and
// normalises the enum fields via their parsers plus the global clamps so
// garbage input can't corrupt the stored row.
func validateAndNormalizeKeySettings(s *services.APIKeySettings) error {
	if err := services.ValidateLang(s.Lang); err != nil {
		return err
	}
	if err := services.ValidateRatingsOrder(s.RatingsOrder); err != nil {
		return err
	}
	if err := services.ValidateRatingsExclude(s.RatingsExclude); err != nil {
		return err
	}
	for _, v := range []struct {
		name string
		val  int32
	}{
		{"ratings_limit", s.RatingsLimit},
		{"logo_ratings_limit", s.LogoRatingsLimit},
		{"backdrop_ratings_limit", s.BackdropRatingsLimit},
		{"episode_ratings_limit", s.EpisodeRatingsLimit},
	} {
		if err := services.ValidateRatingsLimit(v.val); err != nil {
			return fmt.Errorf("%s: %w", v.name, err)
		}
	}
	if err := validateKeySettingsLayouts(s); err != nil {
		return err
	}

	// Normalise enums (garbage falls back to a valid default) and clamp the
	// numeric ranges, mirroring the global settings path.
	s.PosterBadgeStyle = string(services.ParseBadgeStyle(s.PosterBadgeStyle))
	s.LogoBadgeStyle = string(services.ParseBadgeStyle(s.LogoBadgeStyle))
	s.BackdropBadgeStyle = string(services.ParseBadgeStyle(s.BackdropBadgeStyle))
	s.EpisodeBadgeStyle = string(services.ParseBadgeStyle(s.EpisodeBadgeStyle))
	s.PosterLabelStyle = string(services.ParseLabelStyle(s.PosterLabelStyle))
	s.LogoLabelStyle = string(services.ParseLabelStyle(s.LogoLabelStyle))
	s.BackdropLabelStyle = string(services.ParseLabelStyle(s.BackdropLabelStyle))
	s.EpisodeLabelStyle = string(services.ParseLabelStyle(s.EpisodeLabelStyle))
	s.PosterBadgeShape = string(services.ParseBadgeShape(s.PosterBadgeShape))
	s.LogoBadgeShape = string(services.ParseBadgeShape(s.LogoBadgeShape))
	s.BackdropBadgeShape = string(services.ParseBadgeShape(s.BackdropBadgeShape))
	s.EpisodeBadgeShape = string(services.ParseBadgeShape(s.EpisodeBadgeShape))
	// Per-axis badge width/height clamp (50-400), mirroring the global path.
	s.PosterBadgeWidth = int32(services.ClampScalePercent(s.PosterBadgeWidth))
	s.PosterBadgeHeight = int32(services.ClampScalePercent(s.PosterBadgeHeight))
	s.LogoBadgeWidth = int32(services.ClampScalePercent(s.LogoBadgeWidth))
	s.LogoBadgeHeight = int32(services.ClampScalePercent(s.LogoBadgeHeight))
	s.BackdropBadgeWidth = int32(services.ClampScalePercent(s.BackdropBadgeWidth))
	s.BackdropBadgeHeight = int32(services.ClampScalePercent(s.BackdropBadgeHeight))
	s.EpisodeBadgeWidth = int32(services.ClampScalePercent(s.EpisodeBadgeWidth))
	s.EpisodeBadgeHeight = int32(services.ClampScalePercent(s.EpisodeBadgeHeight))
	s.PosterBadgeDirection = string(services.ParseBadgeDirection(s.PosterBadgeDirection))
	s.BackdropBadgeDirection = string(services.ParseBadgeDirection(s.BackdropBadgeDirection))
	s.EpisodeBadgeDirection = string(services.ParseBadgeDirection(s.EpisodeBadgeDirection))
	s.PosterBadgeAlpha = int32(services.ClampBadgeAlpha(s.PosterBadgeAlpha))
	s.LogoBadgeAlpha = int32(services.ClampBadgeAlpha(s.LogoBadgeAlpha))
	s.BackdropBadgeAlpha = int32(services.ClampBadgeAlpha(s.BackdropBadgeAlpha))
	s.EpisodeBadgeAlpha = int32(services.ClampBadgeAlpha(s.EpisodeBadgeAlpha))
	s.PosterTextSize = int32(services.ClampScalePercent(s.PosterTextSize))
	s.LogoTextSize = int32(services.ClampScalePercent(s.LogoTextSize))
	s.BackdropTextSize = int32(services.ClampScalePercent(s.BackdropTextSize))
	s.EpisodeTextSize = int32(services.ClampScalePercent(s.EpisodeTextSize))
	s.PosterBadgeSize = int32(services.ClampScalePercent(s.PosterBadgeSize))
	s.LogoBadgeSize = int32(services.ClampScalePercent(s.LogoBadgeSize))
	s.BackdropBadgeSize = int32(services.ClampScalePercent(s.BackdropBadgeSize))
	s.EpisodeBadgeSize = int32(services.ClampScalePercent(s.EpisodeBadgeSize))
	s.PosterLogoSize = int32(services.ClampScalePercent(s.PosterLogoSize))
	s.LogoLogoSize = int32(services.ClampScalePercent(s.LogoLogoSize))
	s.BackdropLogoSize = int32(services.ClampScalePercent(s.BackdropLogoSize))
	s.EpisodeLogoSize = int32(services.ClampScalePercent(s.EpisodeLogoSize))
	s.BackdropEdgeInsetX = services.ClampEdgeInset(s.BackdropEdgeInsetX)
	s.BackdropEdgeInsetY = services.ClampEdgeInset(s.BackdropEdgeInsetY)

	// Validate and normalise per-source colours, mirroring the global path
	// (admin.go): reject unknown sources / invalid hex, strip default-valued
	// entries so only real overrides are stored.
	if s.Colors != "" {
		var colors map[string]services.SourceColorSet
		if err := json.Unmarshal([]byte(s.Colors), &colors); err != nil {
			return fmt.Errorf("invalid colors: not valid JSON")
		}
		if err := services.ValidateSourceColors(colors); err != nil {
			return err
		}
		normalized := services.NormalizeSourceColors(colors)
		out, err := json.Marshal(normalized)
		if err != nil {
			return fmt.Errorf("invalid colors: %w", err)
		}
		s.Colors = string(out)
	}
	return nil
}

func HandleUpdateKeySettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			httpx.WriteError(w, 400, "invalid id")
			return
		}

		var body keySettingsUpdate
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}
		base, err := loadKeySettingsBase(r.Context(), db, id)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to load settings")
			return
		}
		merged := mergeKeySettingsUpdate(base, &body)
		merged.APIKeyID = id
		if err := validateAndNormalizeKeySettings(&merged); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}
		if err := services.UpsertAPIKeySettingsCtx(r.Context(), db, &merged); err != nil {
			// Log the actual DB error server-side and surface a useful message
			// to the client so the admin per-key edit form shows something
			// diagnostic instead of the generic "Failed to update settings".
			// Safe to expose on a self-hosted admin surface — same origin,
			// same operator.
			slog.Warn("HandleUpdateKeySettings: upsert failed", "api_key_id", id, "err", err)
			httpx.WriteError(w, 500, fmt.Sprintf("Failed to update settings: %v", err))
			return
		}

		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleResetKeySettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			httpx.WriteError(w, 400, "invalid id")
			return
		}

		if err := services.DeleteAPIKeySettingsCtx(r.Context(), db, id); err != nil {
			httpx.WriteError(w, 500, "Failed to reset settings")
			return
		}

		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleSelfKeyInfo(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			httpx.WriteError(w, 401, "Unauthorized")
			return
		}

		k, err := services.FindAPIKeyByIDCtx(r.Context(), db, apiUser.KeyID)
		if err != nil || k == nil {
			httpx.WriteError(w, 404, "Not found")
			return
		}

		httpx.WriteJSON(w, 200, map[string]any{
			"name":       k.Name,
			"key_prefix": k.KeyPrefix,
		})
	}
}

func HandleSelfSettings(db *sql.DB, fanartAvailable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			httpx.WriteError(w, 401, "Unauthorized")
			return
		}

		settings := services.GetEffectiveRenderSettingsCtx(r.Context(), db, apiUser.KeyID, nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perKeySettingsResponse{settings, fanartAvailable})
	}
}

func HandleUpdateSelfSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			httpx.WriteError(w, 401, "Unauthorized")
			return
		}

		var body keySettingsUpdate
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}
		base, err := loadKeySettingsBase(r.Context(), db, apiUser.KeyID)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to load settings")
			return
		}
		merged := mergeKeySettingsUpdate(base, &body)
		merged.APIKeyID = apiUser.KeyID
		if err := validateAndNormalizeKeySettings(&merged); err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}
		if err := services.UpsertAPIKeySettingsCtx(r.Context(), db, &merged); err != nil {
			// Log the actual DB error server-side and surface a useful message
			// to the client so the per-key form shows something diagnostic in
			// formRef.error instead of the generic "Failed to update settings".
			// The wrapped detail is safe to expose on a self-hosted admin
			// surface — same origin, same operator.
			slog.Warn("HandleUpdateSelfSettings: upsert failed", "api_key_id", apiUser.KeyID, "err", err)
			httpx.WriteError(w, 500, fmt.Sprintf("Failed to update settings: %v", err))
			return
		}

		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}

func HandleResetSelfSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		apiUser := GetAPIKeyUser(r)
		if apiUser == nil {
			httpx.WriteError(w, 401, "Unauthorized")
			return
		}

		if err := services.DeleteAPIKeySettingsCtx(r.Context(), db, apiUser.KeyID); err != nil {
			httpx.WriteError(w, 500, "Failed to reset settings")
			return
		}

		httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
	}
}
