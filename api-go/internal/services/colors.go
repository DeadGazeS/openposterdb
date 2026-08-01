package services

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// SourceColorSet holds per-rating-source color overrides. An empty string means
// "use the default". Colors are hex (#RRGGBB or #RRGGBBAA). Accent and value are
// the badge section colors (default black); the per-kind badge alpha controls
// their shared opacity.
type SourceColorSet struct {
	Accent string `json:"accent"`
	Value  string `json:"value"`
	Border string `json:"border"`
	Text   string `json:"text"`
}

// RatingVariant describes a Rotten Tomatoes logo variant that carries its own
// color settings. Rotten Tomatoes picks its logo from the score, so each
// variant (e.g. "Certified Fresh") is its own color row in the UI.
type RatingVariant struct {
	Key      string
	Source   *RatingSource
	Label    string
	MinScore uint32
}

// rtVariants are listed in descending MinScore so RatingColorKey matches the
// right band. The thresholds mirror OfficialIconForBadge.
var rtVariants = []RatingVariant{
	{Key: "rt_cf", Source: SourceRt, Label: "Critics · Certified Fresh", MinScore: 75},
	{Key: "rt_pos", Source: SourceRt, Label: "Critics · Fresh", MinScore: 60},
	{Key: "rt_rot", Source: SourceRt, Label: "Critics · Rotten", MinScore: 0},
	{Key: "rta_hot", Source: SourceRtAudience, Label: "Audience · Verified Hot", MinScore: 75},
	{Key: "rta_pos", Source: SourceRtAudience, Label: "Audience · Fresh", MinScore: 60},
	{Key: "rta_neg", Source: SourceRtAudience, Label: "Audience · Rotten", MinScore: 0},
}

// RTVariantByKey returns the Rotten Tomatoes variant for a color key, or nil.
func RTVariantByKey(key string) *RatingVariant {
	for i := range rtVariants {
		if rtVariants[i].Key == key {
			return &rtVariants[i]
		}
	}
	return nil
}

// IsColorKey reports whether key is a valid color-settings key (a rating source
// key or a Rotten Tomatoes variant key).
func IsColorKey(key string) bool {
	if SourceFromKey(key) != nil {
		return true
	}
	return RTVariantByKey(key) != nil
}

// AllColorKeys returns every key that can carry per-source color settings:
// the rating source keys plus the Rotten Tomatoes logo variant keys.
func AllColorKeys() []string {
	keys := AllSourceKeys()
	for _, v := range rtVariants {
		keys = append(keys, v.Key)
	}
	return keys
}

// badgeScorePercent parses the numeric percentage from a rating value.
func badgeScorePercent(value string) uint32 {
	var result float64
	if _, err := fmt.Sscanf(value, "%f%%", &result); err == nil {
		if result < 0 {
			return 0
		}
		return uint32(result)
	}
	return 0
}

// RatingColorKey returns the color-settings key for a badge. Rotten Tomatoes
// badges map to their specific logo variant based on the score; all other
// sources map to the source key.
func RatingColorKey(badge *RatingBadge) string {
	if badge == nil || badge.Source == nil {
		return ""
	}
	if badge.Source == SourceRt || badge.Source == SourceRtAudience {
		score := badgeScorePercent(badge.Value)
		for _, v := range rtVariants {
			if v.Source == badge.Source && score >= v.MinScore {
				return v.Key
			}
		}
	}
	return badge.Source.Key
}

// DefaultColorSetForKey returns the default color set for a color key, mapping
// Rotten Tomatoes variant keys to their parent source defaults.
func DefaultColorSetForKey(key string) SourceColorSet {
	if v := RTVariantByKey(key); v != nil {
		return DefaultSourceColors(v.Source)
	}
	if s := SourceFromKey(key); s != nil {
		return DefaultSourceColors(s)
	}
	return SourceColorSet{}
}

// HasAny reports whether at least one override is set.
func (c SourceColorSet) HasAny() bool {
	return c.Accent != "" || c.Value != "" || c.Border != "" || c.Text != ""
}

// DefaultSourceColors returns the default colors for a source. Both the accent
// and value sections default to black; the badge alpha controls their opacity.
// The border is empty by default (no border drawn); users opt into a border.
func DefaultSourceColors(source *RatingSource) SourceColorSet {
	return SourceColorSet{
		Accent: "#000000",
		Value:  "#000000",
		Border: "",
		Text:   "#ffffff",
	}
}

// darkValueHex is the default rating-value background: near-black at 78% alpha.
const darkValueHex = "#000000c8"

// EffectiveSourceColors returns the effective color set for every color key
// (rating sources plus Rotten Tomatoes logo variants), filling in defaults for
// any unset override.
func EffectiveSourceColors(settings *RenderSettings) map[string]SourceColorSet {
	keys := AllColorKeys()
	out := make(map[string]SourceColorSet, len(keys))
	for _, key := range keys {
		def := DefaultColorSetForKey(key)
		if settings != nil {
			if ov, ok := settings.Colors[key]; ok {
				if ov.Accent != "" {
					def.Accent = ov.Accent
				}
				if ov.Value != "" {
					def.Value = ov.Value
				}
				if ov.Border != "" {
					def.Border = ov.Border
				}
				if ov.Text != "" {
					def.Text = ov.Text
				}
			}
		}
		out[key] = def
	}
	return out
}

// ValidateSourceColors checks that every provided color is a valid hex value.
func ValidateSourceColors(colors map[string]SourceColorSet) error {
	for key, c := range colors {
		if !IsColorKey(key) {
			return fmt.Errorf("unknown rating source: '%s'", key)
		}
		for name, v := range map[string]string{"accent": c.Accent, "value": c.Value, "border": c.Border, "text": c.Text} {
			if v == "" {
				continue
			}
			if _, ok := ParseHexColor(v); !ok {
				return fmt.Errorf("invalid %s color for %s: %q (expected #RRGGBB or #RRGGBBAA)", name, key, v)
			}
		}
	}
	return nil
}

// NormalizeSourceColors strips entries that match the source defaults, so only
// real overrides are stored — keeping cache keys stable for default colors.
func NormalizeSourceColors(colors map[string]SourceColorSet) map[string]SourceColorSet {
	var out map[string]SourceColorSet
	for _, key := range AllColorKeys() {
		c, ok := colors[key]
		if !ok {
			continue
		}
		def := DefaultColorSetForKey(key)
		var norm SourceColorSet
		if c.Accent != "" && strings.ToLower(c.Accent) != strings.ToLower(def.Accent) {
			norm.Accent = c.Accent
		}
		if c.Value != "" && strings.ToLower(c.Value) != strings.ToLower(def.Value) {
			norm.Value = c.Value
		}
		if c.Border != "" && strings.ToLower(c.Border) != strings.ToLower(def.Border) {
			norm.Border = c.Border
		}
		if c.Text != "" && strings.ToLower(c.Text) != strings.ToLower(def.Text) {
			norm.Text = c.Text
		}
		if norm.HasAny() {
			if out == nil {
				out = make(map[string]SourceColorSet)
			}
			out[key] = norm
		}
	}
	return out
}

// ParseHexColor parses #RRGGBB or #RRGGBBAA into an RGBA color.
func ParseHexColor(s string) (color.RGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	var r, g, b, a uint64
	var err error
	switch len(s) {
	case 6:
		r, err = strconv.ParseUint(s[0:2], 16, 8)
		if err == nil {
			g, err = strconv.ParseUint(s[2:4], 16, 8)
		}
		if err == nil {
			b, err = strconv.ParseUint(s[4:6], 16, 8)
		}
		a = 255
	case 8:
		r, err = strconv.ParseUint(s[0:2], 16, 8)
		if err == nil {
			g, err = strconv.ParseUint(s[2:4], 16, 8)
		}
		if err == nil {
			b, err = strconv.ParseUint(s[4:6], 16, 8)
		}
		if err == nil {
			a, err = strconv.ParseUint(s[6:8], 16, 8)
		}
	default:
		return color.RGBA{}, false
	}
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, true
}

// HexString renders an RGBA color as #RRGGBBAA (or #RRGGBB when opaque).
func HexString(c color.RGBA) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}
