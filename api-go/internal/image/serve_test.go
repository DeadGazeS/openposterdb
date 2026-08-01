package image

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func loadTestFont(t *testing.T) {
	t.Helper()
	if err := LoadFont("../../assets/fonts/Inter-Bold.ttf"); err != nil {
		t.Skipf("font asset not available: %v", err)
	}
	if GetFontFace() == nil {
		t.Fatal("font did not load")
	}
}

// TestConcurrentRendering guards against the shared-opentype.Face data race
// that crashed under parallel image requests (sfnt.LoadGlyph panic). opentype.Face
// is not safe for concurrent use, so each caller must get its own face; the
// underlying *sfnt.Font is immutable and safe to share.
func TestConcurrentRendering(t *testing.T) {
	loadTestFont(t)

	values := []string{"10.0", "100%", "8.1", "85%", "5.0", "10.00", "4.0", "94%", "IMDb", "LB", "RT"}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			face := GetFontFace()
			if face == nil {
				t.Error("GetFontFace returned nil")
				return
			}
			defer face.Close()
			for _, s := range values {
				img := image.NewRGBA(image.Rect(0, 0, 200, 60))
				d := &font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face, Dot: fixed.P(0, 40)}
				d.DrawString(s)
			}
		}()
	}
	wg.Wait()
}
