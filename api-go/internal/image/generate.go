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

func RenderPosterSync(posterBytes []byte, badges []services.RatingBadge, fontFace font.Face, labelFontFace font.Face, quality uint8, position services.BadgePosition, badgeStyle services.BadgeStyle, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeDirection services.BadgeDirection, targetWidth uint32, badgeScale float32, badgeSize services.BadgeSize, posterBadgeSplit bool, posterFit services.PosterFit) ([]byte, error) {
	if appearance.Shape == services.BadgeShapePill {
		badgeStyle = badgeStyle.ForShape(appearance.Shape)
	}

	base, _, err := image.Decode(bytes.NewReader(posterBytes))
	if err != nil {
		return nil, err
	}

	canvas := fitPoster(base, targetWidth, posterFit)

	if len(badges) > 0 {
		var badgeImages []*image.RGBA
		if badgeStyle.IsVertical() {
			for _, b := range badges {
				badgeImages = append(badgeImages, RenderVerticalBadge(&b, fontFace, labelFontFace, labelStyle, appearance, badgeScale))
			}
		} else {
			badgeImages = RenderBadgesUniform(badges, fontFace, labelFontFace, labelStyle, appearance, badgeScale)
		}

		maxPR := maxBadgesPerRow
		if badgeSize == services.BadgeSizeLarge || badgeSize == services.BadgeSizeExtraLarge {
			if badgeStyle == services.BadgeStyleHorizontal {
				maxPR = 2
			} else {
				maxPR = 4
			}
		}
		if badgeStyle.IsVertical() {
			maxPR = maxVertBadgesPerRow
		}

		if posterBadgeSplit && len(badgeImages) >= 2 {
			splitTopBottom := !badgeDirection.IsVertical()
			primary, opposite := position.SplitAnchors(splitTopBottom)
			mid := (len(badgeImages) + 1) / 2
			overlayPosterGroup(canvas, badgeImages[:mid], primary, badgeDirection, maxPR, badgeScale)
			overlayPosterGroup(canvas, badgeImages[mid:], opposite, badgeDirection, maxPR, badgeScale)
		} else {
			overlayPosterGroup(canvas, badgeImages, position, badgeDirection, maxPR, badgeScale)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(quality)}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func overlayPosterGroup(canvas *image.RGBA, badgeImages []*image.RGBA, position services.BadgePosition, badgeDirection services.BadgeDirection, maxPerRow int, badgeScale float32) {
	if badgeDirection.IsVertical() {
		overlayVerticalStack(canvas, badgeImages, position, badgeScale, badgeSideMargin, 0, 0)
	} else {
		overlayHorizontalRows(canvas, badgeImages, position, maxPerRow, badgeScale, badgeSideMargin, 0, 0)
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
		// For blur, we just do pad for now (blur requires image processing not available in stdlib)
		th := posterTargetHeight(targetWidth)
		scaled := image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(th)))
		filled := resizeToFill(base, int(targetWidth), int(th))
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

// --- Logo rendering ---

func RenderLogoSync(logoBytes []byte, badges []services.RatingBadge, fontFace font.Face, labelFontFace font.Face, badgeStyle services.BadgeStyle, labelStyle services.LabelStyle, appearance services.BadgeAppearance, targetWidth uint32, badgeScale float32) ([]byte, error) {
	if appearance.Shape == services.BadgeShapePill {
		badgeStyle = badgeStyle.ForShape(appearance.Shape)
	}

	base, _, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var logoImg *image.RGBA
	if uint32(bounds.Dx()) != targetWidth {
		scale := float64(targetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		logoImg = image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(logoImg, logoImg.Bounds(), base, bounds, draw.Src, nil)
	} else {
		logoImg = image.NewRGBA(bounds)
		draw.Draw(logoImg, bounds, base, bounds.Min, draw.Src)
	}

	if len(badges) == 0 {
		var buf bytes.Buffer
		if err := png.Encode(&buf, logoImg); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if badgeStyle.IsVertical() {
		for _, b := range badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, fontFace, labelFontFace, labelStyle, appearance, badgeScale))
		}
	} else {
		badgeImages = RenderBadgesUniform(badges, fontFace, labelFontFace, labelStyle, appearance, badgeScale)
	}

	logoW := uint32(logoImg.Bounds().Dx())
	logoH := uint32(logoImg.Bounds().Dy())

	logoBadgeSpacing := uint32(math.Round(float64(badgeSpacing) * float64(badgeScale)))
	logoBadgeRowSpacing := uint32(math.Round(float64(badgeRowSpacing) * float64(badgeScale)))
	logoSpacingBelow := uint32(math.Round(15.0 * float64(badgeScale)))

	var rows [][]*image.RGBA
	chunkLen := maxBadgesPerRow
	if badgeStyle.IsVertical() {
		chunkLen = maxVertBadgesPerRow
	}
	for i := 0; i < len(badgeImages); i += chunkLen {
		end := i + chunkLen
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
	totalBadgeH := badgeH*uint32(len(rows)) + logoBadgeRowSpacing*(uint32(len(rows))-1)

	var maxRowW uint32
	for _, row := range rows {
		var rw uint32
		for _, bi := range row {
			rw += uint32(bi.Bounds().Dx())
		}
		rw += logoBadgeSpacing * (uint32(len(row)) - 1)
		if rw > maxRowW {
			maxRowW = rw
		}
	}

	cw := logoW
	if maxRowW > cw {
		cw = maxRowW
	}
	ch := logoH + logoSpacingBelow + totalBadgeH

	canvas := image.NewRGBA(image.Rect(0, 0, int(cw), int(ch)))

	lx := (cw - logoW) / 2
	overlay(canvas, logoImg, int(lx), 0)

	badgesY := logoH + logoSpacingBelow
	for rowIdx, row := range rows {
		var rw uint32
		for _, bi := range row {
			rw += uint32(bi.Bounds().Dx())
		}
		rw += logoBadgeSpacing * (uint32(len(row)) - 1)

		startX := (cw - rw) / 2
		y := badgesY + uint32(rowIdx)*(badgeH+logoBadgeRowSpacing)

		x := startX
		for _, bi := range row {
			bh := uint32(bi.Bounds().Dy())
			by := y + (badgeH-bh)/2
			overlay(canvas, bi, int(x), int(by))
			x += uint32(bi.Bounds().Dx()) + logoBadgeSpacing
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- Backdrop rendering ---

func RenderBackdropSync(backdropBytes []byte, badges []services.RatingBadge, fontFace font.Face, labelFontFace font.Face, quality uint8, position services.BadgePosition, badgeStyle services.BadgeStyle, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeDirection services.BadgeDirection, targetWidth uint32, badgeScale float32, badgeSize services.BadgeSize, edgeInsetX, edgeInsetY int32) ([]byte, error) {
	if appearance.Shape == services.BadgeShapePill {
		badgeStyle = badgeStyle.ForShape(appearance.Shape)
	}

	base, _, err := image.Decode(bytes.NewReader(backdropBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var canvas *image.RGBA
	if uint32(bounds.Dx()) != targetWidth {
		scale := float64(targetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		canvas = image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), base, bounds, draw.Src, nil)
	} else {
		canvas = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(canvas, canvas.Bounds(), base, bounds.Min, draw.Src)
	}

	if len(badges) == 0 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(quality)}); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if badgeStyle.IsVertical() {
		for _, b := range badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, fontFace, labelFontFace, labelStyle, appearance, badgeScale))
		}
	} else {
		badgeImages = RenderBadgesUniform(badges, fontFace, labelFontFace, labelStyle, appearance, badgeScale)
	}

	ix := services.ClampEdgeInset(edgeInsetX)
	iy := services.ClampEdgeInset(edgeInsetY)
	extraX := uint32(math.Round(float64(canvas.Bounds().Dx()) * float64(ix) / 100.0))
	extraY := uint32(math.Round(float64(canvas.Bounds().Dy()) * float64(iy) / 100.0))

	if badgeDirection.IsVertical() {
		overlayVerticalStack(canvas, badgeImages, position, badgeScale, backdropSideMargin, extraX, extraY)
	} else {
		overlayHorizontalRows(canvas, badgeImages, position, len(badgeImages), badgeScale, backdropSideMargin, extraX, extraY)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: int(quality)}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- Episode rendering ---

func RenderEpisodeSync(imageBytes []byte, badges []services.RatingBadge, fontFace font.Face, labelFontFace font.Face, quality uint8, position services.BadgePosition, badgeStyle services.BadgeStyle, labelStyle services.LabelStyle, appearance services.BadgeAppearance, badgeDirection services.BadgeDirection, targetWidth uint32, badgeScale float32, badgeSize services.BadgeSize, blur bool) ([]byte, error) {
	if appearance.Shape == services.BadgeShapePill {
		badgeStyle = badgeStyle.ForShape(appearance.Shape)
	}

	base, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, err
	}

	bounds := base.Bounds()
	var canvas *image.RGBA
	if uint32(bounds.Dx()) != targetWidth {
		scale := float64(targetWidth) / float64(bounds.Dx())
		targetH := uint32(math.Round(float64(bounds.Dy()) * scale))
		canvas = image.NewRGBA(image.Rect(0, 0, int(targetWidth), int(targetH)))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), base, bounds, draw.Src, nil)
	} else {
		canvas = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(canvas, canvas.Bounds(), base, bounds.Min, draw.Src)
	}

	if blur && canvas.Bounds().Dx() >= 8 && canvas.Bounds().Dy() >= 8 {
		cw := canvas.Bounds().Dx()
		ch := canvas.Bounds().Dy()
		small := image.NewRGBA(image.Rect(0, 0, cw/4, ch/4))
		draw.ApproxBiLinear.Scale(small, small.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
		canvas = image.NewRGBA(image.Rect(0, 0, cw, ch))
		draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), small, small.Bounds(), draw.Src, nil)
	}

	if len(badges) == 0 {
		var output bytes.Buffer
		if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: int(quality)}); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}

	var badgeImages []*image.RGBA
	if badgeStyle.IsVertical() {
		for _, b := range badges {
			badgeImages = append(badgeImages, RenderVerticalBadge(&b, fontFace, labelFontFace, labelStyle, appearance, badgeScale))
		}
	} else {
		badgeImages = RenderBadgesUniform(badges, fontFace, labelFontFace, labelStyle, appearance, badgeScale)
	}

	if badgeDirection.IsVertical() {
		overlayVerticalStack(canvas, badgeImages, position, badgeScale, badgeSideMargin, 0, 0)
	} else {
		maxPR := maxBadgesPerRow
		if badgeSize == services.BadgeSizeLarge || badgeSize == services.BadgeSizeExtraLarge {
			if badgeStyle == services.BadgeStyleHorizontal {
				maxPR = 2
			} else {
				maxPR = 4
			}
		}
		if badgeStyle.IsVertical() {
			maxPR = maxVertBadgesPerRow
		}
		overlayHorizontalRows(canvas, badgeImages, position, maxPR, badgeScale, badgeSideMargin, 0, 0)
	}

	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: int(quality)}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
