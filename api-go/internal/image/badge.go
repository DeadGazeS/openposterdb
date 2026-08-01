package image

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"openposterdb/internal/services"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"
)

var darkBG = color.RGBA{0, 0, 0, 200}
var transparentAlpha uint8 = 120
var shadowColor = color.RGBA{0, 0, 0, 160}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, a}
}

func sourceColor(source *services.RatingSource) color.RGBA {
	return color.RGBA{source.ColorR, source.ColorG, source.ColorB, 255}
}

// sectionColors computes the badge background. The label section uses the
// source accent (default black) and the value section uses the value color
// (default black); both share the badge alpha (0–100). Alpha 0 means no box.
// Colors are returned as non-premultiplied NRGBA so image/draw's Over blending
// composites them correctly (color.RGBA is non-premultiplied and overflows).
func sectionColors(alpha services.BadgeAlpha, source *services.RatingSource, override *services.SourceColorSet) (labelBG, valueBG color.NRGBA, hasBG bool) {
	pct := int32(alpha)
	if pct <= 0 {
		return color.NRGBA{}, color.NRGBA{}, false
	}
	if pct > 100 {
		pct = 100
	}
	a := uint8(pct * 255 / 100)

	labelCol := color.NRGBA{0, 0, 0, a}
	if override != nil && override.Accent != "" {
		if c, ok := services.ParseHexColor(override.Accent); ok {
			labelCol = color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
		}
	}
	valueCol := color.NRGBA{0, 0, 0, a}
	if override != nil && override.Value != "" {
		if c, ok := services.ParseHexColor(override.Value); ok {
			valueCol = color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
		}
	}
	return labelCol, valueCol, true
}

func shadowOffset(hasBG bool, badgeDim uint32) int {
	if !hasBG {
		offset := int(float64(badgeDim) / 29.0)
		if offset < 1 {
			offset = 1
		}
		return offset
	}
	return 0
}

func cornerRadius(shape services.BadgeShape, shortAxis, base uint32) uint32 {
	if shape == services.BadgeShapePill {
		return shortAxis / 2
	}
	return base
}

const (
	baseBadgeHeight    = 58
	baseBadgePaddingH  = 6
	baseTextLabelPadH  = 4
	baseBadgeValuePad  = 5
	baseBadgeRadius    = 10
	baseBadgeBorder    = 3
	basePillPadding    = 10
	basePillPaddingV   = 6
	baseFontSize       = 34.0
	baseLabelFontSize  = 26.0
	baseIconHeight     = 48
	baseVertBadgeWidth    = 88
	baseVertBadgePaddingV = 8
	baseVertLabelFontSize = 26.0
	baseVertValueFontSize = 34.0

	badgeSpacing        = 10
	badgeBottomMargin   = 10
	badgeTopMargin      = 20
	badgeSideMargin     = 15
	badgeRowSpacing     = 7
	badgeVertSpacing    = 7
	backdropSideMargin  = 20
	maxBadgesPerRow     = 3
	maxVertBadgesPerRow = 5
)

type scaledDims struct {
	badgeHeight   uint32
	badgePaddingH uint32
	textLabelPadH uint32
	badgeValuePad uint32
	badgeRadius   uint32
	badgeBorder   uint32
	pillPadding   uint32
	pillPaddingV  uint32
	iconHeight    uint32
}

func newScaledDims(badgeScale float32) scaledDims {
	return scaledDims{
		badgeHeight:   uint32(math.Round(float64(baseBadgeHeight) * float64(badgeScale))),
		badgePaddingH: uint32(math.Round(float64(baseBadgePaddingH) * float64(badgeScale))),
		textLabelPadH: uint32(math.Round(float64(baseTextLabelPadH) * float64(badgeScale))),
		badgeValuePad: uint32(math.Round(float64(baseBadgeValuePad) * float64(badgeScale))),
		badgeRadius:   uint32(math.Round(float64(baseBadgeRadius) * float64(badgeScale))),
		badgeBorder:   uint32(math.Round(float64(baseBadgeBorder) * float64(badgeScale))),
		pillPadding:   uint32(math.Round(float64(basePillPadding) * float64(badgeScale))),
		pillPaddingV:  uint32(math.Round(float64(basePillPaddingV) * float64(badgeScale))),
		iconHeight:    uint32(math.Round(float64(baseIconHeight) * float64(badgeScale))),
	}
}

// safeGlyphAdvance returns the advance width for a rune, or 0 if the font
// cannot produce it. Inter-Bold.ttf contains a glyph whose data is malformed,
// and golang.org/x/image/sfnt panics ("index out of range") when that glyph is
// rasterized instead of returning a missing-glyph sentinel. Recovering here
// lets a single bad rune be skipped instead of crashing the whole request.
func safeGlyphAdvance(face font.Face, r rune) fixed.Int26_6 {
	defer func() {
		recover()
	}()
	adv, _ := face.GlyphAdvance(r)
	return adv
}

func textWidth(text string, face font.Face) int {
	var w fixed.Int26_6
	for _, r := range text {
		w += safeGlyphAdvance(face, r)
	}
	return w.Ceil()
}

func drawText(img *image.RGBA, col color.RGBA, x, y int, f font.Face, text string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: f,
	}
	dot := fixed.P(x, y)
	for _, r := range text {
		d.Dot = dot
		func() {
			defer func() {
				recover()
			}()
			d.DrawString(string(r))
		}()
		dot.X += safeGlyphAdvance(f, r)
	}
}

func drawTextShadowed(img *image.RGBA, col color.RGBA, x, y int, f font.Face, text string, shadow int) {
	if shadow > 0 {
		drawText(img, shadowColor, x+shadow, y+shadow, f, text)
	}
	drawText(img, col, x, y, f, text)
}

// corner bits select which corners of a rectangle are rounded.
const (
	cornerTL uint8 = 1 << iota
	cornerTR
	cornerBR
	cornerBL
)

const allCorners = cornerTL | cornerTR | cornerBR | cornerBL

// roundedRectPath appends a rectangle to a vector rasterizer path, rounding
// the corners selected by the `corners` bitmask with radius `r`.
func roundedRectPath(z *vector.Rasterizer, x0, y0, x1, y1 float32, r float32, corners uint8) {
	if r < 0 {
		r = 0
	}
	if maxR := float32(math.Min(float64(x1-x0), float64(y1-y0))) / 2; r > maxR {
		r = maxR
	}
	round := func(bit uint8, cx, cy, tx, ty float32) {
		if corners&bit != 0 && r > 0 {
			z.QuadTo(cx, cy, tx, ty)
		} else {
			// Square corner: go straight to the corner point (cx, cy). Using
			// the end point (tx, ty) instead would cut a bevel off the corner.
			z.LineTo(cx, cy)
		}
	}

	z.MoveTo(x0+r, y0)
	z.LineTo(x1-r, y0)
	round(cornerTR, x1, y0, x1, y0+r)
	z.LineTo(x1, y1-r)
	round(cornerBR, x1, y1, x1-r, y1)
	z.LineTo(x0+r, y1)
	round(cornerBL, x0, y1, x0, y1-r)
	z.LineTo(x0, y0+r)
	round(cornerTL, x0, y0, x0+r, y0)
	z.ClosePath()
}

// roundedRectMask rasterizes a rounded rectangle into an alpha mask covering
// the full (w, h) canvas, with the rectangle placed at (x0,y0,x1,y1).
func roundedRectMask(w, h int, x0, y0, x1, y1 int, r float32, corners uint8) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	z := vector.NewRasterizer(w, h)
	roundedRectPath(z, float32(x0), float32(y0), float32(x1), float32(y1), r, corners)
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

// fillRoundedRect fills the rectangle (x0,y0,x1,y1) on img with `col` using an
// anti-aliased, rounded-corner path (corners per the bitmask).
func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1 int, r float32, corners uint8, col color.Color) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	mask := roundedRectMask(w, h, x0, y0, x1, y1, r, corners)
	draw.DrawMask(img, image.Rect(x0, y0, x1, y1), image.NewUniform(col), image.Point{}, mask, image.Point{x0, y0}, draw.Over)
}

// ringMask returns an alpha mask that is opaque on the stroke of a rounded
// rectangle of `radius` whose edges are inset by `inset` px — i.e. outside the
// inner rounded rect but inside the outer one. The anti-aliased edge blend is
// preserved so the border composites smoothly over the section colors.
func ringMask(w, h, x0, y0, x1, y1 int, radius float32, inset int, corners uint8) *image.Alpha {
	outer := roundedRectMask(w, h, x0, y0, x1, y1, radius, corners)
	inner := roundedRectMask(w, h, x0+inset, y0+inset, x1-inset, y1-inset, radius-float32(inset), corners)
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			oa := uint16(outer.AlphaAt(x, y).A)
			ia := uint16(inner.AlphaAt(x, y).A)
			mask.SetAlpha(x, y, color.Alpha{A: uint8(oa * (255 - ia) / 255)})
		}
	}
	return mask
}

func overlay(img, ov *image.RGBA, ox, oy int) {
	sr := ov.Bounds()
	dr := image.Rect(ox, oy, ox+sr.Dx(), oy+sr.Dy())
	draw.Draw(img, dr, ov, sr.Min, draw.Over)
}

func iconShadow(icon *image.RGBA) *image.RGBA {
	b := icon.Bounds()
	shadow := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := icon.RGBAAt(x, y)
			a := uint8(uint16(c.A) * 3 / 5)
			shadow.Set(x, y, color.RGBA{0, 0, 0, a})
		}
	}
	return shadow
}

// iconScaledWidth scales an icon to a target height, preserving aspect ratio.
func iconScaledWidth(icon *image.RGBA, targetHeight uint32) uint32 {
	if icon == nil || icon.Bounds().Dy() == 0 {
		return targetHeight
	}
	return uint32(math.Ceil(float64(icon.Bounds().Dx()) * float64(targetHeight) / float64(icon.Bounds().Dy())))
}

// iconFitInBox fits an icon within a boxSize x boxSize square, preserving aspect ratio.
func iconFitInBox(icon *image.RGBA, boxSize uint32) (uint32, uint32) {
	if icon == nil || icon.Bounds().Dx() == 0 || icon.Bounds().Dy() == 0 {
		return boxSize, boxSize
	}
	w := float64(icon.Bounds().Dx())
	h := float64(icon.Bounds().Dy())
	scale := math.Min(float64(boxSize)/w, float64(boxSize)/h)
	return uint32(math.Round(w * scale)), uint32(math.Round(h * scale))
}

// badgeIconAndSize selects the icon for a badge and computes its target size.
func badgeIconAndSize(badge *services.RatingBadge, labelStyle services.LabelStyle, iconHeight uint32, icon *image.RGBA) (uint32, uint32) {
	if labelStyle == services.LabelStyleOfficial {
		return iconFitInBox(icon, iconHeight)
	}
	return iconScaledWidth(icon, iconHeight), iconHeight
}

func scaleIcon(icon *image.RGBA, w, h uint32) *image.RGBA {
	if icon == nil {
		return nil
	}
	if uint32(icon.Bounds().Dx()) == w && uint32(icon.Bounds().Dy()) == h {
		return icon
	}
	if w == 0 || h == 0 {
		return icon
	}
	scaled := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), icon, icon.Bounds(), xdraw.Src, nil)
	return scaled
}

func overlayIconShadowed(img, icon *image.RGBA, x, y int, shadow int) {
	if icon == nil {
		return
	}
	if shadow > 0 {
		overlay(img, iconShadow(icon), x+shadow, y+shadow)
	}
	overlay(img, icon, x, y)
}

func iconForBadge(badge *services.RatingBadge, labelStyle services.LabelStyle) *image.RGBA {
	if labelStyle == services.LabelStyleOfficial {
		return OfficialIconForBadge(badge)
	}
	return IconForSource(badge.Source)
}

// colorOverride returns the effective color override for a badge, or nil when
// none is set. Rotten Tomatoes badges look up their specific logo variant key
// (e.g. "rt_cf") and fall back to the parent source key (e.g. "rt") for
// settings saved before the variants were split out.
func colorOverride(colors map[string]services.SourceColorSet, badge *services.RatingBadge) *services.SourceColorSet {
	if colors == nil || badge == nil || badge.Source == nil {
		return nil
	}
	variantKey := services.RatingColorKey(badge)
	for _, key := range []string{variantKey, badge.Source.Key} {
		if c, ok := colors[key]; ok && c.HasAny() {
			return &c
		}
	}
	return nil
}

func renderBadgeInner(badge *services.RatingBadge, fontFace, labelFontFace font.Face, maxLabelW, maxValueW int, labelStyle services.LabelStyle, appearance services.BadgeAppearance, dims scaledDims, colors map[string]services.SourceColorSet) *image.RGBA {
	label := badge.Source.Label
	value := badge.Value
	useIcon := labelStyle.UsesIcon()
	override := colorOverride(colors, badge)

	textCol := color.RGBA{255, 255, 255, 255}
	var borderCol *color.RGBA
	if override != nil {
		if c, ok := services.ParseHexColor(override.Text); ok && override.Text != "" {
			textCol = c
		}
		// Border is opt-in per source: only drawn when a color is set.
		if c, ok := services.ParseHexColor(override.Border); ok && override.Border != "" {
			borderCol = &c
		}
	}

	labelPad := dims.badgePaddingH
	if !useIcon {
		labelPad = dims.textLabelPadH
	}

	var pillPad, pillPadV uint32
	if appearance.Shape == services.BadgeShapePill {
		pillPad = dims.pillPadding
		pillPadV = dims.pillPaddingV
	}

	badgeH := int(dims.badgeHeight + pillPadV)
	labelAreaW := int(pillPad) + int(maxLabelW) + int(labelPad)
	valueX := labelAreaW + int(labelPad)
	totalW := valueX + int(maxValueW) + int(dims.badgeValuePad) + int(dims.badgeValuePad)/2 + 2 + int(pillPad)

	img := image.NewRGBA(image.Rect(0, 0, totalW, badgeH))

	labelBG, valueBG, hasBG := sectionColors(appearance.Alpha, badge.Source, override)
	if hasBG {
		radius := float32(cornerRadius(appearance.Shape, uint32(badgeH), dims.badgeRadius))
		// Draw the two sections as non-overlapping tiles so neither gets
		// composited twice (that made the value section look more opaque than
		// the label section). The border ring is then stroked on top.
		fillRoundedRect(img, 0, 0, valueX, badgeH, radius, cornerTL|cornerBL, labelBG)
		fillRoundedRect(img, valueX, 0, totalW, badgeH, radius, cornerTR|cornerBR, valueBG)
		if dims.badgeBorder > 0 && borderCol != nil {
			inset := int(dims.badgeBorder)
			innerR := radius - float32(inset)
			if innerR < 0 {
				innerR = 0
			}
			ring := ringMask(totalW, badgeH, 0, 0, totalW, badgeH, radius, inset, allCorners)
			draw.DrawMask(img, img.Bounds(), image.NewUniform(*borderCol), image.Point{}, ring, image.Point{}, draw.Over)
		}
	}

	shadowPx := shadowOffset(hasBG, uint32(badgeH))

	ascentVal := fontFace.Metrics().Ascent.Ceil()
	labelAscent := labelFontFace.Metrics().Ascent.Ceil()

	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			iconW, iconH := badgeIconAndSize(badge, labelStyle, dims.iconHeight, icon)
			scaledIcon := scaleIcon(icon, iconW, iconH)
			ix := int(pillPad) + int(labelPad) + (maxLabelW-int(iconW))/2
			iy := (badgeH - int(iconH)) / 2
			overlayIconShadowed(img, scaledIcon, ix, iy, shadowPx)
		} else {
			actualLabelW := textWidth(label, labelFontFace)
			labelX := int(pillPad) + int(labelPad) + (maxLabelW-actualLabelW)/2 + 2
			labelY := badgeH/2 + labelAscent/2 - 2
			drawTextShadowed(img, textCol, labelX, labelY, labelFontFace, label, shadowPx)
		}
	} else {
		actualLabelW := textWidth(label, labelFontFace)
		labelX := int(pillPad) + int(labelPad) + (maxLabelW-actualLabelW)/2 + 2
		labelY := badgeH/2 + labelAscent/2 - 2
		drawTextShadowed(img, textCol, labelX, labelY, labelFontFace, label, shadowPx)
	}

	actualValueW := textWidth(value, fontFace)
	valueTextX := valueX + int(dims.badgeValuePad) + (maxValueW-actualValueW)/2
	valueY := badgeH/2 + ascentVal/2 - 2
	drawTextShadowed(img, textCol, valueTextX, valueY, fontFace, value, shadowPx)

	return img
}

func labelWidthForStyle(badge *services.RatingBadge, labelStyle services.LabelStyle, labelFontFace font.Face, dims scaledDims) int {
	switch labelStyle {
	case services.LabelStyleOfficial:
		return int(dims.iconHeight)
	case services.LabelStyleIcon:
		if icon := IconForSource(badge.Source); icon != nil {
			return int(iconScaledWidth(icon, dims.iconHeight))
		}
		return textWidth(badge.Source.Label, labelFontFace)
	default:
		return textWidth(badge.Source.Label, labelFontFace)
	}
}

func RenderBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, colors map[string]services.SourceColorSet) *image.RGBA {
	dims := newScaledDims(badgeScale)
	maxLabelW := labelWidthForStyle(badge, labelStyle, labelFontFace, dims)
	maxValueW := textWidth(badge.Value, fontFace)
	return renderBadgeInner(badge, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims, colors)
}

func RenderBadgesUniform(badges []services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, colors map[string]services.SourceColorSet) []*image.RGBA {
	if len(badges) == 0 {
		return nil
	}

	dims := newScaledDims(badgeScale)
	var maxLabelW, maxValueW int
	for _, b := range badges {
		if w := labelWidthForStyle(&b, labelStyle, labelFontFace, dims); w > maxLabelW {
			maxLabelW = w
		}
		if w := textWidth(b.Value, fontFace); w > maxValueW {
			maxValueW = w
		}
	}

	result := make([]*image.RGBA, len(badges))
	for i, b := range badges {
		result[i] = renderBadgeInner(&b, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims, colors)
	}
	return result
}

func RenderVerticalBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, colors map[string]services.SourceColorSet) *image.RGBA {
	useIcon := labelStyle.UsesIcon()
	override := colorOverride(colors, badge)
	textCol := color.RGBA{255, 255, 255, 255}
	var borderCol *color.RGBA
	if override != nil {
		if c, ok := services.ParseHexColor(override.Text); ok && override.Text != "" {
			textCol = c
		}
		// Border is opt-in per source: only drawn when a color is set.
		if c, ok := services.ParseHexColor(override.Border); ok && override.Border != "" {
			borderCol = &c
		}
	}
	vertBadgeW := int(math.Round(float64(baseVertBadgeWidth) * float64(badgeScale)))
	var pillPad uint32
	if appearance.Shape == services.BadgeShapePill {
		pillPad = uint32(math.Round(float64(basePillPadding) * float64(badgeScale)))
	}
	dims := newScaledDims(badgeScale)
	vertPadV := uint32(math.Round(float64(baseVertBadgePaddingV)*float64(badgeScale))) + pillPad
	labelH := uint32(math.Round(float64(baseVertLabelFontSize) * float64(badgeScale)))
	valueH := uint32(math.Round(float64(baseVertValueFontSize) * float64(badgeScale)))
	gap := uint32(math.Round(4.0 * float64(badgeScale)))
	iconHeight := uint32(math.Round(float64(baseIconHeight) * float64(badgeScale)))
	labelAreaH := labelH
	if useIcon {
		labelAreaH = iconHeight
	}
	totalH := int(vertPadV + labelAreaH + gap + valueH + vertPadV)

	img := image.NewRGBA(image.Rect(0, 0, vertBadgeW, totalH))
	valueAreaY := int(vertPadV + labelAreaH + gap/2)

	labelBG, valueBG, hasBG := sectionColors(appearance.Alpha, badge.Source, override)
	if hasBG {
		radius := float32(cornerRadius(appearance.Shape, uint32(vertBadgeW), uint32(math.Round(float64(baseBadgeRadius)*float64(badgeScale)))))
		// Draw the two sections as non-overlapping tiles so neither gets
		// composited twice (that made the value section look more opaque than
		// the label section). The border ring is then stroked on top.
		fillRoundedRect(img, 0, 0, vertBadgeW, valueAreaY, radius, cornerTL|cornerTR, labelBG)
		fillRoundedRect(img, 0, valueAreaY, vertBadgeW, totalH, radius, cornerBL|cornerBR, valueBG)
		if dims.badgeBorder > 0 && borderCol != nil {
			inset := int(dims.badgeBorder)
			innerR := radius - float32(inset)
			if innerR < 0 {
				innerR = 0
			}
			ring := ringMask(vertBadgeW, totalH, 0, 0, vertBadgeW, totalH, radius, inset, allCorners)
			draw.DrawMask(img, img.Bounds(), image.NewUniform(*borderCol), image.Point{}, ring, image.Point{}, draw.Over)
		}
	}

	shadowPx := shadowOffset(hasBG, uint32(vertBadgeW))
	ascentVal := fontFace.Metrics().Ascent.Ceil()
	labelAscent := labelFontFace.Metrics().Ascent.Ceil()

	label := badge.Source.Label
	value := badge.Value

	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			iconW, iconH := badgeIconAndSize(badge, labelStyle, iconHeight, icon)
			scaledIcon := scaleIcon(icon, iconW, iconH)
			ix := (vertBadgeW - int(iconW)) / 2
			iy := (valueAreaY - int(iconH)) / 2
			overlayIconShadowed(img, scaledIcon, ix, iy, shadowPx)
		} else {
			labelW := textWidth(label, labelFontFace)
			labelX := (vertBadgeW - labelW) / 2
			labelY := int(vertPadV) + int(labelH)/2 + labelAscent/2
			drawTextShadowed(img, textCol, labelX, labelY, labelFontFace, label, shadowPx)
		}
	} else {
		labelW := textWidth(label, labelFontFace)
		labelX := (vertBadgeW - labelW) / 2
		labelY := int(vertPadV) + int(labelH)/2 + labelAscent/2
		drawTextShadowed(img, textCol, labelX, labelY, labelFontFace, label, shadowPx)
	}

	valueW := textWidth(value, fontFace)
	valueX := (vertBadgeW - valueW) / 2
	valueTextY := valueAreaY + int(valueH)/2 + ascentVal/2
	drawTextShadowed(img, textCol, valueX, valueTextY, fontFace, value, shadowPx)

	return img
}

func overlayVerticalStack(canvas *image.RGBA, badgeImages []*image.RGBA, position services.BadgePosition, badgeScale float32, sideMarginBase uint32, extraX, extraY uint32) {
	vs := uint32(math.Round(float64(badgeVertSpacing) * float64(badgeScale)))
	tm := uint32(math.Round(float64(badgeTopMargin) * float64(badgeScale)))
	bm := uint32(math.Round(float64(badgeBottomMargin) * float64(badgeScale)))
	sm := uint32(math.Round(float64(sideMarginBase) * float64(badgeScale)))

	var totalH uint32
	for _, bi := range badgeImages {
		totalH += uint32(bi.Bounds().Dy())
	}
	totalH += vs * (uint32(len(badgeImages)) - 1)

	var maxW uint32
	for _, bi := range badgeImages {
		if w := uint32(bi.Bounds().Dx()); w > maxW {
			maxW = w
		}
	}

	cw, ch := int(canvas.Bounds().Dx()), int(canvas.Bounds().Dy())
	ciw := int(cw)

	var startY int
	if position.IsTop() {
		startY = int(tm + extraY)
	} else if position.IsBottom() {
		startY = ch - int(totalH) - int(bm) - int(extraY)
	} else {
		startY = (ch - int(totalH)) / 2
	}
	if startY < 0 {
		startY = 0
	}

	var baseX int
	if position.IsLeft() {
		baseX = int(sm + extraX)
	} else if position.IsRight() {
		baseX = ciw - int(maxW) - int(sm) - int(extraX)
	} else {
		baseX = (ciw - int(maxW)) / 2
	}
	if baseX < 0 {
		baseX = 0
	}

	y := startY
	for _, bi := range badgeImages {
		bw, bh := bi.Bounds().Dx(), bi.Bounds().Dy()
		var bx int
		if position.IsLeft() {
			bx = baseX
		} else if position.IsRight() {
			bx = baseX + int(maxW) - bw
		} else {
			bx = baseX + (int(maxW)-bw)/2
		}
		overlay(canvas, bi, bx, y)
		y += bh + int(vs)
	}
}

func overlayHorizontalRows(canvas *image.RGBA, badgeImages []*image.RGBA, position services.BadgePosition, maxPerRow int, badgeScale float32, sideMarginBase uint32, extraX, extraY uint32) {
	sp := uint32(math.Round(float64(badgeSpacing) * float64(badgeScale)))
	rs := uint32(math.Round(float64(badgeRowSpacing) * float64(badgeScale)))
	tm := uint32(math.Round(float64(badgeTopMargin) * float64(badgeScale)))
	bm := uint32(math.Round(float64(badgeBottomMargin) * float64(badgeScale)))
	sm := uint32(math.Round(float64(sideMarginBase) * float64(badgeScale)))

	cw, ch := int(canvas.Bounds().Dx()), int(canvas.Bounds().Dy())
	var rows [][]*image.RGBA
	for i := 0; i < len(badgeImages); i += maxPerRow {
		end := i + maxPerRow
		if end > len(badgeImages) {
			end = len(badgeImages)
		}
		rows = append(rows, badgeImages[i:end])
	}

	var badgeH int
	for _, bi := range badgeImages {
		if h := bi.Bounds().Dy(); h > badgeH {
			badgeH = h
		}
	}
	totalH := badgeH*len(rows) + int(rs)*(len(rows)-1)

	var baseY int
	if position.IsTop() {
		baseY = int(tm) + int(extraY)
	} else if position.IsBottom() {
		baseY = ch - totalH - int(bm) - int(extraY)
	} else {
		baseY = (ch - totalH) / 2
	}
	if baseY < 0 {
		baseY = 0
	}

	for rowIdx, row := range rows {
		rowW := 0
		for _, bi := range row {
			rowW += bi.Bounds().Dx()
		}
		rowW += int(sp) * (len(row) - 1)
		y := baseY + rowIdx*(badgeH+int(rs))

		var startX int
		if position.IsLeft() {
			startX = int(sm) + int(extraX)
		} else if position.IsRight() {
			startX = cw - rowW - int(sm) - int(extraX)
		} else {
			startX = (cw - rowW) / 2
		}
		if startX < 0 {
			startX = 0
		}

		x := startX
		for _, bi := range row {
			bh := bi.Bounds().Dy()
			overlay(canvas, bi, x, y+(badgeH-bh)/2)
			x += bi.Bounds().Dx() + int(sp)
		}
	}
}
