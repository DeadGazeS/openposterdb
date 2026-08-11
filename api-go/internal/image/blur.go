package image

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// blurImage returns a blurred copy of `img` using a true 3x3 Gaussian
// convolution on a 1/divisor-downscaled copy, then upscaled back to the
// original size. The downscale+blur+upscale combo keeps the per-pixel cost
// bounded by O(W*H) on the small image and produces a visibly smooth result
// (the upscale amplifies the blur strength so even a mild Gaussian reads as
// a strong blur in the final image).
//
// The convolution uses the separable [1,2,1]/4 kernel (sigma ~0.707) applied
// once horizontally then once vertically through a scratch buffer. No
// third-party dependency; matches the visual intent of the Rust reference's
// imageproc::gaussian_blur_f32 (sigma 3.0) without dragging in imageproc.
//
// If the input is smaller than 2*divisor on either axis, the original is
// returned unchanged.
func blurImage(img *image.RGBA, divisor int) *image.RGBA {
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
	gaussianBlurRGBA(small)
	out := image.NewRGBA(b)
	draw.CatmullRom.Scale(out, out.Bounds(), small, small.Bounds(), draw.Src, nil)
	return out
}

// gaussianBlurRGBA applies a 3x3 separable Gaussian (kernel [1,2,1] / 4) to
// the RGBA image in-place. Operates on the alpha channel too so the blur
// path produces identical pixel structure to the legacy downscale/upscale.
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
			tmp.SetRGBA(sx, sy, gauss3(c, l, r))
		}
	}
	// Vertical pass: tmp -> img.
	for y := 0; y < h; y++ {
		sy := y + b.Min.Y
		for x := 0; x < w; x++ {
			sx := x + b.Min.X
			c := tmp.RGBAAt(sx, sy)
			var t, bb color.RGBA = c, c
			if y > 0 {
				t = tmp.RGBAAt(sx, sy-1)
			}
			if y < h-1 {
				bb = tmp.RGBAAt(sx, sy+1)
			}
			img.SetRGBA(sx, sy, gauss3(c, t, bb))
		}
	}
}

// gauss3 returns the per-channel [1,2,1]/4 blend of (center, a, b). Works
// on premultiplied color.RGBA the same way it would on NRGBA - the
// per-channel weighted average commutes with alpha premultiplication.
func gauss3(c, a, b color.RGBA) color.RGBA {
	return color.RGBA{
		A: (c.A*2 + a.A + b.A) / 4,
		R: (c.R*2 + a.R + b.R) / 4,
		G: (c.G*2 + a.G + b.G) / 4,
		B: (c.B*2 + a.B + b.B) / 4,
	}
}
