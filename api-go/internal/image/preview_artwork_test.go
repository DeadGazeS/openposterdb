package image

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"openposterdb/internal/services"
)

// gradientArtwork builds a non-uniform (vertical gradient) RGBA image, so that
// cover (stretched fill), pad (bars) and blur (fill + centred image) are
// pixel-distinct rather than collapsing onto one another.
func gradientArtwork(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := uint8(20 + (y*220)/h)
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestPosterPreviewArtworkShapes guards the settings preview's fit selector.
// A genuine 2:3 poster renders identically under every fit, so the artwork
// must be centre-cropped to a wide 3:2 box; artwork that is already non-2:3
// must pass through unchanged. fit=native must additionally come back as a
// 2:3 canvas so the preview box keeps its portrait shape.
func TestPosterPreviewArtworkShapes(t *testing.T) {
	const targetWidth = 300

	// 2:3 portrait source, like the real demo poster / SamplePosterPNG.
	shaped, err := PosterPreviewArtwork(gradientArtwork(300, 450), services.PosterFitCover, targetWidth)
	if err != nil {
		t.Fatalf("PosterPreviewArtwork(cover): %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(shaped))
	if err != nil {
		t.Fatalf("decode shaped artwork: %v", err)
	}
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 300 || h != 200 {
		t.Fatalf("2:3 source should be cropped to a 3:2 (300x200) box, got %dx%d", w, h)
	}

	// fit=native must be composed onto a 2:3 canvas (300x450) so the preview
	// box does not change shape.
	shapedNative, err := PosterPreviewArtwork(gradientArtwork(300, 450), services.PosterFitNative, targetWidth)
	if err != nil {
		t.Fatalf("PosterPreviewArtwork(native): %v", err)
	}
	nativeImg, _, err := image.Decode(bytes.NewReader(shapedNative))
	if err != nil {
		t.Fatalf("decode native artwork: %v", err)
	}
	if w, h := nativeImg.Bounds().Dx(), nativeImg.Bounds().Dy(); w != 300 || h != 450 {
		t.Fatalf("native fit should stay a 2:3 (300x450) canvas, got %dx%d", w, h)
	}

	// Non-2:3 artwork (already wide) must pass through unchanged.
	wide := gradientArtwork(400, 200)
	shapedWide, err := PosterPreviewArtwork(wide, services.PosterFitCover, targetWidth)
	if err != nil {
		t.Fatalf("PosterPreviewArtwork(wide): %v", err)
	}
	if !bytes.Equal(wide, shapedWide) {
		t.Fatal("non-2:3 artwork should pass through unchanged")
	}
}

// TestPosterPreviewArtworkDecodesJPEG guards the format registration: the real
// demo artwork is a JPEG from the TMDB cache while the fallback sample is a
// PNG, so PosterPreviewArtwork must decode both.
func TestPosterPreviewArtworkDecodesJPEG(t *testing.T) {
	// Build a JPEG-encoded 2:3 source (what the TMDB cache serves).
	img := image.NewRGBA(image.Rect(0, 0, 300, 450))
	for y := 0; y < 450; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode jpeg source: %v", err)
	}
	shaped, err := PosterPreviewArtwork(jpg.Bytes(), services.PosterFitCover, 300)
	if err != nil {
		t.Fatalf("PosterPreviewArtwork(jpeg): %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(shaped)); err != nil {
		t.Fatalf("shaped jpeg-derived artwork failed to decode: %v", err)
	}
}

// TestPreviewFitModesDiffer reproduces the reported bug: with a 2:3 demo
// poster, all four fit modes rendered byte-identical output, so changing the
// aspect-ratio (fit) setting showed no visible change — and after cropping the
// artwork to a wide ratio, fit=native rendered a wide box that collapsed the
// preview container. Every mode must now render onto the same 2:3 canvas
// (300x450) while the four outputs remain byte-distinct.
func TestPreviewFitModesDiffer(t *testing.T) {
	const targetWidth = 300
	rendered := make(map[services.PosterFit]*image.RGBA)
	for _, fit := range []services.PosterFit{
		services.PosterFitNative,
		services.PosterFitCover,
		services.PosterFitPad,
		services.PosterFitBlur,
	} {
		shaped, err := PosterPreviewArtwork(gradientArtwork(300, 450), fit, targetWidth)
		if err != nil {
			t.Fatalf("PosterPreviewArtwork(fit=%s): %v", fit, err)
		}
		img, _, err := image.Decode(bytes.NewReader(shaped))
		if err != nil {
			t.Fatalf("decode shaped artwork (fit=%s): %v", fit, err)
		}
		canvas := fitPoster(img, targetWidth, fit)
		rendered[fit] = canvas
		if w, h := canvas.Bounds().Dx(), canvas.Bounds().Dy(); w != 300 || h != 450 {
			t.Errorf("fit=%s rendered %dx%d, want the 2:3 canvas 300x450", fit, w, h)
		}
	}

	seen := make(map[string]services.PosterFit)
	for fit, canvas := range rendered {
		var buf bytes.Buffer
		if err := png.Encode(&buf, canvas); err != nil {
			t.Fatalf("encode fit=%s: %v", fit, err)
		}
		key := string(buf.Bytes())
		if prev, dup := seen[key]; dup {
			t.Errorf("fit=%s rendered byte-identical to fit=%s", fit, prev)
		}
		seen[key] = fit
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct fit outputs, got %d", len(seen))
	}
}
