package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBadgeStyleParse(t *testing.T) {
	if ParseBadgeStyle("lr") != BadgeStyleLogoLeftValueRight {
		t.Error("lr should be LogoLeftValueRight")
	}
	if ParseBadgeStyle("rl") != BadgeStyleValueLeftLogoRight {
		t.Error("rl should be ValueLeftLogoRight")
	}
	if ParseBadgeStyle("tb") != BadgeStyleLogoTB {
		t.Error("tb should be LogoTB")
	}
	if ParseBadgeStyle("bt") != BadgeStyleValueTB {
		t.Error("bt should be ValueTB")
	}
	if ParseBadgeStyle("h") != BadgeStyleLogoLeftValueRight {
		t.Error("h should map to LogoLeftValueRight")
	}
	if ParseBadgeStyle("v") != BadgeStyleLogoTB {
		t.Error("v should map to LogoTB")
	}
	if ParseBadgeStyle("d") != BadgeStyleDefault {
		t.Error("d should be Default")
	}
	if ParseBadgeStyle("invalid") != BadgeStyleDefault {
		t.Error("invalid should default to Default")
	}
}

func TestBadgeStyleIsVertical(t *testing.T) {
	if BadgeStyleLogoLeftValueRight.IsVertical() {
		t.Error("LogoLeftValueRight.IsVertical should be false")
	}
	if BadgeStyleValueLeftLogoRight.IsVertical() {
		t.Error("ValueLeftLogoRight.IsVertical should be false")
	}
	if !BadgeStyleLogoTB.IsVertical() {
		t.Error("LogoTB.IsVertical should be true")
	}
	if !BadgeStyleValueTB.IsVertical() {
		t.Error("ValueTB.IsVertical should be true")
	}
}

func TestBadgeStyleIsMirrored(t *testing.T) {
	if BadgeStyleLogoLeftValueRight.IsMirrored() {
		t.Error("LogoLeftValueRight.IsMirrored should be false")
	}
	if !BadgeStyleValueLeftLogoRight.IsMirrored() {
		t.Error("ValueLeftLogoRight.IsMirrored should be true")
	}
	if BadgeStyleLogoTB.IsMirrored() {
		t.Error("LogoTB.IsMirrored should be false")
	}
	if !BadgeStyleValueTB.IsMirrored() {
		t.Error("ValueTB.IsMirrored should be true")
	}
}

func TestBadgeStyleResolveDefault(t *testing.T) {
	if s := BadgeStyleDefault.ResolveDefault(); s != BadgeStyleLogoLeftValueRight {
		t.Error("Default → LogoLeftValueRight")
	}
	if s := BadgeStyleLogoTB.ResolveDefault(); s != BadgeStyleLogoTB {
		t.Error("explicit LogoTB stays LogoTB")
	}
}

func TestBadgeStyleResolve(t *testing.T) {
	if s := BadgeStyleDefault.Resolve(BadgeDirectionHorizontal); s != BadgeStyleLogoLeftValueRight {
		t.Error("Default+Horizontal → LogoLeftValueRight")
	}
	if s := BadgeStyleDefault.Resolve(BadgeDirectionVertical); s != BadgeStyleLogoTB {
		t.Error("Default+Vertical → LogoTB")
	}
	if s := BadgeStyleLogoTB.Resolve(BadgeDirectionHorizontal); s != BadgeStyleLogoTB {
		t.Error("explicit LogoTB stays LogoTB")
	}
	if s := BadgeStyleValueTB.Resolve(BadgeDirectionHorizontal); s != BadgeStyleValueTB {
		t.Error("explicit ValueTB stays ValueTB")
	}
	if s := BadgeStyle("h").Resolve(BadgeDirectionVertical); s != BadgeStyleLogoLeftValueRight {
		t.Error("legacy h normalises to LogoLeftValueRight")
	}
	if s := BadgeStyle("v").Resolve(BadgeDirectionHorizontal); s != BadgeStyleLogoTB {
		t.Error("legacy v normalises to LogoTB")
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
	if ParseBadgeDirection("invalid") != BadgeDirectionDefault {
		t.Error("invalid should be Default")
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
	if d := BadgeDirectionHorizontal.ResolveDefault(); d != BadgeDirectionHorizontal {
		t.Error("Horizontal stays Horizontal")
	}
}

func TestBadgeStyleForShape(t *testing.T) {
	if s := BadgeStyleLogoTB.ForShape(BadgeShapePill); s != BadgeStyleLogoLeftValueRight {
		t.Error("LogoTB+Pill → LogoLeftValueRight")
	}
	if s := BadgeStyleLogoTB.ForShape(BadgeShapeRounded); s != BadgeStyleLogoTB {
		t.Error("LogoTB+Rounded stays LogoTB")
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
		"image_source":        "f",
		"poster_badge_style":  "v",
		"lang":                "de",
		"ratings_limit":       "5",
		"poster_fit":          "pad",
		"poster_badge_width":  "150",
		"poster_badge_height": "80",
	}
	s := ParseGlobalRenderSettings(globals)
	if s.ImageSource != ImageSourceFanart {
		t.Error("image_source should be Fanart")
	}
	if s.PosterBadgeStyle != BadgeStyleLogoTB {
		t.Error("poster_badge_style should be LogoTB")
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
	if s.PosterBadgeWidth != 150 || s.PosterBadgeHeight != 80 {
		t.Errorf("poster badge width/height: got %d/%d want 150/80", s.PosterBadgeWidth, s.PosterBadgeHeight)
	}
	if s.LogoBadgeWidth != 100 || s.LogoBadgeHeight != 100 {
		t.Errorf("logo badge width/height default: got %d/%d want 100/100", s.LogoBadgeWidth, s.LogoBadgeHeight)
	}
	m := RenderSettingsToMap(&s)
	if m["poster_badge_width"] != "150" || m["poster_badge_height"] != "80" {
		t.Errorf("toMap poster width/height: %q/%q", m["poster_badge_width"], m["poster_badge_height"])
	}
}

func TestParseGlobalRenderSettingsEmptyReturnsDefaults(t *testing.T) {
	s := ParseGlobalRenderSettings(nil)
	if s.ImageSource != ImageSourceTMDB {
		t.Error("empty globals should return defaults")
	}
}

// TestParseGlobalRenderSettings_UseKitsuMALRoundTrip guards NOTES.md #13/#14:
// ParseGlobalRenderSettings previously omitted UseKitsu/UseMAL from its
// returned struct literal entirely, so a saved `false` never survived the
// read path (every request silently got the zero value regardless of what
// was stored). A non-empty globals map — the case that fires on every real
// request once any setting has been saved — must round-trip both fields.
func TestParseGlobalRenderSettings_UseKitsuMALRoundTrip(t *testing.T) {
	globals := map[string]string{
		"image_source": "f",
		"use_kitsu":    "false",
		"use_mal":      "false",
	}
	s := ParseGlobalRenderSettings(globals)
	if s.UseKitsu {
		t.Error("use_kitsu=false in globals should not survive as UseKitsu=true")
	}
	if s.UseMAL {
		t.Error("use_mal=false in globals should not survive as UseMAL=true")
	}

	m := SettingsResponseMap(&s)
	if v, ok := m["use_kitsu"].(bool); !ok || v {
		t.Errorf("SettingsResponseMap use_kitsu: got %#v, want false", m["use_kitsu"])
	}
	if v, ok := m["use_mal"].(bool); !ok || v {
		t.Errorf("SettingsResponseMap use_mal: got %#v, want false", m["use_mal"])
	}
}

// TestParseGlobalRenderSettings_UseKitsuMALDefaultsToTrue guards the other
// direction: omitting the keys (never saved yet) must still default to true,
// matching DefaultRenderSettings.
func TestParseGlobalRenderSettings_UseKitsuMALDefaultsToTrue(t *testing.T) {
	globals := map[string]string{"image_source": "t"}
	s := ParseGlobalRenderSettings(globals)
	if !s.UseKitsu || !s.UseMAL {
		t.Errorf("UseKitsu/UseMAL should default to true when absent from globals, got %v/%v", s.UseKitsu, s.UseMAL)
	}
}

// TestAnimeArtwork_GlobalRoundTrip guards the two hand-written global
// settings paths (the #13 toggle bug class): anime_artwork must survive
// ParseGlobalRenderSettings and SettingsResponseMap, unknown → "id".
func TestAnimeArtwork_GlobalRoundTrip(t *testing.T) {
	for in, want := range map[string]string{"kitsu": "kitsu", "mal": "mal", "id": "id", "bogus": "id", "": "id"} {
		globals := map[string]string{"image_source": "t"}
		if in != "" {
			globals["anime_artwork"] = in
		}
		s := ParseGlobalRenderSettings(globals)
		if string(s.AnimeArtwork) != want {
			t.Errorf("%q: parsed %q, want %q", in, s.AnimeArtwork, want)
		}
		if got := SettingsResponseMap(&s)["anime_artwork"]; got != want {
			t.Errorf("%q: response map %v, want %q", in, got, want)
		}
	}
}

// TestAnimeArtwork_CacheSuffix: defaults leave the image cache key unchanged
// (no mass re-render); each non-default Kitsu/MAL choice adds its token.
func TestAnimeArtwork_CacheSuffix(t *testing.T) {
	base := DefaultRenderSettings()
	def := SettingsCacheSuffix(&base, "poster", nil)
	for _, tok := range []string{".nk", ".nm", ".ak", ".am"} {
		if strings.Contains(def, tok) {
			t.Errorf("default suffix %q contains %s", def, tok)
		}
	}
	cases := []struct {
		mod func(*RenderSettings)
		tok string
	}{
		{func(s *RenderSettings) { s.UseKitsu = false }, ".nk"},
		{func(s *RenderSettings) { s.UseMAL = false }, ".nm"},
		{func(s *RenderSettings) { s.AnimeArtwork = AnimeArtworkKitsu }, ".ak"},
		{func(s *RenderSettings) { s.AnimeArtwork = AnimeArtworkMAL }, ".am"},
	}
	for _, c := range cases {
		s := DefaultRenderSettings()
		c.mod(&s)
		if got := SettingsCacheSuffix(&s, "poster", nil); got != def+c.tok {
			t.Errorf("got %q, want %q", got, def+c.tok)
		}
	}
}

// TestAniListFuzzyDateISO: AniList start dates format like TMDB release
// dates, dropping unknown trailing parts.
func TestAniListFuzzyDateISO(t *testing.T) {
	i := func(v int) *int { return &v }
	cases := []struct {
		d    anilistFuzzyDate
		want string
	}{
		{anilistFuzzyDate{i(2018), i(10), i(7)}, "2018-10-07"},
		{anilistFuzzyDate{i(2018), i(10), nil}, "2018-10"},
		{anilistFuzzyDate{i(2018), nil, i(7)}, "2018"},
	}
	for _, c := range cases {
		if got := c.d.ISO(); got == nil || *got != c.want {
			t.Errorf("ISO() = %v, want %s", got, c.want)
		}
	}
	if got := (anilistFuzzyDate{}).ISO(); got != nil {
		t.Errorf("unknown year: got %v, want nil", *got)
	}
}

// countingTransport counts requests and answers every one with body.
type countingTransport struct {
	n    int
	body string
}

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n++
	rec := httptest.NewRecorder()
	rec.WriteString(c.body)
	return rec.Result(), nil
}

// TestTmdbRatingSkippedWithoutTMDbID: an IMDb-only title (no TMDB entry)
// must not issue a pointless /movie/0 request.
func TestTmdbRatingSkippedWithoutTMDbID(t *testing.T) {
	tr := &countingTransport{body: `{"vote_average":7.5}`}
	tmdb := NewTmdbClient("k", &http.Client{Transport: tr})
	if b := fetchTmdbRatingCtx(context.Background(), tmdb, 0, "movie", 0, 0, 0); b != nil || tr.n != 0 {
		t.Errorf("tmdbID 0: badge=%v requests=%d, want nil and 0", b, tr.n)
	}
	if b := fetchTmdbRatingCtx(context.Background(), tmdb, 278, "movie", 0, 0, 0); b == nil || tr.n != 1 {
		t.Errorf("tmdbID 278: badge=%v requests=%d, want a badge and 1", b, tr.n)
	}
}

// TestResolveMAL_MovieFormat: an AniList format of MOVIE marks a mal: title
// as a movie (it was always typed TV), so IMDb-only rating lookups ask for
// the right media type.
func TestResolveMAL_MovieFormat(t *testing.T) {
	tr := &countingTransport{body: `{"data":{"Media":{"id":1,"idMal":199,"format":"MOVIE","title":{"romaji":"Sen to Chihiro"},"coverImage":{"extraLarge":"https://x/c.jpg"},"bannerImage":null,"startDate":{"year":2001,"month":7,"day":20},"externalLinks":[]}}}`}
	r, err := resolveMALCtx(context.Background(), "199", NewAniListClient(&http.Client{Transport: tr}))
	if err != nil {
		t.Fatal(err)
	}
	if r.MediaType != MediaTypeMovie {
		t.Errorf("MediaType = %v, want movie", r.MediaType)
	}
}
