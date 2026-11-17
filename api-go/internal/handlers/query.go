package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"openposterdb/internal/services"
)

type ImageQuery struct {
	Fallback       *string `json:"fallback"`
	Lang           *string `json:"lang"`
	ImageSize      *string `json:"imageSize"`
	RatingsLimit   *int32  `json:"ratings_limit"`
	RatingsOrder   *string `json:"ratings_order"`
	RatingsExclude *string `json:"ratings_exclude"`
	BadgeStyle     *string `json:"badge_style"`
	LabelStyle     *string `json:"label_style"`
	TextSize       *int32  `json:"text_size"`
	BadgeSize      *int32  `json:"badge_size"`
	BadgeWidth     *int32  `json:"badge_width"`
	BadgeHeight    *int32  `json:"badge_height"`
	LogoSize       *int32  `json:"logo_size"`
	BadgeDirection *string `json:"badge_direction"`
	BadgeShape     *string `json:"badge_shape"`
	BadgeAlpha     *int32  `json:"badge_alpha"`
	Layout         *string `json:"layout"`
	ImageSource    *string `json:"image_source"`
	Textless       *bool   `json:"textless"`
	Blur           *bool   `json:"blur"`
	Fit            *string `json:"fit"`
	EdgeInsetX     *int32  `json:"edge_inset_x"`
	EdgeInsetY     *int32  `json:"edge_inset_y"`

	// Legacy (Rust-era) aliases, mapped onto the current settings:
	// position / split / badge_direction → layout (LayoutFromLegacy),
	// badge_background d|k|t|n → badge alpha, badge_size xs..xl → percent.
	Position        *string `json:"position"`
	Split           *bool   `json:"split"`
	BadgeBackground *string `json:"badge_background"`
	BadgeSizeName   *string `json:"-"` // badge_size given as a size name
}

func (q *ImageQuery) HasOverrides() bool {
	return q.RatingsLimit != nil || q.RatingsOrder != nil || q.RatingsExclude != nil ||
		q.Position != nil || q.Split != nil || q.BadgeBackground != nil || q.BadgeSizeName != nil ||
		q.BadgeStyle != nil || q.LabelStyle != nil || q.TextSize != nil ||
		q.BadgeSize != nil || q.LogoSize != nil ||
		q.BadgeDirection != nil || q.BadgeShape != nil || q.BadgeAlpha != nil ||
		q.BadgeWidth != nil || q.BadgeHeight != nil ||
		q.Layout != nil || q.ImageSource != nil || q.Textless != nil ||
		q.Blur != nil || q.Fit != nil ||
		q.EdgeInsetX != nil || q.EdgeInsetY != nil
}

// kindRenderRefs points at the per-kind RenderSettings fields a query override
// can touch. Resolving the kind once — instead of re-deriving it in a
// `switch kind` per field — means a newly added per-kind field is wired up in
// exactly one place, and a field can't silently miss a kind arm (the struct
// literal per kind is exhaustive or it doesn't compile). This is the drift that
// produced the 2026-08-04 badge-width bug, where a new per-kind field reached
// some sites and not others.
//
// badgeDirection is nil for logo: the logo badge layout is direction-agnostic,
// so ?badge_direction has never applied there.
type kindRenderRefs struct {
	ratingsLimit   *int32
	badgeStyle     *services.BadgeStyle
	labelStyle     *services.LabelStyle
	textSize       *services.ScalePercent
	badgeSize      *services.ScalePercent
	badgeWidth     *services.ScalePercent
	badgeHeight    *services.ScalePercent
	logoSize       *services.ScalePercent
	badgeShape     *services.BadgeShape
	badgeAlpha     *services.BadgeAlpha
	layout         *services.ImageLayout
	badgeDirection *services.BadgeDirection
}

// kindRenderRefsFor returns the field set for kind, or nil for an unrecognised
// kind (in which case per-kind overrides are skipped entirely).
func kindRenderRefsFor(s *services.RenderSettings, kind string) *kindRenderRefs {
	switch kind {
	case "poster":
		return &kindRenderRefs{
			ratingsLimit: &s.RatingsLimit, badgeStyle: &s.PosterBadgeStyle,
			labelStyle: &s.PosterLabelStyle, textSize: &s.PosterTextSize,
			badgeSize: &s.PosterBadgeSize, badgeWidth: &s.PosterBadgeWidth,
			badgeHeight: &s.PosterBadgeHeight, logoSize: &s.PosterLogoSize,
			badgeShape: &s.PosterBadgeShape, badgeAlpha: &s.PosterBadgeAlpha,
			layout: &s.PosterLayout, badgeDirection: &s.PosterBadgeDirection,
		}
	case "logo":
		return &kindRenderRefs{
			ratingsLimit: &s.LogoRatingsLimit, badgeStyle: &s.LogoBadgeStyle,
			labelStyle: &s.LogoLabelStyle, textSize: &s.LogoTextSize,
			badgeSize: &s.LogoBadgeSize, badgeWidth: &s.LogoBadgeWidth,
			badgeHeight: &s.LogoBadgeHeight, logoSize: &s.LogoLogoSize,
			badgeShape: &s.LogoBadgeShape, badgeAlpha: &s.LogoBadgeAlpha,
			layout: &s.LogoLayout, badgeDirection: nil,
		}
	case "backdrop":
		return &kindRenderRefs{
			ratingsLimit: &s.BackdropRatingsLimit, badgeStyle: &s.BackdropBadgeStyle,
			labelStyle: &s.BackdropLabelStyle, textSize: &s.BackdropTextSize,
			badgeSize: &s.BackdropBadgeSize, badgeWidth: &s.BackdropBadgeWidth,
			badgeHeight: &s.BackdropBadgeHeight, logoSize: &s.BackdropLogoSize,
			badgeShape: &s.BackdropBadgeShape, badgeAlpha: &s.BackdropBadgeAlpha,
			layout: &s.BackdropLayout, badgeDirection: &s.BackdropBadgeDirection,
		}
	case "episode":
		return &kindRenderRefs{
			ratingsLimit: &s.EpisodeRatingsLimit, badgeStyle: &s.EpisodeBadgeStyle,
			labelStyle: &s.EpisodeLabelStyle, textSize: &s.EpisodeTextSize,
			badgeSize: &s.EpisodeBadgeSize, badgeWidth: &s.EpisodeBadgeWidth,
			badgeHeight: &s.EpisodeBadgeHeight, logoSize: &s.EpisodeLogoSize,
			badgeShape: &s.EpisodeBadgeShape, badgeAlpha: &s.EpisodeBadgeAlpha,
			layout: &s.EpisodeLayout, badgeDirection: &s.EpisodeBadgeDirection,
		}
	}
	return nil
}

func applyQueryOverrides(settings *services.RenderSettings, query *ImageQuery, kind string) *services.RenderSettings {
	if !query.HasOverrides() {
		return settings
	}

	// An out-of-range ?ratings_limit rejects the whole override set, before
	// anything is mutated.
	if query.RatingsLimit != nil {
		if err := services.ValidateRatingsLimit(*query.RatingsLimit); err != nil {
			return settings
		}
	}

	s := *settings

	if query.RatingsOrder != nil {
		if err := services.ValidateRatingsOrder(*query.RatingsOrder); err == nil {
			s.RatingsOrder = *query.RatingsOrder
		}
	}

	if query.RatingsExclude != nil {
		if err := services.ValidateRatingsExclude(*query.RatingsExclude); err == nil {
			s.RatingsExclude = *query.RatingsExclude
		}
	}

	if k := kindRenderRefsFor(&s, kind); k != nil {
		if query.RatingsLimit != nil {
			*k.ratingsLimit = *query.RatingsLimit
		}
		if query.BadgeStyle != nil {
			*k.badgeStyle = services.ParseBadgeStyle(*query.BadgeStyle)
		}
		if query.LabelStyle != nil {
			*k.labelStyle = services.ParseLabelStyle(*query.LabelStyle)
		}
		if query.TextSize != nil {
			*k.textSize = services.ClampScalePercent(*query.TextSize)
		}
		if query.BadgeSize != nil {
			*k.badgeSize = services.ClampScalePercent(*query.BadgeSize)
		} else if query.BadgeSizeName != nil {
			if pct, ok := services.PercentFromLegacySize(kind, *query.BadgeSizeName); ok {
				*k.badgeSize = pct
			}
		}
		if query.BadgeWidth != nil {
			*k.badgeWidth = services.ClampScalePercent(*query.BadgeWidth)
		}
		if query.BadgeHeight != nil {
			*k.badgeHeight = services.ClampScalePercent(*query.BadgeHeight)
		}
		if query.LogoSize != nil {
			*k.logoSize = services.ClampScalePercent(*query.LogoSize)
		}
		if query.BadgeShape != nil {
			*k.badgeShape = services.ParseBadgeShape(*query.BadgeShape)
		}
		if query.BadgeAlpha != nil {
			*k.badgeAlpha = services.ClampBadgeAlpha(*query.BadgeAlpha)
		} else if query.BadgeBackground != nil {
			if a, ok := services.AlphaFromLegacyBackground(*query.BadgeBackground); ok {
				*k.badgeAlpha = a
			}
		}
		if query.Layout != nil {
			// Parsed values only (never raw query strings) reach the cache
			// key, so garbage can't mint new cache entries; an invalid
			// layout keeps the stored one.
			if l := services.UnmarshalLayout(*query.Layout, k.layout); services.ValidateLayout(&l) == nil {
				*k.layout = l
			}
		} else if query.Position != nil || query.Split != nil || query.BadgeDirection != nil {
			// Legacy placement: rebuilt from position / split / direction with
			// the badge count of the layout it replaces (or ?ratings_limit).
			// Unknown values fall back to the kind's old defaults.
			count := k.layout.Total()
			if query.RatingsLimit != nil {
				count = *query.RatingsLimit
			}
			*k.layout = services.LayoutFromLegacy(kind, deref(query.Position), deref(query.BadgeDirection), query.Split != nil && *query.Split, count)
		}
		if query.BadgeDirection != nil && k.badgeDirection != nil {
			*k.badgeDirection = services.ParseBadgeDirection(*query.BadgeDirection)
		}
	}

	// Kind-exclusive overrides: each applies to exactly one kind, so they stay
	// outside the shared per-kind table.
	switch kind {
	case "poster":
		if query.Fit != nil {
			s.PosterFit = services.ParsePosterFit(*query.Fit)
		}
		if query.Textless != nil {
			s.Textless = *query.Textless
		}
	case "backdrop":
		if query.EdgeInsetX != nil {
			s.BackdropEdgeInsetX = services.ClampEdgeInset(*query.EdgeInsetX)
		}
		if query.EdgeInsetY != nil {
			s.BackdropEdgeInsetY = services.ClampEdgeInset(*query.EdgeInsetY)
		}
	case "episode":
		if query.Blur != nil {
			s.EpisodeBlur = *query.Blur
		}
	}

	if query.ImageSource != nil {
		s.ImageSource = services.ParseImageSource(*query.ImageSource)
	}

	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseImageQuery(r *http.Request) *ImageQuery {
	q := r.URL.Query()
	query := &ImageQuery{}

	if v := q.Get("fallback"); v != "" {
		query.Fallback = &v
	}
	if v := q.Get("lang"); v != "" {
		query.Lang = &v
	}
	if v := q.Get("imageSize"); v != "" {
		query.ImageSize = &v
	}
	if v := q.Get("ratings_limit"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.RatingsLimit = &n
		}
	}
	// Present but empty clears the stored value (?ratings_exclude= shows
	// every source again), so presence is checked, not emptiness.
	if q.Has("ratings_order") {
		v := q.Get("ratings_order")
		query.RatingsOrder = &v
	}
	if q.Has("ratings_exclude") {
		v := q.Get("ratings_exclude")
		query.RatingsExclude = &v
	}
	if v := q.Get("badge_style"); v != "" {
		query.BadgeStyle = &v
	}
	if v := q.Get("label_style"); v != "" {
		query.LabelStyle = &v
	}
	if v := q.Get("badge_size"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			n32 := int32(n)
			query.BadgeSize = &n32
		} else {
			query.BadgeSizeName = &v // legacy xs/s/m/l/xl
		}
	}
	if v := q.Get("badge_width"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeWidth = &n
		}
	}
	if v := q.Get("badge_height"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeHeight = &n
		}
	}
	if v := q.Get("text_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.TextSize = &n
		}
	}
	if v := q.Get("logo_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.LogoSize = &n
		}
	}
	if v := q.Get("badge_direction"); v != "" {
		query.BadgeDirection = &v
	}
	if v := q.Get("badge_shape"); v != "" {
		query.BadgeShape = &v
	}
	if v := q.Get("badge_alpha"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeAlpha = &n
		}
	}
	if v := q.Get("position"); v != "" {
		query.Position = &v
	}
	if v := q.Get("split"); v != "" {
		b := v == "true" || v == "1"
		query.Split = &b
	}
	if v := q.Get("badge_background"); v != "" {
		query.BadgeBackground = &v
	}
	if v := q.Get("image_source"); v != "" {
		query.ImageSource = &v
	}
	if v := q.Get("poster_source"); v != "" {
		query.ImageSource = &v
	}
	if v := q.Get("textless"); v != "" {
		b := v == "true"
		query.Textless = &b
	}
	if v := q.Get("fanart_textless"); v != "" {
		b := v == "true"
		query.Textless = &b
	}
	if v := q.Get("blur"); v != "" {
		b := v == "true"
		query.Blur = &b
	}
	if v := q.Get("layout"); v != "" {
		query.Layout = &v
	}
	if v := q.Get("fit"); v != "" {
		query.Fit = &v
	}
	if v := q.Get("edge_inset_x"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.EdgeInsetX = &n
		}
	}
	if v := q.Get("edge_inset_y"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.EdgeInsetY = &n
		}
	}

	return query
}
