package handlers

import (
	"bytes"
	"database/sql"
	"image"
	"image/png"
	"net/http"
	"strconv"

	appimg "openposterdb/internal/image"
	"openposterdb/internal/services"
)

// HandleIcon serves a rating-source icon for the admin UI (e.g. the logo
// gallery on the settings page). kind is "default" or "official"; the key must
// be whitelisted by the image package.
func HandleIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "Method not allowed")
		return
	}
	if !appimg.ServeIcon(w, r.PathValue("kind"), r.PathValue("key")) {
		writeError(w, 404, "icon not found")
	}
}

// HandleBadgePreview renders a single rating badge (official logo + value +
// background) as a PNG for the settings page's badge gallery. It renders the
// badge exactly like the poster preview does — the sample badges filtered by
// the ratings order/exclude/limit, with uniform row widths — so the gallery
// matches the previews. `source` is the rating source key; `value` is the
// rating shown (for Rotten Tomatoes the value also selects the logo variant).
// Optional query params mirror the preview endpoint so live, unsaved form
// changes (colors, alpha, shape, label style, order) are reflected:
// colors, badge_alpha, badge_shape, label_style, ratings_order,
// ratings_exclude, ratings_limit.
func HandleBadgePreview(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, 405, "Method not allowed")
			return
		}
		src := services.SourceFromKey(r.URL.Query().Get("source"))
		if src == nil {
			writeError(w, 400, "unknown source")
			return
		}
		value := r.URL.Query().Get("value")
		if value == "" {
			writeError(w, 400, "missing value")
			return
		}

		globals, err := services.GetGlobalSettings(db)
		if err != nil {
			writeError(w, 500, "failed to load settings")
			return
		}
		settings := services.ParseGlobalRenderSettings(globals)

		q := r.URL.Query()
		shape := settings.PosterBadgeShape
		if v := q.Get("badge_shape"); v != "" {
			shape = services.BadgeShape(v)
		}
		alpha := settings.PosterBadgeAlpha
		if v := q.Get("badge_alpha"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 32); err == nil {
				alpha = services.ClampBadgeAlpha(int32(n))
			}
		}
		labelStyle := settings.PosterLabelStyle
		if v := q.Get("label_style"); v != "" {
			labelStyle = services.LabelStyle(v)
		}
		order := settings.RatingsOrder
		if v := q.Get("ratings_order"); v != "" {
			order = v
		}
		exclude := settings.RatingsExclude
		if v := q.Get("ratings_exclude"); v != "" {
			exclude = v
		}
		limit := settings.RatingsLimit
		if v := q.Get("ratings_limit"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 32); err == nil {
				limit = int32(n)
			}
		}
		colors := settings.Colors
		if parsed := parsePreviewColors(r); parsed != nil {
			colors = parsed
		}

		// Build the row the same way the poster preview does (sample badges
		// filtered by the order/exclude/limit) so uniform widths match.
		row := services.ApplyRatingPreferences(sampleBadges(), order, exclude, limit)
		found := false
		for i := range row {
			if row[i].Source == src {
				row[i].Value = value
				found = true
			}
		}
		if !found {
			row = append(row, services.RatingBadge{Source: src, Value: value})
		}

		valueFace := appimg.GetValueFontFace()
		labelFace := appimg.GetFontFace()
		if valueFace == nil || labelFace == nil {
			writeError(w, 500, "font not loaded")
			return
		}

		appearance := services.BadgeAppearance{Shape: shape, Alpha: alpha}
		rendered := appimg.RenderBadgesUniform(row, valueFace, labelFace, labelStyle, appearance, 1.0, 1.0, 1.0, colors)

		var target *image.RGBA
		for i, b := range row {
			if b.Source == src && b.Value == value {
				target = rendered[i]
				break
			}
		}
		if target == nil {
			writeError(w, 500, "render failed")
			return
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, target); err != nil {
			writeError(w, 500, "render failed")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(buf.Bytes())
	}
}
