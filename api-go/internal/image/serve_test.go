package image

import (
	"os"
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
	layout := services.DefaultLayout("poster")
	badgeDirection := services.BadgeDirectionHorizontal
	badgeStyle := services.BadgeStyleDefault.Resolve(badgeDirection)
	labelStyle := services.LabelStyleText
	appearance := services.DefaultBadgeAppearance()
	valueFace := GetValueFontFace()
	labelFace := GetFontFace()
	out, err := RenderPosterSync(SamplePosterPNG, badges, valueFace, labelFace, 85,
		layout, badgeStyle, labelStyle, appearance,
		580, 1.2, 1.2, 1.0, 1.0, services.PosterFitNative, nil)
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

// TestHighResIconLoad ensures the highRes SVGs rasterize into usable icons and
// that a badge using LabelStyleHighRes renders icon pixels.
func TestHighResIconLoad(t *testing.T) {
	loadTestFont(t)
	// LoadIcons is a no-op after the first call, so build the cache directly.
	// Assets live relative to the repo root; tests run from the package dir.
	if _, err := os.Stat("../../assets/icons/highRes"); err == nil {
		os.Chdir("../..")
	}
	iconCacheMu.Lock()
	loadHighResIcons()
	iconCacheMu.Unlock()

	if len(highresCache) == 0 {
		t.Fatal("highresCache is empty; highRes SVGs did not load")
	}

	// Every source that has a highRes SVG must resolve to a distinct icon.
	sources := []*services.RatingBadge{
		{Source: services.SourceImdb, Value: "8.5"},
		{Source: services.SourceTmdb, Value: "77%"},
		{Source: services.SourceMetacritic, Value: "80"},
		{Source: services.SourceTrakt, Value: "90%"},
		{Source: services.SourceLetterboxd, Value: "4.0"},
		{Source: services.SourceMal, Value: "8.00"},
		{Source: services.SourceMdblist, Value: "7.2"},
		{Source: services.SourceEbert, Value: "4.0"},
		{Source: services.SourceRt, Value: "90%"},
		{Source: services.SourceRtAudience, Value: "40%"},
	}
	seen := make(map[uint32]bool)
	for _, b := range sources {
		icon := HighResIconForBadge(b)
		if icon == nil {
			t.Fatalf("no highRes icon for %s", b.Source.Label)
		}
		var sum uint32
		bounds := icon.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := icon.RGBAAt(x, y)
				if c.A > 128 {
					sum += uint32(c.R) + uint32(c.G) + uint32(c.B)
				}
			}
		}
		if seen[sum] {
			t.Fatalf("duplicate highRes icon content for %s (colsum %d)", b.Source.Label, sum)
		}
		seen[sum] = true
	}

	// LabelStyleHighRes must render icon pixels into the badge.
	badges := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "8.5"},
		{Source: services.SourceTmdb, Value: "77%"},
	}
	appearance := services.DefaultBadgeAppearance()
	vf := GetValueFontFace()
	lf := GetFontFace()
	bis := RenderBadgesUniform(badges, vf, lf, services.LabelStyleHighRes, appearance, 1.0, 1.0, 1.0, nil)
	if len(bis) != len(badges) {
		t.Fatalf("expected %d badges, got %d", len(badges), len(bis))
	}
	for i, bi := range bis {
		// The left (label) section holds the icon; require colored pixels there.
		b := bi.Bounds()
		iconPx := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Min.X+b.Dx()/3; x++ {
				c := bi.RGBAAt(x, y)
				if c.A > 128 && (c.R != 0 || c.G != 0 || c.B != 0) {
					iconPx++
				}
			}
		}
		if iconPx == 0 {
			t.Fatalf("badge %d (%s) rendered no icon pixels with HighRes style", i, badges[i].Source.Label)
		}
		t.Logf("badge %s: %dx%d, %d icon px", badges[i].Source.Label, b.Dx(), b.Dy(), iconPx)
	}
}
