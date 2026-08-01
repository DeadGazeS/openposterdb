package services

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// --- Cache key suffix construction ---

func PositionCacheSuffix(position string) string {
	return ".p" + position
}

func BadgeStyleCacheSuffix(style string) string {
	return ".s" + style
}

func LabelStyleCacheSuffix(style string) string {
	return ".l" + style
}

func BadgeDirectionCacheSuffix(dir string) string {
	return ".d" + dir
}

func BadgeShapeCacheSuffix(shape string) string {
	return ".sh" + shape
}

func BadgeAlphaCacheSuffix(alpha int32) string {
	return fmt.Sprintf(".ba%d", alpha)
}

func EdgeInsetCacheSuffix(position BadgePosition, insetX, insetY int32) string {
	insetX = ClampEdgeInset(insetX)
	insetY = ClampEdgeInset(insetY)
	var out string
	if (position.IsLeft() || position.IsRight()) && insetX > 0 {
		out += fmt.Sprintf(".eh%d", insetX)
	}
	if (position.IsTop() || position.IsBottom()) && insetY > 0 {
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
		ps := PositionCacheSuffix(string(settings.PosterPosition))
		bs := BadgeStyleCacheSuffix(string(settings.PosterBadgeStyle.ForShape(settings.PosterBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.PosterLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.PosterBadgeDirection))
		bsz := settings.PosterBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.PosterBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.PosterBadgeAlpha))
		split := ""
		if settings.PosterBadgeSplit {
			split = ".x1"
		}
		fit := settings.PosterFit.CacheSuffix()
		result = ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + split + fit + isSuffix

	case "logo":
		bs := BadgeStyleCacheSuffix(string(settings.LogoBadgeStyle.ForShape(settings.LogoBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.LogoLabelStyle))
		bsz := settings.LogoBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.LogoBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.LogoBadgeAlpha))
		result = ratingsSuffix + bs + ls + bsz + shp + bgd + isSuffix

	case "backdrop":
		ps := PositionCacheSuffix(string(settings.BackdropPosition))
		bs := BadgeStyleCacheSuffix(string(settings.BackdropBadgeStyle.ForShape(settings.BackdropBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.BackdropLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.BackdropBadgeDirection))
		bsz := settings.BackdropBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.BackdropBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.BackdropBadgeAlpha))
		ei := EdgeInsetCacheSuffix(settings.BackdropPosition, settings.BackdropEdgeInsetX, settings.BackdropEdgeInsetY)
		result = ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + ei + isSuffix

	case "episode":
		ps := PositionCacheSuffix(string(settings.EpisodePosition))
		bs := BadgeStyleCacheSuffix(string(settings.EpisodeBadgeStyle.ForShape(settings.EpisodeBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.EpisodeLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.EpisodeBadgeDirection))
		bsz := settings.EpisodeBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.EpisodeBadgeShape))
		bgd := BadgeAlphaCacheSuffix(int32(settings.EpisodeBadgeAlpha))
		blur := ""
		if settings.EpisodeBlur {
			blur = ".blur"
		}
		result = ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + blur + isSuffix
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
