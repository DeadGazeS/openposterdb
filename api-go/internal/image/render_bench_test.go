package image

import (
	"image"
	"image/color"
	"testing"

	"openposterdb/internal/services"
)

func benchSource(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	return img
}

// A wide source so the blur fill is visible (bars top/bottom).
func BenchmarkFitPosterBlur(b *testing.B) {
	src := benchSource(1280, 720)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fitPoster(src, 580, services.PosterFitBlur)
	}
}
