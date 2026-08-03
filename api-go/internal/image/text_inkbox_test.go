package image

import (
	"image"
	"image/color"
	"testing"
)

// TestTextInkBBoxMatchesPixels guards the ink-bbox helper in badge.go: the
// box it returns must match the bounding box of the actually-drawn white
// pixels, so centring on it centres the visible ink.
func TestTextInkBBoxMatchesPixels(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	if vf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()

	for _, s := range []string{"10.0", "100%", "8.2", "77%", "4.0"} {
		adv := textWidth(s, vf)
		// Render offscreen, baseline at (0, 40).
		const baseline = 40
		img := image.NewRGBA(image.Rect(0, 0, adv+20, 80))
		drawText(img, color.RGBA{255, 255, 255, 255}, 0, baseline, vf, s)

		pxX0, pxY0, pxX1, pxY1 := img.Bounds().Max.X, img.Bounds().Max.Y, img.Bounds().Min.X, img.Bounds().Min.Y
		found := false
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				c := img.RGBAAt(x, y)
				if c.A > 200 && c.R > 200 && c.G > 200 && c.B > 200 {
					if !found {
						pxX0, pxY0, pxX1, pxY1 = x, y, x, y
						found = true
					} else {
						if x < pxX0 {
							pxX0 = x
						}
						if x > pxX1 {
							pxX1 = x
						}
						if y < pxY0 {
							pxY0 = y
						}
						if y > pxY1 {
							pxY1 = y
						}
					}
				}
			}
		}
		if !found {
			t.Fatalf("%q: no pixels rendered", s)
		}
		// Compare with textInkBBox (which reports relative to pen origin).
		ix0, iy0, ix1, iy1, ok := textInkBBox(vf, s)
		if !ok {
			t.Fatalf("%q: textInkBBox found no ink", s)
		}
		absX0, absY0, absX1, absY1 := ix0, iy0+baseline, ix1, iy1+baseline
		if dx := absInt(pxX0 - absX0); dx > 1 {
			t.Errorf("%q: ink bbox left px=%d helper=%d (off %d)", s, pxX0, absX0, dx)
		}
		if dx := absInt(pxX1 - absX1); dx > 1 {
			t.Errorf("%q: ink bbox right px=%d helper=%d (off %d)", s, pxX1, absX1, dx)
		}
		if dy := absInt(pxY0 - absY0); dy > 1 {
			t.Errorf("%q: ink bbox top px=%d helper=%d (off %d)", s, pxY0, absY0, dy)
		}
		if dy := absInt(pxY1 - absY1); dy > 1 {
			t.Errorf("%q: ink bbox bottom px=%d helper=%d (off %d)", s, pxY1, absY1, dy)
		}
	}
}
