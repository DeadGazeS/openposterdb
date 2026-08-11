package image

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"

	"openposterdb/internal/services"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
)

var blackUniform = image.NewUniform(color.Black)

const maxImagePixels = 8192 * 8192

// Sample artwork used by the admin preview endpoints. Built once at startup so
// previews render without external API calls.
var (
	SamplePosterPNG   []byte
	SampleLogoPNG     []byte
	SampleBackdropPNG []byte
)

func init() {
	SamplePosterPNG = buildSamplePoster()
	SampleLogoPNG = buildSampleLogo()
	SampleBackdropPNG = buildSampleBackdrop()
}

func buildSamplePoster() []byte {
	const w, h = 500, 750
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		v := uint8(42.0 - t*16.0)
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func buildSampleLogo() []byte {
	const w, h = 400, 120
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	const m = 8
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= m && x < w-m && y >= m && y < h-m {
				img.Set(x, y, color.RGBA{220, 220, 220, 240})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func buildSampleBackdrop() []byte {
	const w, h = 1280, 720
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		t := float64(x) / float64(w)
		r := uint8(26.0 + t*16.0)
		b := uint8(42.0 - t*16.0)
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{r, 26, b, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func posterTargetHeight(targetWidth uint32) uint32 {
	return uint32(math.Round(float64(targetWidth) * 1.5))
}

// RenderParams holds every option for the Render*Sync renderers. Fields that do not
// apply to a given kind are ignored (e.g. PosterFit for logos).
type RenderParams struct {
	Badges          []services.RatingBadge
	ValueFontFace   font.Face
	LabelFontFace   font.Face
	Quality         uint8
	Layout          services.ImageLayout
	BadgeStyle      services.BadgeStyle
	LabelStyle      services.LabelStyle
	Appearance      services.BadgeAppearance
	TargetWidth     uint32
	BadgeScale      float32
	BadgeMultiplier float32
	TextScale       float32
	LogoScale       float32
	PosterFit       services.PosterFit
	EdgeInsetX      int32
	EdgeInsetY      int32
	Blur            bool
	Colors          map[string]services.SourceColorSet
}

func RenderPosterSync(posterBytes []byte, params RenderParams) ([]byte, error) {
	if params.Appearance.Shape == services.BadgeShapePill {
		params.BadgeStyle = params.BadgeStyle.ForShape(params.Appearance.Shape)
	}
	params.Appearance.Style = params.BadgeStyle.ResolveDefault()

	base, _, err := image.Decode(bytes.NewReader(posterBytes))
	if err != nil {
		return nil, err
	}

	canvas := fitPoster(base, params.TargetWidth, params.PosterFit)

	if len(params.Badges) > 0 && !params.Layout.IsEmpty() {
		var badgeImages []*image.RGBA
		if params.Appearance.Style.IsVertical() {
			for _, b := range params.Badges {
				badgeImages = append(badgeImages, RenderVerticalBadge(&b, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors))
			}
		} else {
			badgeImages = RenderBadgesUniform(params.Badges, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors)
		}
		overlayLayoutOnCanvas(canvas, badgeImages, &params.Layout, params.BadgeScale, badgeSideMargin, 0, 0)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(params.Quality)}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// overlayLayoutOnCanvas distributes badges across the layout sides and overlays
// each side's block onto the canvas.
func overlayLayoutOnCanvas(canvas *image.RGBA, badgeImages []*image.RGBA, layout *services.ImageLayout, badgeScale float32, sideMarginBase uint32, extraX, extraY uint32) {
	sp := uint32(math.Round(float64(badgeSpacing) * float64(badgeScale)))
	rs := uint32(math.Round(float64(badgeRowSpacing) * float64(badgeScale)))
	bySide := distributeLayout(badgeImages, layout)
	for _, side := range layout.OrderOrDefault() {
		slot := layout.SideSlot(side)
		if slot == nil {
			continue
		}
		group := bySide[side]
		if len(group) == 0 {
			continue
		}
		block := renderSideBlock(group, int(slot.PerRow), int(slot.Rows), sp, rs)
		overlaySideBlock(canvas, block, side, slot.Start, badgeScale, sideMarginBase, extraX, extraY)
	}
}

func fitPoster(base image.Image, targetWidth uint32, fit services.PosterFit) *image.RGBA {
	bounds := base.Bounds()
	srcW := uint32(bounds.Dx())
	srcH := uint32(bounds.Dy())

	switch fit {
	case services.PosterFitNative:
		if srcW == targetWidth {
			rgba := image.NewRGBA(bounds)
			draw.Draw(rgba, bounds, base, bounds.Min, draw.Src)
			return rgba
		}
		scale := float64(targetWidth) / float64(srcW)
		targetH := uint32(math.Round(float64(srcH) * scale))
		if targetH == 0 {
			targetH = 1
		}
		scaled := image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), base, bounds, draw.Src, nil)
		return scaled

	case services.PosterFitCover:
		th := posterTargetHeight(targetWidth)
		return resizeToFill(base, int(targetWidth), int(th))

	case services.PosterFitPad:
		th := posterTargetHeight(targetWidth)
		scaled := image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(th)))
		scaledImg := resizeExact(base, int(targetWidth), int(th))
		ox := (int(targetWidth) - int(scaledImg.Bounds().Dx())) / 2
		oy := (int(th) - int(scaledImg.Bounds().Dy())) / 2
		draw.Draw(scaled, scaled.Bounds(), blackUniform, image.Point{}, draw.Src)
		draw.Draw(scaled, image.Rect(ox, oy, ox+scaledImg.Bounds().Dx(), oy+scaledImg.Bounds().Dy()), scaledImg, image.Point{}, draw.Over)
		return scaled

	case services.PosterFitBlur:
		// Fit the whole poster inside the 2:3 frame and fill the leftover bars
		// with a genuinely blurred, zoomed copy of the poster: the fill is a
		// cover-crop of the artwork, blurred via the same dependency-free
		// downscale/upscale trick the episode spoiler blur uses, with the
		// sharp full poster drawn centred on top.
		th := posterTargetHeight(targetWidth)
		scaled := image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(th)))
		filled := resizeToFill(base, int(targetWidth), int(th))
		filled = blurImage(filled, 4)
		draw.Draw(scaled, scaled.Bounds(), filled, image.Point{}, draw.Src)
		scaledImg := resizeExact(base, int(targetWidth), int(th))
		ox := (int(targetWidth) - int(scaledImg.Bounds().Dx())) / 2
		oy := (int(th) - int(scaledImg.Bounds().Dy())) / 2
		draw.Draw(scaled, image.Rect(ox, oy, ox+scaledImg.Bounds().Dx(), oy+scaledImg.Bounds().Dy()), scaledImg, image.Point{}, draw.Over)
		return scaled
	}

	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, base, bounds.Min, draw.Src)
	return rgba
}

func resizeToFill(img image.Image, targetW, targetH int) *image.RGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := math.Max(float64(targetW)/float64(srcW), float64(targetH)/float64(srcH))
	scaledW := int(math.Round(float64(srcW) * scale))
	scaledH := int(math.Round(float64(srcH) * scale))

	scaled := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, bounds, draw.Src, nil)

	result := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	ox := (scaledW - targetW) / 2
	oy := (scaledH - targetH) / 2
	draw.Draw(result, result.Bounds(), scaled, image.Point{ox, oy}, draw.Src)
	return result
}

func resizeExact(img image.Image, targetW, targetH int) *image.RGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := math.Min(float64(targetW)/float64(srcW), float64(targetH)/float64(srcH))
	nw := int(math.Round(float64(srcW) * scale))
	nh := int(math.Round(float64(srcH) * scale))

	resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Src, nil)
	return resized
}

// blurImage blurs an RGBA image by downscaling it to 1/divisor and scaling it
// back up with bilinear interpolation — a cheap, dependency-free blur used for
// blurred backdrops (the episode spoiler blur and the poster blur-fill fit).
// Very small images are returned unchanged.
// --- Logo rendering ---

func RenderLogoSync(logoBytes []byte, params RenderParams) ([]byte, error) {
	if params.Appearance.Shape == services.BadgeShapePill {
		params.BadgeStyle = params.BadgeStyle.ForShape(params.Appearance.Shape)
	}
	params.Appearance.Style = params.BadgeStyle.ResolveDefault()

	base, _, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var logoImg *image.RGBA
	if uint32(bounds.Dx()) != params.TargetWidth {
		scale := float64(params.TargetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		logoImg = image.NewRGBA(image.Rect(0, 0, int(params.TargetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(logoImg, logoImg.Bounds(), base, bounds, draw.Src, nil)
	} else {
		logoImg = image.NewRGBA(bounds)
		draw.Draw(logoImg, bounds, base, bounds.Min, draw.Src)
	}

	if len(params.Badges) == 0 || params.Layout.IsEmpty() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, logoImg); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if params.Appearance.Style.IsVertical() {
		for _, b := range params.Badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors))
		}
	} else {
		badgeImages = RenderBadgesUniform(params.Badges, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors)
	}

	logoBadgeSpacing := uint32(math.Round(float64(badgeSpacing) * float64(params.BadgeScale)))
	logoBadgeRowSpacing := uint32(math.Round(float64(badgeRowSpacing) * float64(params.BadgeScale)))
	gap := uint32(math.Round(15.0 * float64(params.BadgeScale)))

	bySide := distributeLayout(badgeImages, &params.Layout)
	blocks := make(map[string]*layoutBlock)
	for _, side := range params.Layout.OrderOrDefault() {
		slot := params.Layout.SideSlot(side)
		if slot == nil {
			continue
		}
		group := bySide[side]
		if len(group) == 0 {
			continue
		}
		b := renderSideBlock(group, int(slot.PerRow), int(slot.Rows), logoBadgeSpacing, logoBadgeRowSpacing)
		blocks[side] = &layoutBlock{img: b, w: b.Bounds().Dx(), h: b.Bounds().Dy()}
	}

	canvas := composeLogoLayout(logoImg, blocks, &params.Layout, int(gap))

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// composeLogoLayout grows the canvas to fit the logo plus badge blocks on any
// of the four sides. Blocks sit strictly outside the logo, hugging their edge
// and anchored per the side's start position.
func composeLogoLayout(logoImg *image.RGBA, blocks map[string]*layoutBlock, layout *services.ImageLayout, gap int) *image.RGBA {
	lw := logoImg.Bounds().Dx()
	lh := logoImg.Bounds().Dy()

	leftW, rightW := 0, 0
	if b, ok := blocks["left"]; ok {
		leftW = b.w
	}
	if b, ok := blocks["right"]; ok {
		rightW = b.w
	}
	topW, bottomW := 0, 0
	if b, ok := blocks["top"]; ok {
		topW = b.w
	}
	if b, ok := blocks["bottom"]; ok {
		bottomW = b.w
	}
	topH, bottomH := 0, 0
	if b, ok := blocks["top"]; ok {
		topH = b.h
	}
	if b, ok := blocks["bottom"]; ok {
		bottomH = b.h
	}
	leftH, rightH := 0, 0
	if b, ok := blocks["left"]; ok {
		leftH = b.h
	}
	if b, ok := blocks["right"]; ok {
		rightH = b.h
	}

	// The canvas must be wide enough for the logo + side blocks, and tall enough
	// for any block that is wider/taller than the logo zone so nothing clips.
	// Each present side block keeps a gap between it and the logo, so the logo
	// zone stays symmetric (left + gap + logo + gap + right).
	contentW := lw + leftW + rightW
	if leftW > 0 {
		contentW += gap
	}
	if rightW > 0 {
		contentW += gap
	}
	cw := contentW
	if topW > cw {
		cw = topW
	}
	if bottomW > cw {
		cw = bottomW
	}

	contentH := lh + topH + bottomH
	if topH > 0 {
		contentH += gap
	}
	if bottomH > 0 {
		contentH += gap
	}
	ch := contentH
	if leftH > ch {
		ch = leftH
	}
	if rightH > ch {
		ch = rightH
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cw, ch))

	// Top / bottom blocks are placed full-width of the logo zone, anchored
	// horizontally by their start position.
	if b, ok := blocks["top"]; ok {
		slot := layout.Top
		x := anchorX(slot.Start, cw, b.w, 0)
		overlay(canvas, b.img, x, 0)
	}
	if b, ok := blocks["bottom"]; ok {
		slot := layout.Bottom
		x := anchorX(slot.Start, cw, b.w, 0)
		overlay(canvas, b.img, x, ch-b.h)
	}

	// Left / right blocks are placed at the sides, anchored vertically.
	if b, ok := blocks["left"]; ok {
		slot := layout.Left
		y := anchorY(slot.Start, ch, b.h, 0)
		overlay(canvas, b.img, 0, y)
	}

	// The logo sits in the middle zone: below the top block, above the bottom
	// block, and between the left and right blocks. It is centred within that
	// zone, so with no block on a given axis it centres in the free canvas
	// space instead of hugging the edge, and when the zone exactly fits the
	// logo it sits flush against the neighbouring blocks.
	zoneX0, zoneX1 := 0, cw
	if leftW > 0 {
		zoneX0 = leftW + gap
	}
	if rightW > 0 {
		zoneX1 = cw - rightW - gap
	}
	logoX := zoneX0 + (zoneX1-zoneX0-lw)/2
	zoneY0, zoneY1 := 0, ch
	if topH > 0 {
		zoneY0 = topH + gap
	}
	if bottomH > 0 {
		zoneY1 = ch - bottomH - gap
	}
	logoY := zoneY0 + (zoneY1-zoneY0-lh)/2
	overlay(canvas, logoImg, logoX, logoY)

	if b, ok := blocks["right"]; ok {
		slot := layout.Right
		y := anchorY(slot.Start, ch, b.h, 0)
		overlay(canvas, b.img, cw-b.w, y)
	}

	return canvas
}

// --- Backdrop rendering ---

func RenderBackdropSync(backdropBytes []byte, params RenderParams) ([]byte, error) {
	if params.Appearance.Shape == services.BadgeShapePill {
		params.BadgeStyle = params.BadgeStyle.ForShape(params.Appearance.Shape)
	}
	params.Appearance.Style = params.BadgeStyle.ResolveDefault()

	base, _, err := image.Decode(bytes.NewReader(backdropBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var canvas *image.RGBA
	if uint32(bounds.Dx()) != params.TargetWidth {
		scale := float64(params.TargetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		canvas = image.NewRGBA(image.Rect(0, 0, int(params.TargetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), base, bounds, draw.Src, nil)
	} else {
		canvas = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(canvas, canvas.Bounds(), base, bounds.Min, draw.Src)
	}

	if len(params.Badges) == 0 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(params.Quality)}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if params.Appearance.Style.IsVertical() {
		for _, b := range params.Badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors))
		}
	} else {
		badgeImages = RenderBadgesUniform(params.Badges, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors)
	}

	ix := services.ClampEdgeInset(params.EdgeInsetX)
	iy := services.ClampEdgeInset(params.EdgeInsetY)
	extraX := uint32(math.Round(float64(canvas.Bounds().Dx()) * float64(ix) / 100.0))
	extraY := uint32(math.Round(float64(canvas.Bounds().Dy()) * float64(iy) / 100.0))

	if len(params.Badges) > 0 && !params.Layout.IsEmpty() {
		overlayLayoutOnCanvas(canvas, badgeImages, &params.Layout, params.BadgeScale, backdropSideMargin, extraX, extraY)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(params.Quality)}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- Episode rendering ---

func RenderEpisodeSync(imageBytes []byte, params RenderParams) ([]byte, error) {
	if params.Appearance.Shape == services.BadgeShapePill {
		params.BadgeStyle = params.BadgeStyle.ForShape(params.Appearance.Shape)
	}
	params.Appearance.Style = params.BadgeStyle.ResolveDefault()

	base, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var canvas *image.RGBA
	if uint32(bounds.Dx()) != params.TargetWidth {
		scale := float64(params.TargetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		canvas = image.NewRGBA(image.Rect(0, 0, int(params.TargetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), base, bounds, draw.Src, nil)
	} else {
		canvas = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(canvas, canvas.Bounds(), base, bounds.Min, draw.Src)
	}

	if params.Blur && canvas.Bounds().Dx() >= 8 && canvas.Bounds().Dy() >= 8 {
		canvas = blurImage(canvas, 4)
	}

	if len(params.Badges) == 0 {
		var output bytes.Buffer
		if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: int(params.Quality)}); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if params.Appearance.Style.IsVertical() {
		for _, b := range params.Badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors))
		}
	} else {
		badgeImages = RenderBadgesUniform(params.Badges, params.ValueFontFace, params.LabelFontFace, params.LabelStyle, params.Appearance, params.BadgeScale, params.TextScale, params.LogoScale, params.Colors)
	}

	if len(params.Badges) > 0 && !params.Layout.IsEmpty() {
		overlayLayoutOnCanvas(canvas, badgeImages, &params.Layout, params.BadgeScale, badgeSideMargin, 0, 0)
	}

	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: int(params.Quality)}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
