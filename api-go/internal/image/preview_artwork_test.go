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
	for y := range h {
		v := uint8(20 + (y*220)/h)
		for x := range w {
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestPosterPreviewArtworkPassthrough guards the settings preview's fit
// selector: the demo artwork must pass through unchanged for every fit so the
// preview matches the real render. Previously 2:3 artwork was centre-cropped
// to a wide 3:2 box to make the fit modes look artificially distinct — that
// cut the top/bottom of the poster off under fit=native.
func TestPosterPreviewArtworkPassthrough(t *testing.T) {
	fits := []services.PosterFit{
		services.PosterFitNative,
		services.PosterFitCover,
		services.PosterFitPad,
		services.PosterFitBlur,
	}
	for _, fit := range fits {
		portrait := gradientArtwork(300, 450) // 2:3, like the real demo poster
		shaped, err := PosterPreviewArtwork(portrait, fit, 300)
		if err != nil {
			t.Fatalf("PosterPreviewArtwork(fit=%s): %v", fit, err)
		}
		if !bytes.Equal(portrait, shaped) {
			t.Errorf("fit=%s: 2:3 artwork should pass through unchanged", fit)
		}

		wide := gradientArtwork(400, 200) // already non-2:3
		shapedWide, err := PosterPreviewArtwork(wide, fit, 300)
		if err != nil {
			t.Fatalf("PosterPreviewArtwork(wide, fit=%s): %v", fit, err)
		}
		if !bytes.Equal(wide, shapedWide) {
			t.Errorf("fit=%s: non-2:3 artwork should pass through unchanged", fit)
		}
	}
}

// TestPosterPreviewArtworkDecodesJPEG guards the format registration: the real
// demo artwork is a JPEG from the TMDB cache while the fallback sample is a
// PNG, so PosterPreviewArtwork must accept both and hand them through.
func TestPosterPreviewArtworkDecodesJPEG(t *testing.T) {
	// Build a JPEG-encoded 2:3 source (what the TMDB cache serves).
	img := image.NewRGBA(image.Rect(0, 0, 300, 450))
	for y := range 450 {
		for x := range 300 {
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
	if !bytes.Equal(jpg.Bytes(), shaped) {
		t.Fatal("jpeg artwork should pass through unchanged")
	}
	if _, _, err := image.Decode(bytes.NewReader(shaped)); err != nil {
		t.Fatalf("shaped jpeg-derived artwork failed to decode: %v", err)
	}
}

// TestPreviewFitModesMatchRealRender reproduces the reported bug: the preview
// centre-cropped the demo poster, cutting the top/bottom off under fit=native.
// The preview must now equal the real render:
//
//   - For the 2:3 demo poster every fit renders the full poster onto the same
//     2:3 canvas — the real pipeline does the same (a 2:3 source needs no
//     crop, bars or fill), so all four being identical is honest, not a bug.
//   - For non-2:3 artwork the fit logic must still visibly differ: native
//     keeps the source aspect, cover/pad/blur fill the 2:3 canvas differently.
func TestPreviewFitModesMatchRealRender(t *testing.T) {
	const targetWidth = 300

	fits := []services.PosterFit{
		services.PosterFitNative,
		services.PosterFitCover,
		services.PosterFitPad,
		services.PosterFitBlur,
	}

	// 2:3 portrait demo poster.
	portrait := gradientArtwork(300, 450)
	portraitRendered := make(map[services.PosterFit]*image.RGBA)
	for _, fit := range fits {
		canvas := renderPreviewFit(t, portrait, fit, targetWidth)
		portraitRendered[fit] = canvas
		if w, h := canvas.Bounds().Dx(), canvas.Bounds().Dy(); w != 300 || h != 450 {
			t.Errorf("fit=%s (2:3 source) rendered %dx%d, want the 2:3 canvas 300x450", fit, w, h)
		}
	}
	base := portraitRendered[services.PosterFitNative]
	for fit, canvas := range portraitRendered {
		if !samePixels(base, canvas) {
			t.Errorf("fit=%s (2:3 source) should render identically to native — the real pipeline does", fit)
		}
	}
	// Faithfulness: the native preview equals a straight render of the
	// unchanged artwork (no crop, no dimmed backdrop).
	if !samePixels(base, fitPoster(mustDecode(t, portrait), targetWidth, services.PosterFitNative)) {
		t.Error("native preview should match the real render of the unchanged artwork")
	}

	// Non-2:3 source: the four fits must stay visibly distinct.
	wide := gradientArtwork(400, 200)
	wideRendered := make(map[services.PosterFit]*image.RGBA)
	for _, fit := range fits {
		wideRendered[fit] = renderPreviewFit(t, wide, fit, targetWidth)
	}
	if samePixels(wideRendered[services.PosterFitNative], wideRendered[services.PosterFitCover]) {
		t.Error("native should keep the wide source aspect (differ from cover's 2:3 crop)")
	}
	seen := make(map[string]services.PosterFit)
	for fit, canvas := range wideRendered {
		key := pngKey(t, canvas)
		if prev, dup := seen[key]; dup {
			t.Errorf("fit=%s rendered byte-identical to fit=%s for non-2:3 artwork", fit, prev)
		}
		seen[key] = fit
	}
	if len(seen) != len(fits) {
		t.Fatalf("expected %d distinct fit outputs for non-2:3 artwork, got %d", len(fits), len(seen))
	}
}

// renderPreviewFit runs the exact preview pipeline for a fit: artwork is
// passed through PosterPreviewArtwork (unchanged) and then shaped by
// fitPoster, mirroring the preview handler and the real image endpoint.
func renderPreviewFit(t *testing.T, art []byte, fit services.PosterFit, targetWidth uint32) *image.RGBA {
	t.Helper()
	shaped, err := PosterPreviewArtwork(art, fit, targetWidth)
	if err != nil {
		t.Fatalf("PosterPreviewArtwork(fit=%s): %v", fit, err)
	}
	img, _, err := image.Decode(bytes.NewReader(shaped))
	if err != nil {
		t.Fatalf("decode shaped artwork (fit=%s): %v", fit, err)
	}
	return fitPoster(img, targetWidth, fit)
}

func mustDecode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func samePixels(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	return bytes.Equal(a.Pix, b.Pix)
}

func pngKey(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(buf.Bytes())
}
