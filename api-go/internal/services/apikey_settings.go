package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"maps"
	"strings"
)

type APIKeySettings struct {
	APIKeyID               int64  `json:"api_key_id"`
	ImageSource            string `json:"image_source"`
	Lang                   string `json:"lang"`
	Textless               bool   `json:"textless"`
	RatingsLimit           int32  `json:"ratings_limit"`
	RatingsOrder           string `json:"ratings_order"`
	RatingsExclude         string `json:"ratings_exclude"`
	PosterLayout           string `json:"poster_layout"`
	LogoRatingsLimit       int32  `json:"logo_ratings_limit"`
	BackdropRatingsLimit   int32  `json:"backdrop_ratings_limit"`
	PosterBadgeStyle       string `json:"poster_badge_style"`
	LogoBadgeStyle         string `json:"logo_badge_style"`
	BackdropBadgeStyle     string `json:"backdrop_badge_style"`
	PosterLabelStyle       string `json:"poster_label_style"`
	LogoLabelStyle         string `json:"logo_label_style"`
	BackdropLabelStyle     string `json:"backdrop_label_style"`
	PosterBadgeDirection   string `json:"poster_badge_direction"`
	PosterFit              string `json:"poster_fit"`
	PosterTextSize         int32  `json:"poster_text_size"`
	LogoTextSize           int32  `json:"logo_text_size"`
	BackdropTextSize       int32  `json:"backdrop_text_size"`
	PosterBadgeSize        int32  `json:"poster_badge_size"`
	LogoBadgeSize          int32  `json:"logo_badge_size"`
	BackdropBadgeSize      int32  `json:"backdrop_badge_size"`
	PosterBadgeWidth       int32  `json:"poster_badge_width"`
	PosterBadgeHeight      int32  `json:"poster_badge_height"`
	LogoBadgeWidth         int32  `json:"logo_badge_width"`
	LogoBadgeHeight        int32  `json:"logo_badge_height"`
	BackdropBadgeWidth     int32  `json:"backdrop_badge_width"`
	BackdropBadgeHeight    int32  `json:"backdrop_badge_height"`
	EpisodeBadgeWidth      int32  `json:"episode_badge_width"`
	EpisodeBadgeHeight     int32  `json:"episode_badge_height"`
	PosterLogoSize         int32  `json:"poster_logo_size"`
	LogoLogoSize           int32  `json:"logo_logo_size"`
	BackdropLogoSize       int32  `json:"backdrop_logo_size"`
	LogoLayout             string `json:"logo_layout"`
	BackdropLayout         string `json:"backdrop_layout"`
	BackdropBadgeDirection string `json:"backdrop_badge_direction"`
	EpisodeRatingsLimit    int32  `json:"episode_ratings_limit"`
	EpisodeBadgeStyle      string `json:"episode_badge_style"`
	EpisodeLabelStyle      string `json:"episode_label_style"`
	EpisodeTextSize        int32  `json:"episode_text_size"`
	EpisodeBadgeSize       int32  `json:"episode_badge_size"`
	EpisodeLogoSize        int32  `json:"episode_logo_size"`
	EpisodeLayout          string `json:"episode_layout"`
	EpisodeBadgeDirection  string `json:"episode_badge_direction"`
	EpisodeBlur            bool   `json:"episode_blur"`
	PosterBadgeShape       string `json:"poster_badge_shape"`
	LogoBadgeShape         string `json:"logo_badge_shape"`
	BackdropBadgeShape     string `json:"backdrop_badge_shape"`
	EpisodeBadgeShape      string `json:"episode_badge_shape"`
	PosterBadgeAlpha       int32  `json:"poster_badge_alpha"`
	LogoBadgeAlpha         int32  `json:"logo_badge_alpha"`
	BackdropBadgeAlpha     int32  `json:"backdrop_badge_alpha"`
	EpisodeBadgeAlpha      int32  `json:"episode_badge_alpha"`
	BackdropEdgeInsetX     int32  `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY     int32  `json:"backdrop_edge_inset_y"`
	Colors                 string `json:"colors"`
}

// apiKeySettingsAlias strips the custom UnmarshalJSON from APIKeySettings so
// the plain string layout fields can be decoded without recursion.
type apiKeySettingsAlias APIKeySettings

// layoutJSONField normalises one layout JSON field (poster_layout etc.) from
// either form the client may send: a legacy JSON string holding the layout's
// JSON text, or the layout itself as a JSON object. The object form is
// validated by decoding it as ImageLayout and stored as its compact JSON
// string, because the api_key_settings.poster_layout/logo_layout/backdrop_layout/
// episode_layout DB columns are TEXT and are scanned/bound as strings. Empty
// and "null" values map to "" (the caller's upsert writes what is present and
// reads back full rows).
func layoutJSONField(raw json.RawMessage) (string, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var l ImageLayout
	if err := json.Unmarshal(raw, &l); err != nil {
		return "", err
	}
	return string(raw), nil
}

// colorsJSONField normalises the colors field from either form the client may
// send: a JSON object mapping colour keys to SourceColorSet values, or a legacy
// JSON string holding that object's text. The object form is validated by
// decoding it and stored as its compact JSON string, because the
// api_key_settings.colors DB column is TEXT and is scanned/bound as a string.
// Empty and "null" values map to "" (meaning "no per-key colour overrides").
func colorsJSONField(raw json.RawMessage) (string, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var m map[string]SourceColorSet
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// UnmarshalJSON accepts the four layout fields in either form: a legacy JSON
// string ({"top": ...} as text) or a JSON object. The explicit outer
// json.RawMessage fields shadow the embedded alias fields with the same json
// tags during decode, so each layout is captured raw and normalised via
// layoutJSONField while all other fields decode straight into the embedded
// alias (which points at the receiver).
func (s *APIKeySettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		*apiKeySettingsAlias
		PosterLayout   json.RawMessage `json:"poster_layout"`
		LogoLayout     json.RawMessage `json:"logo_layout"`
		BackdropLayout json.RawMessage `json:"backdrop_layout"`
		EpisodeLayout  json.RawMessage `json:"episode_layout"`
		Colors         json.RawMessage `json:"colors"`
	}
	raw.apiKeySettingsAlias = (*apiKeySettingsAlias)(s)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	fields := []struct {
		raw  json.RawMessage
		dst  *string
		norm func(json.RawMessage) (string, error)
	}{
		{raw.PosterLayout, &s.PosterLayout, layoutJSONField},
		{raw.LogoLayout, &s.LogoLayout, layoutJSONField},
		{raw.BackdropLayout, &s.BackdropLayout, layoutJSONField},
		{raw.EpisodeLayout, &s.EpisodeLayout, layoutJSONField},
		{raw.Colors, &s.Colors, colorsJSONField},
	}
	for _, f := range fields {
		v, err := f.norm(f.raw)
		if err != nil {
			return err
		}
		*f.dst = v
	}
	return nil
}

func GetAPIKeySettingsCtx(ctx context.Context, db *sql.DB, apiKeyID int64) (*APIKeySettings, error) {
	var s APIKeySettings
	err := db.QueryRowContext(ctx, `SELECT
		api_key_id, image_source, lang, textless, ratings_limit, ratings_order, ratings_exclude,
		poster_layout, logo_ratings_limit, backdrop_ratings_limit,
		poster_badge_style, logo_badge_style, backdrop_badge_style,
		poster_label_style, logo_label_style, backdrop_label_style,
		poster_badge_direction, poster_fit,
		poster_text_size, logo_text_size, backdrop_text_size,
		poster_badge_size, logo_badge_size, backdrop_badge_size,
		poster_badge_width, poster_badge_height,
		logo_badge_width, logo_badge_height,
		backdrop_badge_width, backdrop_badge_height,
		poster_logo_size, logo_logo_size, backdrop_logo_size,
		logo_layout,
		backdrop_layout, backdrop_badge_direction,
		episode_ratings_limit, episode_badge_style, episode_label_style, episode_text_size,
		episode_badge_size, episode_badge_width, episode_badge_height, episode_logo_size,
		episode_layout, episode_badge_direction, episode_blur,
		poster_badge_shape, logo_badge_shape, backdrop_badge_shape, episode_badge_shape,
		poster_badge_alpha, logo_badge_alpha, backdrop_badge_alpha, episode_badge_alpha,
		backdrop_edge_inset_x, backdrop_edge_inset_y, colors
		FROM api_key_settings WHERE api_key_id = ?`, apiKeyID).Scan(
		&s.APIKeyID, &s.ImageSource, &s.Lang, &s.Textless, &s.RatingsLimit, &s.RatingsOrder, &s.RatingsExclude,
		&s.PosterLayout, &s.LogoRatingsLimit, &s.BackdropRatingsLimit,
		&s.PosterBadgeStyle, &s.LogoBadgeStyle, &s.BackdropBadgeStyle,
		&s.PosterLabelStyle, &s.LogoLabelStyle, &s.BackdropLabelStyle,
		&s.PosterBadgeDirection, &s.PosterFit,
		&s.PosterTextSize, &s.LogoTextSize, &s.BackdropTextSize,
		&s.PosterBadgeSize, &s.LogoBadgeSize, &s.BackdropBadgeSize,
		&s.PosterBadgeWidth, &s.PosterBadgeHeight,
		&s.LogoBadgeWidth, &s.LogoBadgeHeight,
		&s.BackdropBadgeWidth, &s.BackdropBadgeHeight,
		&s.PosterLogoSize, &s.LogoLogoSize, &s.BackdropLogoSize,
		&s.LogoLayout,
		&s.BackdropLayout, &s.BackdropBadgeDirection,
		&s.EpisodeRatingsLimit, &s.EpisodeBadgeStyle, &s.EpisodeLabelStyle, &s.EpisodeTextSize,
		&s.EpisodeBadgeSize, &s.EpisodeBadgeWidth, &s.EpisodeBadgeHeight, &s.EpisodeLogoSize,
		&s.EpisodeLayout, &s.EpisodeBadgeDirection, &s.EpisodeBlur,
		&s.PosterBadgeShape, &s.LogoBadgeShape, &s.BackdropBadgeShape, &s.EpisodeBadgeShape,
		&s.PosterBadgeAlpha, &s.LogoBadgeAlpha, &s.BackdropBadgeAlpha, &s.EpisodeBadgeAlpha,
		&s.BackdropEdgeInsetX, &s.BackdropEdgeInsetY, &s.Colors,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func GetAPIKeySettings(db *sql.DB, apiKeyID int64) (*APIKeySettings, error) {
	return GetAPIKeySettingsCtx(context.Background(), db, apiKeyID)
}

func UpsertAPIKeySettingsCtx(ctx context.Context, db *sql.DB, s *APIKeySettings) error {
	_, err := db.ExecContext(ctx, `INSERT INTO api_key_settings (
		api_key_id, image_source, lang, textless, ratings_limit, ratings_order, ratings_exclude,
		poster_layout, logo_ratings_limit, backdrop_ratings_limit,
		poster_badge_style, logo_badge_style, backdrop_badge_style,
		poster_label_style, logo_label_style, backdrop_label_style,
		poster_badge_direction, poster_fit,
		poster_text_size, logo_text_size, backdrop_text_size,
		poster_badge_size, logo_badge_size, backdrop_badge_size,
		poster_badge_width, poster_badge_height,
		logo_badge_width, logo_badge_height,
		backdrop_badge_width, backdrop_badge_height,
		poster_logo_size, logo_logo_size, backdrop_logo_size,
		logo_layout,
		backdrop_layout, backdrop_badge_direction,
		episode_ratings_limit, episode_badge_style, episode_label_style, episode_text_size,
		episode_badge_size, episode_badge_width, episode_badge_height, episode_logo_size,
		episode_layout, episode_badge_direction, episode_blur,
		poster_badge_shape, logo_badge_shape, backdrop_badge_shape, episode_badge_shape,
		poster_badge_alpha, logo_badge_alpha, backdrop_badge_alpha, episode_badge_alpha,
		backdrop_edge_inset_x, backdrop_edge_inset_y, colors
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(api_key_id) DO UPDATE SET
		image_source = excluded.image_source,
		lang = excluded.lang,
		textless = excluded.textless,
		ratings_limit = excluded.ratings_limit,
		ratings_order = excluded.ratings_order,
		ratings_exclude = excluded.ratings_exclude,
		poster_layout = excluded.poster_layout,
		logo_ratings_limit = excluded.logo_ratings_limit,
		backdrop_ratings_limit = excluded.backdrop_ratings_limit,
		poster_badge_style = excluded.poster_badge_style,
		logo_badge_style = excluded.logo_badge_style,
		backdrop_badge_style = excluded.backdrop_badge_style,
		poster_label_style = excluded.poster_label_style,
		logo_label_style = excluded.logo_label_style,
		backdrop_label_style = excluded.backdrop_label_style,
		poster_badge_direction = excluded.poster_badge_direction,
		poster_fit = excluded.poster_fit,
		poster_text_size = excluded.poster_text_size,
		logo_text_size = excluded.logo_text_size,
		backdrop_text_size = excluded.backdrop_text_size,
		poster_badge_size = excluded.poster_badge_size,
		logo_badge_size = excluded.logo_badge_size,
		backdrop_badge_size = excluded.backdrop_badge_size,
		poster_badge_width = excluded.poster_badge_width,
		poster_badge_height = excluded.poster_badge_height,
		logo_badge_width = excluded.logo_badge_width,
		logo_badge_height = excluded.logo_badge_height,
		backdrop_badge_width = excluded.backdrop_badge_width,
		backdrop_badge_height = excluded.backdrop_badge_height,
		poster_logo_size = excluded.poster_logo_size,
		logo_logo_size = excluded.logo_logo_size,
		backdrop_logo_size = excluded.backdrop_logo_size,
		logo_layout = excluded.logo_layout,
		backdrop_layout = excluded.backdrop_layout,
		backdrop_badge_direction = excluded.backdrop_badge_direction,
		episode_ratings_limit = excluded.episode_ratings_limit,
		episode_badge_style = excluded.episode_badge_style,
		episode_label_style = excluded.episode_label_style,
		episode_text_size = excluded.episode_text_size,
		episode_badge_size = excluded.episode_badge_size,
		episode_badge_width = excluded.episode_badge_width,
		episode_badge_height = excluded.episode_badge_height,
		episode_logo_size = excluded.episode_logo_size,
		episode_layout = excluded.episode_layout,
		episode_badge_direction = excluded.episode_badge_direction,
		episode_blur = excluded.episode_blur,
		poster_badge_shape = excluded.poster_badge_shape,
		logo_badge_shape = excluded.logo_badge_shape,
		backdrop_badge_shape = excluded.backdrop_badge_shape,
		episode_badge_shape = excluded.episode_badge_shape,
		poster_badge_alpha = excluded.poster_badge_alpha,
		logo_badge_alpha = excluded.logo_badge_alpha,
		backdrop_badge_alpha = excluded.backdrop_badge_alpha,
		episode_badge_alpha = excluded.episode_badge_alpha,
		backdrop_edge_inset_x = excluded.backdrop_edge_inset_x,
		backdrop_edge_inset_y = excluded.backdrop_edge_inset_y,
		colors = excluded.colors`,
		s.APIKeyID, s.ImageSource, s.Lang, s.Textless, s.RatingsLimit, s.RatingsOrder, s.RatingsExclude,
		s.PosterLayout, s.LogoRatingsLimit, s.BackdropRatingsLimit,
		s.PosterBadgeStyle, s.LogoBadgeStyle, s.BackdropBadgeStyle,
		s.PosterLabelStyle, s.LogoLabelStyle, s.BackdropLabelStyle,
		s.PosterBadgeDirection, s.PosterFit,
		s.PosterTextSize, s.LogoTextSize, s.BackdropTextSize,
		s.PosterBadgeSize, s.LogoBadgeSize, s.BackdropBadgeSize,
		s.PosterBadgeWidth, s.PosterBadgeHeight,
		s.LogoBadgeWidth, s.LogoBadgeHeight,
		s.BackdropBadgeWidth, s.BackdropBadgeHeight,
		s.PosterLogoSize, s.LogoLogoSize, s.BackdropLogoSize,
		s.LogoLayout,
		s.BackdropLayout, s.BackdropBadgeDirection,
		s.EpisodeRatingsLimit, s.EpisodeBadgeStyle, s.EpisodeLabelStyle, s.EpisodeTextSize,
		s.EpisodeBadgeSize, s.EpisodeBadgeWidth, s.EpisodeBadgeHeight, s.EpisodeLogoSize,
		s.EpisodeLayout, s.EpisodeBadgeDirection, s.EpisodeBlur,
		s.PosterBadgeShape, s.LogoBadgeShape, s.BackdropBadgeShape, s.EpisodeBadgeShape,
		s.PosterBadgeAlpha, s.LogoBadgeAlpha, s.BackdropBadgeAlpha, s.EpisodeBadgeAlpha,
		s.BackdropEdgeInsetX, s.BackdropEdgeInsetY, s.Colors,
	)
	return err
}

func UpsertAPIKeySettings(db *sql.DB, s *APIKeySettings) error {
	return UpsertAPIKeySettingsCtx(context.Background(), db, s)
}

func DeleteAPIKeySettingsCtx(ctx context.Context, db *sql.DB, apiKeyID int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM api_key_settings WHERE api_key_id = ?", apiKeyID)
	return err
}

func DeleteAPIKeySettings(db *sql.DB, apiKeyID int64) error {
	return DeleteAPIKeySettingsCtx(context.Background(), db, apiKeyID)
}

// --- Effective render settings ---

func GetEffectiveRenderSettingsCtx(ctx context.Context, db *sql.DB, apiKeyID int64, cachedGlobals *RenderSettings) RenderSettings {
	defaults := DefaultRenderSettings()
	perKey, err := GetAPIKeySettingsCtx(ctx, db, apiKeyID)
	if err != nil {
		// The per-key SELECT failed (transient connection issue, schema drift,
		// or a corrupt row). The previous version silently fell back to globals
		// with no log — exactly the masking that hid the 2026-08-14 7-column
		// bug for multiple revisions (#10.3). Log and continue so the caller
		// still gets a usable RenderSettings instead of a 500.
		slog.Warn("GetEffectiveRenderSettingsCtx: per-key row read failed, falling back to globals", "api_key_id", apiKeyID, "err", err)
	}
	if err == nil && perKey != nil {
		// Effective colours: the global effective colours (the stored globals,
		// or the caller's cached globals) overlaid with the key's own overrides,
		// so a key without colour overrides renders with the global colours.
		base := defaults
		if cachedGlobals != nil {
			base = *cachedGlobals
		} else if globals, gerr := GetGlobalSettingsCtx(ctx, db); gerr == nil {
			base = ParseGlobalRenderSettings(globals)
		}
		colors := EffectiveSourceColors(&base)
		if perKey.Colors != "" {
			var overrides map[string]SourceColorSet
			if uerr := json.Unmarshal([]byte(perKey.Colors), &overrides); uerr == nil {
				maps.Copy(colors, overrides)
			}
		}
		return RenderSettings{
			ImageSource:            ImageSource(perKey.ImageSource),
			Lang:                   strOrDefault(perKey.Lang, "en"),
			Textless:               perKey.Textless,
			RatingsLimit:           perKey.RatingsLimit,
			RatingsOrder:           perKey.RatingsOrder,
			RatingsExclude:         perKey.RatingsExclude,
			IsDefault:              false,
			PosterLayout:           UnmarshalLayout(perKey.PosterLayout, &defaults.PosterLayout),
			LogoRatingsLimit:       perKey.LogoRatingsLimit,
			BackdropRatingsLimit:   perKey.BackdropRatingsLimit,
			PosterBadgeStyle:       ParseBadgeStyle(perKey.PosterBadgeStyle),
			LogoBadgeStyle:         ParseBadgeStyle(perKey.LogoBadgeStyle),
			BackdropBadgeStyle:     ParseBadgeStyle(perKey.BackdropBadgeStyle),
			PosterLabelStyle:       LabelStyle(perKey.PosterLabelStyle),
			LogoLabelStyle:         LabelStyle(perKey.LogoLabelStyle),
			BackdropLabelStyle:     LabelStyle(perKey.BackdropLabelStyle),
			PosterBadgeDirection:   BadgeDirection(perKey.PosterBadgeDirection),
			PosterFit:              PosterFit(perKey.PosterFit),
			PosterTextSize:         ClampScalePercent(perKey.PosterTextSize),
			LogoTextSize:           ClampScalePercent(perKey.LogoTextSize),
			BackdropTextSize:       ClampScalePercent(perKey.BackdropTextSize),
			PosterBadgeSize:        ClampScalePercent(perKey.PosterBadgeSize),
			LogoBadgeSize:          ClampScalePercent(perKey.LogoBadgeSize),
			BackdropBadgeSize:      ClampScalePercent(perKey.BackdropBadgeSize),
			PosterBadgeWidth:       ClampScalePercent(perKey.PosterBadgeWidth),
			PosterBadgeHeight:      ClampScalePercent(perKey.PosterBadgeHeight),
			LogoBadgeWidth:         ClampScalePercent(perKey.LogoBadgeWidth),
			LogoBadgeHeight:        ClampScalePercent(perKey.LogoBadgeHeight),
			BackdropBadgeWidth:     ClampScalePercent(perKey.BackdropBadgeWidth),
			BackdropBadgeHeight:    ClampScalePercent(perKey.BackdropBadgeHeight),
			EpisodeBadgeWidth:      ClampScalePercent(perKey.EpisodeBadgeWidth),
			EpisodeBadgeHeight:     ClampScalePercent(perKey.EpisodeBadgeHeight),
			PosterLogoSize:         ClampScalePercent(perKey.PosterLogoSize),
			LogoLogoSize:           ClampScalePercent(perKey.LogoLogoSize),
			BackdropLogoSize:       ClampScalePercent(perKey.BackdropLogoSize),
			LogoLayout:             UnmarshalLayout(perKey.LogoLayout, &defaults.LogoLayout),
			BackdropLayout:         UnmarshalLayout(perKey.BackdropLayout, &defaults.BackdropLayout),
			BackdropBadgeDirection: BadgeDirection(perKey.BackdropBadgeDirection),
			BackdropEdgeInsetX:     ClampEdgeInset(perKey.BackdropEdgeInsetX),
			BackdropEdgeInsetY:     ClampEdgeInset(perKey.BackdropEdgeInsetY),
			EpisodeRatingsLimit:    perKey.EpisodeRatingsLimit,
			EpisodeBadgeStyle:      ParseBadgeStyle(perKey.EpisodeBadgeStyle),
			EpisodeLabelStyle:      LabelStyle(perKey.EpisodeLabelStyle),
			EpisodeTextSize:        ClampScalePercent(perKey.EpisodeTextSize),
			EpisodeBadgeSize:       ClampScalePercent(perKey.EpisodeBadgeSize),
			EpisodeLogoSize:        ClampScalePercent(perKey.EpisodeLogoSize),
			EpisodeLayout:          UnmarshalLayout(perKey.EpisodeLayout, &defaults.EpisodeLayout),
			EpisodeBadgeDirection:  BadgeDirection(perKey.EpisodeBadgeDirection),
			EpisodeBlur:            perKey.EpisodeBlur,
			PosterBadgeShape:       BadgeShape(perKey.PosterBadgeShape),
			LogoBadgeShape:         BadgeShape(perKey.LogoBadgeShape),
			BackdropBadgeShape:     BadgeShape(perKey.BackdropBadgeShape),
			EpisodeBadgeShape:      BadgeShape(perKey.EpisodeBadgeShape),
			PosterBadgeAlpha:       ClampBadgeAlpha(perKey.PosterBadgeAlpha),
			LogoBadgeAlpha:         ClampBadgeAlpha(perKey.LogoBadgeAlpha),
			BackdropBadgeAlpha:     ClampBadgeAlpha(perKey.BackdropBadgeAlpha),
			EpisodeBadgeAlpha:      ClampBadgeAlpha(perKey.EpisodeBadgeAlpha),
			Colors:                 colors,
		}
	}

	if cachedGlobals != nil {
		return *cachedGlobals
	}

	globals, gerr := GetGlobalSettingsCtx(ctx, db)
	if gerr != nil {
		return DefaultRenderSettings()
	}
	return ParseGlobalRenderSettings(globals)
}

func GetEffectiveRenderSettings(db *sql.DB, apiKeyID int64, cachedGlobals *RenderSettings) RenderSettings {
	return GetEffectiveRenderSettingsCtx(context.Background(), db, apiKeyID, cachedGlobals)
}

// --- Available ratings ---
