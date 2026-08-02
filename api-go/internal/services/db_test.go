package services

import (
	"testing"
)

func TestBadgeStyleParse(t *testing.T) {
	if ParseBadgeStyle("h") != BadgeStyleHorizontal {
		t.Error("h should be Horizontal")
	}
	if ParseBadgeStyle("v") != BadgeStyleVertical {
		t.Error("v should be Vertical")
	}
	if ParseBadgeStyle("d") != BadgeStyleDefault {
		t.Error("d should be Default")
	}
	if ParseBadgeStyle("invalid") != BadgeStyleDefault {
		t.Error("invalid should default to Default")
	}
}

func TestBadgeStyleIsVertical(t *testing.T) {
	if BadgeStyleHorizontal.IsVertical() {
		t.Error("Horizontal.IsVertical should be false")
	}
	if !BadgeStyleVertical.IsVertical() {
		t.Error("Vertical.IsVertical should be true")
	}
}

func TestBadgeStyleResolve(t *testing.T) {
	def := BadgeStyleDefault
	if s := def.Resolve(BadgeDirectionHorizontal); s != BadgeStyleHorizontal {
		t.Error("Default+Horizontal → Horizontal")
	}
	if s := def.Resolve(BadgeDirectionVertical); s != BadgeStyleVertical {
		t.Error("Default+Vertical → Vertical")
	}
	if s := BadgeStyleVertical.Resolve(BadgeDirectionHorizontal); s != BadgeStyleVertical {
		t.Error("explicit Vertical stays Vertical")
	}
}

func TestBadgeStyleForShape(t *testing.T) {
	if s := BadgeStyleVertical.ForShape(BadgeShapePill); s != BadgeStyleHorizontal {
		t.Error("Vertical+Pill → Horizontal")
	}
	if s := BadgeStyleVertical.ForShape(BadgeShapeRounded); s != BadgeStyleVertical {
		t.Error("Vertical+Rounded stays Vertical")
	}
}

func TestBadgeDirectionParse(t *testing.T) {
	if ParseBadgeDirection("h") != BadgeDirectionHorizontal {
		t.Error("h should be Horizontal")
	}
	if ParseBadgeDirection("v") != BadgeDirectionVertical {
		t.Error("v should be Vertical")
	}
	if ParseBadgeDirection("d") != BadgeDirectionDefault {
		t.Error("d should be Default")
	}
}

func TestBadgeDirectionResolve(t *testing.T) {
	def := BadgeDirectionDefault
	if d := def.ResolveDefault(); d != BadgeDirectionHorizontal {
		t.Error("Default → Horizontal (layout rows)")
	}
	if d := BadgeDirectionVertical.ResolveDefault(); d != BadgeDirectionVertical {
		t.Error("Vertical stays Vertical")
	}
}

func TestBadgePositionHelpers(t *testing.T) {
	if !PositionTopCenter.IsTop() {
		t.Error("TopCenter.IsTop should be true")
	}
	if !PositionBottomCenter.IsBottom() {
		t.Error("BottomCenter.IsBottom should be true")
	}
	if !PositionLeft.IsLeft() {
		t.Error("Left.IsLeft should be true")
	}
	if !PositionRight.IsRight() {
		t.Error("Right.IsRight should be true")
	}
	if !PositionBottomCenter.IsCenterHorizontal() {
		t.Error("BottomCenter.IsCenterHorizontal should be true")
	}
}

func TestSplitAnchors(t *testing.T) {
	primary, opposite := PositionBottomCenter.SplitAnchors(true)
	if primary != PositionBottomCenter || opposite != PositionTopCenter {
		t.Errorf("BottomCenter split got %s/%s", primary, opposite)
	}

	primary, opposite = PositionLeft.SplitAnchors(false)
	if primary != PositionLeft || opposite != PositionRight {
		t.Errorf("Left split got %s/%s", primary, opposite)
	}

	primary, opposite = PositionTopRight.SplitAnchors(false)
	if primary != PositionTopRight || opposite != PositionTopLeft {
		t.Errorf("TopRight split got %s/%s", primary, opposite)
	}
}

func TestLabelStyle(t *testing.T) {
	if !LabelStyleIcon.UsesIcon() {
		t.Error("Icon.UsesIcon should be true")
	}
	if !LabelStyleOfficial.UsesIcon() {
		t.Error("Official.UsesIcon should be true")
	}
	if LabelStyleText.UsesIcon() {
		t.Error("Text.UsesIcon should be false")
	}
}

func TestScalePercent(t *testing.T) {
	if DefaultScalePercent() != 100 {
		t.Error("default scale should be 100")
	}
	if got := ClampScalePercent(30); got != 50 {
		t.Errorf("clamp low: got %d, want 50", got)
	}
	if got := ClampScalePercent(500); got != 400 {
		t.Errorf("clamp high: got %d, want 400", got)
	}
	if ScalePercent(150).Percent() != 1.5 {
		t.Errorf("Percent: got %v, want 1.5", ScalePercent(150).Percent())
	}
	if ScaleCacheSuffix("ts", 100) != "" {
		t.Error("default scale should add no cache suffix")
	}
	if ScaleCacheSuffix("ts", 150) != ".ts150" {
		t.Errorf("150 should suffix .ts150, got %q", ScaleCacheSuffix("ts", 150))
	}
	if ScaleCacheSuffix("bz", 145) != ".bz145" {
		t.Errorf("badge size suffix wrong: %q", ScaleCacheSuffix("bz", 145))
	}
}

func TestValidateRatingsLimit(t *testing.T) {
	for i := int32(0); i <= 10; i++ {
		if ValidateRatingsLimit(i) != nil {
			t.Errorf("limit %d should be valid", i)
		}
	}
	if ValidateRatingsLimit(-1) == nil {
		t.Error("-1 should fail")
	}
	if ValidateRatingsLimit(11) == nil {
		t.Error("11 should fail")
	}
}

func TestValidateRatingsOrder(t *testing.T) {
	if ValidateRatingsOrder("") != nil {
		t.Error("empty should pass")
	}
	if ValidateRatingsOrder("imdb,tmdb,rt") != nil {
		t.Error("valid keys should pass")
	}
	if ValidateRatingsOrder("imdb,bogus") == nil {
		t.Error("bogus key should fail")
	}
	if ValidateRatingsOrder("imdb,imdb") == nil {
		t.Error("duplicate should fail")
	}
}

func TestValidateLang(t *testing.T) {
	if ValidateLang("en") != nil {
		t.Error("en should pass")
	}
	if ValidateLang("pt-BR") != nil {
		t.Error("pt-BR should pass")
	}
	if ValidateLang("e") == nil {
		t.Error("too short should fail")
	}
	if ValidateLang("") == nil {
		t.Error("empty should fail")
	}
	if ValidateLang("abcdef") == nil {
		t.Error("too long should fail")
	}
	if ValidateLang("../../") == nil {
		t.Error("slashes should fail")
	}
}

func TestClampEdgeInset(t *testing.T) {
	if ClampEdgeInset(0) != 0 {
		t.Error("0 should stay 0")
	}
	if ClampEdgeInset(30) != 30 {
		t.Error("30 should stay 30")
	}
	if ClampEdgeInset(999) != MaxEdgeInset() {
		t.Error("999 should clamp to max")
	}
	if ClampEdgeInset(-5) != 0 {
		t.Error("-5 should clamp to 0")
	}
}

func TestPosterFitCacheSuffix(t *testing.T) {
	if PosterFitNative.CacheSuffix() != "" {
		t.Error("Native should have empty suffix")
	}
	if PosterFitCover.CacheSuffix() != ".fc" {
		t.Error("Cover suffix wrong")
	}
	if PosterFitPad.CacheSuffix() != ".fp" {
		t.Error("Pad suffix wrong")
	}
	if PosterFitBlur.CacheSuffix() != ".fb" {
		t.Error("Blur suffix wrong")
	}
}

func TestDefaultRenderSettings(t *testing.T) {
	s := DefaultRenderSettings()
	if s.ImageSource != ImageSourceTMDB {
		t.Error("default image_source should be TMDB")
	}
	if s.Lang != "en" {
		t.Error("default lang should be en")
	}
	if s.RatingsLimit != 3 {
		t.Error("default ratings_limit should be 3")
	}
	if s.PosterBadgeStyle != BadgeStyleDefault {
		t.Error("default poster_badge_style should be Default")
	}
	if s.PosterFit != PosterFitNative {
		t.Error("default poster_fit should be Native")
	}
	if !s.IsDefault {
		t.Error("IsDefault should be true")
	}
}

func TestParseGlobalRenderSettings(t *testing.T) {
	globals := map[string]string{
		"image_source":       "f",
		"poster_badge_style": "v",
		"lang":               "de",
		"ratings_limit":      "5",
		"poster_fit":         "pad",
	}
	s := ParseGlobalRenderSettings(globals)
	if s.ImageSource != ImageSourceFanart {
		t.Error("image_source should be Fanart")
	}
	if s.PosterBadgeStyle != BadgeStyleVertical {
		t.Error("poster_badge_style should be Vertical")
	}
	if s.Lang != "de" {
		t.Error("lang should be de")
	}
	if s.RatingsLimit != 5 {
		t.Error("ratings_limit should be 5")
	}
	if s.PosterFit != PosterFitPad {
		t.Error("poster_fit should be Pad")
	}
}

func TestParseGlobalRenderSettingsEmptyReturnsDefaults(t *testing.T) {
	s := ParseGlobalRenderSettings(nil)
	if s.ImageSource != ImageSourceTMDB {
		t.Error("empty globals should return defaults")
	}
}
