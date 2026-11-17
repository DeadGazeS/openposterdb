package image

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// episodeBlurDivisor / episodeBlurPasses reproduce the Rust spoiler blur: a
// 1/8 downscale, a Gaussian with sigma 3 on the small copy (each [1,2,1]/4
// pass adds a variance of 0.5, so 18 passes give sigma² = 9), then an
// upscale back to full size. Stills must be unreadable, not just soft.
const (
	episodeBlurDivisor = 8
	episodeBlurPasses  = 18
)

// blurImage returns a blurred copy of img: downscaled to 1/divisor, blurred
// with `passes` passes of the [1,2,1]/4 Gaussian, and scaled back up. The
// work happens on the small copy, so the cost stays O(W*H/divisor²) per pass.
//
// If the input is smaller than 2*divisor on either axis, the original is
// returned unchanged.
func blurImage(img *image.RGBA, divisor, passes int) *image.RGBA {
	if divisor < 2 {
		divisor = 2
	}
	b := img.Bounds()
	cw, ch := b.Dx(), b.Dy()
	if cw < divisor*2 || ch < divisor*2 {
		return img
	}
	small := image.NewRGBA(image.Rect(0, 0, cw/divisor, ch/divisor))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), img, b, draw.Src, nil)
	for range max(passes, 1) {
		gaussianBlurRGBA(small)
	}
	out := image.NewRGBA(b)
	draw.CatmullRom.Scale(out, out.Bounds(), small, small.Bounds(), draw.Src, nil)
	return out
}

// gaussianBlurRGBA applies a 3x3 separable Gaussian (kernel [1,2,1] / 4) to
// the RGBA image in-place, alpha included. Edge pixels repeat the border.
func gaussianBlurRGBA(img *image.RGBA) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 3 || h < 3 {
		return
	}
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	// Horizontal pass: img -> tmp.
	for y := 0; y < h; y++ {
		sy := y + b.Min.Y
		for x := 0; x < w; x++ {
			sx := x + b.Min.X
			c := img.RGBAAt(sx, sy)
			var l, r color.RGBA = c, c
			if x > 0 {
				l = img.RGBAAt(sx-1, sy)
			}
			if x < w-1 {
				r = img.RGBAAt(sx+1, sy)
			}
			tmp.SetRGBA(x, y, gauss3(c, l, r))
		}
	}
	// Vertical pass: tmp -> img.
	for y := 0; y < h; y++ {
		sy := y + b.Min.Y
		for x := 0; x < w; x++ {
			sx := x + b.Min.X
			c := tmp.RGBAAt(x, y)
			var t, bb color.RGBA = c, c
			if y > 0 {
				t = tmp.RGBAAt(x, y-1)
			}
			if y < h-1 {
				bb = tmp.RGBAAt(x, y+1)
			}
			img.SetRGBA(sx, sy, gauss3(c, t, bb))
		}
	}
}

// gauss3 returns the per-channel [1,2,1]/4 blend of (center, a, b). The sum
// is taken in uint16: in uint8, 2*255 + 255 + 255 wrapped around and turned
// white into 63. Works on premultiplied color.RGBA the same way as on NRGBA
// (the weighted average commutes with premultiplication).
func gauss3(c, a, b color.RGBA) color.RGBA {
	mix := func(c, a, b uint8) uint8 {
		return uint8((uint16(c)*2 + uint16(a) + uint16(b) + 2) / 4)
	}
	return color.RGBA{
		R: mix(c.R, a.R, b.R),
		G: mix(c.G, a.G, b.G),
		B: mix(c.B, a.B, b.B),
		A: mix(c.A, a.A, b.A),
	}
}
