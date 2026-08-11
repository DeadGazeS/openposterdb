package services

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
)

func DefaultRatingsOrder() string { return "mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert" }

func MaxEdgeInset() int32 { return 50 }

func ClampEdgeInset(value int32) int32 {
	if value < 0 {
		return 0
	}
	if value > MaxEdgeInset() {
		return MaxEdgeInset()
	}
	return value
}

func ValidateRatingsLimit(limit int32) error {
	if limit >= 0 && limit <= 10 {
		return nil
	}
	return fmt.Errorf("ratings_limit must be between 0 and 10")
}

// ClampBadgesPerRow clamps a badges-per-row count to a sane range. 0 means
// "all badges in a single row".
func ClampBadgesPerRow(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func ValidateRatingsOrder(order string) error {
	if order == "" {
		return nil
	}
	seen := make(map[string]bool)
	for key := range strings.SplitSeq(order, ",") {
		key = strings.TrimSpace(key)
		if !isValidRatingKey(key) {
			return fmt.Errorf("unknown rating source key: '%s'. Valid keys: %s", key, allRatingKeys())
		}
		if seen[key] {
			return fmt.Errorf("duplicate rating source key: '%s'", key)
		}
		seen[key] = true
	}
	return nil
}

func ValidateLang(lang string) error {
	if len(lang) < 2 || len(lang) > 5 {
		return fmt.Errorf("lang must be 2-5 ASCII alphanumeric characters (e.g. 'en', 'de', 'pt-BR')")
	}
	for _, c := range lang {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Errorf("lang must be 2-5 ASCII alphanumeric characters (e.g. 'en', 'de', 'pt-BR')")
		}
	}
	return nil
}

func ValidateRatingsExclude(exclude string) error {
	return ValidateRatingsOrder(exclude)
}

// --- RenderSettings ---

type RenderSettings struct {
	ImageSource            ImageSource    `json:"image_source"`
	Lang                   string         `json:"lang"`
	Textless               bool           `json:"textless"`
	RatingsLimit           int32          `json:"ratings_limit"`
	RatingsOrder           string         `json:"ratings_order"`
	RatingsExclude         string         `json:"ratings_exclude"`
	IsDefault              bool           `json:"is_default"`
	PosterLayout           ImageLayout    `json:"poster_layout"`
	LogoRatingsLimit       int32          `json:"logo_ratings_limit"`
	BackdropRatingsLimit   int32          `json:"backdrop_ratings_limit"`
	PosterBadgeStyle       BadgeStyle     `json:"poster_badge_style"`
	LogoBadgeStyle         BadgeStyle     `json:"logo_badge_style"`
	BackdropBadgeStyle     BadgeStyle     `json:"backdrop_badge_style"`
	PosterLabelStyle       LabelStyle     `json:"poster_label_style"`
	LogoLabelStyle         LabelStyle     `json:"logo_label_style"`
	BackdropLabelStyle     LabelStyle     `json:"backdrop_label_style"`
	PosterBadgeDirection   BadgeDirection `json:"poster_badge_direction"`
	PosterFit              PosterFit      `json:"poster_fit"`
	PosterTextSize         ScalePercent   `json:"poster_text_size"`
	LogoTextSize           ScalePercent   `json:"logo_text_size"`
	BackdropTextSize       ScalePercent   `json:"backdrop_text_size"`
	PosterBadgeSize        ScalePercent   `json:"poster_badge_size"`
	LogoBadgeSize          ScalePercent   `json:"logo_badge_size"`
	BackdropBadgeSize      ScalePercent   `json:"backdrop_badge_size"`
	PosterBadgeWidth       ScalePercent   `json:"poster_badge_width"`
	PosterBadgeHeight      ScalePercent   `json:"poster_badge_height"`
	LogoBadgeWidth         ScalePercent   `json:"logo_badge_width"`
	LogoBadgeHeight        ScalePercent   `json:"logo_badge_height"`
	BackdropBadgeWidth     ScalePercent   `json:"backdrop_badge_width"`
	BackdropBadgeHeight    ScalePercent   `json:"backdrop_badge_height"`
	EpisodeBadgeWidth      ScalePercent   `json:"episode_badge_width"`
	EpisodeBadgeHeight     ScalePercent   `json:"episode_badge_height"`
	PosterLogoSize         ScalePercent   `json:"poster_logo_size"`
	LogoLogoSize           ScalePercent   `json:"logo_logo_size"`
	BackdropLogoSize       ScalePercent   `json:"backdrop_logo_size"`
	LogoLayout             ImageLayout    `json:"logo_layout"`
	BackdropLayout         ImageLayout    `json:"backdrop_layout"`
	BackdropBadgeDirection BadgeDirection `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX     int32          `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY     int32          `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit    int32          `json:"episode_ratings_limit"`
	EpisodeBadgeStyle      BadgeStyle     `json:"episode_badge_style"`
	EpisodeLabelStyle      LabelStyle     `json:"episode_label_style"`
	EpisodeTextSize        ScalePercent   `json:"episode_text_size"`
	EpisodeBadgeSize       ScalePercent   `json:"episode_badge_size"`
	EpisodeLogoSize        ScalePercent   `json:"episode_logo_size"`
	EpisodeLayout          ImageLayout    `json:"episode_layout"`
	EpisodeBadgeDirection  BadgeDirection `json:"episode_badge_direction"`
	EpisodeBlur            bool           `json:"episode_blur"`
	PosterBadgeShape       BadgeShape     `json:"poster_badge_shape"`
	LogoBadgeShape         BadgeShape     `json:"logo_badge_shape"`
	BackdropBadgeShape     BadgeShape     `json:"backdrop_badge_shape"`
	EpisodeBadgeShape      BadgeShape     `json:"episode_badge_shape"`
	PosterBadgeAlpha       BadgeAlpha     `json:"poster_badge_alpha"`
	LogoBadgeAlpha         BadgeAlpha     `json:"logo_badge_alpha"`
	BackdropBadgeAlpha     BadgeAlpha     `json:"backdrop_badge_alpha"`
	EpisodeBadgeAlpha      BadgeAlpha     `json:"episode_badge_alpha"`

	// Colors holds per-rating-source color overrides. Only non-default colors
	// are stored; empty means "use the source default".
	Colors map[string]SourceColorSet `json:"colors"`
}

func DefaultRenderSettings() RenderSettings {
	return RenderSettings{
		ImageSource:            ImageSourceTMDB,
		Lang:                   "en",
		Textless:               false,
		RatingsLimit:           3,
		RatingsOrder:           "mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert",
		RatingsExclude:         "",
		IsDefault:              true,
		PosterLayout:           DefaultLayout("poster"),
		LogoRatingsLimit:       5,
		BackdropRatingsLimit:   5,
		PosterBadgeStyle:       BadgeStyleDefault,
		LogoBadgeStyle:         BadgeStyleLogoTB,
		BackdropBadgeStyle:     BadgeStyleLogoTB,
		PosterLabelStyle:       LabelStyleOfficial,
		LogoLabelStyle:         LabelStyleOfficial,
		BackdropLabelStyle:     LabelStyleOfficial,
		PosterBadgeDirection:   BadgeDirectionDefault,
		PosterFit:              PosterFitNative,
		PosterTextSize:         DefaultScalePercent(),
		LogoTextSize:           DefaultScalePercent(),
		BackdropTextSize:       DefaultScalePercent(),
		PosterBadgeSize:        DefaultScalePercent(),
		LogoBadgeSize:          DefaultScalePercent(),
		BackdropBadgeSize:      DefaultScalePercent(),
		PosterBadgeWidth:       DefaultScalePercent(),
		PosterBadgeHeight:      DefaultScalePercent(),
		LogoBadgeWidth:         DefaultScalePercent(),
		LogoBadgeHeight:        DefaultScalePercent(),
		BackdropBadgeWidth:     DefaultScalePercent(),
		BackdropBadgeHeight:    DefaultScalePercent(),
		EpisodeBadgeWidth:      DefaultScalePercent(),
		EpisodeBadgeHeight:     DefaultScalePercent(),
		PosterLogoSize:         DefaultScalePercent(),
		LogoLogoSize:           DefaultScalePercent(),
		BackdropLogoSize:       DefaultScalePercent(),
		LogoLayout:             DefaultLayout("logo"),
		BackdropLayout:         DefaultLayout("backdrop"),
		BackdropBadgeDirection: BadgeDirectionDefault,
		BackdropEdgeInsetX:     0,
		BackdropEdgeInsetY:     0,
		EpisodeRatingsLimit:    1,
		EpisodeBadgeStyle:      BadgeStyleLogoTB,
		EpisodeLabelStyle:      LabelStyleOfficial,
		EpisodeTextSize:        DefaultScalePercent(),
		EpisodeBadgeSize:       DefaultScalePercent(),
		EpisodeLogoSize:        DefaultScalePercent(),
		EpisodeLayout:          DefaultLayout("episode"),
		EpisodeBadgeDirection:  BadgeDirectionVertical,
		EpisodeBlur:            false,
		PosterBadgeShape:       BadgeShapeRounded,
		LogoBadgeShape:         BadgeShapeRounded,
		BackdropBadgeShape:     BadgeShapeRounded,
		EpisodeBadgeShape:      BadgeShapeRounded,
		PosterBadgeAlpha:       DefaultBadgeAlpha(),
		LogoBadgeAlpha:         DefaultBadgeAlpha(),
		BackdropBadgeAlpha:     DefaultBadgeAlpha(),
		EpisodeBadgeAlpha:      DefaultBadgeAlpha(),
	}
}

func (s *RenderSettings) PosterAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.PosterBadgeShape, Alpha: s.PosterBadgeAlpha, Width: s.PosterBadgeWidth, Height: s.PosterBadgeHeight}
}

func (s *RenderSettings) LogoAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.LogoBadgeShape, Alpha: s.LogoBadgeAlpha, Width: s.LogoBadgeWidth, Height: s.LogoBadgeHeight}
}

func (s *RenderSettings) BackdropAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.BackdropBadgeShape, Alpha: s.BackdropBadgeAlpha, Width: s.BackdropBadgeWidth, Height: s.BackdropBadgeHeight}
}

func (s *RenderSettings) EpisodeAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.EpisodeBadgeShape, Alpha: s.EpisodeBadgeAlpha, Width: s.EpisodeBadgeWidth, Height: s.EpisodeBadgeHeight}
}

func ParseGlobalRenderSettings(globals map[string]string) RenderSettings {
	if len(globals) == 0 {
		return DefaultRenderSettings()
	}
	defaults := DefaultRenderSettings()

	return RenderSettings{
		ImageSource:            ImageSource(stringOr(globals, "image_source", string(defaults.ImageSource))),
		Lang:                   stringOr(globals, "lang", defaults.Lang),
		Textless:               boolOr(globals, "textless", defaults.Textless),
		RatingsLimit:           int32Or(globals, "ratings_limit", defaults.RatingsLimit),
		RatingsOrder:           stringOr(globals, "ratings_order", defaults.RatingsOrder),
		RatingsExclude:         stringOr(globals, "ratings_exclude", defaults.RatingsExclude),
		IsDefault:              true,
		PosterLayout:           UnmarshalLayout(stringOr(globals, "poster_layout", ""), &defaults.PosterLayout),
		LogoRatingsLimit:       int32Or(globals, "logo_ratings_limit", defaults.LogoRatingsLimit),
		BackdropRatingsLimit:   int32Or(globals, "backdrop_ratings_limit", defaults.BackdropRatingsLimit),
		PosterBadgeStyle:       ParseBadgeStyle(stringOr(globals, "poster_badge_style", string(defaults.PosterBadgeStyle))),
		LogoBadgeStyle:         ParseBadgeStyle(stringOr(globals, "logo_badge_style", string(defaults.LogoBadgeStyle))),
		BackdropBadgeStyle:     ParseBadgeStyle(stringOr(globals, "backdrop_badge_style", string(defaults.BackdropBadgeStyle))),
		PosterLabelStyle:       LabelStyle(stringOr(globals, "poster_label_style", string(defaults.PosterLabelStyle))),
		LogoLabelStyle:         LabelStyle(stringOr(globals, "logo_label_style", string(defaults.LogoLabelStyle))),
		BackdropLabelStyle:     LabelStyle(stringOr(globals, "backdrop_label_style", string(defaults.BackdropLabelStyle))),
		PosterBadgeDirection:   BadgeDirection(stringOr(globals, "poster_badge_direction", string(defaults.PosterBadgeDirection))),
		PosterFit:              PosterFit(stringOr(globals, "poster_fit", string(defaults.PosterFit))),
		PosterTextSize:         ClampScalePercent(int32Or(globals, "poster_text_size", int32(defaults.PosterTextSize))),
		LogoTextSize:           ClampScalePercent(int32Or(globals, "logo_text_size", int32(defaults.LogoTextSize))),
		BackdropTextSize:       ClampScalePercent(int32Or(globals, "backdrop_text_size", int32(defaults.BackdropTextSize))),
		PosterBadgeSize:        ClampScalePercent(int32Or(globals, "poster_badge_size", int32(defaults.PosterBadgeSize))),
		LogoBadgeSize:          ClampScalePercent(int32Or(globals, "logo_badge_size", int32(defaults.LogoBadgeSize))),
		BackdropBadgeSize:      ClampScalePercent(int32Or(globals, "backdrop_badge_size", int32(defaults.BackdropBadgeSize))),
		PosterBadgeWidth:       ClampScalePercent(int32Or(globals, "poster_badge_width", int32(defaults.PosterBadgeWidth))),
		PosterBadgeHeight:      ClampScalePercent(int32Or(globals, "poster_badge_height", int32(defaults.PosterBadgeHeight))),
		LogoBadgeWidth:         ClampScalePercent(int32Or(globals, "logo_badge_width", int32(defaults.LogoBadgeWidth))),
		LogoBadgeHeight:        ClampScalePercent(int32Or(globals, "logo_badge_height", int32(defaults.LogoBadgeHeight))),
		BackdropBadgeWidth:     ClampScalePercent(int32Or(globals, "backdrop_badge_width", int32(defaults.BackdropBadgeWidth))),
		BackdropBadgeHeight:    ClampScalePercent(int32Or(globals, "backdrop_badge_height", int32(defaults.BackdropBadgeHeight))),
		EpisodeBadgeWidth:      ClampScalePercent(int32Or(globals, "episode_badge_width", int32(defaults.EpisodeBadgeWidth))),
		EpisodeBadgeHeight:     ClampScalePercent(int32Or(globals, "episode_badge_height", int32(defaults.EpisodeBadgeHeight))),
		PosterLogoSize:         ClampScalePercent(int32Or(globals, "poster_logo_size", int32(defaults.PosterLogoSize))),
		LogoLogoSize:           ClampScalePercent(int32Or(globals, "logo_logo_size", int32(defaults.LogoLogoSize))),
		BackdropLogoSize:       ClampScalePercent(int32Or(globals, "backdrop_logo_size", int32(defaults.BackdropLogoSize))),
		LogoLayout:             UnmarshalLayout(stringOr(globals, "logo_layout", ""), &defaults.LogoLayout),
		BackdropLayout:         UnmarshalLayout(stringOr(globals, "backdrop_layout", ""), &defaults.BackdropLayout),
		BackdropBadgeDirection: BadgeDirection(stringOr(globals, "backdrop_badge_direction", string(defaults.BackdropBadgeDirection))),
		BackdropEdgeInsetX:     int32ClampOr(int32Or(globals, "backdrop_edge_inset_x", defaults.BackdropEdgeInsetX)),
		BackdropEdgeInsetY:     int32ClampOr(int32Or(globals, "backdrop_edge_inset_y", defaults.BackdropEdgeInsetY)),
		EpisodeRatingsLimit:    int32Or(globals, "episode_ratings_limit", defaults.EpisodeRatingsLimit),
		EpisodeBadgeStyle:      ParseBadgeStyle(stringOr(globals, "episode_badge_style", string(defaults.EpisodeBadgeStyle))),
		EpisodeLabelStyle:      LabelStyle(stringOr(globals, "episode_label_style", string(defaults.EpisodeLabelStyle))),
		EpisodeTextSize:        ClampScalePercent(int32Or(globals, "episode_text_size", int32(defaults.EpisodeTextSize))),
		EpisodeBadgeSize:       ClampScalePercent(int32Or(globals, "episode_badge_size", int32(defaults.EpisodeBadgeSize))),
		EpisodeLogoSize:        ClampScalePercent(int32Or(globals, "episode_logo_size", int32(defaults.EpisodeLogoSize))),
		EpisodeLayout:          UnmarshalLayout(stringOr(globals, "episode_layout", ""), &defaults.EpisodeLayout),
		EpisodeBadgeDirection:  BadgeDirection(stringOr(globals, "episode_badge_direction", string(defaults.EpisodeBadgeDirection))),
		EpisodeBlur:            boolOr(globals, "episode_blur", defaults.EpisodeBlur),
		PosterBadgeShape:       BadgeShape(stringOr(globals, "poster_badge_shape", string(defaults.PosterBadgeShape))),
		LogoBadgeShape:         BadgeShape(stringOr(globals, "logo_badge_shape", string(defaults.LogoBadgeShape))),
		BackdropBadgeShape:     BadgeShape(stringOr(globals, "backdrop_badge_shape", string(defaults.BackdropBadgeShape))),
		EpisodeBadgeShape:      BadgeShape(stringOr(globals, "episode_badge_shape", string(defaults.EpisodeBadgeShape))),
		PosterBadgeAlpha:       ClampBadgeAlpha(int32Or(globals, "poster_badge_alpha", int32(defaults.PosterBadgeAlpha))),
		LogoBadgeAlpha:         ClampBadgeAlpha(int32Or(globals, "logo_badge_alpha", int32(defaults.LogoBadgeAlpha))),
		BackdropBadgeAlpha:     ClampBadgeAlpha(int32Or(globals, "backdrop_badge_alpha", int32(defaults.BackdropBadgeAlpha))),
		EpisodeBadgeAlpha:      ClampBadgeAlpha(int32Or(globals, "episode_badge_alpha", int32(defaults.EpisodeBadgeAlpha))),
		Colors:                 parseSourceColors(globals),
	}
}

func stringOr(m map[string]string, key, def string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

func boolOr(m map[string]string, key string, def bool) bool {
	if v, ok := m[key]; ok {
		return v == "true"
	}
	return def
}

func int32Or(m map[string]string, key string, def int32) int32 {
	if v, ok := m[key]; ok {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func int32ClampOr(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 50 {
		return 50
	}
	return v
}

// fieldSpec describes one user-facing JSON field on RenderSettings. The slice
// is the single source of truth for the wire/storage format: both the admin
// PUT decoder (handlers.HandleUpdateSettings) and the storage-layer
// serialiser RenderSettingsToMap walk this table. Building it from reflection
// on RenderSettings means a new field added there with a JSON tag
// automatically flows to both the decoder and the serialiser — eliminating
// the field-by-field drift that produced the 2026-08-04 badge-width bug
// (where a new per-kind field reached RenderSettingsToMap but not the
// parallel hand-written request struct).
type FieldSpec struct {
	JSONName string // JSON tag value (e.g. "poster_badge_width")
	GoName   string // RenderSettings struct field name (e.g. "PosterBadgeWidth")
	Kind     string // "common" | "poster" | "logo" | "backdrop" | "episode" — informational
	GoType   string // category used to drive decode/serialise dispatch; see fieldType* constants
}

// GoType values used to drive fieldSpec-aware logic. Each one corresponds to a
// branch in applyFieldToSettings (decode) and RenderSettingsToMap (serialise).
const (
	fieldTypeString       = "string"        // string or any `type X string` alias
	fieldTypeBool         = "bool"          // bool
	fieldTypeInt          = "int"           // plain int32 (validated separately by ValidateRenderSettings)
	fieldTypeScalePercent = "scale_percent" // ScalePercent (needs ClampScalePercent on decode)
	fieldTypeBadgeAlpha   = "badge_alpha"   // BadgeAlpha (needs ClampBadgeAlpha on decode)
	fieldTypeEdgeInset    = "edge_inset"    // BackdropEdgeInsetX/Y (needs ClampEdgeInset on decode)
	fieldTypeLayout       = "layout"        // ImageLayout (JSON object on wire, marshalled string in storage)
	fieldTypeColors       = "colors"        // map[string]SourceColorSet (JSON object on wire, flat color_* keys in storage)
)

// RenderSettingFields is the shared, reflection-built spec slice. Exported so
// the admin handler (and the consistency regression test) can walk it.
var RenderSettingFields = buildRenderSettingFields()

func buildRenderSettingFields() []FieldSpec {
	rt := reflect.TypeOf(RenderSettings{})
	out := make([]FieldSpec, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		// IsDefault is an internal-use field: set by ParseGlobalRenderSettings
		// / DefaultRenderSettings, never user-settable, never serialised to
		// storage, never returned by SettingsResponseMap. Skip it so the
		// spec only covers the wire/storage surface.
		if name == "is_default" {
			continue
		}
		out = append(out, FieldSpec{
			JSONName: name,
			GoName:   f.Name,
			Kind:     fieldKindFromName(f.Name),
			GoType:   fieldTypeFromField(f),
		})
	}
	return out
}

func fieldKindFromName(name string) string {
	switch {
	case strings.HasPrefix(name, "Poster"):
		return "poster"
	case strings.HasPrefix(name, "Logo"):
		return "logo"
	case strings.HasPrefix(name, "Backdrop"):
		return "backdrop"
	case strings.HasPrefix(name, "Episode"):
		return "episode"
	}
	return "common"
}

func fieldTypeFromField(f reflect.StructField) string {
	// Named types come first — they're the special cases.
	switch f.Type.Name() {
	case "ImageLayout":
		return fieldTypeLayout
	case "ScalePercent":
		return fieldTypeScalePercent
	case "BadgeAlpha":
		return fieldTypeBadgeAlpha
	}
	// Fall back to the underlying kind. Plain int32 with a BackdropEdgeInset*
	// name needs ClampEdgeInset; other plain int32 fields (ratings_limit,
	// *_ratings_limit) are direct and validated separately.
	switch f.Type.Kind() {
	case reflect.String:
		return fieldTypeString
	case reflect.Bool:
		return fieldTypeBool
	case reflect.Int32:
		if strings.HasPrefix(f.Name, "BackdropEdgeInset") {
			return fieldTypeEdgeInset
		}
		return fieldTypeInt
	case reflect.Map:
		return fieldTypeColors
	}
	return ""
}

// applyFieldToSettings decodes one payload key into s via the spec entry. A
// JSON null leaves the field untouched, matching the original pointer-based
// decoder (where null on a *T field stays nil). Returns an error if the JSON
// value is the wrong shape for the field's type.
func applyFieldToSettings(s *RenderSettings, spec FieldSpec, raw json.RawMessage) error {
	if string(raw) == "null" {
		return nil
	}
	field := reflect.ValueOf(s).Elem().FieldByName(spec.GoName)
	switch spec.GoType {
	case fieldTypeString:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetString(v)
	case fieldTypeBool:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetBool(v)
	case fieldTypeInt:
		var v int32
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetInt(int64(v))
	case fieldTypeScalePercent:
		var v int32
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetInt(int64(ClampScalePercent(v)))
	case fieldTypeBadgeAlpha:
		var v int32
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetInt(int64(ClampBadgeAlpha(v)))
	case fieldTypeEdgeInset:
		var v int32
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.SetInt(int64(ClampEdgeInset(v)))
	case fieldTypeLayout:
		var v ImageLayout
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		field.Set(reflect.ValueOf(v))
	case fieldTypeColors:
		var v map[string]SourceColorSet
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		if err := ValidateSourceColors(v); err != nil {
			return fmt.Errorf("invalid %s: %w", spec.JSONName, err)
		}
		v = NormalizeSourceColors(v)
		field.Set(reflect.ValueOf(v))
	}
	return nil
}

// ApplyUpdatePayload walks the shared spec slice and applies every payload
// key it recognises to s. Unknown keys are ignored (the previous hand-written
// request struct silently dropped them too via pointer-absent behaviour, so
// callers that send extras still get the same result). Errors are returned for
// malformed values; validation of the resulting s is the caller's
// responsibility (ValidateRenderSettings).
func ApplyUpdatePayload(s *RenderSettings, payload map[string]json.RawMessage) error {
	for _, spec := range RenderSettingFields {
		raw, ok := payload[spec.JSONName]
		if !ok {
			continue
		}
		if err := applyFieldToSettings(s, spec, raw); err != nil {
			return err
		}
	}
	return nil
}

// RenderSettingsToMap converts effective render settings back into the flat
// key/value form stored in global_settings. Walks the shared RenderSettingFields
// spec (the same table the admin PUT decoder uses) so a new RenderSettings
// field with a JSON tag automatically lands here.
func RenderSettingsToMap(s *RenderSettings) map[string]string {
	m := make(map[string]string, len(RenderSettingFields))
	sv := reflect.ValueOf(s).Elem()
	for _, spec := range RenderSettingFields {
		if spec.GoType == fieldTypeColors {
			// Colors flatten into multiple color_<key>_<attr> keys; merged
			// into m below via colorsToMap.
			continue
		}
		fv := sv.FieldByName(spec.GoName)
		switch spec.GoType {
		case fieldTypeString:
			m[spec.JSONName] = fv.String()
		case fieldTypeBool:
			m[spec.JSONName] = boolStr(fv.Bool())
		case fieldTypeInt, fieldTypeScalePercent, fieldTypeBadgeAlpha, fieldTypeEdgeInset:
			m[spec.JSONName] = int32Str(int32(fv.Int()))
		case fieldTypeLayout:
			layout := fv.Interface().(ImageLayout)
			m[spec.JSONName] = mustMarshalLayout(&layout)
		}
	}
	maps.Copy(m, colorsToMap(s.Colors))
	return m
}

// SettingsResponseMap returns the canonical settings fields as a map suitable
// for HTTP JSON responses. Shared by HandleGetSettings (admin) and
// HandleFreeKeySettings so the per-field enumeration lives in one place.
// Returned types match the existing wire format: strings for badge/label/direction
// styles, int32 for percentage/scale fields, layouts pre-marshalled to JSON,
// colors in the {"color_<source>_accent/value/border/text": "..."} form.
func SettingsResponseMap(s *RenderSettings) map[string]any {
	m := map[string]any{
		"image_source":             string(s.ImageSource),
		"lang":                     s.Lang,
		"textless":                 s.Textless,
		"ratings_limit":            s.RatingsLimit,
		"ratings_order":            s.RatingsOrder,
		"ratings_exclude":          s.RatingsExclude,
		"poster_layout":            mustMarshalLayout(&s.PosterLayout),
		"logo_ratings_limit":       s.LogoRatingsLimit,
		"backdrop_ratings_limit":   s.BackdropRatingsLimit,
		"poster_badge_style":       string(s.PosterBadgeStyle),
		"logo_badge_style":         string(s.LogoBadgeStyle),
		"backdrop_badge_style":     string(s.BackdropBadgeStyle),
		"poster_label_style":       string(s.PosterLabelStyle),
		"logo_label_style":         string(s.LogoLabelStyle),
		"backdrop_label_style":     string(s.BackdropLabelStyle),
		"poster_badge_direction":   string(s.PosterBadgeDirection),
		"poster_fit":               string(s.PosterFit),
		"poster_text_size":         int32(s.PosterTextSize),
		"logo_text_size":           int32(s.LogoTextSize),
		"backdrop_text_size":       int32(s.BackdropTextSize),
		"poster_badge_size":        int32(s.PosterBadgeSize),
		"logo_badge_size":          int32(s.LogoBadgeSize),
		"backdrop_badge_size":      int32(s.BackdropBadgeSize),
		"poster_badge_width":       int32(s.PosterBadgeWidth),
		"poster_badge_height":      int32(s.PosterBadgeHeight),
		"logo_badge_width":         int32(s.LogoBadgeWidth),
		"logo_badge_height":        int32(s.LogoBadgeHeight),
		"backdrop_badge_width":     int32(s.BackdropBadgeWidth),
		"backdrop_badge_height":    int32(s.BackdropBadgeHeight),
		"episode_badge_width":      int32(s.EpisodeBadgeWidth),
		"episode_badge_height":     int32(s.EpisodeBadgeHeight),
		"poster_logo_size":         int32(s.PosterLogoSize),
		"logo_logo_size":           int32(s.LogoLogoSize),
		"backdrop_logo_size":       int32(s.BackdropLogoSize),
		"logo_layout":              mustMarshalLayout(&s.LogoLayout),
		"backdrop_layout":          mustMarshalLayout(&s.BackdropLayout),
		"backdrop_badge_direction": string(s.BackdropBadgeDirection),
		"backdrop_edge_inset_x":    s.BackdropEdgeInsetX,
		"backdrop_edge_inset_y":    s.BackdropEdgeInsetY,
		"episode_ratings_limit":    s.EpisodeRatingsLimit,
		"episode_badge_style":      string(s.EpisodeBadgeStyle),
		"episode_label_style":      string(s.EpisodeLabelStyle),
		"episode_text_size":        int32(s.EpisodeTextSize),
		"episode_badge_size":       int32(s.EpisodeBadgeSize),
		"episode_logo_size":        int32(s.EpisodeLogoSize),
		"episode_layout":           mustMarshalLayout(&s.EpisodeLayout),
		"episode_badge_direction":  string(s.EpisodeBadgeDirection),
		"episode_blur":             s.EpisodeBlur,
		"poster_badge_shape":       string(s.PosterBadgeShape),
		"logo_badge_shape":         string(s.LogoBadgeShape),
		"backdrop_badge_shape":     string(s.BackdropBadgeShape),
		"episode_badge_shape":      string(s.EpisodeBadgeShape),
		"poster_badge_alpha":       int32(s.PosterBadgeAlpha),
		"logo_badge_alpha":         int32(s.LogoBadgeAlpha),
		"backdrop_badge_alpha":     int32(s.BackdropBadgeAlpha),
		"episode_badge_alpha":      int32(s.EpisodeBadgeAlpha),
		"colors":                   EffectiveSourceColors(s),
	}
	return m
}

// ValidateRenderSettings validates the effective render settings, returning an
// error string suitable for a 400 response.
func ValidateRenderSettings(s *RenderSettings) error {
	if err := ValidateLang(s.Lang); err != nil {
		return err
	}
	if err := ValidateRatingsLimit(s.RatingsLimit); err != nil {
		return err
	}
	if err := ValidateRatingsOrder(s.RatingsOrder); err != nil {
		return err
	}
	if err := ValidateRatingsExclude(s.RatingsExclude); err != nil {
		return err
	}
	if err := ValidateRatingsLimit(s.LogoRatingsLimit); err != nil {
		return err
	}
	if err := ValidateRatingsLimit(s.BackdropRatingsLimit); err != nil {
		return err
	}
	if err := ValidateRatingsLimit(s.EpisodeRatingsLimit); err != nil {
		return err
	}
	for _, layout := range []ImageLayout{s.PosterLayout, s.LogoLayout, s.BackdropLayout, s.EpisodeLayout} {
		if err := ValidateLayout(&layout); err != nil {
			return err
		}
	}
	return nil
}

func mustMarshalLayout(l *ImageLayout) string {
	s, err := MarshalLayout(l)
	if err != nil {
		return ""
	}
	return s
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func int32Str(v int32) string {
	return fmt.Sprintf("%d", v)
}

func parseSourceColors(globals map[string]string) map[string]SourceColorSet {
	var out map[string]SourceColorSet
	for _, key := range AllColorKeys() {
		var set SourceColorSet
		set.Accent = globals["color_"+key+"_accent"]
		set.Value = globals["color_"+key+"_value"]
		set.Border = globals["color_"+key+"_border"]
		set.Text = globals["color_"+key+"_text"]
		if set.HasAny() {
			if out == nil {
				out = make(map[string]SourceColorSet)
			}
			out[key] = set
		}
	}
	return out
}

func colorsToMap(colors map[string]SourceColorSet) map[string]string {
	out := map[string]string{}
	for key, set := range colors {
		if set.Accent != "" {
			out["color_"+key+"_accent"] = set.Accent
		}
		if set.Value != "" {
			out["color_"+key+"_value"] = set.Value
		}
		if set.Border != "" {
			out["color_"+key+"_border"] = set.Border
		}
		if set.Text != "" {
			out["color_"+key+"_text"] = set.Text
		}
	}
	return out
}

// --- Admin user CRUD ---
