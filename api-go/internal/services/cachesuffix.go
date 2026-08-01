package services

import (
	"fmt"
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

func BadgeBackgroundCacheSuffix(background string) string {
	return ".bg" + background
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

	switch kind {
	case "poster":
		ps := PositionCacheSuffix(string(settings.PosterPosition))
		bs := BadgeStyleCacheSuffix(string(settings.PosterBadgeStyle.ForShape(settings.PosterBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.PosterLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.PosterBadgeDirection))
		bsz := settings.PosterBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.PosterBadgeShape))
		bgd := BadgeBackgroundCacheSuffix(string(settings.PosterBadgeBackground))
		split := ""
		if settings.PosterBadgeSplit {
			split = ".x1"
		}
		fit := settings.PosterFit.CacheSuffix()
		return ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + split + fit + isSuffix

	case "logo":
		bs := BadgeStyleCacheSuffix(string(settings.LogoBadgeStyle.ForShape(settings.LogoBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.LogoLabelStyle))
		bsz := settings.LogoBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.LogoBadgeShape))
		bgd := BadgeBackgroundCacheSuffix(string(settings.LogoBadgeBackground))
		return ratingsSuffix + bs + ls + bsz + shp + bgd + isSuffix

	case "backdrop":
		ps := PositionCacheSuffix(string(settings.BackdropPosition))
		bs := BadgeStyleCacheSuffix(string(settings.BackdropBadgeStyle.ForShape(settings.BackdropBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.BackdropLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.BackdropBadgeDirection))
		bsz := settings.BackdropBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.BackdropBadgeShape))
		bgd := BadgeBackgroundCacheSuffix(string(settings.BackdropBadgeBackground))
		ei := EdgeInsetCacheSuffix(settings.BackdropPosition, settings.BackdropEdgeInsetX, settings.BackdropEdgeInsetY)
		return ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + ei + isSuffix

	case "episode":
		ps := PositionCacheSuffix(string(settings.EpisodePosition))
		bs := BadgeStyleCacheSuffix(string(settings.EpisodeBadgeStyle.ForShape(settings.EpisodeBadgeShape)))
		ls := LabelStyleCacheSuffix(string(settings.EpisodeLabelStyle))
		bd := BadgeDirectionCacheSuffix(string(settings.EpisodeBadgeDirection))
		bsz := settings.EpisodeBadgeSize.CacheSuffix()
		shp := BadgeShapeCacheSuffix(string(settings.EpisodeBadgeShape))
		bgd := BadgeBackgroundCacheSuffix(string(settings.EpisodeBadgeBackground))
		blur := ""
		if settings.EpisodeBlur {
			blur = ".blur"
		}
		return ratingsSuffix + ps + bs + ls + bd + bsz + shp + bgd + blur + isSuffix
	}

	return ""
}
