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
	baseBadgeHeight   = 58
	baseBadgePaddingH = 6
	baseTextLabelPadH = 4
	baseBadgeValuePad = 5
	baseBadgeRadius   = 10
	baseBadgeBorder   = 3
	// badgeContentGapBase is the minimum distance between content ink (value
	// text, label text, source logo) and the badge's outer border. Oversized
	// content is clipped at (border − gap) instead of AT the border, so scaled-up
	// text/logos are cut off before touching the badge edge. It is derived from
	// the border width so content never sits on the border itself.
	badgeContentGapBase = baseBadgeBorder
	// badgeInwardGap is the tiny gap between the source logo's INWARD side
	// (facing the value text) and the section seam at 100%, so the text box
	// owns the remaining space and centres the text.
	badgeInwardGap        = 2
	basePillPadding       = 10
	basePillPaddingV      = 6
	baseFontSize          = 34.0
	baseLabelFontSize     = 26.0
	baseIconHeight        = 48
	baseVertBadgeWidth    = 88
	baseVertBadgePaddingV = 8

	badgeSpacing       = 10
	badgeBottomMargin  = 10
	badgeTopMargin     = 20
	badgeSideMargin    = 15
	badgeRowSpacing    = 7
	badgeVertSpacing   = 7
	backdropSideMargin = 20
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
	contentGap    uint32
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
		contentGap:    uint32(math.Round(float64(badgeContentGapBase) * float64(badgeScale))),
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

// drawTextSub is drawText with an arbitrary destination image, so the drawing
// can be clipped to a sub-image of the badge (font.Drawer clips glyph ink to
// dst.Bounds()).
func drawTextSub(dst draw.Image, col color.RGBA, x, y int, f font.Face, text string) {
	d := &font.Drawer{
		Dst:  dst,
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

// drawTextShadowedInRect draws text (and its shadow) clipped to clip, so
// oversized text is cut off exactly at the rect's edges instead of spilling
// into the neighbouring section.
func drawTextShadowedInRect(img *image.RGBA, col color.RGBA, x, y int, f font.Face, text string, shadow int, clip image.Rectangle) {
	if clip.Dx() <= 0 || clip.Dy() <= 0 {
		return
	}
	dst, ok := img.SubImage(clip).(*image.RGBA)
	if !ok {
		return
	}
	if shadow > 0 {
		drawTextSub(dst, shadowColor, x+shadow, y+shadow, f, text)
	}
	drawTextSub(dst, col, x, y, f, text)
}

// textInkBBox measures the exact bounding box of the ink pixels that drawText
// produces for text rendered with face, relative to the pen origin (0,0). It
// renders the text once into an offscreen image and scans it, so the box
// includes glyph side bearings and optical overhangs. Centring on this box
// centres the visible ink, unlike centring on the advance width (which leaves
// the ink shifted by the side-bearing asymmetry) or the face.Glyph mask rect
// (which includes empty padding on the right/bottom).
func textInkBBox(f font.Face, text string) (x0, y0, x1, y1 int, ok bool) {
	adv := textWidth(text, f)
	if adv <= 0 {
		return 0, 0, 0, 0, false
	}
	m := f.Metrics()
	height := m.Height.Ceil() + 4
	baseline := m.Ascent.Ceil() + 2
	img := image.NewRGBA(image.Rect(0, 0, adv+4, height))
	drawText(img, color.RGBA{255, 255, 255, 255}, 0, baseline, f, text)
	x0, y0 = adv+4, height
	x1, y1 = 0, 0
	for y := 0; y < height; y++ {
		for x := 0; x < adv+4; x++ {
			if img.RGBAAt(x, y).A > 0 {
				if x < x0 {
					x0 = x
				}
				if x > x1 {
					x1 = x
				}
				if y < y0 {
					y0 = y
				}
				if y > y1 {
					y1 = y
				}
			}
		}
	}
	if x0 > x1 || y0 > y1 {
		return 0, 0, 0, 0, false
	}
	// Ink coordinates relative to the pen (baseline) origin.
	return x0, y0 - baseline, x1, y1 - baseline, true
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
	if labelStyle == services.LabelStyleOfficial || labelStyle == services.LabelStyleHighRes {
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

// overlayIconShadowedInRect draws an icon (and its shadow) clipped to clip, so
// an oversized logo is cut off at the rect's edges — inset from the badge
// border by the content gap — instead of touching the badge border or spilling
// into the neighbouring section.
func overlayIconShadowedInRect(img, icon *image.RGBA, x, y int, shadow int, clip image.Rectangle) {
	if icon == nil || clip.Dx() <= 0 || clip.Dy() <= 0 {
		return
	}
	dst, ok := img.SubImage(clip).(*image.RGBA)
	if !ok {
		return
	}
	if shadow > 0 {
		overlay(dst, iconShadow(icon), x+shadow, y+shadow)
	}
	overlay(dst, icon, x, y)
}

func iconForBadge(badge *services.RatingBadge, labelStyle services.LabelStyle) *image.RGBA {
	if labelStyle == services.LabelStyleHighRes {
		return HighResIconForBadge(badge)
	}
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

func renderBadgeInner(badge *services.RatingBadge, fontFace, labelFontFace font.Face, maxLabelW, maxValueW int, labelStyle services.LabelStyle, appearance services.BadgeAppearance, dims scaledDims, textScale float32, logoScale float32, colors map[string]services.SourceColorSet) *image.RGBA {
	label := badge.Source.Label
	value := badge.Value
	useIcon := labelStyle.UsesIcon()
	override := colorOverride(colors, badge)

	// The badge style controls the orientation: mirrored styles (rl/bt) put the
	// value on the left / top and the logo on the right / bottom.
	mirrored := appearance.Style.IsMirrored()

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

	// The badge box is sized purely by badge_size (via dims) and the default
	// (100%) content layout — it does NOT grow with text_size/logo_size. Text or
	// logos larger than the box are truncated (clipped) at the badge edges.
	// The badge is split at the section boundary into a left (logo/label)
	// section and a right (value) section. The icon is pinned against the seam
	// (its inward side keeps barely any margin) with equal outer margins on
	// the remaining three sides; the value text is centred in the right
	// section.
	//
	// Per-axis badge width/height (badge_width/badge_height) scale the badge
	// box independently: the width applies to the section split and the total
	// width, the height to the box height. The logo and text sizes are NOT
	// affected (they scale only with logo_size/text_size).
	wPct := appearance.Width.Percent()
	hPct := appearance.Height.Percent()
	if wPct <= 0 {
		wPct = 1
	}
	if hPct <= 0 {
		hPct = 1
	}
	badgeH := int(math.Round(float64(dims.badgeHeight+pillPadV) * float64(hPct)))

	// The logo scales ONLY with logo_size (not badge_size): decoupled from the
	// badge box so changing badge size never resizes the source logo.
	iconH := uint32(math.Round(float64(baseIconHeight) * float64(logoScale)))
	var iconW, iconH2 uint32
	var scaledIcon *image.RGBA
	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			iconW, iconH2 = badgeIconAndSize(badge, labelStyle, iconH, icon)
			scaledIcon = scaleIcon(icon, iconW, iconH2)
		}
	}

	labelSectionW := int(pillPad) + int(maxLabelW) + 2*int(labelPad)
	// Icon badges: size the label section from the BASE logo (100% logo_size,
	// badge-scaled) so the badge box stays fixed — base logo width + the equal
	// outer margin (iconPad) + the tiny inward gap — so the logo hugs the seam
	// instead of being centred, and its three OUTER margins are equal at 100%.
	var iconPad int
	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			baseIconW, baseIconH2 := badgeIconAndSize(badge, labelStyle, dims.iconHeight, icon)
			// The outer margin is fixed by the 100%-height reference so the
			// badge width never tracks badge_height (per-axis); the drawn logo
			// is vertically centred by the ACTUAL badge height (see iy below),
			// so extra height becomes padding, not a wider section.
			refBadgeH := int(dims.badgeHeight)
			if appearance.Shape == services.BadgeShapePill {
				refBadgeH += int(dims.pillPaddingV)
			}
			iconPad = (refBadgeH - int(baseIconH2)) / 2
			if iconPad < 0 {
				iconPad = 0
			}
			labelSectionW = int(baseIconW) + iconPad + badgeInwardGap
		}
	}
	valueSectionW := int(maxValueW) + int(dims.badgeValuePad) + int(dims.badgeValuePad)/2 + 2
	// Per-axis width: scale the section split and the total badge width by the
	// badge-width percentage (the content inside keeps its own size).
	labelSectionW = int(math.Round(float64(labelSectionW) * float64(wPct)))
	valueSectionW = int(math.Round(float64(valueSectionW) * float64(wPct)))
	scaledPillPad := int(math.Round(float64(pillPad) * float64(wPct)))
	totalW := labelSectionW + valueSectionW + scaledPillPad

	// Sections split the badge: label/logo occupies labelSectionW, value text
	// occupies valueSectionW. For mirrored styles the value section is on the
	// left and the logo/label on the right. The pill cap padding (pillPad)
	// always lands on the VALUE section's outer end, so the logo's outer
	// margin stays equal for both lr and rl (true mirror images).
	valueSectionX := labelSectionW
	labelSectionX := 0
	if mirrored {
		labelSectionX = scaledPillPad + valueSectionW
		valueSectionX = scaledPillPad
	}

	img := image.NewRGBA(image.Rect(0, 0, totalW, badgeH))

	labelBG, valueBG, hasBG := sectionColors(appearance.Alpha, badge.Source, override)
	if hasBG {
		radius := float32(cornerRadius(appearance.Shape, uint32(badgeH), dims.badgeRadius))
		// Draw the two sections as non-overlapping tiles so neither gets
		// composited twice (that made the value section look more opaque than
		// the label section). The border ring is then stroked on top. In
		// mirrored styles (rl) the value section sits on the LEFT and the
		// label/logo on the RIGHT, so the outer left corners belong to the
		// value section and the outer right corners to the label section.
		labelCorners := cornerTL | cornerBL
		valueCorners := cornerTR | cornerBR
		if mirrored {
			labelCorners = cornerTR | cornerBR
			valueCorners = cornerTL | cornerBL
		}
		fillRoundedRect(img, labelSectionX, 0, labelSectionX+labelSectionW, badgeH, radius, labelCorners, labelBG)
		// The value section's outer tile extends to the badge edge so the pill
		// cap padding (pillPad) is filled instead of left transparent (the
		// seam between the two tiles stays non-overlapping).
		if mirrored {
			fillRoundedRect(img, 0, 0, valueSectionX+valueSectionW, badgeH, radius, valueCorners, valueBG)
		} else {
			fillRoundedRect(img, valueSectionX, 0, totalW, badgeH, radius, valueCorners, valueBG)
		}
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

	// The minimum gap between content ink and the badge border. At 100% the
	// section padding already keeps the text and logo well off the border; the
	// clip rects below only matter for oversized content, cutting it off
	// BEFORE it reaches the border (content that already sits ≥ gap from the
	// border is never clipped).
	contentGap := int(dims.contentGap)
	// The logo's INWARD side may reach the section seam when it fits (its tiny
	// inward gap keeps it off at 100%); oversized logos keep the content-gap
	// inset on both sides so they clip evenly and never touch the badge border.
	iconFitsSeam := useIcon && scaledIcon != nil && int(iconW) <= labelSectionW-badgeInwardGap
	labelClip := image.Rect(labelSectionX+contentGap, contentGap, labelSectionX+labelSectionW-contentGap, badgeH-contentGap)
	if iconFitsSeam {
		if mirrored {
			labelClip = image.Rect(labelSectionX, contentGap, labelSectionX+labelSectionW-contentGap, badgeH-contentGap)
		} else {
			labelClip = image.Rect(labelSectionX+contentGap, contentGap, labelSectionX+labelSectionW, badgeH-contentGap)
		}
	}
	valueClip := image.Rect(valueSectionX+contentGap, contentGap, valueSectionX+valueSectionW-contentGap, badgeH-contentGap)

	// Logo / label. At 100% the icon's INWARD side (facing the value text)
	// keeps only badgeInwardGap from the seam, and the three OUTER margins
	// (lr: left/top/bottom; rl: right/top/bottom) are equal. Logos wider than
	// the section minus the inward gap are centred so they clip at the content
	// gap on both sides.
	if useIcon && scaledIcon != nil {
		var ix int
		if int(iconW) <= labelSectionW-badgeInwardGap {
			if mirrored {
				ix = labelSectionX + badgeInwardGap
			} else {
				ix = labelSectionX + labelSectionW - badgeInwardGap - int(iconW)
			}
		} else {
			ix = labelSectionX + (labelSectionW-int(iconW))/2
		}
		iy := (badgeH - int(iconH2)) / 2
		if iy < 0 {
			iy = 0
		}
		overlayIconShadowedInRect(img, scaledIcon, ix, iy, shadowPx, labelClip)
	} else {
		actualLabelW := textWidth(label, labelFontFace)
		labelX := labelSectionX + (labelSectionW-actualLabelW)/2
		labelY := badgeH/2 + labelAscent/2 - 2
		// Clip the label to its section (inset from the badge border by the
		// content gap) so oversized label text is cut off before touching the
		// badge border and never spills into the value section.
		drawTextShadowedInRect(img, textCol, labelX, labelY, labelFontFace, label, shadowPx, labelClip)
	}

	// Value text, centred in its section. The text is centred by its ink
	// bounding box (not the advance width) so glyph side bearings don't shift
	// the visible centre horizontally, and the baseline is derived from the ink
	// extent (not the "-2" ascent hack) so the ink is vertically centred too.
	// Oversized text is clipped at the value section edges inset by the content
	// gap, so it cuts off evenly on both sides without touching the badge border.
	if inkX0, inkY0, inkX1, inkY1, ok := textInkBBox(fontFace, value); ok {
		valueTextX := valueSectionX + (valueSectionW-(inkX0+inkX1))/2
		valueY := badgeH/2 - (inkY0+inkY1)/2
		drawTextShadowedInRect(img, textCol, valueTextX, valueY, fontFace, value, shadowPx, valueClip)
	} else {
		actualValueW := textWidth(value, fontFace)
		valueTextX := valueSectionX + (valueSectionW-actualValueW)/2
		valueY := badgeH/2 + ascentVal/2 - 2
		drawTextShadowed(img, textCol, valueTextX, valueY, fontFace, value, shadowPx)
	}

	return img
}

// labelWidthForStyle returns the base (100%) label-section width, independent
// of text_size/logo_size, so the badge box stays fixed. Oversized content is
// clipped at the badge edges.
func labelWidthForStyle(badge *services.RatingBadge, labelStyle services.LabelStyle, labelFontFace font.Face, dims scaledDims, textScale float32, logoScale float32) int {
	_ = logoScale
	switch labelStyle {
	case services.LabelStyleOfficial, services.LabelStyleHighRes:
		return int(dims.iconHeight)
	case services.LabelStyleIcon:
		if icon := IconForSource(badge.Source); icon != nil {
			return int(iconScaledWidth(icon, dims.iconHeight))
		}
		return baseTextWidth(badge.Source.Label, labelFontFace, textScale)
	default:
		return baseTextWidth(badge.Source.Label, labelFontFace, textScale)
	}
}

// baseTextWidth measures a string's width at the default (100%) font size by
// scaling the measured width (taken from a text_size-scaled face) back by the
// text scale factor. Glyph advances scale linearly with font size.
func baseTextWidth(text string, face font.Face, textScale float32) int {
	if textScale <= 0 {
		textScale = 1
	}
	return int(math.Round(float64(textWidth(text, face)) / float64(textScale)))
}

func RenderBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, textScale float32, logoScale float32, colors map[string]services.SourceColorSet) *image.RGBA {
	dims := newScaledDims(badgeScale)
	maxLabelW := labelWidthForStyle(badge, labelStyle, labelFontFace, dims, textScale, logoScale)
	maxValueW := baseTextWidth(badge.Value, fontFace, textScale)
	return renderBadgeInner(badge, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims, textScale, logoScale, colors)
}

func RenderBadgesUniform(badges []services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, textScale float32, logoScale float32, colors map[string]services.SourceColorSet) []*image.RGBA {
	if len(badges) == 0 {
		return nil
	}

	dims := newScaledDims(badgeScale)
	var maxLabelW, maxValueW int
	for _, b := range badges {
		if w := labelWidthForStyle(&b, labelStyle, labelFontFace, dims, textScale, logoScale); w > maxLabelW {
			maxLabelW = w
		}
		if w := baseTextWidth(b.Value, fontFace, textScale); w > maxValueW {
			maxValueW = w
		}
	}

	result := make([]*image.RGBA, len(badges))
	for i, b := range badges {
		result[i] = renderBadgeInner(&b, fontFace, labelFontFace, maxLabelW, maxValueW, labelStyle, appearance, dims, textScale, logoScale, colors)
	}
	return result
}

func RenderVerticalBadge(badge *services.RatingBadge, fontFace, labelFontFace font.Face, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeScale float32, textScale float32, logoScale float32, colors map[string]services.SourceColorSet) *image.RGBA {
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
	// Per-axis width/height: scale the vertical badge box independently.
	// The logo and text sizes are NOT affected (logo_size/text_size only).
	wPct := appearance.Width.Percent()
	hPct := appearance.Height.Percent()
	if wPct <= 0 {
		wPct = 1
	}
	if hPct <= 0 {
		hPct = 1
	}
	vertBadgeW = int(math.Round(float64(vertBadgeW) * float64(wPct)))
	var pillPad uint32
	if appearance.Shape == services.BadgeShapePill {
		pillPad = uint32(math.Round(float64(basePillPadding) * float64(badgeScale)))
	}
	dims := newScaledDims(badgeScale)
	vertPadV := uint32(math.Round(float64(baseVertBadgePaddingV)*float64(badgeScale))) + pillPad
	// Minimum gap between content ink and the badge border (see badgeContentGapBase).
	contentGap := int(dims.contentGap)

	// Each stacked section mirrors the horizontal badge box: it is as tall as
	// the horizontal badge height (badgeHeight + pill vertical padding) and is
	// fixed by badge_size — it does NOT grow with text_size. At the same 100%
	// badge/text settings the value text and the label/icon therefore occupy
	// exactly the same relative space as in the lr/rl badges. Enlarged text is
	// clipped at the section edges (cut off evenly on both sides), exactly like
	// the fixed horizontal badge box.
	//
	// For logo badges (icon label styles) the label section HUGS the logo
	// instead: the logo starts at the outer margin and its inward side keeps
	// only badgeInwardGap from the seam, so the section grows to fit the logo
	// (a wider logo needs a taller section) and no dead space sits between the
	// logo and the value text.
	sectionH := int(math.Round(float64(dims.badgeHeight) * float64(hPct)))
	if appearance.Shape == services.BadgeShapePill {
		sectionH += int(math.Round(float64(dims.pillPaddingV) * float64(hPct)))
	}
	// The INWARD side of the logo (facing the value text) keeps only
	// badgeInwardGap from the seam, so the text box owns the remaining space
	// and centres the text; the OUTER margins (top/bottom + sides) are equal.
	// The logo's HEIGHT scales only with logo_size (not badge_size); its
	// vertical width then fills the badge minus the equal outer margins.
	iconHeight := uint32(math.Round(float64(baseIconHeight) * float64(logoScale)))
	// The inter-section gap mirrors the logo's inward gap: barely any margin
	// between the label content and the value text.
	gap := uint32(badgeInwardGap)
	vertPadVSc := uint32(math.Round(float64(vertPadV) * float64(hPct)))

	var vLogoW, vLogoH, hMarginV int
	labelAreaH := sectionH
	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			_, hIconH := badgeIconAndSize(badge, labelStyle, iconHeight, icon)
			hMarginV = (sectionH - int(hIconH)) / 2 // top/bottom margin at this badge height
			if hMarginV < 0 {
				hMarginV = 0
			}
			// The logo WIDTH margin is fixed by the 100%-height reference so
			// the logo size never tracks badge_height (per-axis): extra height
			// becomes padding (hMarginV), exactly like the horizontal badge
			// keeps its logo at logo_size. At 100% hMarginVRef == hMarginV.
			refSectionH := int(dims.badgeHeight)
			if appearance.Shape == services.BadgeShapePill {
				refSectionH += int(dims.pillPaddingV)
			}
			hMarginVRef := (refSectionH - int(hIconH)) / 2
			if hMarginVRef < 0 {
				hMarginVRef = 0
			}
			// The logo fills the badge width minus the equal outer margin on
			// each side, so at 100% the three OUTER margins are equal (tb:
			// left/top/right; bt: left/bottom/right) and the INWARD side keeps
			// only badgeInwardGap from the seam.
			vLogoW = vertBadgeW - 2*hMarginVRef
			if icon.Bounds().Dx() > 0 && icon.Bounds().Dy() > 0 {
				vLogoH = int(math.Round(float64(vLogoW) * float64(icon.Bounds().Dy()) / float64(icon.Bounds().Dx())))
			}
			if vLogoW < 1 {
				vLogoW = 1
			}
			if vLogoH < 1 {
				vLogoH = 1
			}
			// The label section hugs the logo: the logo starts at the outer
			// margin (hMarginV) and its INWARD side keeps only a tiny gap from
			// the seam, so no dead space sits between logo and value text. The
			// logo is positioned at iy = hMarginV from the badge edge, while the
			// section begins at vertPadVSc — so subtract that offset.
			labelAreaH = hMarginV + vLogoH + badgeInwardGap - int(vertPadVSc)
			if labelAreaH < 1 {
				labelAreaH = 1
			}
		}
	}
	valueH := sectionH
	totalH := int(vertPadVSc + uint32(labelAreaH) + gap + uint32(valueH) + vertPadVSc)

	img := image.NewRGBA(image.Rect(0, 0, vertBadgeW, totalH))
	// For the mirrored vertical style (bt) the value sits on top and the
	// logo/label on the bottom. Both styles share the same symmetric geometry:
	// the top section starts after the vertPadV top padding and the bottom
	// section starts after the top section plus the full gap, so tb and bt are
	// exact mirror images (matching how the lr/rl horizontal pair is laid out).
	labelTop := !appearance.Style.IsMirrored()
	labelAreaY := int(vertPadVSc)
	valueAreaY := int(vertPadVSc) + labelAreaH + int(gap)
	if !labelTop {
		labelAreaY = int(vertPadVSc) + valueH + int(gap)
		valueAreaY = int(vertPadVSc)
	}

	labelBG, valueBG, hasBG := sectionColors(appearance.Alpha, badge.Source, override)
	if hasBG {
		radius := float32(cornerRadius(appearance.Shape, uint32(vertBadgeW), uint32(math.Round(float64(baseBadgeRadius)*float64(badgeScale)))))
		// Fill the full badge height contiguously so the padding and gap
		// regions above/between/below the sections are not left transparent
		// (that let the artwork show through the middle and bottom of the
		// badge). The top tile covers [0, bottomSectionY] and owns the rounded
		// top corners; the bottom tile covers [bottomSectionY, totalH] and owns
		// the rounded bottom corners; the seam at bottomSectionY is a straight
		// line inside the badge. The tiles never overlap, so neither section is
		// composited twice (that made the value section look more opaque than
		// the label section). The border ring is then stroked on top.
		topBG := labelBG
		bottomBG := valueBG
		bottomSectionY := valueAreaY // tb: label on top, value below
		if !labelTop {
			topBG = valueBG
			bottomBG = labelBG
			bottomSectionY = labelAreaY // bt: value on top, label below
		}
		fillRoundedRect(img, 0, 0, vertBadgeW, bottomSectionY, radius, cornerTL|cornerTR, topBG)
		fillRoundedRect(img, 0, bottomSectionY, vertBadgeW, totalH, radius, cornerBL|cornerBR, bottomBG)
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

	label := badge.Source.Label
	value := badge.Value

	if useIcon {
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			scaledIcon := scaleIcon(icon, uint32(vLogoW), uint32(vLogoH))
			ix := (vertBadgeW - vLogoW) / 2
			var iy int
			if labelTop {
				iy = hMarginV
			} else {
				iy = totalH - hMarginV - vLogoH
			}
			// Clip the logo so it is always at least contentGap from the badge's
			// outer borders (and never spills past the label section's inner
			// seam). The logo's INWARD side may reach the seam (its tiny
			// badgeInwardGap keeps it off at 100%), so that side of the clip is
			// not inset; oversized logos are cut at the seam instead.
			var logoClip image.Rectangle
			if labelTop {
				logoClip = image.Rect(contentGap, contentGap, vertBadgeW-contentGap, labelAreaY+labelAreaH)
			} else {
				logoClip = image.Rect(contentGap, labelAreaY, vertBadgeW-contentGap, totalH-contentGap)
			}
			overlayIconShadowedInRect(img, scaledIcon, ix, iy, shadowPx, logoClip)
		} else {
			drawVerticalLabel(img, textCol, label, labelFontFace, labelAreaY, labelAreaH, vertBadgeW, shadowPx, contentGap)
		}
	} else {
		drawVerticalLabel(img, textCol, label, labelFontFace, labelAreaY, labelAreaH, vertBadgeW, shadowPx, contentGap)
	}

	// Value text, centred in its (fixed) value section — ink-based centring on
	// both axes, exactly like the horizontal (lr/rl) badge — so the text box
	// owns the remaining space. It is clipped to the section inset by the
	// content gap, so oversized text cuts off evenly without touching the
	// badge border.
	if inkX0, inkY0, inkX1, inkY1, ok := textInkBBox(fontFace, value); ok {
		valueX := (vertBadgeW - (inkX0 + inkX1)) / 2
		valueTextY := valueAreaY + valueH/2 - (inkY0+inkY1)/2
		drawTextShadowedInRect(img, textCol, valueX, valueTextY, fontFace, value, shadowPx, image.Rect(contentGap, valueAreaY+contentGap, vertBadgeW-contentGap, valueAreaY+valueH-contentGap))
	} else {
		valueW := textWidth(value, fontFace)
		valueX := (vertBadgeW - valueW) / 2
		valueTextY := valueAreaY + int(valueH)/2 + ascentVal/2 - 2
		drawTextShadowed(img, textCol, valueX, valueTextY, fontFace, value, shadowPx)
	}

	return img
}

// drawVerticalLabel draws the source label centred in a vertical badge's label
// section: ink-based centring on both axes, clipped to the section inset by the
// content gap so oversized text cuts off evenly without touching the badge
// border.
func drawVerticalLabel(img *image.RGBA, col color.RGBA, label string, labelFontFace font.Face, labelAreaY, labelAreaH, vertBadgeW, shadowPx, contentGap int) {
	if inkX0, inkY0, inkX1, inkY1, ok := textInkBBox(labelFontFace, label); ok {
		labelX := (vertBadgeW - (inkX0 + inkX1)) / 2
		labelY := labelAreaY + labelAreaH/2 - (inkY0+inkY1)/2
		drawTextShadowedInRect(img, col, labelX, labelY, labelFontFace, label, shadowPx, image.Rect(contentGap, labelAreaY+contentGap, vertBadgeW-contentGap, labelAreaY+labelAreaH-contentGap))
		return
	}
	labelW := textWidth(label, labelFontFace)
	labelX := (vertBadgeW - labelW) / 2
	labelY := labelAreaY + labelAreaH/2 + labelFontFace.Metrics().Ascent.Ceil()/2 - 2
	drawTextShadowed(img, col, labelX, labelY, labelFontFace, label, shadowPx)
}
