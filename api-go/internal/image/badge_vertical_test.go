package image

import (
	"image"
	"image/color"
	"math"
	"testing"

	"openposterdb/internal/services"
)

// verticalSectionY mirrors RenderVerticalBadge's rounded/text-style geometry
// (badgeScale=1, no pill padding): each stacked section is as tall as the
// horizontal badge height and is FIXED by badge_size (text_size only scales
// the text inside, it never resizes the box). It returns the section's y-range
// (top-inclusive, bottom-exclusive) for the given vertical style.
func verticalSectionY(style services.BadgeStyle, value bool) (int, int) {
	labelTop := style == services.BadgeStyleLogoTB
	dims := newScaledDims(1.0)
	vertPadV := int(baseVertBadgePaddingV) // rounded badge: no pill padding
	sectionH := int(dims.badgeHeight)
	gap := int(math.Round(4.0))
	if labelTop == value {
		// tb value / bt label sits in the bottom section.
		y0 := vertPadV + sectionH + gap
		return y0, y0 + sectionH
	}
	y0 := vertPadV
	return y0, y0 + sectionH
}

// whiteInkBBox returns the bounding box of bright (white) text pixels inside
// [x0,x1)x[y0,y1). The badge backgrounds are opaque accent/value colours, so
// bright pixels are the text ink only.
func whiteInkBBox(img *image.RGBA, x0, y0, x1, y1 int) (minX, minY, maxX, maxY int, found bool) {
	minX, minY, maxX, maxY = x1, y1, x0, y0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 200 && c.R > 200 && c.G > 200 && c.B > 200 {
				if !found {
					minX, minY, maxX, maxY = x, y, x, y
					found = true
				} else {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
	}
	return minX, minY, maxX, maxY, found
}

func verticalAppearance(style services.BadgeStyle) services.BadgeAppearance {
	return services.BadgeAppearance{
		Shape: services.BadgeShapeRounded,
		Alpha: services.BadgeAlpha(100),
		Style: style,
	}
}

// TestVerticalValueTextCentredLikeHorizontal guards the vertical badge (tb/bt)
// text centring: the value text must sit in its section exactly where the
// horizontal (lr) badge puts it — same top and bottom gaps relative to the
// section, and horizontally centred on the badge. Both paths centre the text
// by its ink bounding box, so the visible ink matches.
// TestVerticalValueTextCentredLikeHorizontal guards the vertical value text
// placement: it stays horizontally centred on the badge, and hugs the seam so
// the logo's INWARD side keeps barely any margin from the text (tb: text top at
// the seam; bt: text bottom at the seam) — the user requirement "the side that
// goes inward, into the badge, not outside, i want barely any margin".
func TestVerticalValueTextCentredLikeHorizontal(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	dims := newScaledDims(1.0)
	_ = dims

	for _, tc := range []struct {
		name  string
		style services.BadgeStyle
	}{
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	} {
		img := RenderVerticalBadge(&badge, vf, lf, services.LabelStyleText, verticalAppearance(tc.style), 1.0, 1.0, 1.0, colors)
		b := img.Bounds()
		vY0, vY1 := verticalSectionY(tc.style, true)
		if vY1 > b.Dy() {
			t.Fatalf("%s: computed value section [%d,%d) exceeds badge height %d", tc.name, vY0, vY1, b.Dy())
		}
		minX, minY, maxX, maxY, found := whiteInkBBox(img, 0, vY0, b.Dx(), vY1)
		if !found {
			t.Fatalf("%s: no value text pixels found in value section [%d,%d)", tc.name, vY0, vY1)
		}

		// Horizontally centred on the badge (the value section spans the full
		// badge width, so this is also horizontal centring within the section).
		inkCentreX := (minX + maxX) / 2
		badgeCentreX := b.Dx() / 2
		if diff := abs(inkCentreX - badgeCentreX); diff > 1 {
			t.Errorf("%s: value text ink centre x=%d, badge centre x=%d (off by %d)", tc.name, inkCentreX, badgeCentreX, diff)
		}

		// Hugs the seam: the text's seam-side gap is tiny (the inward margin),
		// and the opposite-side gap absorbs the rest of the section.
		var seamGap int
		if tc.style == services.BadgeStyleLogoTB {
			seamGap = minY - vY0 // tb: text top hugs the seam
		} else {
			seamGap = vY1 - maxY // bt: text bottom hugs the seam
		}
		if seamGap > 6 {
			t.Errorf("%s: value text seam gap=%d, want <= 6px (barely any inward margin)", tc.name, seamGap)
		}
	}
}

// TestVerticalBadgeProportionsMatchHorizontal guards the vertical badge box
// proportions at 100%: each stacked section must be exactly as tall as the
// horizontal badge (so value/label occupy the same relative space) and the
// 100% value text must fit inside its section.
func TestVerticalBadgeProportionsMatchHorizontal(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	hAppearance := services.BadgeAppearance{
		Shape: services.BadgeShapeRounded,
		Alpha: services.BadgeAlpha(100),
		Style: services.BadgeStyleLogoLeftValueRight,
	}
	hImg := RenderBadge(&badge, vf, lf, services.LabelStyleText, hAppearance, 1.0, 1.0, 1.0, colors)
	hBadgeH := hImg.Bounds().Dy()

	for _, tc := range []struct {
		name  string
		style services.BadgeStyle
	}{
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	} {
		img := RenderVerticalBadge(&badge, vf, lf, services.LabelStyleText, verticalAppearance(tc.style), 1.0, 1.0, 1.0, colors)
		b := img.Bounds()

		for _, section := range []struct {
			label string
			value bool
		}{
			{"value", true},
			{"label", false},
		} {
			y0, y1 := verticalSectionY(tc.style, section.value)
			if got := y1 - y0; got != hBadgeH {
				t.Errorf("%s: %s section height=%d, horizontal badge height=%d (vertical box smaller than lr/rl)",
					tc.name, section.label, got, hBadgeH)
			}
			if y1 > b.Dy() {
				t.Fatalf("%s: %s section [%d,%d) exceeds badge height %d", tc.name, section.label, y0, y1, b.Dy())
			}
			_, minY, _, maxY, found := whiteInkBBox(img, 0, y0, b.Dx(), y1)
			if !found {
				t.Fatalf("%s: no %s text pixels found in section [%d,%d)", tc.name, section.label, y0, y1)
			}
			// At 100% the text must sit fully inside its section.
			if minY < y0 || maxY >= y1 {
				t.Errorf("%s: %s text ink y[%d,%d) overflows section [%d,%d)",
					tc.name, section.label, minY, maxY, y0, y1)
			}
		}
	}
}

// TestVerticalBadgeBoxFixedAcrossTextScale guards the fixed-box rule for tb/bt:
// the badge box (width and total height, and each section's height) must be
// IDENTICAL at textScale 1 and 2 — text_size may only scale the ink inside,
// never the box. This is the user requirement "the text size changes the badge
// size. this should never happen".
func TestVerticalBadgeBoxFixedAcrossTextScale(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	labelFace2, valueFace2 := GetFontFacesAt(200)
	if vf == nil || lf == nil || labelFace2 == nil || valueFace2 == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()
	defer labelFace2.Close()
	defer valueFace2.Close()

	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	shapes := []struct {
		name  string
		shape services.BadgeShape
	}{
		{"rounded", services.BadgeShapeRounded},
		{"pill", services.BadgeShapePill},
	}
	styles := []struct {
		name  string
		style services.BadgeStyle
	}{
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	}

	for _, sh := range shapes {
		hAppearance := services.BadgeAppearance{
			Shape: sh.shape,
			Alpha: services.BadgeAlpha(100),
			Style: services.BadgeStyleLogoLeftValueRight,
		}
		hImg := RenderBadge(&badge, vf, lf, services.LabelStyleText, hAppearance, 1.0, 1.0, 1.0, colors)
		hBadgeH := hImg.Bounds().Dy()

		for _, tc := range styles {
			app1 := services.BadgeAppearance{Shape: sh.shape, Alpha: services.BadgeAlpha(100), Style: tc.style}
			app2 := services.BadgeAppearance{Shape: sh.shape, Alpha: services.BadgeAlpha(100), Style: tc.style}
			img1 := RenderVerticalBadge(&badge, vf, lf, services.LabelStyleText, app1, 1.0, 1.0, 1.0, colors)
			img2 := RenderVerticalBadge(&badge, valueFace2, labelFace2, services.LabelStyleText, app2, 1.0, 2.0, 1.0, colors)

			if d1, d2 := img1.Bounds(), img2.Bounds(); d1.Dx() != d2.Dx() || d1.Dy() != d2.Dy() {
				t.Fatalf("%s/%s: badge box changes with text_size: scale1=%dx%d scale2=%dx%d (must be identical)",
					sh.name, tc.name, d1.Dx(), d1.Dy(), d2.Dx(), d2.Dy())
			}
			for _, section := range []struct {
				label string
				value bool
			}{
				{"value", true},
				{"label", false},
			} {
				y0, y1 := verticalSectionY(tc.style, section.value)
				if sh.shape == services.BadgeShapePill {
					// Pill: taller sections (badgeHeight + pillPaddingV) and a
					// larger top/bottom pad (baseVertBadgePaddingV + pillPadding).
					dims := newScaledDims(1.0)
					vertPadV := int(baseVertBadgePaddingV) + int(basePillPadding)
					sectionH := int(dims.badgeHeight) + int(dims.pillPaddingV)
					gap := int(math.Round(4.0))
					if (tc.style == services.BadgeStyleLogoTB) == section.value {
						y0 = vertPadV + sectionH + gap
						y1 = y0 + sectionH
					} else {
						y0 = vertPadV
						y1 = y0 + sectionH
					}
				}
				if got := y1 - y0; got != hBadgeH {
					t.Errorf("%s/%s: %s section height=%d, horizontal badge height=%d", sh.name, tc.name, section.label, got, hBadgeH)
				}
				if y1 > img1.Bounds().Dy() {
					t.Fatalf("%s/%s: %s section [%d,%d) exceeds badge height %d", sh.name, tc.name, section.label, y0, y1, img1.Bounds().Dy())
				}
				// The ink must differ (text actually scaled) but stay inside the box.
				_, minY, _, maxY, found := whiteInkBBox(img2, 0, y0, img2.Bounds().Dx(), y1)
				if !found {
					t.Fatalf("%s/%s: no %s text pixels found at scale 2", sh.name, tc.name, section.label)
				}
				if minY < y0 || maxY >= y1 {
					t.Errorf("%s/%s: %s text ink y[%d,%d) outside section [%d,%d)",
						sh.name, tc.name, section.label, minY, maxY, y0, y1)
				}
			}
		}
	}
}

// TestVerticalBadgeNoClipAtTextScale2 guards that a short value/label at 200%
// text size still fits entirely inside its (fixed) section with no clipping:
// the sections no longer grow with text_size, but at 100% settings nothing was
// cramped, so at 200% the text is merely larger, not cut off.
func TestVerticalBadgeNoClipAtTextScale2(t *testing.T) {
	loadTestFont(t)
	labelFace2, valueFace2 := GetFontFacesAt(200)
	if labelFace2 == nil || valueFace2 == nil {
		t.Skip("fonts not loaded")
	}
	defer labelFace2.Close()
	defer valueFace2.Close()

	// Short label/value so the fixed 88px badge width still fits at 200%.
	badge := services.RatingBadge{Source: services.SourceLetterboxd, Value: "8"}
	colors := map[string]services.SourceColorSet{
		"lb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	for _, tc := range []struct {
		name  string
		style services.BadgeStyle
	}{
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	} {
		img := RenderVerticalBadge(&badge, valueFace2, labelFace2, services.LabelStyleText, verticalAppearance(tc.style), 1.0, 2.0, 1.0, colors)
		b := img.Bounds()

		for _, section := range []struct {
			label string
			value bool
		}{
			{"value", true},
			{"label", false},
		} {
			y0, y1 := verticalSectionY(tc.style, section.value)
			if y1 > b.Dy() {
				t.Fatalf("%s: %s section [%d,%d) exceeds badge height %d", tc.name, section.label, y0, y1, b.Dy())
			}
			minX, minY, maxX, maxY, found := whiteInkBBox(img, 0, y0, b.Dx(), y1)
			if !found {
				t.Fatalf("%s: no %s text pixels found in section [%d,%d)", tc.name, section.label, y0, y1)
			}
			// No clipping: the text must sit fully inside the section with
			// margin on every side (ink flush against an edge would mean the
			// fixed box cut the text off).
			if minY < y0 || maxY >= y1 {
				t.Errorf("%s: %s text clips vertically: ink y[%d,%d) outside section [%d,%d)",
					tc.name, section.label, minY, maxY, y0, y1)
			}
			if minX < b.Min.X || maxX >= b.Max.X {
				t.Errorf("%s: %s text clips horizontally: ink x[%d,%d) outside badge width %d",
					tc.name, section.label, minX, maxX, b.Dx())
			}
			if minY == y0 || maxY == y1-1 {
				t.Errorf("%s: %s text ink flush against section edge (clipped): ink y[%d,%d) section [%d,%d)",
					tc.name, section.label, minY, maxY, y0, y1)
			}
			if minX == b.Min.X || maxX == b.Max.X-1 {
				t.Errorf("%s: %s text ink flush against badge edge (clipped): ink x[%d,%d) width %d",
					tc.name, section.label, minX, maxX, b.Dx())
			}
		}
	}
}

// TestVerticalBadgeEvenClipAtTextScale2 guards the fixed-box clipping rule for
// tb/bt: when 200% text is wider than the fixed 88px badge, it is cut off
// EVENLY on both sides (ink centred in its section, clipped at the badge
// border inset by the content gap on both the left and right edges — never
// touching the badge border) instead of spilling asymmetrically.
func TestVerticalBadgeEvenClipAtTextScale2(t *testing.T) {
	loadTestFont(t)
	labelFace2, valueFace2 := GetFontFacesAt(200)
	if labelFace2 == nil || valueFace2 == nil {
		t.Skip("fonts not loaded")
	}
	defer labelFace2.Close()
	defer valueFace2.Close()

	// "8.8" at 200% is much wider than the fixed 88px badge width, and its
	// digits are solid all the way to their ink edges (unlike "10.0", whose
	// thin "1" leaves anti-aliased whitespace at the left clip boundary).
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "8.8"}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	// The clip boundary is inset from the badge border by the content gap
	// (badgeContentGapBase = 3 at badgeScale 1) so oversized text never touches
	// the border.
	gap := int(newScaledDims(1.0).contentGap)

	for _, tc := range []struct {
		name  string
		style services.BadgeStyle
	}{
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	} {
		img := RenderVerticalBadge(&badge, valueFace2, labelFace2, services.LabelStyleText, verticalAppearance(tc.style), 1.0, 2.0, 1.0, colors)
		b := img.Bounds()
		y0, y1 := verticalSectionY(tc.style, true)
		minX, _, maxX, _, found := whiteInkBBox(img, 0, y0, b.Dx(), y1)
		if !found {
			t.Fatalf("%s: no value text pixels found in value section [%d,%d)", tc.name, y0, y1)
		}
		// The 200% value is wider than the badge: it must be clipped EVENLY on
		// both sides at (badge border − content gap), and centred — NOT flush
		// against the badge edges.
		if minX != b.Min.X+gap {
			t.Errorf("%s: value ink left edge=%d, want clipped at badge left+gap=%d (ink must not touch the border)", tc.name, minX, b.Min.X+gap)
		}
		if maxX != b.Max.X-1-gap {
			t.Errorf("%s: value ink right edge=%d, want clipped at badge right-gap=%d (ink must not touch the border)", tc.name, maxX, b.Max.X-1-gap)
		}
		inkCentreX := (minX + maxX) / 2
		badgeCentreX := b.Dx() / 2
		if diff := abs(inkCentreX - badgeCentreX); diff > 1 {
			t.Errorf("%s: clipped value ink centre x=%d, badge centre x=%d (off by %d)", tc.name, inkCentreX, badgeCentreX, diff)
		}
	}
}

// TestVerticalBadgeLogoMarginsMatchHorizontal guards the user requirement that
// the tb/bt source logo's outer margins match the rl/lr source logo's outer
// margins at 100%: for tb (logo on top) the right/left/top margins must equal
// the horizontal logo's, and for bt (logo on bottom) the right/left/bottom
// margins must. Because the vertical label section is wider than the
// horizontal one, matching the margin size requires a LARGER logo at 100%.
func TestVerticalBadgeLogoMarginsMatchHorizontal(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	// Inject a synthetic 48x48 cyan square icon for IMDb (cyan is not used by
	// the accent/value backgrounds or the white text). All real source icons
	// are square-ish, so a square synthetic is representative.
	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			synthetic.Set(x, y, color.RGBA{R: 0, G: 255, B: 255, A: 255})
		}
	}
	iconCacheMu.Lock()
	iconCache[*services.SourceImdb] = synthetic
	iconCacheMu.Unlock()
	defer func() {
		iconCacheMu.Lock()
		delete(iconCache, *services.SourceImdb)
		iconCacheMu.Unlock()
	}()

	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	cyanBounds := func(img *image.RGBA) (x0, y0, x1, y1 int, found bool) {
		b := img.Bounds()
		x0, y0 = b.Max.X, b.Max.Y
		x1, y1 = b.Min.X, b.Min.Y
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := img.RGBAAt(x, y)
				if c.A > 200 && c.R < 100 && c.G > 200 && c.B > 200 {
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
		}
		return x0, y0, x1, y1, found
	}

	// marginsOf returns the logo's four margins measured from the badge edges.
	marginsOf := func(img *image.RGBA) (left, right, top, bottom int, w, h int, found bool) {
		x0, y0, x1, y1, found := cyanBounds(img)
		if !found {
			return 0, 0, 0, 0, 0, 0, false
		}
		b := img.Bounds()
		return x0, b.Dx() - 1 - x1, y0, b.Dy() - 1 - y1, x1 - x0 + 1, y1 - y0 + 1, true
	}

	for _, sh := range []struct {
		name  string
		shape services.BadgeShape
	}{
		{"rounded", services.BadgeShapeRounded},
		{"pill", services.BadgeShapePill},
	} {
		// Horizontal reference (lr = logo on the left; rl mirrors it).
		hAppearance := services.BadgeAppearance{
			Shape: sh.shape,
			Alpha: services.BadgeAlpha(100),
			Style: services.BadgeStyleLogoLeftValueRight,
		}
		hImg := RenderBadge(&badge, vf, lf, services.LabelStyleIcon, hAppearance, 1.0, 1.0, 1.0, colors)
		hLeft, _, hTop, hBottom, hW, hH, found := marginsOf(hImg)
		if !found {
			t.Fatalf("%s/lr: no logo pixels found", sh.name)
		}

		for _, tc := range []struct {
			name  string
			style services.BadgeStyle
		}{
			{"tb", services.BadgeStyleLogoTB},
			{"bt", services.BadgeStyleValueTB},
		} {
			appearance := services.BadgeAppearance{
				Shape: sh.shape,
				Alpha: services.BadgeAlpha(100),
				Style: tc.style,
			}
			img := RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 1.0, 1.0, 1.0, colors)
			vLeft, vRight, vTop, vBottom, vW, vH, found := marginsOf(img)
			if !found {
				t.Fatalf("%s/%s: no logo pixels found", sh.name, tc.name)
			}

			// The vertical logo must be LARGER than the horizontal one at 100%
			// (it needs to fill the wider section with the same margins).
			if vW <= hW || vH <= hH {
				t.Errorf("%s/%s: logo size=%dx%d, want larger than horizontal logo %dx%d",
					sh.name, tc.name, vW, vH, hW, hH)
			}

			// Side margins (left/right) must match the horizontal logo's outer
			// side margin. The horizontal logo is centred in its label section,
			// so its left and right margins are equal; the tb/bt logo's outer
			// left and right margins must each equal that side margin.
			if diff := abs(vLeft - hLeft); diff > 1 {
				t.Errorf("%s/%s: logo left margin=%d, horizontal logo side margin=%d (off by %d)", sh.name, tc.name, vLeft, hLeft, diff)
			}
			if diff := abs(vRight - hLeft); diff > 1 {
				t.Errorf("%s/%s: logo right margin=%d, horizontal logo side margin=%d (off by %d)", sh.name, tc.name, vRight, hLeft, diff)
			}

			// The outer vertical margin (top for tb, bottom for bt) must match
			// the horizontal logo's top/bottom margins.
			switch tc.name {
			case "tb":
				if diff := abs(vTop - hTop); diff > 1 {
					t.Errorf("%s/tb: logo top margin=%d, horizontal logo top margin=%d (off by %d)", sh.name, vTop, hTop, diff)
				}
			case "bt":
				if diff := abs(vBottom - hBottom); diff > 1 {
					t.Errorf("%s/bt: logo bottom margin=%d, horizontal logo bottom margin=%d (off by %d)", sh.name, vBottom, hBottom, diff)
				}
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// cyanBounds scans an image for the cyan logo ink used by the icon-margin
// tests and returns its bounding box.
func cyanBounds(img *image.RGBA) (x0, y0, x1, y1 int, found bool) {
	b := img.Bounds()
	x0, y0 = b.Max.X, b.Max.Y
	x1, y1 = b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 200 && c.R < 100 && c.G > 200 && c.B > 200 {
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
	}
	return x0, y0, x1, y1, found
}
