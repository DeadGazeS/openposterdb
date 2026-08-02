package services

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// --- Cache key suffix construction ---

func BadgeStyleCacheSuffix(style string) string {
	return ".s" + style
}

func LabelStyleCacheSuffix(style string) string {
	return ".l" + style
}

func BadgeDirectionCacheSuffix(dir string) string {
	return ".d" + dir
}

// LayoutCacheSuffix returns a short stable token for an image layout. The
// default layout for the kind adds no token so existing cache keys stay stable.
func LayoutCacheSuffix(layout *ImageLayout, kind string) string {
	def := DefaultLayout(kind)
	if layout != nil && layoutEqual(layout, &def) {
		return ""
	}
	return ".ly" + layoutToken(layout)
}

func layoutEqual(a, b *ImageLayout) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Top != b.Top || a.Right != b.Right || a.Bottom != b.Bottom || a.Left != b.Left {
		return false
	}
	return sameStrings(a.Order, b.Order)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func layoutToken(l *ImageLayout) string {
	if l == nil {
		return ""
	}
	h := sha256.New()
	for _, side := range SideNames {
		slot := l.SideSlot(side)
		if slot == nil {
			continue
		}
		fmt.Fprintf(h, "%s:%d:%d:%s;", side, slot.PerRow, slot.Rows, slot.Start)
	}
	fmt.Fprintf(h, "o:%s", l.FillOrder())
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

func BadgeShapeCacheSuffix(shape string) string {
	return ".sh" + shape
}

func BadgeAlphaCacheSuffix(alpha int32) string {
	return fmt.Sprintf(".ba%d", alpha)
}

func EdgeInsetCacheSuffix(insetX, insetY int32) string {
	insetX = ClampEdgeInset(insetX)
	insetY = ClampEdgeInset(insetY)
	var out string
	if insetX > 0 {
		out += fmt.Sprintf(".eh%d", insetX)
	}
	if insetY > 0 {
		out += fmt.Sprintf(".ev%d", insetY)
	}
	return out
}

func ImageSizeCacheSuffix(size *string) string {
	if size == nil {
		return ".zm"
	}
	s := ParseImageSize(*size)
	return s.CacheSuffix()
}

func SettingsCacheSuffix(settings *RenderSettings, kind string, imageSizeStr *string) string {
	var ratingsSuffix string
	switch kind {
	case "poster":
		ratingsSuffix = RatingsCacheSuffix(settings.RatingsOrder, settings.RatingsExclude, settings.RatingsLimit)
	case "logo":
		ratingsSuffix = RatingsCacheSuffix(settings.RatingsOrder, settings.RatingsExclude, settings.LogoRatingsLimit)
	case "backdrop":
		ratingsSuffix = RatingsCacheSuffix(settings.RatingsOrder, settings.RatingsExclude, settings.BackdropRatingsLimit)
	case "episode":
		ratingsSuffix = RatingsCacheSuffix(settings.RatingsOrder, settings.RatingsExclude, settings.EpisodeRatingsLimit)
	}
	return SettingsCacheSuffixWithRatings(settings, kind, imageSizeStr, ratingsSuffix)
}

func SettingsCacheSuffixWithRatings(settings *RenderSettings, kind string, imageSizeStr *string, ratingsSuffix string) string {
	isSuffix := ImageSizeCacheSuffix(imageSizeStr)

	var result string
	switch kind {
	case "poster":
		bs := BadgeStyleCacheSuffix(string(settings.PosterBadgeStyle.ForShape(settings.PosterBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.PosterLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.PosterBadgeDirection))
		ly := LayoutCacheSuffix(&settings.PosterLayout, "poster")
		ts := ScaleCacheSuffix("ts", settings.PosterTextSize)
		bsz := ScaleCacheSuffix("bz", settings.PosterBadgeSize)
		lgs := ScaleCacheSuffix("ls", settings.PosterLogoSize)
		shp := BadgeShapeCacheSuffix(string(settings.PosterBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.PosterBadgeAlpha))
		fit := settings.PosterFit.CacheSuffix()
		result = ratingsSuffix + bs + ls + bd + ly + ts + bsz + lgs + shp + bgd + fit + isSuffix

	case "logo":
		bs := BadgeStyleCacheSuffix(string(settings.LogoBadgeStyle.ForShape(settings.LogoBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.LogoLabelStyle))
		ts := ScaleCacheSuffix("ts", settings.LogoTextSize)
		bsz := ScaleCacheSuffix("bz", settings.LogoBadgeSize)
		lgs := ScaleCacheSuffix("ls", settings.LogoLogoSize)
		shp := BadgeShapeCacheSuffix(string(settings.LogoBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.LogoBadgeAlpha))
		ly := LayoutCacheSuffix(&settings.LogoLayout, "logo")
		result = ratingsSuffix + bs + ls + ly + ts + bsz + lgs + shp + bgd + isSuffix

	case "backdrop":
		bs := BadgeStyleCacheSuffix(string(settings.BackdropBadgeStyle.ForShape(settings.BackdropBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.BackdropLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.BackdropBadgeDirection))
		ly := LayoutCacheSuffix(&settings.BackdropLayout, "backdrop")
		ts := ScaleCacheSuffix("ts", settings.BackdropTextSize)
		bsz := ScaleCacheSuffix("bz", settings.BackdropBadgeSize)
		lgs := ScaleCacheSuffix("ls", settings.BackdropLogoSize)
		shp := BadgeShapeCacheSuffix(string(settings.BackdropBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.BackdropBadgeAlpha))
		ei := EdgeInsetCacheSuffix(settings.BackdropEdgeInsetX, settings.BackdropEdgeInsetY)
		result = ratingsSuffix + bs + ls + bd + ly + ts + bsz + lgs + shp + bgd + ei + isSuffix

	case "episode":
		bs := BadgeStyleCacheSuffix(string(settings.EpisodeBadgeStyle.ForShape(settings.EpisodeBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.EpisodeLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.EpisodeBadgeDirection))
		ly := LayoutCacheSuffix(&settings.EpisodeLayout, "episode")
		ts := ScaleCacheSuffix("ts", settings.EpisodeTextSize)
		bsz := ScaleCacheSuffix("bz", settings.EpisodeBadgeSize)
		lgs := ScaleCacheSuffix("ls", settings.EpisodeLogoSize)
		shp := BadgeShapeCacheSuffix(string(settings.EpisodeBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.EpisodeBadgeAlpha))
		blur := ""
		if settings.EpisodeBlur {
			blur = ".blur"
		}
		result = ratingsSuffix + bs + ls + bd + ly + ts + bsz + lgs + shp + bgd + blur + isSuffix
	}

	// Recolored sources affect the rendered image, so fold them into the cache
	// key. Defaults produce no token, keeping existing keys stable.
	if len(settings.Colors) > 0 {
		result += ".col" + ColorsCacheToken(settings.Colors)
	}
	return result
}

// ColorsCacheToken returns a short stable token for a set of color overrides.
func ColorsCacheToken(colors map[string]SourceColorSet) string {
	if len(colors) == 0 {
		return ""
	}
	keys := make([]string, 0, len(colors))
	for k := range colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		c := colors[k]
		fmt.Fprintf(h, "%s:%s|%s|%s|%s\n", k, c.Accent, c.Value, c.Border, c.Text)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}
