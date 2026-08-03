package image

import (
	"image"
	"image/color"
	"testing"

	"openposterdb/internal/services"

	"golang.org/x/image/draw"
)

// solidRGBA returns an opaque w×h RGBA image filled with c.
func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// redBounds scans the canvas and returns the inclusive bounding box of red
// pixels. The canvas starts transparent black; the logo and blocks are opaque
// solid colours, so only the logo's own pixels match red.
func redBounds(t *testing.T, img *image.RGBA) (x0, y0, x1, y1 int, found bool) {
	t.Helper()
	b := img.Bounds()
	x0, y0 = b.Max.X, b.Max.Y
	x1, y1 = b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.A < 200 || c.R < 200 || c.G > 80 || c.B > 80 {
				continue
			}
			found = true
			if x < x0 {
				x0 = x
			}
			if x > x1 {
				x1 = x
			}
			if y < y0 {
				y0 = y
			}
			if y > y1 {
				y1 = y
			}
		}
	}
	return x0, y0, x1, y1, found
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TestLogoCenteredWithSideBlocks covers a 100×100 red logo flanked by tall
// left/right blocks. The canvas must grow to left+gap+logo+gap+right =
// 100+30+30+2*gap wide and 400 tall, and the logo must be centred both
// horizontally and vertically inside it.
func TestLogoCenteredWithSideBlocks(t *testing.T) {
	const (
		logoW, logoH = 100, 100
		blockW       = 30
		blockH       = 400
		gap          = 10
	)
	logo := solidRGBA(logoW, logoH, color.RGBA{R: 255, A: 255})
	side := color.RGBA{B: 255, A: 255}
	left := &layoutBlock{img: solidRGBA(blockW, blockH, side), w: blockW, h: blockH}
	right := &layoutBlock{img: solidRGBA(blockW, blockH, side), w: blockW, h: blockH}

	layout := &services.ImageLayout{
		Left:  services.SideSlot{PerRow: 2, Rows: 3, Start: "c"},
		Right: services.SideSlot{PerRow: 1, Rows: 3, Start: "c"},
		Order: []string{"left", "right"},
	}

	canvas := composeLogoLayout(logo, map[string]*layoutBlock{"left": left, "right": right}, layout, gap)
	cw, ch := canvas.Bounds().Dx(), canvas.Bounds().Dy()
	if want := logoW + 2*blockW + 2*gap; cw != want {
		t.Errorf("canvas width = %d, want %d (%d+2*%d+2*gap)", cw, want, logoW, blockW)
	}
	if ch != blockH {
		t.Errorf("canvas height = %d, want %d", ch, blockH)
	}

	x0, y0, x1, y1, found := redBounds(t, canvas)
	if !found {
		t.Fatal("no red logo pixels on canvas")
	}
	if w, h := x1-x0+1, y1-y0+1; w != logoW || h != logoH {
		t.Fatalf("red logo bbox = %dx%d at (%d,%d), want %dx%d", w, h, x0, y0, logoW, logoH)
	}

	// Vertically centred: top ≈ (400-100)/2 ±1.
	if want := (ch - logoH) / 2; absInt(y0-want) > 1 {
		t.Errorf("logo top = %d, want %d (±1)", y0, want)
	}
	// Horizontally centred: left ≈ (180-100)/2 ±1.
	if want := (cw - logoW) / 2; absInt(x0-want) > 1 {
		t.Errorf("logo left = %d, want %d (±1)", x0, want)
	}
}

// TestLogoHorizontallyCenteredUnderWideTopBlock covers a 100×100 red logo
// under a full-width 400×30 top block. The canvas is 400 wide and the logo
// must be horizontally centred inside it (left ≈ (400-100)/2 ±1).
func TestLogoHorizontallyCenteredUnderWideTopBlock(t *testing.T) {
	const (
		logoW, logoH = 100, 100
		blockW       = 400
		blockH       = 30
		gap          = 10
	)
	logo := solidRGBA(logoW, logoH, color.RGBA{R: 255, A: 255})
	top := &layoutBlock{img: solidRGBA(blockW, blockH, color.RGBA{B: 255, A: 255}), w: blockW, h: blockH}

	layout := &services.ImageLayout{
		Top:   services.SideSlot{PerRow: 4, Rows: 1, Start: "c"},
		Order: []string{"top"},
	}

	canvas := composeLogoLayout(logo, map[string]*layoutBlock{"top": top}, layout, gap)
	if cw := canvas.Bounds().Dx(); cw != blockW {
		t.Errorf("canvas width = %d, want %d", cw, blockW)
	}

	x0, _, x1, _, found := redBounds(t, canvas)
	if !found {
		t.Fatal("no red logo pixels on canvas")
	}
	if w := x1 - x0 + 1; w != logoW {
		t.Fatalf("red logo width = %d, want %d", w, logoW)
	}
	if want := (blockW - logoW) / 2; absInt(x0-want) > 1 {
		t.Errorf("logo left = %d, want %d (±1)", x0, want)
	}
}
