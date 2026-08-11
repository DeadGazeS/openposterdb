package services

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
