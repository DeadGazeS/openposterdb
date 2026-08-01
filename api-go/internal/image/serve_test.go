package image

import (
	"bytes"
	"image"
	"image/color"
	"sync"
	"testing"

	"openposterdb/internal/services"

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

func TestSampleRenderHasBadges(t *testing.T) {
	loadTestFont(t)
	badges := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
		{Source: services.SourceTmdb, Value: "77%"},
		{Source: services.SourceRt, Value: "100%"},
	}
	badges = services.ApplyRatingPreferences(badges, "imdb,tmdb,rt", "", 3)
	position := services.PositionBottomCenter
	badgeDirection := services.BadgeDirectionHorizontal
	badgeStyle := services.BadgeStyleHorizontal.Resolve(badgeDirection)
	labelStyle := services.LabelStyleText
	appearance := services.DefaultBadgeAppearance()
	valueFace := GetValueFontFace()
	labelFace := GetFontFace()
	out, err := RenderPosterSync(SamplePosterPNG, badges, valueFace, labelFace, 85,
		position, badgeStyle, labelStyle, appearance, badgeDirection,
		580, 1.2, 1.2, 1.0, false, services.PosterFitNative, nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	b := img.Bounds()
	bright := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			if r>>8 > 150 || g>>8 > 150 || bb>>8 > 150 {
				bright++
			}
		}
	}
	if bright == 0 {
		t.Fatal("rendered poster has no bright (badge) pixels")
	}
	t.Logf("bright pixels: %d", bright)
}
