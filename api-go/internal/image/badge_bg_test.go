package image

import (
	"testing"

	"openposterdb/internal/services"
)

// TestVerticalBadgeFullHeightBackground guards the vertical badge background
// fill spanning the full badge height. Before the fix only the label and value
// content rects were filled, leaving the padding/gap strips between and below
// the sections transparent (the artwork showed through the middle and bottom
// of the badge). The badge is rendered with distinct opaque section colors
// (label = accent red, value = green) so a probe row tells both "opaque" and
// "which section color" apart.
func TestVerticalBadgeFullHeightBackground(t *testing.T) {
	loadTestFont(t)
	badges := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
	}
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	cases := []struct {
		name      string
		style     services.BadgeStyle
		probeRows map[int]bool // y -> expect label (accent) color; false = value color
	}{
		{
			// tb: top pad [0,8], label [8,66] (text ink ~[22,48]),
			// gap [66,70], value [70,128] (text ink ~[81,111]),
			// bottom pad [128,136].
			name:  "tb",
			style: services.BadgeStyleLogoTB,
			probeRows: map[int]bool{
				4:   true,  // top padding -> label color
				60:  true,  // label section, below the label text -> label color
				120: false, // value section, below the value text -> value color
			},
		},
		{
			// bt: top pad [0,8], value [8,66] (text ink ~[19,50]),
			// gap [66,70], label [70,128] (text ink ~[91,110]),
			// bottom pad [128,136].
			name:  "bt",
			style: services.BadgeStyleValueTB,
			probeRows: map[int]bool{
				4:   false, // top padding -> value color
				12:  false, // value section, above the value text -> value color
				120: true,  // label section, below the label text -> label color
			},
		},
	}

	for _, tc := range cases {
		appearance := services.BadgeAppearance{
			Shape: services.BadgeShapeRounded,
			Alpha: services.BadgeAlpha(100),
			Style: tc.style,
		}
		img := RenderVerticalBadge(&badges[0], vf, lf, services.LabelStyleText, appearance, 1.0, 1.0, 1.0, colors)
		b := img.Bounds()
		centerX := b.Min.X + b.Dx()/2

		// Sanity: the badge is a full-height opaque block; the corner pixel
		// should NOT be opaque (rounded) but the centre of the top edge should.
		if c := img.RGBAAt(centerX, b.Min.Y); c.A == 0 {
			t.Errorf("%s: top edge centre is transparent", tc.name)
		}

		for y, wantLabel := range tc.probeRows {
			if y < b.Min.Y || y >= b.Max.Y {
				t.Fatalf("%s: probe y=%d outside badge bounds %v", tc.name, y, b)
			}
			c := img.RGBAAt(centerX, y)
			if c.A < 255 {
				t.Errorf("%s: y=%d alpha=%d, want opaque background in previously-gap strip", tc.name, y, c.A)
				continue
			}
			if wantLabel && !(c.R >= 200 && c.G <= 100) {
				t.Errorf("%s: y=%d got %+v, want label accent (red) background", tc.name, y, c)
			}
			if !wantLabel && !(c.G >= 200 && c.R <= 100) {
				t.Errorf("%s: y=%d got %+v, want value (green) background", tc.name, y, c)
			}
		}
	}
}

// TestMirroredHorizontalBadgeCorners guards the mirrored horizontal (rl) badge
// rounding: the value section sits on the LEFT and the label/logo on the RIGHT,
// so the outer left corners must be rounded on the value section and the outer
// right corners on the label section (the seam corners stay square).
func TestMirroredHorizontalBadgeCorners(t *testing.T) {
	loadTestFont(t)
	badges := []services.RatingBadge{
		{Source: services.SourceImdb, Value: "10.0"},
	}
	vf := GetValueFontFace()
	lf := GetFontFace()
	if vf == nil || lf == nil {
		t.Skip("fonts not loaded")
	}

	colors := map[string]services.SourceColorSet{
		"imdb": {Accent: "#ff0000", Value: "#00ff00"},
	}

	for _, style := range []struct {
		name string
		val  services.BadgeStyle
	}{
		{"lr", services.BadgeStyleLogoLeftValueRight},
		{"rl", services.BadgeStyleValueLeftLogoRight},
	} {
		appearance := services.BadgeAppearance{
			Shape: services.BadgeShapeRounded,
			Alpha: services.BadgeAlpha(100),
			Style: style.val,
		}
		img := RenderBadge(&badges[0], vf, lf, services.LabelStyleText, appearance, 1.0, 1.0, 1.0, colors)
		b := img.Bounds()

		// The badge background is opaque; a fully transparent corner pixel means
		// that corner is rounded (cut away). Corner inset to probe: beyond the
		// corner radius the fill is square, so probe 1px in from the corner.
		cornerTransparent := func(x, y int) bool {
			return img.RGBAAt(x, y).A == 0
		}
		left := b.Min.X + 1
		right := b.Max.X - 2
		top := b.Min.Y + 1

		switch style.name {
		case "lr":
			// Label (accent red) is on the left -> its outer (left) corners are
			// rounded; value section on the right owns the right corners.
			if !cornerTransparent(left, top) {
				t.Errorf("lr: top-left corner should be rounded (label outer corner)")
			}
			if !cornerTransparent(right, top) {
				t.Errorf("lr: top-right corner should be rounded (value outer corner)")
			}
		case "rl":
			// Value is on the left -> left corners rounded; label on the right.
			if !cornerTransparent(left, top) {
				t.Errorf("rl: top-left corner should be rounded (value outer corner)")
			}
			if !cornerTransparent(right, top) {
				t.Errorf("rl: top-right corner should be rounded (label outer corner)")
			}
		}
		// Seam pixel (centre of the badge) must be opaque, not a rounded notch.
		seamX := b.Min.X + b.Dx()/2
		if c := img.RGBAAt(seamX, top); c.A == 0 {
			t.Errorf("%s: seam between sections should be opaque, got transparent", style.name)
		}
	}
}
