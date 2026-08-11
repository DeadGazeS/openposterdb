package services

import "fmt"

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
