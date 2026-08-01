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

func sectionColors(background services.BadgeBackground, labelStyle services.LabelStyle, source *services.RatingSource) (labelBG, valueBG color.RGBA, hasBG bool) {
	baseLabel := sourceColor(source)
	if labelStyle == services.LabelStyleOfficial {
		baseLabel = darkBG
	}

	switch background {
	case services.BadgeBackgroundDefault:
		return baseLabel, darkBG, true
	case services.BadgeBackgroundDark:
		return darkBG, darkBG, true
	case services.BadgeBackgroundTransparent:
		return withAlpha(baseLabel, transparentAlpha), withAlpha(darkBG, transparentAlpha), true
	case services.BadgeBackgroundNone:
		return color.RGBA{}, color.RGBA{}, false
	}
	return baseLabel, darkBG, true
}

func shadowOffset(bg services.BadgeBackground, badgeDim uint32) int {
	if bg == services.BadgeBackgroundNone {
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

func roundCorners(img *image.RGBA, r uint32) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	rr := int(r)
	if rr > w/2 {
		rr = w / 2
	}
	if rr > h/2 {
		rr = h / 2
	}
	if rr == 0 {
		return
	}

	r2 := rr * rr
	transparent := color.RGBA{0, 0, 0, 0}
	for dy := 0; dy < rr; dy++ {
		for dx := 0; dx < rr; dx++ {
			if (rr-dx)*(rr-dx)+(rr-dy)*(rr-dy) > r2 {
				img.Set(dx, dy, transparent)
				img.Set(w-1-dx, dy, transparent)
				img.Set(dx, h-1-dy, transparent)
				img.Set(w-1-dx, h-1-dy, transparent)
			}
		}
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			img.Set(x+dx, y+dy, c)
		}
	}
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

func renderBadgeInner(badge *services.RatingBadge, fontFace, labelFontFace font.Face, maxLabelW, maxValueW int, labelStyle services.LabelStyle, appearance services.BadgeAppearance, dims scaledDims) *image.RGBA {
	label := badge.Source.Label
	value := badge.Value
	useIcon := labelStyle.UsesIcon()

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

	labelBG, valueBG, hasBG := sectionColors(appearance.Background, labelStyle, badge.Source)
	if hasBG {
		fillRect(img, 0, 0, valueX, badgeH, labelBG)
		fillRect(img, valueX, 0, totalW-valueX, badgeH, valueBG)
		roundCorners(img, cornerRadius(appearance.Shape, uint32(badgeH), dims.badgeRadius))
	}

	shadowPx := shadowOffset(appearance.Background, uint32(badgeH))

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
			drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, labelX, labelY, labelFontFace, label, shadowPx)
		}
	} else {
		actualLabelW := textWidth(label, labelFontFace)
		labelX := int(pillPad) + int(labelPad) + (maxLabelW-actualLabelW)/2 + 2
		labelY := badgeH/2 + labelAscent/2 - 2
		drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, labelX, labelY, labelFontFace, label, shadowPx)
	}

	actualValueW := textWidth(value, fontFace)
	valueTextX := valueX + int(dims.badgeValuePad) + (maxValueW-actualValueW)/2
	valueY := badgeH/2 + ascentVal/2 - 2
	drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, valueTextX, valueY, fontFace, value, shadowPx)

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

func RenderBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32) *image.RGBA {
	dims := newScaledDims(badgeScale)
	maxLabelW := labelWidthForStyle(badge, labelStyle, labelFontFace, dims)
	maxValueW := textWidth(badge.Value, fontFace)
	return renderBadgeInner(badge, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims)
}

func RenderBadgesUniform(badges []services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32) []*image.RGBA {
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
		result[i] = renderBadgeInner(&b, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims)
	}
	return result
}

func RenderVerticalBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32) *image.RGBA {
	useIcon := labelStyle.UsesIcon()
	vertBadgeW := int(math.Round(float64(baseVertBadgeWidth) * float64(badgeScale)))
	var pillPad uint32
	if appearance.Shape == services.BadgeShapePill {
		pillPad = uint32(math.Round(float64(basePillPadding) * float64(badgeScale)))
	}
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

	labelBG, valueBG, hasBG := sectionColors(appearance.Background, labelStyle, badge.Source)
	if hasBG {
		fillRect(img, 0, 0, vertBadgeW, totalH, labelBG)
		fillRect(img, 0, valueAreaY, vertBadgeW, totalH-valueAreaY, valueBG)
		roundCorners(img, cornerRadius(appearance.Shape, uint32(vertBadgeW), uint32(math.Round(float64(baseBadgeRadius)*float64(badgeScale)))))
	}

	shadowPx := shadowOffset(appearance.Background, uint32(vertBadgeW))
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
			drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, labelX, labelY, labelFontFace, label, shadowPx)
		}
	} else {
		labelW := textWidth(label, labelFontFace)
		labelX := (vertBadgeW - labelW) / 2
		labelY := int(vertPadV) + int(labelH)/2 + labelAscent/2
		drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, labelX, labelY, labelFontFace, label, shadowPx)
	}

	valueW := textWidth(value, fontFace)
	valueX := (vertBadgeW - valueW) / 2
	valueTextY := valueAreaY + int(valueH)/2 + ascentVal/2
	drawTextShadowed(img, color.RGBA{255, 255, 255, 255}, valueX, valueTextY, fontFace, value, shadowPx)

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

	cw, ch := uint32(canvas.Bounds().Dx()), uint32(canvas.Bounds().Dy())

	var startY uint32
	if position.IsTop() {
		startY = tm + extraY
	} else if position.IsBottom() {
		startY = ch - totalH - bm - extraY
	} else {
		startY = (ch - totalH) / 2
	}

	var baseX uint32
	if position.IsLeft() {
		baseX = sm + extraX
	} else if position.IsRight() {
		baseX = cw - maxW - sm - extraX
	} else {
		baseX = (cw - maxW) / 2
	}

	y := startY
	for _, bi := range badgeImages {
		bw, bh := uint32(bi.Bounds().Dx()), uint32(bi.Bounds().Dy())
		var bx uint32
		if position.IsLeft() {
			bx = baseX
		} else if position.IsRight() {
			bx = baseX + maxW - bw
		} else {
			bx = baseX + (maxW-bw)/2
		}
		overlay(canvas, bi, int(bx), int(y))
		y += bh + vs
	}
}

func overlayHorizontalRows(canvas *image.RGBA, badgeImages []*image.RGBA, position services.BadgePosition, maxPerRow int, badgeScale float32, sideMarginBase uint32, extraX, extraY uint32) {
	sp := uint32(math.Round(float64(badgeSpacing) * float64(badgeScale)))
	rs := uint32(math.Round(float64(badgeRowSpacing) * float64(badgeScale)))
	tm := uint32(math.Round(float64(badgeTopMargin) * float64(badgeScale)))
	bm := uint32(math.Round(float64(badgeBottomMargin) * float64(badgeScale)))
	sm := uint32(math.Round(float64(sideMarginBase) * float64(badgeScale)))

	cw, ch := uint32(canvas.Bounds().Dx()), uint32(canvas.Bounds().Dy())
	var rows [][]*image.RGBA
	for i := 0; i < len(badgeImages); i += maxPerRow {
		end := i + maxPerRow
		if end > len(badgeImages) {
			end = len(badgeImages)
		}
		rows = append(rows, badgeImages[i:end])
	}

	var badgeH uint32
	for _, bi := range badgeImages {
		if h := uint32(bi.Bounds().Dy()); h > badgeH {
			badgeH = h
		}
	}
	totalH := badgeH*uint32(len(rows)) + rs*(uint32(len(rows))-1)

	var baseY uint32
	if position.IsTop() {
		baseY = tm + extraY
	} else if position.IsBottom() {
		baseY = ch - totalH - bm - extraY
	} else {
		baseY = (ch - totalH) / 2
	}

	for rowIdx, row := range rows {
		var rowW uint32
		for _, bi := range row {
			rowW += uint32(bi.Bounds().Dx())
		}
		rowW += sp * (uint32(len(row)) - 1)
		y := baseY + uint32(rowIdx)*(badgeH+rs)

		var startX uint32
		if position.IsLeft() {
			startX = sm + extraX
		} else if position.IsRight() {
			startX = cw - rowW - sm - extraX
		} else {
			startX = (cw - rowW) / 2
		}

		x := startX
		for _, bi := range row {
			bh := uint32(bi.Bounds().Dy())
			overlay(canvas, bi, int(x), int(y+(badgeH-bh)/2))
			x += uint32(bi.Bounds().Dx()) + sp
		}
	}
}
