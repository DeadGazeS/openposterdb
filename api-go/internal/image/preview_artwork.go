package image

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	"openposterdb/internal/services"

	"golang.org/x/image/draw"
)

// posterAspectTolerance is how close to 2:3 a source must be to be treated as
// a standard poster whose fit modes would otherwise render identically.
const posterAspectTolerance = 0.02

// posterPreviewBackdropKeep is the fraction of the artwork's brightness kept
// in the native-mode backdrop, so native reads as a subtle echo of the artwork
// rather than pad's pure black bars.
const posterPreviewBackdropKeep = 0.35

// PosterPreviewArtwork shapes demo poster artwork so the settings preview's
// fit selector (native/cover/pad/blur) produces visibly different results
// while the output canvas always stays a 2:3 portrait. The web preview box is
// sized from the rendered image's natural dimensions, so a fit=native wide box
// collapses the container and looks broken — every fit must therefore render
// onto a 2:3 canvas.
//
// A genuine 2:3 poster renders identically under every fit (no crop, no bars,
// no blur needed), so 2:3 artwork is first centre-cropped to a 3:2 landscape
// box. Artwork that is already non-2:3 (e.g. a future demo title) passes
// through unchanged.
//
// For fit=native the wide artwork is composed onto a 2:3 canvas at its natural
// ratio over a dimmed backdrop, because the standard native fit would keep the
// wide aspect and change the preview's shape. The other fits return the plain
// (wide) artwork and let the standard fitPoster logic put it into the 2:3
// canvas (cover crops, pad letterboxes with black bars, blur fills).
func PosterPreviewArtwork(posterBytes []byte, fit services.PosterFit, targetWidth uint32) ([]byte, error) {
	base, _, err := image.Decode(bytes.NewReader(posterBytes))
	if err != nil {
		return nil, err
	}
	bounds := base.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("poster artwork has empty bounds")
	}

	art := base
	if isPosterAspect(srcW, srcH) {
		// 3:2 landscape crop: height = 2/3 of the width, centred vertically.
		cropH := int(math.Round(float64(srcW) * 2.0 / 3.0))
		crop := image.NewRGBA(image.Rect(0, 0, srcW, cropH))
		draw.Draw(crop, crop.Bounds(), base, image.Point{bounds.Min.X, (srcH-cropH)/2 + bounds.Min.Y}, draw.Src)
		art = crop
	}

	if fit != services.PosterFitNative {
		return encodePNG(art)
	}

	// Native: keep the full artwork at its natural ratio on a 2:3 canvas, with
	// the artwork stretched to fill the canvas behind it and dimmed. This keeps
	// the preview box a 2:3 portrait while staying visibly distinct from cover
	// (full-bleed crop), pad (pure black bars) and blur (undimmed fill).
	canvasW := int(targetWidth)
	canvasH := int(posterTargetHeight(targetWidth))
	canvas := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	fill := resizeToFill(art, canvasW, canvasH)
	draw.Draw(canvas, canvas.Bounds(), fill, image.Point{}, draw.Src)
	dim := uint8(math.Round(255 * posterPreviewBackdropKeep))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, dim}), image.Point{}, draw.Over)

	// The artwork itself is scaled to fit inside the canvas at its natural
	// ratio and centred.
	artW0 := art.Bounds().Dx()
	artH0 := art.Bounds().Dy()
	if artW0 <= 0 || artH0 <= 0 {
		return nil, fmt.Errorf("poster artwork has empty bounds")
	}
	scale := math.Min(float64(canvasW)/float64(artW0), float64(canvasH)/float64(artH0))
	artW := int(math.Round(float64(artW0) * scale))
	artH := int(math.Round(float64(artH0) * scale))
	if artW < 1 {
		artW = 1
	}
	if artH < 1 {
		artH = 1
	}
	scaled := image.NewRGBA(image.Rect(0, 0, artW, artH))
	draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), art, art.Bounds(), draw.Src, nil)
	ox := (canvasW - artW) / 2
	oy := (canvasH - artH) / 2
	draw.Draw(canvas, scaled.Bounds().Add(image.Pt(ox, oy)), scaled, image.Point{}, draw.Src)

	return encodePNG(canvas)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isPosterAspect(w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	ratio := float64(w) / float64(h)
	return math.Abs(ratio-2.0/3.0) <= posterAspectTolerance
}
