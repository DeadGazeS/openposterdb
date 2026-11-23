package image

import (
	"image"
	"image/color"
	"testing"
)

func uniformRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// A uniform image must survive the blur unchanged. In uint8 the [1,2,1] sum
// of 255s wrapped around and white came out as 63.
func TestGaussianBlur_UniformUnchanged(t *testing.T) {
	for _, c := range []color.RGBA{
		{255, 255, 255, 255},
		{128, 128, 128, 255},
		{0, 0, 0, 255},
	} {
		img := uniformRGBA(16, 16, c)
		for range episodeBlurPasses {
			gaussianBlurRGBA(img)
		}
		for y := range 16 {
			for x := range 16 {
				if got := img.RGBAAt(x, y); got != c {
					t.Fatalf("%v at (%d,%d) became %v", c, x, y, got)
				}
			}
		}
		out := blurImage(uniformRGBA(160, 90, c), episodeBlurDivisor, episodeBlurPasses)
		if got := out.RGBAAt(80, 45); got != c {
			t.Errorf("blurImage(%v) centre = %v", c, got)
		}
	}
}

// The episode blur must erase detail the size of on-screen text: a 4 px
// checkerboard ends up within a few levels of flat grey.
func TestEpisodeBlur_RemovesDetail(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		for x := range 320 {
			v := uint8(0)
			if (x/4+y/4)%2 == 0 {
				v = 255
			}
			src.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	out := blurImage(src, episodeBlurDivisor, episodeBlurPasses)
	lo, hi := uint8(255), uint8(0)
	for y := 40; y < 140; y++ {
		for x := 40; x < 280; x++ {
			r := out.RGBAAt(x, y).R
			lo, hi = min(lo, r), max(hi, r)
		}
	}
	if hi-lo > 16 {
		t.Errorf("blurred checkerboard still varies %d..%d", lo, hi)
	}
}
