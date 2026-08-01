package image

import (
	"testing"

	"openposterdb/internal/services"
)

func TestBadgesAndOverlayAtScale(t *testing.T) {
	loadTestFont(t)
	badges := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
		{Source: services.SourceTmdb, Value: "77%"},
		{Source: services.SourceRt, Value: "100%"},
	}
	vf := GetValueFontFace()
	lf := GetFontFace()
	appearance := services.DefaultBadgeAppearance()
	for _, scale := range []float32{1.0, 1.2} {
		bis := RenderBadgesUniform(badges, vf, lf, services.LabelStyleText, appearance, scale, 1.0, nil)
		t.Logf("scale=%v badge count=%d", scale, len(bis))
		for i, bi := range bis {
			b := bi.Bounds()
			bright := 0
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r, g, bb, _ := bi.RGBAAt(x, y).RGBA()
					if r>>8 > 150 || g>>8 > 150 || bb>>8 > 150 {
						bright++
					}
				}
			}
			t.Logf("  badge[%d] size=%dx%d bright=%d", i, b.Dx(), b.Dy(), bright)
		}
	}
}
