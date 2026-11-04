package image

import (
	"bytes"
	stdimage "image"
	"image/png"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, stdimage.NewGray(stdimage.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The pixel cap alone let a 1×N strip through, and scaling it to the target
// width asked for a canvas of gigabytes (an unrecoverable OOM).
func TestDecodeImage_RejectsExtremeTallAspect(t *testing.T) {
	if _, err := decodeImage(pngOf(t, 2, 4000)); err == nil {
		t.Fatal("1:2000 strip accepted")
	}
	for _, sz := range [][2]int{{500, 750}, {1920, 1080}, {800, 310}, {300, 600}} {
		if _, err := decodeImage(pngOf(t, sz[0], sz[1])); err != nil {
			t.Errorf("%dx%d rejected: %v", sz[0], sz[1], err)
		}
	}
}
