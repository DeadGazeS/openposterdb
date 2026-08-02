package image

import (
	"image"
	"math"

	"openposterdb/internal/services"
)

// layoutBlock is a rendered grid of badges for one side.
type layoutBlock struct {
	img *image.RGBA
	w   int
	h   int
}

// renderSideBlock lays badge images into a Rows × PerRow grid (all horizontal
// rows) and returns the block image. Rows are read top-to-bottom, badges
// left-to-right within a row.
func renderSideBlock(badges []*image.RGBA, perRow, rows int, spacing, rowSpacing uint32) *image.RGBA {
	var grid [][]*image.RGBA
	idx := 0
	for r := 0; r < rows; r++ {
		var row []*image.RGBA
		for c := 0; c < perRow; c++ {
			if idx >= len(badges) {
				break
			}
			row = append(row, badges[idx])
			idx++
		}
		if len(row) > 0 {
			grid = append(grid, row)
		}
		if idx >= len(badges) {
			break
		}
	}

	var badgeH uint32
	for _, bi := range badges {
		if h := uint32(bi.Bounds().Dy()); h > badgeH {
			badgeH = h
		}
	}

	var maxRowW uint32
	for _, row := range grid {
		var rw uint32
		for _, bi := range row {
			rw += uint32(bi.Bounds().Dx())
		}
		rw += spacing * (uint32(len(row)) - 1)
		if rw > maxRowW {
			maxRowW = rw
		}
	}
	totalH := badgeH*uint32(len(grid)) + rowSpacing*(uint32(len(grid))-1)

	img := image.NewRGBA(image.Rect(0, 0, int(maxRowW), int(totalH)))
	y := 0
	for _, row := range grid {
		var rw uint32
		for _, bi := range row {
			rw += uint32(bi.Bounds().Dx())
		}
		rw += spacing * (uint32(len(row)) - 1)
		x := (int(maxRowW) - int(rw)) / 2
		for _, bi := range row {
			bh := uint32(bi.Bounds().Dy())
			by := y + (int(badgeH)-int(bh))/2
			overlay(img, bi, x, by)
			x += bi.Bounds().Dx() + int(spacing)
		}
		y += int(badgeH) + int(rowSpacing)
	}
	return img
}

// distributeLayout assigns badge images to sides according to the layout's fill
// order, each side consuming up to its capacity (per-row × rows) from the
// preference-ordered list.
func distributeLayout(badgeImages []*image.RGBA, layout *services.ImageLayout) map[string][]*image.RGBA {
	bySide := make(map[string][]*image.RGBA)
	remaining := badgeImages
	for _, side := range layout.OrderOrDefault() {
		slot := layout.SideSlot(side)
		if slot == nil {
			continue
		}
		cap := int(slot.Count())
		if cap <= 0 || len(remaining) == 0 {
			continue
		}
		n := cap
		if n > len(remaining) {
			n = len(remaining)
		}
		bySide[side] = remaining[:n]
		remaining = remaining[n:]
	}
	return bySide
}

// overlaySideBlock places a badge block on one side of a fixed canvas, hugging
// that edge and anchored per the start position.
func overlaySideBlock(canvas *image.RGBA, block *image.RGBA, side, start string, badgeScale float32, sideMarginBase uint32, extraX, extraY uint32) {
	cw := canvas.Bounds().Dx()
	ch := canvas.Bounds().Dy()
	bw := block.Bounds().Dx()
	bh := block.Bounds().Dy()
	sm := uint32(math.Round(float64(sideMarginBase) * float64(badgeScale)))

	var x, y int
	switch side {
	case "top":
		y = int(sm) + int(extraY)
		x = anchorX(start, cw, bw, int(sm)+int(extraX))
	case "bottom":
		y = ch - bh - int(sm) - int(extraY)
		x = anchorX(start, cw, bw, int(sm)+int(extraX))
	case "left":
		x = int(sm) + int(extraX)
		y = anchorY(start, ch, bh, int(sm)+int(extraY))
	case "right":
		x = cw - bw - int(sm) - int(extraX)
		y = anchorY(start, ch, bh, int(sm)+int(extraY))
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	overlay(canvas, block, x, y)
}

func anchorX(start string, cw, bw, margin int) int {
	switch start {
	case "l":
		return margin
	case "r":
		return cw - bw - margin
	default:
		return (cw - bw) / 2
	}
}

func anchorY(start string, ch, bh, margin int) int {
	switch start {
	case "t":
		return margin
	case "b":
		return ch - bh - margin
	default:
		return (ch - bh) / 2
	}
}
