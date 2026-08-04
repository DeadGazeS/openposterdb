package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func nowUTC() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func strOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

// --- Setting value constants ---

const (
	SourceTMDB   = "t"
	SourceFanart = "f"
)

type ImageSource string

const (
	ImageSourceTMDB   ImageSource = "t"
	ImageSourceFanart ImageSource = "f"
)

func ParseImageSource(s string) ImageSource {
	switch s {
	case SourceFanart:
		return ImageSourceFanart
	default:
		return ImageSourceTMDB
	}
}

func (s ImageSource) IsFanart() bool {
	return s == ImageSourceFanart
}

// --- BadgeStyle ---

type BadgeStyle string

const (
	// BadgeStyleLogoLeftValueRight: logo on the left, value on the right.
	BadgeStyleLogoLeftValueRight BadgeStyle = "lr"
	// BadgeStyleValueLeftLogoRight: value on the left, logo on the right.
	BadgeStyleValueLeftLogoRight BadgeStyle = "rl"
	// BadgeStyleLogoTB: logo on top, value on the bottom.
	BadgeStyleLogoTB BadgeStyle = "tb"
	// BadgeStyleValueTB: value on the top, logo on the bottom.
	BadgeStyleValueTB BadgeStyle = "bt"
	// BadgeStyleDefault resolves to the historical default (logo left, value right).
	BadgeStyleDefault BadgeStyle = "d"
)

func ParseBadgeStyle(s string) BadgeStyle {
	switch s {
	case "lr", "h":
		return BadgeStyleLogoLeftValueRight
	case "rl":
		return BadgeStyleValueLeftLogoRight
	case "tb", "v":
		return BadgeStyleLogoTB
	case "bt":
		return BadgeStyleValueTB
	case "d":
		return BadgeStyleDefault
	default:
		return BadgeStyleDefault
	}
}

// IsVertical reports whether the badge stacks logo and value vertically.
func (s BadgeStyle) IsVertical() bool {
	return s == BadgeStyleLogoTB || s == BadgeStyleValueTB
}

// IsMirrored reports whether the value sits on the "secondary" side (right for
// vertical pairs, left for horizontal pairs).
func (s BadgeStyle) IsMirrored() bool {
	return s == BadgeStyleValueLeftLogoRight || s == BadgeStyleValueTB
}

// Resolve resolves a default badge style via the badge direction: "d" becomes
// the horizontal logo-left/value-right style for horizontal directions and the
// logo-top/value-bottom style for vertical directions. Explicit styles are
// returned as-is; legacy "h"/"v" values are normalised to lr/tb.
func (s BadgeStyle) Resolve(direction BadgeDirection) BadgeStyle {
	if s == BadgeStyleDefault {
		if direction.IsVertical() {
			return BadgeStyleLogoTB
		}
		return BadgeStyleLogoLeftValueRight
	}
	return ParseBadgeStyle(string(s))
}

// ResolveDefault resolves a default badge style (logo left, value right).
func (s BadgeStyle) ResolveDefault() BadgeStyle {
	return s.Resolve(BadgeDirectionDefault)
}

func (s BadgeStyle) ForShape(shape BadgeShape) BadgeStyle {
	if shape == BadgeShapePill {
		return BadgeStyleLogoLeftValueRight
	}
	return s
}

// --- BadgeDirection ---

type BadgeDirection string

const (
	BadgeDirectionHorizontal BadgeDirection = "h"
	BadgeDirectionVertical   BadgeDirection = "v"
	BadgeDirectionDefault    BadgeDirection = "d"
)

func ParseBadgeDirection(s string) BadgeDirection {
	switch s {
	case "h":
		return BadgeDirectionHorizontal
	case "v":
		return BadgeDirectionVertical
	case "d":
		return BadgeDirectionDefault
	default:
		return BadgeDirectionDefault
	}
}

func (d BadgeDirection) IsVertical() bool {
	return d == BadgeDirectionVertical
}

// ResolveDefault resolves a default badge direction. Layouts place badges in
// horizontal rows on every side, so the default resolves to horizontal.
func (d BadgeDirection) ResolveDefault() BadgeDirection {
	if d != BadgeDirectionDefault {
		return d
	}
	return BadgeDirectionHorizontal
}

// --- LabelStyle ---

type LabelStyle string

const (
	LabelStyleIcon     LabelStyle = "i"
	LabelStyleText     LabelStyle = "t"
	LabelStyleOfficial LabelStyle = "o"
	LabelStyleHighRes  LabelStyle = "h"
)

func ParseLabelStyle(s string) LabelStyle {
	switch s {
	case "t":
		return LabelStyleText
	case "i":
		return LabelStyleIcon
	case "o":
		return LabelStyleOfficial
	case "h":
		return LabelStyleHighRes
	default:
		return LabelStyleOfficial
	}
}

func (l LabelStyle) UsesIcon() bool {
	return l == LabelStyleIcon || l == LabelStyleOfficial || l == LabelStyleHighRes
}

// --- BadgeShape ---

type BadgeShape string

const (
	BadgeShapeRounded BadgeShape = "r"
	BadgeShapePill    BadgeShape = "p"
)

func ParseBadgeShape(s string) BadgeShape {
	switch s {
	case "p":
		return BadgeShapePill
	default:
		return BadgeShapeRounded
	}
}

// --- BadgeAlpha ---

// BadgeAlpha is the badge background opacity as a percentage (0–100).
// 0 = fully transparent (no background), 100 = opaque.
type BadgeAlpha int32

func DefaultBadgeAlpha() BadgeAlpha { return 80 }

func ClampBadgeAlpha(v int32) BadgeAlpha {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return BadgeAlpha(v)
}

// --- BadgeAppearance ---

type BadgeAppearance struct {
	Shape  BadgeShape
	Alpha  BadgeAlpha
	Style  BadgeStyle
	Width  ScalePercent
	Height ScalePercent
}

func DefaultBadgeAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: BadgeShapeRounded, Alpha: DefaultBadgeAlpha(), Style: BadgeStyleLogoLeftValueRight, Width: DefaultScalePercent(), Height: DefaultScalePercent()}
}

// --- BadgePosition ---

type BadgePosition string

const (
	PositionBottomCenter BadgePosition = "bc"
	PositionTopCenter    BadgePosition = "tc"
	PositionLeft         BadgePosition = "l"
	PositionRight        BadgePosition = "r"
	PositionTopLeft      BadgePosition = "tl"
	PositionTopRight     BadgePosition = "tr"
	PositionBottomLeft   BadgePosition = "bl"
	PositionBottomRight  BadgePosition = "br"
)

func ParseBadgePosition(s string) BadgePosition {
	switch s {
	case "tc":
		return PositionTopCenter
	case "l":
		return PositionLeft
	case "r":
		return PositionRight
	case "tl":
		return PositionTopLeft
	case "tr":
		return PositionTopRight
	case "bl":
		return PositionBottomLeft
	case "br":
		return PositionBottomRight
	default:
		return PositionBottomCenter
	}
}

func (p BadgePosition) IsTop() bool {
	return p == PositionTopCenter || p == PositionTopLeft || p == PositionTopRight
}

func (p BadgePosition) IsBottom() bool {
	return p == PositionBottomCenter || p == PositionBottomLeft || p == PositionBottomRight
}

func (p BadgePosition) IsLeft() bool {
	return p == PositionLeft || p == PositionTopLeft || p == PositionBottomLeft
}

func (p BadgePosition) IsRight() bool {
	return p == PositionRight || p == PositionTopRight || p == PositionBottomRight
}

func (p BadgePosition) IsCenterHorizontal() bool {
	return p == PositionBottomCenter || p == PositionTopCenter
}

func (p BadgePosition) TopBottomVariants() (BadgePosition, BadgePosition) {
	if p.IsLeft() {
		return PositionTopLeft, PositionBottomLeft
	} else if p.IsRight() {
		return PositionTopRight, PositionBottomRight
	}
	return PositionTopCenter, PositionBottomCenter
}

func (p BadgePosition) LeftRightVariants() (BadgePosition, BadgePosition) {
	if p.IsTop() {
		return PositionTopLeft, PositionTopRight
	} else if p.IsBottom() {
		return PositionBottomLeft, PositionBottomRight
	}
	return PositionLeft, PositionRight
}

func (p BadgePosition) SplitAnchors(splitTopBottom bool) (BadgePosition, BadgePosition) {
	if splitTopBottom {
		top, bottom := p.TopBottomVariants()
		if p.IsTop() {
			return top, bottom
		}
		return bottom, top
	}
	left, right := p.LeftRightVariants()
	if p.IsRight() {
		return right, left
	}
	return left, right
}

// --- ScalePercent ---

// ScalePercent is a per-kind render scale as a percentage (50–400). 100 is the
// default size. Used for three independent per-kind controls: badge text size
// (`text_size`), overall badge size (`badge_size`) and rating-logo size
// (`logo_size`). At 100 each reproduces the historical default output.
type ScalePercent int32

func DefaultScalePercent() ScalePercent { return 100 }

func ClampScalePercent(v int32) ScalePercent {
	if v < 50 {
		return 50
	}
	if v > 400 {
		return 400
	}
	return ScalePercent(v)
}

func ParseScalePercent(s string) ScalePercent {
	var n int32
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		return ClampScalePercent(n)
	}
	return DefaultScalePercent()
}

// Percent returns the percentage as a fraction (e.g. 150 → 1.5). Used to scale
// font faces / logos / the badge frame; 100 → 1.0.
func (s ScalePercent) Percent() float32 {
	return float32(s) / 100.0
}

// ScaleCacheSuffix returns a cache token for a non-default scale value, using
// the given prefix (e.g. "ts" → ".ts150"). Default values add no token, keeping
// existing cache keys stable.
func ScaleCacheSuffix(prefix string, s ScalePercent) string {
	if s == DefaultScalePercent() {
		return ""
	}
	return fmt.Sprintf(".%s%d", prefix, int32(s))
}

// --- ImageSize ---

type ImageSize string

const (
	ImageSizeSmall     ImageSize = "small"
	ImageSizeMedium    ImageSize = "medium"
	ImageSizeLarge     ImageSize = "large"
	ImageSizeVeryLarge ImageSize = "very-large"
)

func ParseImageSize(s string) ImageSize {
	switch s {
	case "small":
		return ImageSizeSmall
	case "large":
		return ImageSizeLarge
	case "very-large", "verylarge":
		return ImageSizeVeryLarge
	default:
		return ImageSizeMedium
	}
}

func (s ImageSize) PosterTargetWidth() uint32 {
	switch s {
	case ImageSizeMedium:
		return 580
	case ImageSizeLarge:
		return 1280
	case ImageSizeVeryLarge:
		return 2000
	}
	return 580
}

func (s ImageSize) LogoTargetWidth() uint32 {
	switch s {
	case ImageSizeMedium:
		return 780
	case ImageSizeLarge:
		return 1722
	case ImageSizeVeryLarge:
		return 2689
	}
	return 780
}

func (s ImageSize) BackdropTargetWidth() uint32 {
	switch s {
	case ImageSizeSmall:
		return 1280
	case ImageSizeMedium:
		return 1920
	case ImageSizeLarge:
		return 3840
	case ImageSizeVeryLarge:
		return 3840
	}
	return 1920
}

func (s ImageSize) EpisodeTargetWidth() uint32 {
	switch s {
	case ImageSizeSmall:
		return 480
	case ImageSizeMedium:
		return 780
	case ImageSizeLarge:
		return 1280
	case ImageSizeVeryLarge:
		return 1920
	}
	return 780
}

func (s ImageSize) BadgeScale(kind string) float32 {
	switch kind {
	case "poster":
		return float32(s.PosterTargetWidth()) / 580.0
	case "logo":
		return float32(s.LogoTargetWidth()) / 780.0
	case "backdrop":
		return float32(s.BackdropTargetWidth()) / 1920.0
	case "episode":
		return float32(s.EpisodeTargetWidth()) / 780.0
	}
	return 1.0
}

func (s ImageSize) CacheSuffix() string {
	switch s {
	case ImageSizeSmall:
		return ".zs"
	case ImageSizeMedium:
		return ".zm"
	case ImageSizeLarge:
		return ".zl"
	case ImageSizeVeryLarge:
		return ".zvl"
	}
	return ".zm"
}

func (s ImageSize) QueryStr() string {
	return string(s)
}

func (s ImageSize) TmdbSize() string {
	switch s {
	case ImageSizeMedium:
		return "w780"
	case ImageSizeLarge, ImageSizeVeryLarge:
		return "original"
	}
	return "w780"
}

// --- PosterFit ---

type PosterFit string

const (
	PosterFitNative PosterFit = "native"
	PosterFitCover  PosterFit = "cover"
	PosterFitPad    PosterFit = "pad"
	PosterFitBlur   PosterFit = "blur"
)

func ParsePosterFit(s string) PosterFit {
	switch s {
	case "cover":
		return PosterFitCover
	case "pad":
		return PosterFitPad
	case "blur":
		return PosterFitBlur
	default:
		return PosterFitNative
	}
}

func (f PosterFit) CacheSuffix() string {
	switch f {
	case PosterFitCover:
		return ".fc"
	case PosterFitPad:
		return ".fp"
	case PosterFitBlur:
		return ".fb"
	default:
		return ""
	}
}

// --- Default values ---

func DefaultLang() string                           { return "en" }
func DefaultRatingsLimit() int32                    { return 3 }
func DefaultLogoBackdropRatingsLimit() int32        { return 5 }
func DefaultEpisodeRatingsLimit() int32             { return 1 }
func DefaultRatingsOrder() string                   { return "mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert" }
func DefaultRatingsExclude() string                 { return "" }
func DefaultPosterPosition() BadgePosition          { return PositionBottomCenter }
func DefaultPosterBadgeStyle() BadgeStyle           { return BadgeStyleDefault }
func DefaultLogoBadgeStyle() BadgeStyle             { return BadgeStyleLogoTB }
func DefaultBackdropBadgeStyle() BadgeStyle         { return BadgeStyleLogoTB }
func DefaultPosterBadgeDirection() BadgeDirection   { return BadgeDirectionDefault }
func DefaultLabelStyle() LabelStyle                 { return LabelStyleOfficial }
func DefaultBadgeShape() BadgeShape                 { return BadgeShapeRounded }
func DefaultBackdropPosition() BadgePosition        { return PositionTopRight }
func DefaultBackdropBadgeDirection() BadgeDirection { return BadgeDirectionDefault }
func DefaultEpisodePosition() BadgePosition         { return PositionTopRight }
func DefaultEpisodeBadgeStyle() BadgeStyle          { return BadgeStyleLogoTB }
func DefaultEpisodeBadgeDirection() BadgeDirection  { return BadgeDirectionVertical }
func DefaultPosterFit() PosterFit                   { return PosterFitNative }
func DefaultBackdropEdgeInset() int32               { return 0 }
func MaxEdgeInset() int32                           { return 50 }

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

func ValidateBadgesPerRow(perRow int32) error {
	if perRow >= 0 && perRow <= 10 {
		return nil
	}
	return fmt.Errorf("badges_per_row must be between 0 and 10 (0 = all in one row)")
}

func ValidateRatingsOrder(order string) error {
	if order == "" {
		return nil
	}
	seen := make(map[string]bool)
	for _, key := range strings.Split(order, ",") {
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

// RenderSettingsToMap converts effective render settings back into the flat
// key/value form stored in global_settings. Used by the admin settings update
// path so the stored representation always reflects the current settings.
func RenderSettingsToMap(s *RenderSettings) map[string]string {
	m := map[string]string{
		"image_source":             string(s.ImageSource),
		"lang":                     s.Lang,
		"textless":                 boolStr(s.Textless),
		"ratings_limit":            int32Str(s.RatingsLimit),
		"ratings_order":            s.RatingsOrder,
		"ratings_exclude":          s.RatingsExclude,
		"poster_layout":            mustMarshalLayout(&s.PosterLayout),
		"logo_ratings_limit":       int32Str(s.LogoRatingsLimit),
		"backdrop_ratings_limit":   int32Str(s.BackdropRatingsLimit),
		"poster_badge_style":       string(s.PosterBadgeStyle),
		"logo_badge_style":         string(s.LogoBadgeStyle),
		"backdrop_badge_style":     string(s.BackdropBadgeStyle),
		"poster_label_style":       string(s.PosterLabelStyle),
		"logo_label_style":         string(s.LogoLabelStyle),
		"backdrop_label_style":     string(s.BackdropLabelStyle),
		"poster_badge_direction":   string(s.PosterBadgeDirection),
		"poster_fit":               string(s.PosterFit),
		"poster_text_size":         int32Str(int32(s.PosterTextSize)),
		"logo_text_size":           int32Str(int32(s.LogoTextSize)),
		"backdrop_text_size":       int32Str(int32(s.BackdropTextSize)),
		"poster_badge_size":        int32Str(int32(s.PosterBadgeSize)),
		"logo_badge_size":          int32Str(int32(s.LogoBadgeSize)),
		"backdrop_badge_size":      int32Str(int32(s.BackdropBadgeSize)),
		"poster_badge_width":       int32Str(int32(s.PosterBadgeWidth)),
		"poster_badge_height":      int32Str(int32(s.PosterBadgeHeight)),
		"logo_badge_width":         int32Str(int32(s.LogoBadgeWidth)),
		"logo_badge_height":        int32Str(int32(s.LogoBadgeHeight)),
		"backdrop_badge_width":     int32Str(int32(s.BackdropBadgeWidth)),
		"backdrop_badge_height":    int32Str(int32(s.BackdropBadgeHeight)),
		"episode_badge_width":      int32Str(int32(s.EpisodeBadgeWidth)),
		"episode_badge_height":     int32Str(int32(s.EpisodeBadgeHeight)),
		"poster_logo_size":         int32Str(int32(s.PosterLogoSize)),
		"logo_logo_size":           int32Str(int32(s.LogoLogoSize)),
		"backdrop_logo_size":       int32Str(int32(s.BackdropLogoSize)),
		"logo_layout":              mustMarshalLayout(&s.LogoLayout),
		"backdrop_layout":          mustMarshalLayout(&s.BackdropLayout),
		"backdrop_badge_direction": string(s.BackdropBadgeDirection),
		"backdrop_edge_inset_x":    int32Str(s.BackdropEdgeInsetX),
		"backdrop_edge_inset_y":    int32Str(s.BackdropEdgeInsetY),
		"episode_ratings_limit":    int32Str(s.EpisodeRatingsLimit),
		"episode_badge_style":      string(s.EpisodeBadgeStyle),
		"episode_label_style":      string(s.EpisodeLabelStyle),
		"episode_text_size":        int32Str(int32(s.EpisodeTextSize)),
		"episode_badge_size":       int32Str(int32(s.EpisodeBadgeSize)),
		"episode_logo_size":        int32Str(int32(s.EpisodeLogoSize)),
		"episode_layout":           mustMarshalLayout(&s.EpisodeLayout),
		"episode_badge_direction":  string(s.EpisodeBadgeDirection),
		"episode_blur":             boolStr(s.EpisodeBlur),
		"poster_badge_shape":       string(s.PosterBadgeShape),
		"logo_badge_shape":         string(s.LogoBadgeShape),
		"backdrop_badge_shape":     string(s.BackdropBadgeShape),
		"episode_badge_shape":      string(s.EpisodeBadgeShape),
		"poster_badge_alpha":       int32Str(int32(s.PosterBadgeAlpha)),
		"logo_badge_alpha":         int32Str(int32(s.LogoBadgeAlpha)),
		"backdrop_badge_alpha":     int32Str(int32(s.BackdropBadgeAlpha)),
		"episode_badge_alpha":      int32Str(int32(s.EpisodeBadgeAlpha)),
	}
	for k, v := range colorsToMap(s.Colors) {
		m[k] = v
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

func CountAdminUsers(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count)
	return count, err
}

func CreateAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	now := nowUTC()
	result, err := db.Exec(
		"INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func CreateFirstAdminUser(db *sql.DB, username, passwordHash string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, fmt.Errorf("Setup already completed")
	}

	now := nowUTC()
	result, err := tx.Exec(
		"INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, now,
	)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func FindAdminUserByUsername(db *sql.DB, username string) (id int64, usernameOut string, passwordHash string, err error) {
	err = db.QueryRow("SELECT id, username, password_hash FROM admin_users WHERE username = ?", username).
		Scan(&id, &usernameOut, &passwordHash)
	return
}

func FindAdminUserByID(db *sql.DB, id int64) (username string, passwordHash string, err error) {
	err = db.QueryRow("SELECT username, password_hash FROM admin_users WHERE id = ?", id).
		Scan(&username, &passwordHash)
	return
}

// --- Refresh token CRUD ---

func CreateRefreshToken(db *sql.DB, userID int64, tokenHash, expiresAt string) (int64, error) {
	now := nowUTC()
	result, err := db.Exec(
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)",
		userID, tokenHash, expiresAt, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt string
	CreatedAt string
}

func FindRefreshTokenByHash(db *sql.DB, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken
	err := db.QueryRow(
		"SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func DeleteRefreshToken(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE id = ?", id)
	return err
}

func DeleteRefreshTokensForUser(db *sql.DB, userID int64) error {
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}

func DeleteExpiredRefreshTokens(db *sql.DB) (int64, error) {
	now := nowUTC()
	result, err := db.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- API key CRUD ---

func CreateAPIKey(db *sql.DB, name, keyHash, keyPrefix string, createdBy int64) (int64, error) {
	now := nowUTC()
	result, err := db.Exec(
		"INSERT INTO api_keys (name, key_hash, key_prefix, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		name, keyHash, keyPrefix, createdBy, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

type APIKey struct {
	ID         int64
	Name       string
	KeyHash    string
	KeyPrefix  string
	CreatedBy  int64
	CreatedAt  string
	LastUsedAt *string
}

func FindAPIKeyByHash(db *sql.DB, keyHash string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRow(
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE key_hash = ?",
		keyHash,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func FindAPIKeyByID(db *sql.DB, id int64) (*APIKey, error) {
	var k APIKey
	err := db.QueryRow(
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE id = ?",
		id,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// FindAPIKeyByName returns the API key with the given name, or nil. Used by the
// settings importer to update an existing key instead of recreating it.
func FindAPIKeyByName(db *sql.DB, name string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRow(
		"SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys WHERE name = ?",
		name,
	).Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func ListAPIKeys(db *sql.DB) ([]APIKey, error) {
	rows, err := db.Query("SELECT id, name, key_hash, key_prefix, created_by, created_at, last_used_at FROM api_keys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func DeleteAPIKey(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	return err
}

func CountAPIKeys(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&count)
	return count, err
}

// GetUserPrefs returns the admin user's stored UI preferences (JSON map).
// Unknown/missing prefs come back as an empty map.
func GetUserPrefs(db *sql.DB, username string) (map[string]string, error) {
	var raw string
	err := db.QueryRow("SELECT prefs FROM admin_users WHERE username = ?", username).Scan(&raw)
	if err != nil {
		return nil, err
	}
	prefs := map[string]string{}
	if raw != "" && raw != "{}" {
		if uerr := json.Unmarshal([]byte(raw), &prefs); uerr != nil {
			return nil, uerr
		}
	}
	return prefs, nil
}

// SetUserPrefs stores the admin user's UI preferences (JSON map).
func SetUserPrefs(db *sql.DB, username string, prefs map[string]string) error {
	if prefs == nil {
		prefs = map[string]string{}
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE admin_users SET prefs = ? WHERE username = ?", string(raw), username)
	return err
}

func BatchUpdateLastUsed(db *sql.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := nowUTC()
	batchSize := 100
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk)+1)
		args[0] = now
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j+1] = id
		}
		query := fmt.Sprintf("UPDATE api_keys SET last_used_at = ? WHERE id IN (%s)", strings.Join(placeholders, ","))
		if _, err := db.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

// --- Image meta queries ---

func CountImageMeta(db *sql.DB, imageType string) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM image_meta WHERE image_type = ?", imageType).Scan(&count)
	return count, err
}

type ImageMetaItem struct {
	CacheKey    string  `json:"cache_key"`
	ReleaseDate *string `json:"release_date"`
	ImageType   string  `json:"image_type"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

func ListImageMetaByKind(db *sql.DB, imageType string, page, pageSize int64) ([]ImageMetaItem, int64, error) {
	var total int64
	if err := db.QueryRow("SELECT COUNT(*) FROM image_meta WHERE image_type = ?", imageType).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := db.Query(
		"SELECT cache_key, release_date, image_type, created_at, updated_at FROM image_meta WHERE image_type = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		imageType, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []ImageMetaItem
	for rows.Next() {
		var item ImageMetaItem
		if err := rows.Scan(&item.CacheKey, &item.ReleaseDate, &item.ImageType, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

// --- Global settings ---

func GetGlobalSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func GetGlobalSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM global_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, nil
}

func SetGlobalSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

func SetGlobalSettingsBatch(db *sql.DB, settings map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range settings {
		if _, err := tx.Exec(
			"INSERT INTO global_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
			key, value, value,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveGlobalSettings deletes the given keys from global_settings. Used to
// drop settings that are no longer set (e.g. per-source color overrides that
// were reset to their default), since SetGlobalSettingsBatch only upserts.
func RemoveGlobalSettings(db *sql.DB, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, key := range keys {
		if _, err := tx.Exec("DELETE FROM global_settings WHERE key = ?", key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneStaleColorSettings deletes per-source color rows (color_<key>_<attr>)
// that exist in the current globals but are absent from the new batch. This
// keeps reset colors / toggled-off borders from resurrecting on the next load,
// because the batch only carries non-default overrides.
func PruneStaleColorSettings(db *sql.DB, globals, batch map[string]string) error {
	var stale []string
	for key := range globals {
		if !strings.HasPrefix(key, "color_") {
			continue
		}
		if _, ok := batch[key]; !ok {
			stale = append(stale, key)
		}
	}
	return RemoveGlobalSettings(db, stale)
}

// --- Per-key settings ---

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

func GetAPIKeySettings(db *sql.DB, apiKeyID int64) (*APIKeySettings, error) {
	var s APIKeySettings
	err := db.QueryRow(`SELECT
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

func UpsertAPIKeySettings(db *sql.DB, s *APIKeySettings) error {
	_, err := db.Exec(`INSERT INTO api_key_settings (
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

func DeleteAPIKeySettings(db *sql.DB, apiKeyID int64) error {
	_, err := db.Exec("DELETE FROM api_key_settings WHERE api_key_id = ?", apiKeyID)
	return err
}

// --- Effective render settings ---

func GetEffectiveRenderSettings(db *sql.DB, apiKeyID int64, cachedGlobals *RenderSettings) RenderSettings {
	defaults := DefaultRenderSettings()
	perKey, err := GetAPIKeySettings(db, apiKeyID)
	if err == nil && perKey != nil {
		// Effective colours: the global effective colours (the stored globals,
		// or the caller's cached globals) overlaid with the key's own overrides,
		// so a key without colour overrides renders with the global colours.
		base := defaults
		if cachedGlobals != nil {
			base = *cachedGlobals
		} else if globals, gerr := GetGlobalSettings(db); gerr == nil {
			base = ParseGlobalRenderSettings(globals)
		}
		colors := EffectiveSourceColors(&base)
		if perKey.Colors != "" {
			var overrides map[string]SourceColorSet
			if uerr := json.Unmarshal([]byte(perKey.Colors), &overrides); uerr == nil {
				for k, v := range overrides {
					colors[k] = v
				}
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

	globals, err := GetGlobalSettings(db)
	if err != nil {
		return DefaultRenderSettings()
	}
	return ParseGlobalRenderSettings(globals)
}

// --- Available ratings ---

func ReadAvailableRatings(db *sql.DB, idKey string) (sources string, updatedAt int64, releaseDate *string, err error) {
	err = db.QueryRow(
		"SELECT sources, updated_at, release_date FROM available_ratings WHERE id_key = ?",
		idKey,
	).Scan(&sources, &updatedAt, &releaseDate)
	if err == sql.ErrNoRows {
		return "", 0, nil, nil
	}
	return
}

func UpsertAvailableRatings(db *sql.DB, idKey, sources string, releaseDate *string) error {
	now := nowUnix()
	_, err := db.Exec(
		`INSERT INTO available_ratings (id_key, sources, updated_at, release_date) VALUES (?, ?, ?, ?)
		ON CONFLICT(id_key) DO UPDATE SET sources = ?, updated_at = ?, release_date = ?`,
		idKey, sources, now, releaseDate, sources, now, releaseDate,
	)
	return err
}

func DeleteAvailableRatings(db *sql.DB, idKey string) (int64, error) {
	result, err := db.Exec("DELETE FROM available_ratings WHERE id_key = ?", idKey)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteAllAvailableRatings(db *sql.DB) (int64, error) {
	result, err := db.Exec("DELETE FROM available_ratings")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Image meta bulk operations ---

func DeleteImageMetaByKind(db *sql.DB, imageType string) (int64, error) {
	result, err := db.Exec("DELETE FROM image_meta WHERE image_type = ?", imageType)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func DeleteAllImageMeta(db *sql.DB) (int64, error) {
	result, err := db.Exec("DELETE FROM image_meta")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func UpsertImageMeta(db *sql.DB, cacheKey string, releaseDate *string, imageType string) error {
	now := nowUnix()
	_, err := db.Exec(
		`INSERT INTO image_meta (cache_key, release_date, image_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET release_date = ?, updated_at = ?`,
		cacheKey, releaseDate, imageType, now, now, releaseDate, now,
	)
	return err
}

func ReadImageMeta(db *sql.DB, cacheKey string) (*string, error) {
	var releaseDate *string
	err := db.QueryRow("SELECT release_date FROM image_meta WHERE cache_key = ?", cacheKey).Scan(&releaseDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return releaseDate, err
}

// --- Rating key validation ---

var validRatingKeys = map[string]bool{
	"imdb": true, "tmdb": true, "rt": true, "rta": true,
	"mc": true, "trakt": true, "lb": true, "mal": true,
	"mdblist": true, "ebert": true,
}

func isValidRatingKey(key string) bool {
	return validRatingKeys[key]
}

func allRatingKeys() string {
	return "imdb, tmdb, rt, rta, mc, trakt, lb, mal, mdblist, ebert"
}

// --- Marshal helpers (used by admin endpoints) ---

func MarshalStruct(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// --- Image type helpers ---

func ImageSubdirByType(imageType string) string {
	switch imageType {
	case "p":
		return "posters"
	case "l":
		return "logos"
	case "b":
		return "backdrops"
	case "e":
		return "episodes"
	}
	return ""
}

func ImageExtByType(imageType string) string {
	switch imageType {
	case "l":
		return "png"
	default:
		return "jpg"
	}
}

// --- Single variant exact delete ---

func DeleteImageMetaExact(db *sql.DB, imageType, cacheKey string) (int64, error) {
	result, err := db.Exec(
		"DELETE FROM image_meta WHERE cache_key = ? AND image_type = ?",
		cacheKey, imageType,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Title-scoped delete ---

func DeleteImageMetaForTitle(db *sql.DB, imageType, idType, idValue string) (int64, error) {
	prefix := idType + "/" + idValue

	rows, err := db.Query(
		"SELECT cache_key FROM image_meta WHERE image_type = ? AND cache_key LIKE ?",
		imageType, prefix+"%",
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			continue
		}
		if TitleFileMatch(k, prefix) {
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		placeholders[i] = "?"
		args[i] = k
	}

	query := fmt.Sprintf(
		"DELETE FROM image_meta WHERE cache_key IN (%s)",
		strings.Join(placeholders, ","),
	)
	result, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
