package handlers

import (
	"fmt"
	"net/http"

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
}

func (q *ImageQuery) HasOverrides() bool {
	return q.RatingsLimit != nil || q.RatingsOrder != nil || q.RatingsExclude != nil ||
		q.BadgeStyle != nil || q.LabelStyle != nil || q.TextSize != nil ||
		q.BadgeSize != nil || q.LogoSize != nil ||
		q.BadgeDirection != nil || q.BadgeShape != nil || q.BadgeAlpha != nil ||
		q.BadgeWidth != nil || q.BadgeHeight != nil ||
		q.Layout != nil || q.ImageSource != nil || q.Textless != nil ||
		q.Blur != nil || q.Fit != nil ||
		q.EdgeInsetX != nil || q.EdgeInsetY != nil
}

func applyQueryOverrides(settings *services.RenderSettings, query *ImageQuery, kind string) *services.RenderSettings {
	if !query.HasOverrides() {
		return settings
	}

	s := *settings

	if query.RatingsLimit != nil {
		limit := *query.RatingsLimit
		if err := services.ValidateRatingsLimit(limit); err != nil {
			return settings
		}
		switch kind {
		case "poster":
			s.RatingsLimit = limit
		case "logo":
			s.LogoRatingsLimit = limit
		case "backdrop":
			s.BackdropRatingsLimit = limit
		case "episode":
			s.EpisodeRatingsLimit = limit
		}
	}

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

	if query.BadgeStyle != nil {
		style := services.ParseBadgeStyle(*query.BadgeStyle)
		switch kind {
		case "poster":
			s.PosterBadgeStyle = style
		case "logo":
			s.LogoBadgeStyle = style
		case "backdrop":
			s.BackdropBadgeStyle = style
		case "episode":
			s.EpisodeBadgeStyle = style
		}
	}

	if query.LabelStyle != nil {
		style := services.LabelStyle(*query.LabelStyle)
		switch kind {
		case "poster":
			s.PosterLabelStyle = style
		case "logo":
			s.LogoLabelStyle = style
		case "backdrop":
			s.BackdropLabelStyle = style
		case "episode":
			s.EpisodeLabelStyle = style
		}
	}

	if query.TextSize != nil {
		size := services.ClampScalePercent(*query.TextSize)
		switch kind {
		case "poster":
			s.PosterTextSize = size
		case "logo":
			s.LogoTextSize = size
		case "backdrop":
			s.BackdropTextSize = size
		case "episode":
			s.EpisodeTextSize = size
		}
	}

	if query.BadgeSize != nil {
		size := services.ClampScalePercent(*query.BadgeSize)
		switch kind {
		case "poster":
			s.PosterBadgeSize = size
		case "logo":
			s.LogoBadgeSize = size
		case "backdrop":
			s.BackdropBadgeSize = size
		case "episode":
			s.EpisodeBadgeSize = size
		}
	}

	if query.BadgeWidth != nil {
		width := services.ClampScalePercent(*query.BadgeWidth)
		switch kind {
		case "poster":
			s.PosterBadgeWidth = width
		case "logo":
			s.LogoBadgeWidth = width
		case "backdrop":
			s.BackdropBadgeWidth = width
		case "episode":
			s.EpisodeBadgeWidth = width
		}
	}

	if query.BadgeHeight != nil {
		height := services.ClampScalePercent(*query.BadgeHeight)
		switch kind {
		case "poster":
			s.PosterBadgeHeight = height
		case "logo":
			s.LogoBadgeHeight = height
		case "backdrop":
			s.BackdropBadgeHeight = height
		case "episode":
			s.EpisodeBadgeHeight = height
		}
	}

	if query.LogoSize != nil {
		size := services.ClampScalePercent(*query.LogoSize)
		switch kind {
		case "poster":
			s.PosterLogoSize = size
		case "logo":
			s.LogoLogoSize = size
		case "backdrop":
			s.BackdropLogoSize = size
		case "episode":
			s.EpisodeLogoSize = size
		}
	}

	if query.BadgeShape != nil {
		shape := services.BadgeShape(*query.BadgeShape)
		switch kind {
		case "poster":
			s.PosterBadgeShape = shape
		case "logo":
			s.LogoBadgeShape = shape
		case "backdrop":
			s.BackdropBadgeShape = shape
		case "episode":
			s.EpisodeBadgeShape = shape
		}
	}

	if query.BadgeAlpha != nil {
		alpha := services.ClampBadgeAlpha(*query.BadgeAlpha)
		switch kind {
		case "poster":
			s.PosterBadgeAlpha = alpha
		case "logo":
			s.LogoBadgeAlpha = alpha
		case "backdrop":
			s.BackdropBadgeAlpha = alpha
		case "episode":
			s.EpisodeBadgeAlpha = alpha
		}
	}

	if query.Layout != nil {
		def := services.DefaultLayout(kind)
		l := services.UnmarshalLayout(*query.Layout, &def)
		switch kind {
		case "poster":
			s.PosterLayout = l
		case "logo":
			s.LogoLayout = l
		case "backdrop":
			s.BackdropLayout = l
		case "episode":
			s.EpisodeLayout = l
		}
	}

	if kind == "poster" {
		if query.BadgeDirection != nil {
			s.PosterBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Fit != nil {
			s.PosterFit = services.PosterFit(*query.Fit)
		}
		if query.Textless != nil {
			s.Textless = *query.Textless
		}
	}

	if kind == "backdrop" {
		if query.BadgeDirection != nil {
			s.BackdropBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.EdgeInsetX != nil {
			s.BackdropEdgeInsetX = services.ClampEdgeInset(*query.EdgeInsetX)
		}
		if query.EdgeInsetY != nil {
			s.BackdropEdgeInsetY = services.ClampEdgeInset(*query.EdgeInsetY)
		}
	}

	if kind == "logo" {
		// no per-kind logo query params beyond the shared layout
	}

	if kind == "episode" {
		if query.BadgeDirection != nil {
			s.EpisodeBadgeDirection = services.BadgeDirection(*query.BadgeDirection)
		}
		if query.Blur != nil {
			s.EpisodeBlur = *query.Blur
		}
	}

	if query.ImageSource != nil {
		s.ImageSource = services.ImageSource(*query.ImageSource)
	}

	return &s
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
	if v := q.Get("ratings_order"); v != "" {
		query.RatingsOrder = &v
	}
	if v := q.Get("ratings_exclude"); v != "" {
		query.RatingsExclude = &v
	}
	if v := q.Get("badge_style"); v != "" {
		query.BadgeStyle = &v
	}
	if v := q.Get("label_style"); v != "" {
		query.LabelStyle = &v
	}
	if v := q.Get("badge_size"); v != "" {
		var n int32
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			query.BadgeSize = &n
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
		_ = v
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
