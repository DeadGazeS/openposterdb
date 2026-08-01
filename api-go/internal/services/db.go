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
	BadgeStyleHorizontal BadgeStyle = "h"
	BadgeStyleVertical   BadgeStyle = "v"
	BadgeStyleDefault    BadgeStyle = "d"
)

func ParseBadgeStyle(s string) BadgeStyle {
	switch s {
	case "h":
		return BadgeStyleHorizontal
	case "v":
		return BadgeStyleVertical
	case "d":
		return BadgeStyleDefault
	default:
		return BadgeStyleDefault
	}
}

func (s BadgeStyle) IsVertical() bool {
	return s == BadgeStyleVertical
}

func (s BadgeStyle) Resolve(direction BadgeDirection) BadgeStyle {
	if s != BadgeStyleDefault {
		return s
	}
	if direction.IsVertical() {
		return BadgeStyleVertical
	}
	return BadgeStyleHorizontal
}

func (s BadgeStyle) ForShape(shape BadgeShape) BadgeStyle {
	if shape == BadgeShapePill {
		return BadgeStyleHorizontal
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

func (d BadgeDirection) Resolve(position BadgePosition) BadgeDirection {
	if d != BadgeDirectionDefault {
		return d
	}
	if position.IsCenterHorizontal() {
		return BadgeDirectionHorizontal
	}
	return BadgeDirectionVertical
}

// --- LabelStyle ---

type LabelStyle string

const (
	LabelStyleIcon     LabelStyle = "i"
	LabelStyleText     LabelStyle = "t"
	LabelStyleOfficial LabelStyle = "o"
)

func ParseLabelStyle(s string) LabelStyle {
	switch s {
	case "t":
		return LabelStyleText
	case "i":
		return LabelStyleIcon
	case "o":
		return LabelStyleOfficial
	default:
		return LabelStyleOfficial
	}
}

func (l LabelStyle) UsesIcon() bool {
	return l == LabelStyleIcon || l == LabelStyleOfficial
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

// --- BadgeBackground ---

type BadgeBackground string

const (
	BadgeBackgroundDefault     BadgeBackground = "d"
	BadgeBackgroundDark        BadgeBackground = "k"
	BadgeBackgroundTransparent BadgeBackground = "t"
	BadgeBackgroundNone        BadgeBackground = "n"
)

func ParseBadgeBackground(s string) BadgeBackground {
	switch s {
	case "k":
		return BadgeBackgroundDark
	case "t":
		return BadgeBackgroundTransparent
	case "n":
		return BadgeBackgroundNone
	default:
		return BadgeBackgroundDefault
	}
}

// --- BadgeAppearance ---

type BadgeAppearance struct {
	Shape      BadgeShape
	Background BadgeBackground
}

func DefaultBadgeAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: BadgeShapeRounded, Background: BadgeBackgroundDefault}
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

// --- BadgeSize ---

type BadgeSize string

const (
	BadgeSizeExtraSmall BadgeSize = "xs"
	BadgeSizeSmall      BadgeSize = "s"
	BadgeSizeMedium     BadgeSize = "m"
	BadgeSizeLarge      BadgeSize = "l"
	BadgeSizeExtraLarge BadgeSize = "xl"
)

func ParseBadgeSize(s string) BadgeSize {
	switch s {
	case "xs":
		return BadgeSizeExtraSmall
	case "s":
		return BadgeSizeSmall
	case "l":
		return BadgeSizeLarge
	case "xl":
		return BadgeSizeExtraLarge
	default:
		return BadgeSizeMedium
	}
}

func (s BadgeSize) ScaleFactor() float32 {
	switch s {
	case BadgeSizeExtraSmall:
		return 0.7
	case BadgeSizeSmall:
		return 0.95
	case BadgeSizeMedium:
		return 1.2
	case BadgeSizeLarge:
		return 1.45
	case BadgeSizeExtraLarge:
		return 1.7
	}
	return 1.2
}

func (s BadgeSize) CacheSuffix() string {
	switch s {
	case BadgeSizeExtraSmall:
		return ".bxs"
	case BadgeSizeSmall:
		return ".bs"
	case BadgeSizeMedium:
		return ".bm"
	case BadgeSizeLarge:
		return ".bl"
	case BadgeSizeExtraLarge:
		return ".bxl"
	}
	return ".bm"
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

func DefaultLang() string                       { return "en" }
func DefaultRatingsLimit() int32                { return 3 }
func DefaultLogoBackdropRatingsLimit() int32    { return 5 }
func DefaultEpisodeRatingsLimit() int32         { return 1 }
func DefaultRatingsOrder() string               { return "mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert" }
func DefaultRatingsExclude() string             { return "" }
func DefaultPosterPosition() BadgePosition      { return PositionBottomCenter }
func DefaultPosterBadgeStyle() BadgeStyle       { return BadgeStyleDefault }
func DefaultLogoBadgeStyle() BadgeStyle         { return BadgeStyleVertical }
func DefaultBackdropBadgeStyle() BadgeStyle     { return BadgeStyleVertical }
func DefaultLabelStyle() LabelStyle             { return LabelStyleOfficial }
func DefaultBadgeShape() BadgeShape             { return BadgeShapeRounded }
func DefaultBadgeBackground() BadgeBackground    { return BadgeBackgroundDefault }
func DefaultPosterBadgeDirection() BadgeDirection { return BadgeDirectionDefault }
func DefaultBackdropPosition() BadgePosition    { return PositionTopRight }
func DefaultBackdropBadgeDirection() BadgeDirection { return BadgeDirectionDefault }
func DefaultEpisodePosition() BadgePosition     { return PositionTopRight }
func DefaultEpisodeBadgeStyle() BadgeStyle      { return BadgeStyleVertical }
func DefaultEpisodeBadgeDirection() BadgeDirection { return BadgeDirectionVertical }
func DefaultEpisodeBadgeSize() BadgeSize        { return BadgeSizeLarge }
func DefaultBadgeSize() BadgeSize               { return BadgeSizeMedium }
func DefaultPosterFit() PosterFit               { return PosterFitNative }
func DefaultBackdropEdgeInset() int32           { return 0 }
func MaxEdgeInset() int32                       { return 50 }

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
	ImageSource               ImageSource     `json:"image_source"`
	Lang                      string          `json:"lang"`
	Textless                  bool            `json:"textless"`
	RatingsLimit              int32           `json:"ratings_limit"`
	RatingsOrder              string          `json:"ratings_order"`
	RatingsExclude            string          `json:"ratings_exclude"`
	IsDefault                 bool            `json:"is_default"`
	PosterPosition            BadgePosition   `json:"poster_position"`
	LogoRatingsLimit          int32           `json:"logo_ratings_limit"`
	BackdropRatingsLimit      int32           `json:"backdrop_ratings_limit"`
	PosterBadgeStyle          BadgeStyle      `json:"poster_badge_style"`
	LogoBadgeStyle            BadgeStyle      `json:"logo_badge_style"`
	BackdropBadgeStyle        BadgeStyle      `json:"backdrop_badge_style"`
	PosterLabelStyle          LabelStyle      `json:"poster_label_style"`
	LogoLabelStyle            LabelStyle      `json:"logo_label_style"`
	BackdropLabelStyle        LabelStyle      `json:"backdrop_label_style"`
	PosterBadgeDirection      BadgeDirection  `json:"poster_badge_direction"`
	PosterBadgeSplit          bool            `json:"poster_badge_split"`
	PosterFit                 PosterFit       `json:"poster_fit"`
	PosterBadgeSize           BadgeSize       `json:"poster_badge_size"`
	LogoBadgeSize             BadgeSize       `json:"logo_badge_size"`
	BackdropBadgeSize         BadgeSize       `json:"backdrop_badge_size"`
	BackdropPosition          BadgePosition   `json:"backdrop_position"`
	BackdropBadgeDirection    BadgeDirection  `json:"backdrop_badge_direction"`
	BackdropEdgeInsetX        int32           `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY        int32           `json:"backdrop_edge_inset_y"`
	EpisodeRatingsLimit       int32           `json:"episode_ratings_limit"`
	EpisodeBadgeStyle         BadgeStyle      `json:"episode_badge_style"`
	EpisodeLabelStyle         LabelStyle      `json:"episode_label_style"`
	EpisodeBadgeSize          BadgeSize       `json:"episode_badge_size"`
	EpisodePosition           BadgePosition   `json:"episode_position"`
	EpisodeBadgeDirection     BadgeDirection  `json:"episode_badge_direction"`
	EpisodeBlur               bool            `json:"episode_blur"`
	PosterBadgeShape          BadgeShape      `json:"poster_badge_shape"`
	LogoBadgeShape            BadgeShape      `json:"logo_badge_shape"`
	BackdropBadgeShape        BadgeShape      `json:"backdrop_badge_shape"`
	EpisodeBadgeShape         BadgeShape      `json:"episode_badge_shape"`
	PosterBadgeBackground     BadgeBackground `json:"poster_badge_background"`
	LogoBadgeBackground       BadgeBackground `json:"logo_badge_background"`
	BackdropBadgeBackground   BadgeBackground `json:"backdrop_badge_background"`
	EpisodeBadgeBackground    BadgeBackground `json:"episode_badge_background"`
}

func DefaultRenderSettings() RenderSettings {
	return RenderSettings{
		ImageSource:               ImageSourceTMDB,
		Lang:                      "en",
		Textless:                  false,
		RatingsLimit:              3,
		RatingsOrder:              "mal,imdb,lb,rt,mc,rta,tmdb,trakt,mdblist,ebert",
		RatingsExclude:            "",
		IsDefault:                 true,
		PosterPosition:            PositionBottomCenter,
		LogoRatingsLimit:          5,
		BackdropRatingsLimit:      5,
		PosterBadgeStyle:          BadgeStyleDefault,
		LogoBadgeStyle:            BadgeStyleVertical,
		BackdropBadgeStyle:        BadgeStyleVertical,
		PosterLabelStyle:          LabelStyleOfficial,
		LogoLabelStyle:            LabelStyleOfficial,
		BackdropLabelStyle:        LabelStyleOfficial,
		PosterBadgeDirection:      BadgeDirectionDefault,
		PosterBadgeSplit:          false,
		PosterFit:                 PosterFitNative,
		PosterBadgeSize:           BadgeSizeMedium,
		LogoBadgeSize:             BadgeSizeMedium,
		BackdropBadgeSize:         BadgeSizeMedium,
		BackdropPosition:          PositionTopRight,
		BackdropBadgeDirection:    BadgeDirectionDefault,
		BackdropEdgeInsetX:        0,
		BackdropEdgeInsetY:        0,
		EpisodeRatingsLimit:       1,
		EpisodeBadgeStyle:         BadgeStyleVertical,
		EpisodeLabelStyle:         LabelStyleOfficial,
		EpisodeBadgeSize:          BadgeSizeLarge,
		EpisodePosition:           PositionTopRight,
		EpisodeBadgeDirection:     BadgeDirectionVertical,
		EpisodeBlur:               false,
		PosterBadgeShape:          BadgeShapeRounded,
		LogoBadgeShape:            BadgeShapeRounded,
		BackdropBadgeShape:        BadgeShapeRounded,
		EpisodeBadgeShape:         BadgeShapeRounded,
		PosterBadgeBackground:     BadgeBackgroundDefault,
		LogoBadgeBackground:       BadgeBackgroundDefault,
		BackdropBadgeBackground:   BadgeBackgroundDefault,
		EpisodeBadgeBackground:    BadgeBackgroundDefault,
	}
}

func (s *RenderSettings) PosterAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.PosterBadgeShape, Background: s.PosterBadgeBackground}
}

func (s *RenderSettings) LogoAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.LogoBadgeShape, Background: s.LogoBadgeBackground}
}

func (s *RenderSettings) BackdropAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.BackdropBadgeShape, Background: s.BackdropBadgeBackground}
}

func (s *RenderSettings) EpisodeAppearance() BadgeAppearance {
	return BadgeAppearance{Shape: s.EpisodeBadgeShape, Background: s.EpisodeBadgeBackground}
}

func ParseGlobalRenderSettings(globals map[string]string) RenderSettings {
	if len(globals) == 0 {
		return DefaultRenderSettings()
	}
	defaults := DefaultRenderSettings()

	return RenderSettings{
		ImageSource:              ImageSource(stringOr(globals, "image_source", string(defaults.ImageSource))),
		Lang:                     stringOr(globals, "lang", defaults.Lang),
		Textless:                 boolOr(globals, "textless", defaults.Textless),
		RatingsLimit:             int32Or(globals, "ratings_limit", defaults.RatingsLimit),
		RatingsOrder:             stringOr(globals, "ratings_order", defaults.RatingsOrder),
		RatingsExclude:           stringOr(globals, "ratings_exclude", defaults.RatingsExclude),
		IsDefault:                true,
		PosterPosition:           BadgePosition(stringOr(globals, "poster_position", string(defaults.PosterPosition))),
		LogoRatingsLimit:         int32Or(globals, "logo_ratings_limit", defaults.LogoRatingsLimit),
		BackdropRatingsLimit:     int32Or(globals, "backdrop_ratings_limit", defaults.BackdropRatingsLimit),
		PosterBadgeStyle:         BadgeStyle(stringOr(globals, "poster_badge_style", string(defaults.PosterBadgeStyle))),
		LogoBadgeStyle:           BadgeStyle(stringOr(globals, "logo_badge_style", string(defaults.LogoBadgeStyle))),
		BackdropBadgeStyle:       BadgeStyle(stringOr(globals, "backdrop_badge_style", string(defaults.BackdropBadgeStyle))),
		PosterLabelStyle:         LabelStyle(stringOr(globals, "poster_label_style", string(defaults.PosterLabelStyle))),
		LogoLabelStyle:           LabelStyle(stringOr(globals, "logo_label_style", string(defaults.LogoLabelStyle))),
		BackdropLabelStyle:       LabelStyle(stringOr(globals, "backdrop_label_style", string(defaults.BackdropLabelStyle))),
		PosterBadgeDirection:     BadgeDirection(stringOr(globals, "poster_badge_direction", string(defaults.PosterBadgeDirection))),
		PosterBadgeSplit:         boolOr(globals, "poster_badge_split", defaults.PosterBadgeSplit),
		PosterFit:                PosterFit(stringOr(globals, "poster_fit", string(defaults.PosterFit))),
		PosterBadgeSize:          BadgeSize(stringOr(globals, "poster_badge_size", string(defaults.PosterBadgeSize))),
		LogoBadgeSize:            BadgeSize(stringOr(globals, "logo_badge_size", string(defaults.LogoBadgeSize))),
		BackdropBadgeSize:        BadgeSize(stringOr(globals, "backdrop_badge_size", string(defaults.BackdropBadgeSize))),
		BackdropPosition:         BadgePosition(stringOr(globals, "backdrop_position", string(defaults.BackdropPosition))),
		BackdropBadgeDirection:   BadgeDirection(stringOr(globals, "backdrop_badge_direction", string(defaults.BackdropBadgeDirection))),
		BackdropEdgeInsetX:       int32ClampOr(int32Or(globals, "backdrop_edge_inset_x", defaults.BackdropEdgeInsetX)),
		BackdropEdgeInsetY:       int32ClampOr(int32Or(globals, "backdrop_edge_inset_y", defaults.BackdropEdgeInsetY)),
		EpisodeRatingsLimit:      int32Or(globals, "episode_ratings_limit", defaults.EpisodeRatingsLimit),
		EpisodeBadgeStyle:        BadgeStyle(stringOr(globals, "episode_badge_style", string(defaults.EpisodeBadgeStyle))),
		EpisodeLabelStyle:        LabelStyle(stringOr(globals, "episode_label_style", string(defaults.EpisodeLabelStyle))),
		EpisodeBadgeSize:         BadgeSize(stringOr(globals, "episode_badge_size", string(defaults.EpisodeBadgeSize))),
		EpisodePosition:          BadgePosition(stringOr(globals, "episode_position", string(defaults.EpisodePosition))),
		EpisodeBadgeDirection:    BadgeDirection(stringOr(globals, "episode_badge_direction", string(defaults.EpisodeBadgeDirection))),
		EpisodeBlur:              boolOr(globals, "episode_blur", defaults.EpisodeBlur),
		PosterBadgeShape:         BadgeShape(stringOr(globals, "poster_badge_shape", string(defaults.PosterBadgeShape))),
		LogoBadgeShape:           BadgeShape(stringOr(globals, "logo_badge_shape", string(defaults.LogoBadgeShape))),
		BackdropBadgeShape:       BadgeShape(stringOr(globals, "backdrop_badge_shape", string(defaults.BackdropBadgeShape))),
		EpisodeBadgeShape:        BadgeShape(stringOr(globals, "episode_badge_shape", string(defaults.EpisodeBadgeShape))),
		PosterBadgeBackground:    BadgeBackground(stringOr(globals, "poster_badge_background", string(defaults.PosterBadgeBackground))),
		LogoBadgeBackground:      BadgeBackground(stringOr(globals, "logo_badge_background", string(defaults.LogoBadgeBackground))),
		BackdropBadgeBackground:  BadgeBackground(stringOr(globals, "backdrop_badge_background", string(defaults.BackdropBadgeBackground))),
		EpisodeBadgeBackground:   BadgeBackground(stringOr(globals, "episode_badge_background", string(defaults.EpisodeBadgeBackground))),
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
	return map[string]string{
		"image_source":              string(s.ImageSource),
		"lang":                      s.Lang,
		"textless":                  boolStr(s.Textless),
		"ratings_limit":             int32Str(s.RatingsLimit),
		"ratings_order":             s.RatingsOrder,
		"ratings_exclude":           s.RatingsExclude,
		"poster_position":           string(s.PosterPosition),
		"logo_ratings_limit":        int32Str(s.LogoRatingsLimit),
		"backdrop_ratings_limit":    int32Str(s.BackdropRatingsLimit),
		"poster_badge_style":        string(s.PosterBadgeStyle),
		"logo_badge_style":          string(s.LogoBadgeStyle),
		"backdrop_badge_style":      string(s.BackdropBadgeStyle),
		"poster_label_style":        string(s.PosterLabelStyle),
		"logo_label_style":          string(s.LogoLabelStyle),
		"backdrop_label_style":      string(s.BackdropLabelStyle),
		"poster_badge_direction":    string(s.PosterBadgeDirection),
		"poster_badge_split":        boolStr(s.PosterBadgeSplit),
		"poster_fit":                string(s.PosterFit),
		"poster_badge_size":         string(s.PosterBadgeSize),
		"logo_badge_size":           string(s.LogoBadgeSize),
		"backdrop_badge_size":       string(s.BackdropBadgeSize),
		"backdrop_position":         string(s.BackdropPosition),
		"backdrop_badge_direction":  string(s.BackdropBadgeDirection),
		"backdrop_edge_inset_x":     int32Str(s.BackdropEdgeInsetX),
		"backdrop_edge_inset_y":     int32Str(s.BackdropEdgeInsetY),
		"episode_ratings_limit":     int32Str(s.EpisodeRatingsLimit),
		"episode_badge_style":       string(s.EpisodeBadgeStyle),
		"episode_label_style":       string(s.EpisodeLabelStyle),
		"episode_badge_size":        string(s.EpisodeBadgeSize),
		"episode_position":          string(s.EpisodePosition),
		"episode_badge_direction":   string(s.EpisodeBadgeDirection),
		"episode_blur":              boolStr(s.EpisodeBlur),
		"poster_badge_shape":        string(s.PosterBadgeShape),
		"logo_badge_shape":          string(s.LogoBadgeShape),
		"backdrop_badge_shape":      string(s.BackdropBadgeShape),
		"episode_badge_shape":       string(s.EpisodeBadgeShape),
		"poster_badge_background":   string(s.PosterBadgeBackground),
		"logo_badge_background":     string(s.LogoBadgeBackground),
		"backdrop_badge_background": string(s.BackdropBadgeBackground),
		"episode_badge_background":  string(s.EpisodeBadgeBackground),
	}
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
	return nil
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

// --- Per-key settings ---

type APIKeySettings struct {
	APIKeyID                 int64  `json:"api_key_id"`
	ImageSource              string `json:"image_source"`
	Lang                     string `json:"lang"`
	Textless                 bool   `json:"textless"`
	RatingsLimit             int32  `json:"ratings_limit"`
	RatingsOrder             string `json:"ratings_order"`
	RatingsExclude           string `json:"ratings_exclude"`
	PosterPosition           string `json:"poster_position"`
	LogoRatingsLimit         int32  `json:"logo_ratings_limit"`
	BackdropRatingsLimit     int32  `json:"backdrop_ratings_limit"`
	PosterBadgeStyle         string `json:"poster_badge_style"`
	LogoBadgeStyle           string `json:"logo_badge_style"`
	BackdropBadgeStyle       string `json:"backdrop_badge_style"`
	PosterLabelStyle         string `json:"poster_label_style"`
	LogoLabelStyle           string `json:"logo_label_style"`
	BackdropLabelStyle       string `json:"backdrop_label_style"`
	PosterBadgeDirection     string `json:"poster_badge_direction"`
	PosterBadgeSplit         bool   `json:"poster_badge_split"`
	PosterFit                string `json:"poster_fit"`
	PosterBadgeSize          string `json:"poster_badge_size"`
	LogoBadgeSize            string `json:"logo_badge_size"`
	BackdropBadgeSize        string `json:"backdrop_badge_size"`
	BackdropPosition         string `json:"backdrop_position"`
	BackdropBadgeDirection   string `json:"backdrop_badge_direction"`
	EpisodeRatingsLimit      int32  `json:"episode_ratings_limit"`
	EpisodeBadgeStyle        string `json:"episode_badge_style"`
	EpisodeLabelStyle        string `json:"episode_label_style"`
	EpisodeBadgeSize         string `json:"episode_badge_size"`
	EpisodePosition          string `json:"episode_position"`
	EpisodeBadgeDirection    string `json:"episode_badge_direction"`
	EpisodeBlur              bool   `json:"episode_blur"`
	PosterBadgeShape         string `json:"poster_badge_shape"`
	LogoBadgeShape           string `json:"logo_badge_shape"`
	BackdropBadgeShape       string `json:"backdrop_badge_shape"`
	EpisodeBadgeShape        string `json:"episode_badge_shape"`
	PosterBadgeBackground    string `json:"poster_badge_background"`
	LogoBadgeBackground      string `json:"logo_badge_background"`
	BackdropBadgeBackground  string `json:"backdrop_badge_background"`
	EpisodeBadgeBackground   string `json:"episode_badge_background"`
	BackdropEdgeInsetX       int32  `json:"backdrop_edge_inset_x"`
	BackdropEdgeInsetY       int32  `json:"backdrop_edge_inset_y"`
}

func GetAPIKeySettings(db *sql.DB, apiKeyID int64) (*APIKeySettings, error) {
	var s APIKeySettings
	err := db.QueryRow(`SELECT
		api_key_id, image_source, lang, textless, ratings_limit, ratings_order, ratings_exclude,
		poster_position, logo_ratings_limit, backdrop_ratings_limit,
		poster_badge_style, logo_badge_style, backdrop_badge_style,
		poster_label_style, logo_label_style, backdrop_label_style,
		poster_badge_direction, poster_badge_split, poster_fit,
		poster_badge_size, logo_badge_size, backdrop_badge_size,
		backdrop_position, backdrop_badge_direction,
		episode_ratings_limit, episode_badge_style, episode_label_style, episode_badge_size,
		episode_position, episode_badge_direction, episode_blur,
		poster_badge_shape, logo_badge_shape, backdrop_badge_shape, episode_badge_shape,
		poster_badge_background, logo_badge_background, backdrop_badge_background, episode_badge_background,
		backdrop_edge_inset_x, backdrop_edge_inset_y
		FROM api_key_settings WHERE api_key_id = ?`, apiKeyID).Scan(
		&s.APIKeyID, &s.ImageSource, &s.Lang, &s.Textless, &s.RatingsLimit, &s.RatingsOrder, &s.RatingsExclude,
		&s.PosterPosition, &s.LogoRatingsLimit, &s.BackdropRatingsLimit,
		&s.PosterBadgeStyle, &s.LogoBadgeStyle, &s.BackdropBadgeStyle,
		&s.PosterLabelStyle, &s.LogoLabelStyle, &s.BackdropLabelStyle,
		&s.PosterBadgeDirection, &s.PosterBadgeSplit, &s.PosterFit,
		&s.PosterBadgeSize, &s.LogoBadgeSize, &s.BackdropBadgeSize,
		&s.BackdropPosition, &s.BackdropBadgeDirection,
		&s.EpisodeRatingsLimit, &s.EpisodeBadgeStyle, &s.EpisodeLabelStyle, &s.EpisodeBadgeSize,
		&s.EpisodePosition, &s.EpisodeBadgeDirection, &s.EpisodeBlur,
		&s.PosterBadgeShape, &s.LogoBadgeShape, &s.BackdropBadgeShape, &s.EpisodeBadgeShape,
		&s.PosterBadgeBackground, &s.LogoBadgeBackground, &s.BackdropBadgeBackground, &s.EpisodeBadgeBackground,
		&s.BackdropEdgeInsetX, &s.BackdropEdgeInsetY,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func UpsertAPIKeySettings(db *sql.DB, s *APIKeySettings) error {
	_, err := db.Exec(`INSERT INTO api_key_settings (
		api_key_id, image_source, lang, textless, ratings_limit, ratings_order, ratings_exclude,
		poster_position, logo_ratings_limit, backdrop_ratings_limit,
		poster_badge_style, logo_badge_style, backdrop_badge_style,
		poster_label_style, logo_label_style, backdrop_label_style,
		poster_badge_direction, poster_badge_split, poster_fit,
		poster_badge_size, logo_badge_size, backdrop_badge_size,
		backdrop_position, backdrop_badge_direction,
		episode_ratings_limit, episode_badge_style, episode_label_style, episode_badge_size,
		episode_position, episode_badge_direction, episode_blur,
		poster_badge_shape, logo_badge_shape, backdrop_badge_shape, episode_badge_shape,
		poster_badge_background, logo_badge_background, backdrop_badge_background, episode_badge_background,
		backdrop_edge_inset_x, backdrop_edge_inset_y
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(api_key_id) DO UPDATE SET
		image_source = excluded.image_source,
		lang = excluded.lang,
		textless = excluded.textless,
		ratings_limit = excluded.ratings_limit,
		ratings_order = excluded.ratings_order,
		ratings_exclude = excluded.ratings_exclude,
		poster_position = excluded.poster_position,
		logo_ratings_limit = excluded.logo_ratings_limit,
		backdrop_ratings_limit = excluded.backdrop_ratings_limit,
		poster_badge_style = excluded.poster_badge_style,
		logo_badge_style = excluded.logo_badge_style,
		backdrop_badge_style = excluded.backdrop_badge_style,
		poster_label_style = excluded.poster_label_style,
		logo_label_style = excluded.logo_label_style,
		backdrop_label_style = excluded.backdrop_label_style,
		poster_badge_direction = excluded.poster_badge_direction,
		poster_badge_split = excluded.poster_badge_split,
		poster_fit = excluded.poster_fit,
		poster_badge_size = excluded.poster_badge_size,
		logo_badge_size = excluded.logo_badge_size,
		backdrop_badge_size = excluded.backdrop_badge_size,
		backdrop_position = excluded.backdrop_position,
		backdrop_badge_direction = excluded.backdrop_badge_direction,
		episode_ratings_limit = excluded.episode_ratings_limit,
		episode_badge_style = excluded.episode_badge_style,
		episode_label_style = excluded.episode_label_style,
		episode_badge_size = excluded.episode_badge_size,
		episode_position = excluded.episode_position,
		episode_badge_direction = excluded.episode_badge_direction,
		episode_blur = excluded.episode_blur,
		poster_badge_shape = excluded.poster_badge_shape,
		logo_badge_shape = excluded.logo_badge_shape,
		backdrop_badge_shape = excluded.backdrop_badge_shape,
		episode_badge_shape = excluded.episode_badge_shape,
		poster_badge_background = excluded.poster_badge_background,
		logo_badge_background = excluded.logo_badge_background,
		backdrop_badge_background = excluded.backdrop_badge_background,
		episode_badge_background = excluded.episode_badge_background,
		backdrop_edge_inset_x = excluded.backdrop_edge_inset_x,
		backdrop_edge_inset_y = excluded.backdrop_edge_inset_y`,
		s.APIKeyID, s.ImageSource, s.Lang, s.Textless, s.RatingsLimit, s.RatingsOrder, s.RatingsExclude,
		s.PosterPosition, s.LogoRatingsLimit, s.BackdropRatingsLimit,
		s.PosterBadgeStyle, s.LogoBadgeStyle, s.BackdropBadgeStyle,
		s.PosterLabelStyle, s.LogoLabelStyle, s.BackdropLabelStyle,
		s.PosterBadgeDirection, s.PosterBadgeSplit, s.PosterFit,
		s.PosterBadgeSize, s.LogoBadgeSize, s.BackdropBadgeSize,
		s.BackdropPosition, s.BackdropBadgeDirection,
		s.EpisodeRatingsLimit, s.EpisodeBadgeStyle, s.EpisodeLabelStyle, s.EpisodeBadgeSize,
		s.EpisodePosition, s.EpisodeBadgeDirection, s.EpisodeBlur,
		s.PosterBadgeShape, s.LogoBadgeShape, s.BackdropBadgeShape, s.EpisodeBadgeShape,
		s.PosterBadgeBackground, s.LogoBadgeBackground, s.BackdropBadgeBackground, s.EpisodeBadgeBackground,
		s.BackdropEdgeInsetX, s.BackdropEdgeInsetY,
	)
	return err
}

func DeleteAPIKeySettings(db *sql.DB, apiKeyID int64) error {
	_, err := db.Exec("DELETE FROM api_key_settings WHERE api_key_id = ?", apiKeyID)
	return err
}

// --- Effective render settings ---

func GetEffectiveRenderSettings(db *sql.DB, apiKeyID int64, cachedGlobals *RenderSettings) RenderSettings {
	perKey, err := GetAPIKeySettings(db, apiKeyID)
	if err == nil && perKey != nil {
		return RenderSettings{
			ImageSource:            ImageSource(perKey.ImageSource),
			Lang:                   strOrDefault(perKey.Lang, "en"),
			Textless:               perKey.Textless,
			RatingsLimit:           perKey.RatingsLimit,
			RatingsOrder:           perKey.RatingsOrder,
			RatingsExclude:         perKey.RatingsExclude,
			IsDefault:              false,
			PosterPosition:         BadgePosition(perKey.PosterPosition),
			LogoRatingsLimit:       perKey.LogoRatingsLimit,
			BackdropRatingsLimit:   perKey.BackdropRatingsLimit,
			PosterBadgeStyle:       BadgeStyle(perKey.PosterBadgeStyle),
			LogoBadgeStyle:         BadgeStyle(perKey.LogoBadgeStyle),
			BackdropBadgeStyle:     BadgeStyle(perKey.BackdropBadgeStyle),
			PosterLabelStyle:       LabelStyle(perKey.PosterLabelStyle),
			LogoLabelStyle:         LabelStyle(perKey.LogoLabelStyle),
			BackdropLabelStyle:     LabelStyle(perKey.BackdropLabelStyle),
			PosterBadgeDirection:   BadgeDirection(perKey.PosterBadgeDirection),
			PosterBadgeSplit:       perKey.PosterBadgeSplit,
			PosterFit:              PosterFit(perKey.PosterFit),
			PosterBadgeSize:        BadgeSize(perKey.PosterBadgeSize),
			LogoBadgeSize:          BadgeSize(perKey.LogoBadgeSize),
			BackdropBadgeSize:      BadgeSize(perKey.BackdropBadgeSize),
			BackdropPosition:       BadgePosition(perKey.BackdropPosition),
			BackdropBadgeDirection: BadgeDirection(perKey.BackdropBadgeDirection),
			BackdropEdgeInsetX:     ClampEdgeInset(perKey.BackdropEdgeInsetX),
			BackdropEdgeInsetY:     ClampEdgeInset(perKey.BackdropEdgeInsetY),
			EpisodeRatingsLimit:    perKey.EpisodeRatingsLimit,
			EpisodeBadgeStyle:      BadgeStyle(perKey.EpisodeBadgeStyle),
			EpisodeLabelStyle:      LabelStyle(perKey.EpisodeLabelStyle),
			EpisodeBadgeSize:       BadgeSize(perKey.EpisodeBadgeSize),
			EpisodePosition:        BadgePosition(perKey.EpisodePosition),
			EpisodeBadgeDirection:  BadgeDirection(perKey.EpisodeBadgeDirection),
			EpisodeBlur:            perKey.EpisodeBlur,
			PosterBadgeShape:       BadgeShape(perKey.PosterBadgeShape),
			LogoBadgeShape:         BadgeShape(perKey.LogoBadgeShape),
			BackdropBadgeShape:     BadgeShape(perKey.BackdropBadgeShape),
			EpisodeBadgeShape:      BadgeShape(perKey.EpisodeBadgeShape),
			PosterBadgeBackground:  BadgeBackground(perKey.PosterBadgeBackground),
			LogoBadgeBackground:    BadgeBackground(perKey.LogoBadgeBackground),
			BackdropBadgeBackground: BadgeBackground(perKey.BackdropBadgeBackground),
			EpisodeBadgeBackground: BadgeBackground(perKey.EpisodeBadgeBackground),
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


