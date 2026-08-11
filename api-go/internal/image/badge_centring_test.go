package image

import (
	"image"
	"image/color"
	"testing"

	"openposterdb/internal/services"

	"golang.org/x/image/font"
)

// horizontalValueSection returns the value section x-range [x0,x1) for a
// rendered horizontal badge, mirroring renderBadgeInner's layout math.
func horizontalValueSection(badge *services.RatingBadge, style services.BadgeStyle, labelFontFace, valueFontFace font.Face, labelStyle services.LabelStyle, dims scaledDims, textScale float32) (x0, x1 int) {
	maxLabelW := labelWidthForStyle(badge, labelStyle, labelFontFace, dims, textScale)
	maxValueW := baseTextWidth(badge.Value, valueFontFace, textScale)
	labelSectionW := 0
	if labelStyle.UsesIcon() {
		// Icon badges: the label section hugs the base logo (base width +
		// equal outer margin + tiny inward gap), mirroring renderBadgeInner.
		if icon := iconForBadge(badge, labelStyle); icon != nil {
			baseIconW, baseIconH2 := badgeIconAndSize(badge, labelStyle, dims.iconHeight, icon)
			iconPad := max(
				// pillPad = 0 for rounded
				(int(dims.badgeHeight)-int(baseIconH2))/2, 0)
			labelSectionW = int(baseIconW) + iconPad + badgeInwardGap
		}
	} else {
		labelSectionW = maxLabelW + 2*int(dims.textLabelPadH) // pillPad = 0 for rounded
	}
	valueSectionW := maxValueW + int(dims.badgeValuePad) + int(dims.badgeValuePad)/2 + 2
	if style.IsMirrored() {
		return 0, valueSectionW
	}
	return labelSectionW, labelSectionW + valueSectionW
}

// valueInkBBox probes the bright (white) value-text ink inside the value
// section [x0,x1) x the full badge height.
func valueInkBBox(img *image.RGBA, x0, x1 int) (minX, minY, maxX, maxY int, found bool) {
	minX, minY, maxX, maxY = x1, img.Bounds().Max.Y, x0, img.Bounds().Min.Y
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
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

// TestValueTextCentredInValueSection guards Task 1: the value text in the
// horizontal badge (lr and rl) must be visually centred in the value section —
// its ink bounding box centre within ~1px of the section centre on BOTH axes,
// at textScale 1 and 2. Before the fix the text was centred by advance width
// and a "-2" baseline hack, leaving the ink 1-2px off on the x axis and 1px
// low on the y axis.
func TestValueTextCentredInValueSection(t *testing.T) {
	loadTestFont(t)

	cases := []struct {
		name      string
		value     string
		textScale float32
		faces     int
	}{
		{"scale1", "10.0", 1.0, 100},
		{"scale2", "10.0", 2.0, 200},
	}
	styles := []struct {
		name  string
		style services.BadgeStyle
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight},
		{"rl", services.BadgeStyleValueLeftLogoRight},
	}

	for _, c := range cases {
		labelFace, valueFace := GetFontFacesAt(float64(c.faces))
		if labelFace == nil || valueFace == nil {
			t.Skip("fonts not loaded")
		}
		defer labelFace.Close()
		defer valueFace.Close()
		// At scale 2 use a short label ("LB") so the unclipped horizontal label
		// text stays inside the label section and cannot pollute the value-section
		// pixel probe.
		src := services.SourceImdb
		colorsKey := "imdb"
		if c.faces == 200 {
			src = services.SourceLetterboxd
			colorsKey = "lb"
		}
		colors := map[string]services.SourceColorSet{
			colorsKey: {Accent: "#ff0000", Value: "#00ff00"},
		}
		for _, st := range styles {
			badge := services.RatingBadge{Source: src, Value: c.value}
			appearance := services.BadgeAppearance{
				Shape: services.BadgeShapeRounded,
				Alpha: services.BadgeAlpha(100),
				Style: st.style,
			}
			img := RenderBadge(&badge, valueFace, labelFace, services.LabelStyleText, appearance, 1.0, c.textScale, 1.0, colors)
			b := img.Bounds()
			dims := newScaledDims(1.0)
			x0, x1 := horizontalValueSection(&badge, st.style, labelFace, valueFace, services.LabelStyleText, dims, c.textScale)
			if x1 > b.Dx() {
				t.Fatalf("%s/%s: value section [%d,%d) exceeds badge width %d", c.name, st.name, x0, x1, b.Dx())
			}
			minX, minY, maxX, maxY, found := valueInkBBox(img, x0, x1)
			if !found {
				t.Fatalf("%s/%s: no value text ink in section [%d,%d)", c.name, st.name, x0, x1)
			}

			inkCentreX := (minX + maxX) / 2
			sectionCentreX := (x0 + x1) / 2
			if diff := abs(inkCentreX - sectionCentreX); diff > 1 {
				t.Errorf("%s/%s: value ink centre x=%d, section centre x=%d (off by %d)",
					c.name, st.name, inkCentreX, sectionCentreX, diff)
			}
			inkCentreY := (minY + maxY) / 2
			badgeCentreY := b.Dy() / 2
			if diff := abs(inkCentreY - badgeCentreY); diff > 1 {
				t.Errorf("%s/%s: value ink centre y=%d, badge centre y=%d (off by %d)",
					c.name, st.name, inkCentreY, badgeCentreY, diff)
			}
		}
	}
}

// TestValueTextCentredAcrossValues sweeps several real rating values at 100%
// and asserts the value ink stays centred in the value section (x and y).
func TestValueTextCentredAcrossValues(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	values := []string{"10.0", "100%", "8.2", "77%", "4.0", "94%"}
	styles := []services.BadgeStyle{services.BadgeStyleLogoLeftValueRight, services.BadgeStyleValueLeftLogoRight}
	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	for _, value := range values {
		for _, style := range styles {
			badge := services.RatingBadge{Source: services.SourceImdb, Value: value}
			appearance := services.BadgeAppearance{
				Shape: services.BadgeShapeRounded,
				Alpha: services.BadgeAlpha(100),
				Style: style,
			}
			img := RenderBadge(&badge, vf, lf, services.LabelStyleText, appearance, 1.0, 1.0, 1.0, colors)
			b := img.Bounds()
			dims := newScaledDims(1.0)
			x0, x1 := horizontalValueSection(&badge, style, lf, vf, services.LabelStyleText, dims, 1.0)
			minX, minY, maxX, maxY, found := valueInkBBox(img, x0, x1)
			if !found {
				t.Fatalf("%q/%s: no value text ink", value, style)
			}
			inkCentreX := (minX + maxX) / 2
			sectionCentreX := (x0 + x1) / 2
			if diff := abs(inkCentreX - sectionCentreX); diff > 1 {
				t.Errorf("%q/%s: value ink centre x=%d, section centre x=%d (off by %d)",
					value, style, inkCentreX, sectionCentreX, diff)
			}
			inkCentreY := (minY + maxY) / 2
			badgeCentreY := b.Dy() / 2
			if diff := abs(inkCentreY - badgeCentreY); diff > 1 {
				t.Errorf("%q/%s: value ink centre y=%d, badge centre y=%d (off by %d)",
					value, style, inkCentreY, badgeCentreY, diff)
			}
		}
	}
}

// TestOversizedValueClippedToValueSection guards the even-clipping rule for
// oversized value text: at textScale 2 the value is wider than its section,
// so it must be cut off at the value-section edges — NOT spill into the label
// section. The label section of the oversized badge must be pixel-identical to
// a control badge rendered with an empty value (which draws no value ink).
func TestOversizedValueClippedToValueSection(t *testing.T) {
	loadTestFont(t)
	labelFace, valueFace := GetFontFacesAt(200)
	if labelFace == nil || valueFace == nil {
		t.Skip("fonts not loaded")
	}
	defer labelFace.Close()
	defer valueFace.Close()

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	appearance := services.BadgeAppearance{
		Shape: services.BadgeShapeRounded,
		Alpha: services.BadgeAlpha(100),
		Style: services.BadgeStyleLogoLeftValueRight,
	}

	full := RenderBadge(&services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}, valueFace, labelFace, services.LabelStyleText, appearance, 1.0, 2.0, 1.0, colors)
	control := RenderBadge(&services.RatingBadge{Source: services.SourceImdb, Value: ""}, valueFace, labelFace, services.LabelStyleText, appearance, 1.0, 2.0, 1.0, colors)

	// Both badges share the same label section width.
	dims := newScaledDims(1.0)
	labelSectionW := int(labelWidthForStyle(&services.RatingBadge{Source: services.SourceImdb}, services.LabelStyleText, labelFace, dims, 2.0)) + 2*int(dims.textLabelPadH)
	if labelSectionW > full.Bounds().Dx() || labelSectionW > control.Bounds().Dx() {
		t.Fatalf("label section %d exceeds badge widths %d/%d", labelSectionW, full.Bounds().Dx(), control.Bounds().Dx())
	}

	for y := 0; y < full.Bounds().Dy(); y++ {
		for x := range labelSectionW {
			a := full.RGBAAt(x, y)
			b := control.RGBAAt(x, y)
			if a != b {
				t.Errorf("oversized value ink spilled into label section at (%d,%d): got %+v want %+v", x, y, a, b)
			}
		}
	}

	// And the oversized value ink stays centred in the value section (even
	// clipping) — it must not be skewed to one side.
	valueSectionX := labelSectionW
	minX, _, maxX, _, found := valueInkBBox(full, valueSectionX, full.Bounds().Dx())
	if !found {
		t.Fatal("no value ink in value section")
	}
	inkCentreX := (minX + maxX) / 2
	sectionCentreX := (valueSectionX + full.Bounds().Dx()) / 2
	if diff := abs(inkCentreX - sectionCentreX); diff > 1 {
		t.Errorf("oversized value ink centre x=%d, value section centre x=%d (off by %d)", inkCentreX, sectionCentreX, diff)
	}
}

// TestValueTextCentredWithIconLabel guards the lr/rl value text centring when
// the label is a source icon (the default official/icon label styles): the
// value ink must stay centred in the value section on both axes at 100%,
// regardless of the label-section width (which is set by the icon). This
// covers the scenario reported as "the text is not centre anymore for lr rl".
func TestValueTextCentredWithIconLabel(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	// A synthetic 48x48 square icon matches every real source icon (white
	// icons are 48x48, official/highRes are fit in a 48x48 box).
	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	values := []string{"10.0", "100%", "8.2", "77%", "4.0"}
	styles := []services.BadgeStyle{services.BadgeStyleLogoLeftValueRight, services.BadgeStyleValueLeftLogoRight}

	for _, value := range values {
		for _, style := range styles {
			badge := services.RatingBadge{Source: services.SourceImdb, Value: value}
			appearance := services.BadgeAppearance{
				Shape: services.BadgeShapeRounded,
				Alpha: services.BadgeAlpha(100),
				Style: style,
			}
			img := RenderBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 1.0, 1.0, 1.0, colors)
			b := img.Bounds()
			dims := newScaledDims(1.0)
			x0, x1 := horizontalValueSection(&badge, style, lf, vf, services.LabelStyleIcon, dims, 1.0)
			minX, minY, maxX, maxY, found := valueInkBBox(img, x0, x1)
			if !found {
				t.Fatalf("%q/%s: no value text ink", value, style)
			}
			inkCentreX := (minX + maxX) / 2
			sectionCentreX := (x0 + x1) / 2
			if diff := abs(inkCentreX - sectionCentreX); diff > 1 {
				t.Errorf("%q/%s(icon): value ink centre x=%d, section centre x=%d (off by %d)",
					value, style, inkCentreX, sectionCentreX, diff)
			}
			inkCentreY := (minY + maxY) / 2
			badgeCentreY := b.Dy() / 2
			if diff := abs(inkCentreY - badgeCentreY); diff > 1 {
				t.Errorf("%q/%s(icon): value ink centre y=%d, badge centre y=%d (off by %d)",
					value, style, inkCentreY, badgeCentreY, diff)
			}
		}
	}
}

// TestUniformRowValueTextCentred guards the badge-preview path (RenderBadgesUniform
// lays lr/rl badges in a row sharing the widest label/value sections): each
// badge's value text must still be centred in its (shared) value section on
// both axes at 100%. This is the path the badge gallery / preview serves.
func TestUniformRowValueTextCentred(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	row := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
		{Source: services.SourceTmdb, Value: "77%"},
		{Source: services.SourceRt, Value: "100%"},
	}
	appearance := services.BadgeAppearance{
		Shape: services.BadgeShapeRounded,
		Alpha: services.BadgeAlpha(100),
		Style: services.BadgeStyleLogoLeftValueRight,
	}
	imgs := RenderBadgesUniform(row, vf, lf, services.LabelStyleText, appearance, 1.0, 1.0, 1.0, colors)

	// The uniform path sizes both sections by the row maxima.
	dims := newScaledDims(1.0)
	var maxLabelW, maxValueW int
	for i := range row {
		if w := labelWidthForStyle(&row[i], services.LabelStyleText, lf, dims, 1.0); w > maxLabelW {
			maxLabelW = w
		}
		if w := baseTextWidth(row[i].Value, vf, 1.0); w > maxValueW {
			maxValueW = w
		}
	}
	labelSectionW := maxLabelW + 2*int(dims.textLabelPadH)
	valueSectionW := maxValueW + int(dims.badgeValuePad) + int(dims.badgeValuePad)/2 + 2

	for i, img := range imgs {
		b := img.Bounds()
		x0, x1 := labelSectionW, labelSectionW+valueSectionW
		if x1 > b.Dx() {
			t.Fatalf("[%d] %s: value section [%d,%d) exceeds badge width %d", i, row[i].Source.Key, x0, x1, b.Dx())
		}
		minX, minY, maxX, maxY, found := valueInkBBox(img, x0, x1)
		if !found {
			t.Fatalf("[%d] %s: no value text ink", i, row[i].Source.Key)
		}
		inkCentreX := (minX + maxX) / 2
		sectionCentreX := (x0 + x1) / 2
		if diff := abs(inkCentreX - sectionCentreX); diff > 1 {
			t.Errorf("[%d] %s: value ink centre x=%d, section centre x=%d (off by %d)",
				i, row[i].Source.Key, inkCentreX, sectionCentreX, diff)
		}
		inkCentreY := (minY + maxY) / 2
		badgeCentreY := b.Dy() / 2
		if diff := abs(inkCentreY - badgeCentreY); diff > 1 {
			t.Errorf("[%d] %s: value ink centre y=%d, badge centre y=%d (off by %d)",
				i, row[i].Source.Key, inkCentreY, badgeCentreY, diff)
		}
	}
}

// TestContentNeverTouchesBadgeBorder guards the user requirement that neither
// the value text nor the source logo ever touches the badge's outer border:
// the ink must stay at least `contentGap` (badgeContentGapBase × badgeScale)
// away from every badge border. At 100% the section padding already provides
// that gap; at textScale/logoScale 2 oversized content is clipped at
// (border − gap) instead of AT the border. Fail-before: at scale 2 the value
// text clipped flush at the badge right edge (0px gap) and the logo touched
// the top/left/bottom borders.
func TestContentNeverTouchesBadgeBorder(t *testing.T) {
	loadTestFont(t)

	// A synthetic 48x48 square icon matches every real source icon.
	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}

	// inkBBox returns the bounding box of pixels matching `match` and the
	// distances from that box to the four badge borders (L, T, R, B).
	inkBBox := func(img *image.RGBA, match func(color.RGBA) bool) (x0, y0, x1, y1 int, gaps [4]int, found bool) {
		b := img.Bounds()
		x0, y0, x1, y1 = b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if match(img.RGBAAt(x, y)) {
					if !found {
						x0, y0, x1, y1 = x, y, x, y
						found = true
					} else {
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
		}
		if !found {
			return 0, 0, 0, 0, [4]int{}, false
		}
		return x0, y0, x1, y1,
			[4]int{x0 - b.Min.X, y0 - b.Min.Y, b.Max.X - 1 - x1, b.Max.Y - 1 - y1}, true
	}
	isTextInk := func(c color.RGBA) bool { return c.A > 200 && c.R > 200 && c.G > 200 && c.B > 200 }
	isLogoInk := func(c color.RGBA) bool { return c.A > 200 && c.R < 100 && c.G > 200 && c.B > 200 }

	gap := int(newScaledDims(1.0).contentGap)
	cases := []struct {
		name  string
		style services.BadgeStyle
		shape services.BadgeShape
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight, services.BadgeShapeRounded},
		{"lr-pill", services.BadgeStyleLogoLeftValueRight, services.BadgeShapePill},
		{"tb", services.BadgeStyleLogoTB, services.BadgeShapeRounded},
		{"tb-pill", services.BadgeStyleLogoTB, services.BadgeShapePill},
	}
	scales := []struct {
		name      string
		textScale float32
		logoScale float32
		facePct   int
	}{
		{"s1", 1.0, 1.0, 100},
		{"s2", 2.0, 2.0, 200},
	}

	for _, tc := range cases {
		for _, sc := range scales {
			labelFace, valueFace := GetFontFacesAt(float64(sc.facePct))
			if labelFace == nil || valueFace == nil {
				t.Skip("fonts not loaded")
			}
			appearance := services.BadgeAppearance{
				Shape: tc.shape,
				Alpha: services.BadgeAlpha(100),
				Style: tc.style,
			}
			var img *image.RGBA
			if tc.style.IsVertical() {
				img = RenderVerticalBadge(&badge, valueFace, labelFace, services.LabelStyleIcon, appearance, 1.0, sc.textScale, sc.logoScale, colors)
			} else {
				img = RenderBadge(&badge, valueFace, labelFace, services.LabelStyleIcon, appearance, 1.0, sc.textScale, sc.logoScale, colors)
			}
			labelFace.Close()
			valueFace.Close()

			_, _, _, _, tg, found := inkBBox(img, isTextInk)
			if !found {
				t.Fatalf("%s/%s: no value text ink found", tc.name, sc.name)
			}
			for i, side := range []string{"left", "top", "right", "bottom"} {
				if tg[i] < gap {
					t.Errorf("%s/%s: value text ink only %dpx from %s border (want >= %d)", tc.name, sc.name, tg[i], side, gap)
				}
			}
			_, _, _, _, lg, found := inkBBox(img, isLogoInk)
			if !found {
				t.Fatalf("%s/%s: no logo ink found", tc.name, sc.name)
			}
			for i, side := range []string{"left", "top", "right", "bottom"} {
				if lg[i] < gap {
					t.Errorf("%s/%s: logo ink only %dpx from %s border (want >= %d)", tc.name, sc.name, lg[i], side, gap)
				}
			}
		}
	}
}

// TestLogoDecoupledFromBadgeScale guards the size-independence requirement:
// the source logo must scale ONLY with logo_size, never with badge_size.
// Rendering the same icon badge at badgeScale 1 vs 2 (logoScale=1) must yield
// an identical logo ink size, while the badge box itself grows.
func TestLogoDecoupledFromBadgeScale(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}

	logoSize := func(img *image.RGBA) (w, h int) {
		b := img.Bounds()
		minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		found := false
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := img.RGBAAt(x, y)
				if c.A > 200 && c.B > 200 && c.R < 100 && c.G > 200 { // cyan synthetic icon
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
		if !found {
			return 0, 0
		}
		return maxX - minX + 1, maxY - minY + 1
	}

	appearance := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: services.BadgeStyleLogoLeftValueRight}
	img1 := RenderBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 1.0, 1.0, 1.0, colors)
	img2 := RenderBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 2.0, 1.0, 1.0, colors)

	w1, h1 := logoSize(img1)
	w2, h2 := logoSize(img2)
	if w1 == 0 || h1 == 0 {
		t.Fatal("no logo ink found at badgeScale 1")
	}
	if w1 != w2 || h1 != h2 {
		t.Errorf("logo size changed with badgeScale: scale1 %dx%d vs scale2 %dx%d (must be identical)", w1, h1, w2, h2)
	}
	if img2.Bounds().Dx() <= img1.Bounds().Dx() || img2.Bounds().Dy() <= img1.Bounds().Dy() {
		t.Errorf("badge box should grow with badgeScale: %v -> %v", img1.Bounds(), img2.Bounds())
	}
}

// TestBadgeBoxScalesPerAxis guards per-kind badge width/height: badge_width
// scales the box width only, badge_height the box height only, and the logo
// size stays unchanged.
func TestBadgeBoxScalesPerAxis(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	base := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: services.BadgeStyleLogoLeftValueRight, Width: 100, Height: 100}
	wide := base
	wide.Width = 200
	tall := base
	tall.Height = 200

	imgBase := RenderBadge(&badge, vf, lf, services.LabelStyleText, base, 1.0, 1.0, 1.0, colors)
	imgWide := RenderBadge(&badge, vf, lf, services.LabelStyleText, wide, 1.0, 1.0, 1.0, colors)
	imgTall := RenderBadge(&badge, vf, lf, services.LabelStyleText, tall, 1.0, 1.0, 1.0, colors)

	bw, bh := imgBase.Bounds().Dx(), imgBase.Bounds().Dy()
	ww, wh := imgWide.Bounds().Dx(), imgWide.Bounds().Dy()
	tw, th := imgTall.Bounds().Dx(), imgTall.Bounds().Dy()

	// With text labels the label/logo block stays fixed at its 100% size and
	// only the value section widens. Compute the fixed label and base value
	// widths from the same layout math used by renderBadgeInner.
	dims := newScaledDims(1.0)
	labelSectionW := int(labelWidthForStyle(&badge, services.LabelStyleText, lf, dims, 1.0)) + 2*int(dims.textLabelPadH)
	valueSectionW := baseTextWidth(badge.Value, vf, 1.0) + int(dims.badgeValuePad) + int(dims.badgeValuePad)/2 + 2

	if expectedWideW := labelSectionW + 2*valueSectionW; ww != expectedWideW {
		t.Errorf("badge_width=200: width %d, want %d (label %d + 2×value %d)", ww, expectedWideW, labelSectionW, valueSectionW)
	}
	if wh != bh {
		t.Errorf("badge_width=200 changed height: %d, want %d", wh, bh)
	}
	if th != bh*2 {
		t.Errorf("badge_height=200: height %d, want %d (2x base %d)", th, bh*2, bh)
	}
	if tw != bw {
		t.Errorf("badge_height=200 changed width: %d, want %d", tw, bw)
	}
}

// TestLogoUniformOuterMarginsAndInwardMinimal guards the user requirement:
// at 100% the source logo's three OUTER margins (facing the badge frame) are
// EQUAL for every style (lr/rl/tb/bt, rounded + pill), and the INWARD side
// (facing the value text) keeps barely any margin (~2px) so the text box owns
// the remaining space and centres the text.
func TestLogoUniformOuterMarginsAndInwardMinimal(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	// A synthetic 48x48 square icon matches every real default source icon.
	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	styles := []struct {
		name  string
		style services.BadgeStyle
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight},
		{"rl", services.BadgeStyleValueLeftLogoRight},
		{"tb", services.BadgeStyleLogoTB},
		{"bt", services.BadgeStyleValueTB},
	}
	shapes := []struct {
		name  string
		shape services.BadgeShape
	}{
		{"rounded", services.BadgeShapeRounded},
		{"pill", services.BadgeShapePill},
	}

	for _, sh := range shapes {
		for _, tc := range styles {
			appearance := services.BadgeAppearance{Shape: sh.shape, Alpha: services.BadgeAlpha(100), Style: tc.style}
			var img *image.RGBA
			if tc.style.IsVertical() {
				img = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 1.0, 1.0, 1.0, colors)
			} else {
				img = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, appearance, 1.0, 1.0, 1.0, colors)
			}
			b := img.Bounds()
			lx0, ly0, lx1, ly1, found := cyanBounds(img)
			if !found {
				t.Fatalf("%s/%s: no logo ink found", sh.name, tc.name)
			}
			left, right := lx0-b.Min.X, b.Max.X-1-lx1
			top, bottom := ly0-b.Min.Y, b.Max.Y-1-ly1

			// Locate the value section (green background band) so the logo's
			// inward gap to the text box can be measured directly (for lr/rl
			// the band is vertical, for tb/bt horizontal).
			gMinX, gMinY := b.Max.X, b.Max.Y
			gMaxX, gMaxY := b.Min.X, b.Min.Y
			gFound := false
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					c := img.RGBAAt(x, y)
					if c.A > 200 && c.R < 100 && c.G > 200 && c.B < 100 {
						if x < gMinX {
							gMinX = x
						}
						if x > gMaxX {
							gMaxX = x
						}
						if y < gMinY {
							gMinY = y
						}
						if y > gMaxY {
							gMaxY = y
						}
						gFound = true
					}
				}
			}
			if !gFound {
				t.Fatalf("%s/%s: no value-section background found", sh.name, tc.name)
			}

			var outer [3]int
			var inward int
			switch tc.style {
			case services.BadgeStyleLogoLeftValueRight: // lr: outer L/T/B, inward R
				outer = [3]int{left, top, bottom}
				inward = gMinX - lx1 - 1
			case services.BadgeStyleValueLeftLogoRight: // rl: outer R/T/B, inward L
				outer = [3]int{right, top, bottom}
				inward = lx0 - gMaxX - 1
			case services.BadgeStyleLogoTB: // tb: outer L/T/R, inward B
				outer = [3]int{left, top, right}
				inward = gMinY - ly1 - 1
			case services.BadgeStyleValueTB: // bt: outer L/B/R, inward T
				outer = [3]int{left, bottom, right}
				inward = ly0 - gMaxY - 1
			}
			for i := 1; i < len(outer); i++ {
				if diff := abs(outer[i] - outer[0]); diff > 1 {
					t.Errorf("%s/%s: logo outer margins %v not equal (base %d)", sh.name, tc.name, outer, outer[0])
				}
			}
			if inward > 5 {
				t.Errorf("%s/%s: logo inward margin=%d, want <= 5px (barely any margin)", sh.name, tc.name, inward)
			}
			// The pill cap padding on the value section's outer end must be
			// filled (not transparent), so artwork never shows through the
			// pill tip.
			if sh.shape == services.BadgeShapePill && (tc.style == services.BadgeStyleLogoLeftValueRight || tc.style == services.BadgeStyleValueLeftLogoRight) {
				probeX := b.Max.X - 2
				if tc.style == services.BadgeStyleValueLeftLogoRight {
					probeX = b.Min.X + 2
				}
				probeY := b.Min.Y + b.Dy()/2
				if c := img.RGBAAt(probeX, probeY); c.A == 0 {
					t.Errorf("%s/%s: pill tip at (%d,%d) is transparent (alpha 0)", sh.name, tc.name, probeX, probeY)
				}
			}
			t.Logf("%s/%s: outer margins L=%d R=%d T=%d B=%d, inward=%d", sh.name, tc.name, left, right, top, bottom, inward)
		}
	}
}

// TestLogoAnchorMarginConstantAcrossBadgeSize guards the logo-anchor rule: the
// unique outer-side margin (lr→left, rl→right, tb→top, bt→bottom) stays
// constant when the badge grows on the axis that is supposed to feed the number
// section. The non-anchor outer sides may change, but the anchor side must not.
func TestLogoAnchorMarginConstantAcrossBadgeSize(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	// A synthetic square icon makes the margins predictable.
	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}

	cases := []struct {
		name    string
		style   services.BadgeStyle
		axisPct int // which appearance axis to double: 100=Width, 101=Height
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight, 100},
		{"rl", services.BadgeStyleValueLeftLogoRight, 100},
		{"tb", services.BadgeStyleLogoTB, 101},
		{"bt", services.BadgeStyleValueTB, 101},
	}
	shapes := []struct {
		name  string
		shape services.BadgeShape
	}{
		{"rounded", services.BadgeShapeRounded},
		{"pill", services.BadgeShapePill},
	}

	for _, sh := range shapes {
		for _, tc := range cases {
			baseAppearance := services.BadgeAppearance{Shape: sh.shape, Alpha: services.BadgeAlpha(100), Style: tc.style, Width: 100, Height: 100}
			bigAppearance := baseAppearance
			if tc.axisPct == 100 {
				bigAppearance.Width = 200
			} else {
				bigAppearance.Height = 200
			}

			var img1, img2 *image.RGBA
			if tc.style.IsVertical() {
				img1 = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, baseAppearance, 1.0, 1.0, 1.0, colors)
				img2 = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, bigAppearance, 1.0, 1.0, 1.0, colors)
			} else {
				img1 = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, baseAppearance, 1.0, 1.0, 1.0, colors)
				img2 = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, bigAppearance, 1.0, 1.0, 1.0, colors)
			}

			x0a, y0a, x1a, y1a, ok1 := cyanBounds(img1)
			x0b, y0b, x1b, y1b, ok2 := cyanBounds(img2)
			if !ok1 || !ok2 {
				t.Fatalf("%s/%s: logo not found (%v %v)", sh.name, tc.name, ok1, ok2)
			}

			ba, bb := img1.Bounds(), img2.Bounds()
			var marginA, marginB int
			switch tc.style {
			case services.BadgeStyleLogoLeftValueRight:
				marginA, marginB = x0a-ba.Min.X, x0b-bb.Min.X
			case services.BadgeStyleValueLeftLogoRight:
				marginA, marginB = ba.Max.X-1-x1a, bb.Max.X-1-x1b
			case services.BadgeStyleLogoTB:
				marginA, marginB = y0a-ba.Min.Y, y0b-bb.Min.Y
			case services.BadgeStyleValueTB:
				marginA, marginB = ba.Max.Y-1-y1a, bb.Max.Y-1-y1b
			}
			if diff := abs(marginA - marginB); diff > 1 {
				t.Errorf("%s/%s: anchor margin changed from %d to %d when scaling %s",
					sh.name, tc.name, marginA, marginB, map[int]string{100: "width", 101: "height"}[tc.axisPct])
			}
		}
	}
}

// TestSliderOrthogonality guards the user rule "every size slider only changes
// its own size": badge_width only changes badge width, badge_height only
// changes badge height, text_size only changes text ink, logo_size only
// changes logo ink. All other measured dimensions must stay identical.
func TestSliderOrthogonality(t *testing.T) {
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

	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}

	logoSize := func(img *image.RGBA) (w, h int) {
		x0, y0, x1, y1, ok := cyanBounds(img)
		if !ok {
			return 0, 0
		}
		return x1 - x0 + 1, y1 - y0 + 1
	}
	textSize := func(img *image.RGBA) (w, h int) {
		b := img.Bounds()
		x0, y0, x1, y1, ok := whiteInkBBox(img, b.Min.X, b.Min.Y, b.Max.X, b.Max.Y)
		if !ok {
			return 0, 0
		}
		return x1 - x0 + 1, y1 - y0 + 1
	}

	cases := []struct {
		name         string
		style        services.BadgeStyle
		vertical     bool
		render       func(appearance services.BadgeAppearance, textScale, logoScale float32, labelFace, valueFace font.Face) *image.RGBA
		dimension    string // "width" or "height"
		expectedAxis int    // 0=none, 1=width, 2=height
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight, false, func(app services.BadgeAppearance, ts, ls float32, lff, vff font.Face) *image.RGBA {
			return RenderBadge(&badge, vff, lff, services.LabelStyleIcon, app, 1.0, ts, ls, colors)
		}, "width", 1},
		{"lr-height", services.BadgeStyleLogoLeftValueRight, false, func(app services.BadgeAppearance, ts, ls float32, lff, vff font.Face) *image.RGBA {
			return RenderBadge(&badge, vff, lff, services.LabelStyleIcon, app, 1.0, ts, ls, colors)
		}, "height", 2},
		{"tb", services.BadgeStyleLogoTB, true, func(app services.BadgeAppearance, ts, ls float32, lff, vff font.Face) *image.RGBA {
			return RenderVerticalBadge(&badge, vff, lff, services.LabelStyleIcon, app, 1.0, ts, ls, colors)
		}, "height", 2},
		{"tb-width", services.BadgeStyleLogoTB, true, func(app services.BadgeAppearance, ts, ls float32, lff, vff font.Face) *image.RGBA {
			return RenderVerticalBadge(&badge, vff, lff, services.LabelStyleIcon, app, 1.0, ts, ls, colors)
		}, "width", 1},
	}

	for _, tc := range cases {
		base := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: tc.style, Width: 100, Height: 100}
		var changed services.BadgeAppearance
		switch tc.dimension {
		case "width":
			changed = base
			changed.Width = 200
		case "height":
			changed = base
			changed.Height = 200
		}

		img1 := tc.render(base, 1.0, 1.0, lf, vf)
		img2 := tc.render(changed, 1.0, 1.0, lf, vf)

		b1, b2 := img1.Bounds(), img2.Bounds()
		widthChanged := b1.Dx() != b2.Dx()
		heightChanged := b1.Dy() != b2.Dy()

		switch tc.expectedAxis {
		case 1:
			if !widthChanged {
				t.Errorf("%s: expected width to change, got %dx%d vs %dx%d", tc.name, b1.Dx(), b1.Dy(), b2.Dx(), b2.Dy())
			}
			if heightChanged {
				t.Errorf("%s: height changed with %s slider", tc.name, tc.dimension)
			}
		case 2:
			if !heightChanged {
				t.Errorf("%s: expected height to change, got %dx%d vs %dx%d", tc.name, b1.Dx(), b1.Dy(), b2.Dx(), b2.Dy())
			}
			if widthChanged {
				t.Errorf("%s: width changed with %s slider", tc.name, tc.dimension)
			}
		}

		lw1, lh1 := logoSize(img1)
		lw2, lh2 := logoSize(img2)
		if lw1 != lw2 || lh1 != lh2 {
			t.Errorf("%s: logo size changed with %s slider: %dx%d -> %dx%d",
				tc.name, tc.dimension, lw1, lh1, lw2, lh2)
		}
		tw1, th1 := textSize(img1)
		tw2, th2 := textSize(img2)
		if tw1 != tw2 || th1 != th2 {
			t.Errorf("%s: text size changed with %s slider: %dx%d -> %dx%d",
				tc.name, tc.dimension, tw1, th1, tw2, th2)
		}
	}

	// text_size slider: only text ink changes.
	for _, tc := range []struct {
		name     string
		style    services.BadgeStyle
		vertical bool
	}{
		{"lr-text", services.BadgeStyleLogoLeftValueRight, false},
		{"tb-text", services.BadgeStyleLogoTB, true},
	} {
		app := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: tc.style, Width: 100, Height: 100}
		var img1, img2 *image.RGBA
		if tc.vertical {
			img1 = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 1.0, colors)
			img2 = RenderVerticalBadge(&badge, valueFace2, labelFace2, services.LabelStyleIcon, app, 1.0, 2.0, 1.0, colors)
		} else {
			img1 = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 1.0, colors)
			img2 = RenderBadge(&badge, valueFace2, labelFace2, services.LabelStyleIcon, app, 1.0, 2.0, 1.0, colors)
		}
		b1, b2 := img1.Bounds(), img2.Bounds()
		if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() {
			t.Errorf("%s: badge box changed with text_size: %dx%d -> %dx%d", tc.name, b1.Dx(), b1.Dy(), b2.Dx(), b2.Dy())
		}
		lw1, lh1 := logoSize(img1)
		lw2, lh2 := logoSize(img2)
		if lw1 != lw2 || lh1 != lh2 {
			t.Errorf("%s: logo size changed with text_size: %dx%d -> %dx%d", tc.name, lw1, lh1, lw2, lh2)
		}
		tw1, th1 := textSize(img1)
		tw2, th2 := textSize(img2)
		if tw1 == tw2 || th1 == th2 {
			t.Errorf("%s: text size did not increase with text_size: %dx%d -> %dx%d", tc.name, tw1, th1, tw2, th2)
		}
	}

	// logo_size slider: only logo ink changes.
	for _, tc := range []struct {
		name     string
		style    services.BadgeStyle
		vertical bool
	}{
		{"lr-logo", services.BadgeStyleLogoLeftValueRight, false},
		{"tb-logo", services.BadgeStyleLogoTB, true},
	} {
		app := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: tc.style, Width: 100, Height: 100}
		var img1, img2 *image.RGBA
		if tc.vertical {
			img1 = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 1.0, colors)
			img2 = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 2.0, colors)
		} else {
			img1 = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 1.0, colors)
			img2 = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 2.0, colors)
		}
		b1, b2 := img1.Bounds(), img2.Bounds()
		if b1.Dx() != b2.Dx() || b1.Dy() != b2.Dy() {
			t.Errorf("%s: badge box changed with logo_size: %dx%d -> %dx%d", tc.name, b1.Dx(), b1.Dy(), b2.Dx(), b2.Dy())
		}
		tw1, th1 := textSize(img1)
		tw2, th2 := textSize(img2)
		if tw1 != tw2 || th1 != th2 {
			t.Errorf("%s: text size changed with logo_size: %dx%d -> %dx%d", tc.name, tw1, th1, tw2, th2)
		}
		lw1, lh1 := logoSize(img1)
		lw2, lh2 := logoSize(img2)
		if lw1 == lw2 || lh1 == lh2 {
			t.Errorf("%s: logo size did not increase with logo_size: %dx%d -> %dx%d", tc.name, lw1, lh1, lw2, lh2)
		}
	}
}

// TestOversizedLogoClippingPreservesAnchor guards that when the logo outgrows
// its section, the unique outer-side margin is preserved (not clipped to the
// content gap), the non-anchor outer sides clip at the content gap, and the
// inward side clips at the section seam.
func TestOversizedLogoClippingPreservesAnchor(t *testing.T) {
	loadTestFont(t)
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}
	defer vf.Close()
	defer lf.Close()

	synthetic := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := range 48 {
		for x := range 48 {
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

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}
	badge := services.RatingBadge{Source: services.SourceImdb, Value: "10.0"}
	gap := int(newScaledDims(1.0).contentGap)

	cases := []struct {
		name         string
		style        services.BadgeStyle
		vertical     bool
		anchorMargin int // expected unique-side margin (iconPad for lr/rl, anchorV for tb/bt)
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight, false, (baseBadgeHeight - baseIconHeight) / 2},
		{"rl", services.BadgeStyleValueLeftLogoRight, false, (baseBadgeHeight - baseIconHeight) / 2},
		{"tb", services.BadgeStyleLogoTB, true, (baseBadgeHeight - baseIconHeight) / 2},
		{"bt", services.BadgeStyleValueTB, true, (baseBadgeHeight - baseIconHeight) / 2},
	}

	for _, tc := range cases {
		app := services.BadgeAppearance{Shape: services.BadgeShapeRounded, Alpha: services.BadgeAlpha(100), Style: tc.style, Width: 100, Height: 100}
		var img *image.RGBA
		if tc.vertical {
			img = RenderVerticalBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 3.0, colors)
		} else {
			img = RenderBadge(&badge, vf, lf, services.LabelStyleIcon, app, 1.0, 1.0, 3.0, colors)
		}
		b := img.Bounds()
		x0, y0, x1, y1, ok := cyanBounds(img)
		if !ok {
			t.Fatalf("%s: no logo ink found", tc.name)
		}
		left, right := x0-b.Min.X, b.Max.X-1-x1
		top, bottom := y0-b.Min.Y, b.Max.Y-1-y1

		switch tc.style {
		case services.BadgeStyleLogoLeftValueRight:
			if diff := abs(left - tc.anchorMargin); diff > 1 {
				t.Errorf("%s: anchor (left) margin=%d, want %d", tc.name, left, tc.anchorMargin)
			}
			if top != gap || bottom != gap {
				t.Errorf("%s: top/bottom margins should clip at content gap %d, got %d/%d", tc.name, gap, top, bottom)
			}
		case services.BadgeStyleValueLeftLogoRight:
			if diff := abs(right - tc.anchorMargin); diff > 1 {
				t.Errorf("%s: anchor (right) margin=%d, want %d", tc.name, right, tc.anchorMargin)
			}
			if top != gap || bottom != gap {
				t.Errorf("%s: top/bottom margins should clip at content gap %d, got %d/%d", tc.name, gap, top, bottom)
			}
		case services.BadgeStyleLogoTB:
			if diff := abs(top - tc.anchorMargin); diff > 1 {
				t.Errorf("%s: anchor (top) margin=%d, want %d", tc.name, top, tc.anchorMargin)
			}
			if left != gap || right != gap {
				t.Errorf("%s: left/right margins should clip at content gap %d, got %d/%d", tc.name, gap, left, right)
			}
		case services.BadgeStyleValueTB:
			if diff := abs(bottom - tc.anchorMargin); diff > 1 {
				t.Errorf("%s: anchor (bottom) margin=%d, want %d", tc.name, bottom, tc.anchorMargin)
			}
			if left != gap || right != gap {
				t.Errorf("%s: left/right margins should clip at content gap %d, got %d/%d", tc.name, gap, left, right)
			}
		}
	}
}
