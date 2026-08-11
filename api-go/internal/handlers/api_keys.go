package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

func HandleListKeys(db *sql.DB) http.HandlerFunc {
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
			CreatedAt  string  `json:"created_at"`
			LastUsedAt *string `json:"last_used_at"`
		}

		result := make([]keyResp, len(keys))
		for i, k := range keys {
			result[i] = keyResp{
				ID:         k.ID,
				Name:       k.Name,
				KeyPrefix:  k.KeyPrefix,
				CreatedAt:  k.CreatedAt,
				LastUsedAt: k.LastUsedAt,
			}
		}

		httpx.WriteJSON(w, 200, result)
	}
}

func HandleCreateKey(db *sql.DB) http.HandlerFunc {
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

		var createdBy int64 = 1
		id, err := services.CreateAPIKeyCtx(r.Context(), db, body.Name, hash, prefix, createdBy)
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

// keySettingsUpdate is the partial-update payload for per-key settings. The
// four ratings limits and the three badge directions the web client omits are
// pointers so an omitted field preserves the stored value while an explicit 0
// for a limit is honoured. Everything else decodes into Body via
// APIKeySettings' layout-normalising UnmarshalJSON.
type keySettingsUpdate struct {
	RatingsLimit           *int32                   `json:"ratings_limit"`
	LogoRatingsLimit       *int32                   `json:"logo_ratings_limit"`
	BackdropRatingsLimit   *int32                   `json:"backdrop_ratings_limit"`
	EpisodeRatingsLimit    *int32                   `json:"episode_ratings_limit"`
	PosterBadgeDirection   *services.BadgeDirection `json:"poster_badge_direction"`
	BackdropBadgeDirection *services.BadgeDirection `json:"backdrop_badge_direction"`
	EpisodeBadgeDirection  *services.BadgeDirection `json:"episode_badge_direction"`
	Body                   services.APIKeySettings  `json:"-"`
}

func (u *keySettingsUpdate) UnmarshalJSON(data []byte) error {
	var p struct {
		RatingsLimit           *int32                   `json:"ratings_limit"`
		LogoRatingsLimit       *int32                   `json:"logo_ratings_limit"`
		BackdropRatingsLimit   *int32                   `json:"backdrop_ratings_limit"`
		EpisodeRatingsLimit    *int32                   `json:"episode_ratings_limit"`
		PosterBadgeDirection   *services.BadgeDirection `json:"poster_badge_direction"`
		BackdropBadgeDirection *services.BadgeDirection `json:"backdrop_badge_direction"`
		EpisodeBadgeDirection  *services.BadgeDirection `json:"episode_badge_direction"`
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
		return nil, err
	}
	var s services.APIKeySettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// loadKeySettingsBase returns the stored settings row for a key, or the
// effective (globals-derived) defaults when no row exists yet, so omitted
// optional fields on a first save get meaningful values instead of zeroes.
func loadKeySettingsBase(ctx context.Context, db *sql.DB, apiKeyID int64) (*services.APIKeySettings, error) {
	base, err := services.GetAPIKeySettingsCtx(ctx, db, apiKeyID)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return apiKeySettingsFromEffective(ctx, db, apiKeyID)
	}
	return base, nil
}

// mergeKeySettingsUpdate overlays the client payload onto the stored (or
// effective-default) settings: omitted optional fields keep the stored value,
// an explicit 0 for a ratings limit is honoured, and all other fields take the
// payload's values (the web client always sends them).
func mergeKeySettingsUpdate(base *services.APIKeySettings, body *keySettingsUpdate) services.APIKeySettings {
	merged := body.Body
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
			httpx.WriteError(w, 500, "Failed to update settings")
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
			httpx.WriteError(w, 500, "Failed to update settings")
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
