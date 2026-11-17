package services

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
)

// Conversion of the Rust-era badge options (position / direction / split,
// named badge_background and badge_size enums) into the current model
// (ImageLayout, badge alpha, badge size percent). Semantics follow the Rust
// renderer (image/generate.rs, services/db.rs BadgePosition):
//
//   - direction "d" resolves to horizontal rows for the centred positions
//     (bc, tc) and to a vertical column for every other position;
//   - horizontal rows hug the top or bottom edge for the corner/centre
//     positions and sit vertically centred on the left/right edge for "l"/"r";
//   - a vertical column hugs the left or right edge, or the top/bottom edge
//     (centred) for "tc"/"bc";
//   - split puts the first ceil(n/2) badges at the configured position and the
//     rest at its mirror: top/bottom for rows, left/right for a column.

// legacyDefaults are the Rust defaults per kind. A row still holding all of
// them was never customised, so it keeps the current default layout (and its
// cache keys) instead of a literal translation.
var legacyDefaults = map[string]struct {
	position  string
	direction string
	limit     int32
}{
	"poster":   {"bc", "d", 3},
	"logo":     {"bc", "d", 5},
	"backdrop": {"tr", "d", 5},
	"episode":  {"tr", "v", 1},
}

var legacyPositions = map[string]bool{"bc": true, "tc": true, "l": true, "r": true, "tl": true, "tr": true, "bl": true, "br": true}

func legacyIsTop(p string) bool    { return p == "tl" || p == "tc" || p == "tr" }
func legacyIsBottom(p string) bool { return p == "bl" || p == "bc" || p == "br" }
func legacyIsLeft(p string) bool   { return p == "l" || p == "tl" || p == "bl" }
func legacyIsRight(p string) bool  { return p == "r" || p == "tr" || p == "br" }

// legacySplitAnchors mirrors Rust BadgePosition::split_anchors.
func legacySplitAnchors(p string, topBottom bool) (primary, opposite string) {
	if topBottom {
		top, bottom := "tc", "bc"
		if legacyIsLeft(p) {
			top, bottom = "tl", "bl"
		} else if legacyIsRight(p) {
			top, bottom = "tr", "br"
		}
		if legacyIsTop(p) {
			return top, bottom
		}
		return bottom, top
	}
	left, right := "l", "r"
	if legacyIsTop(p) {
		left, right = "tl", "tr"
	} else if legacyIsBottom(p) {
		left, right = "bl", "br"
	}
	if legacyIsRight(p) {
		return right, left
	}
	return left, right
}

// legacyGroupSlot places n badges at a legacy position.
func legacyGroupSlot(p string, vertical bool, n int32) (string, SideSlot) {
	if vertical {
		start := "c"
		if legacyIsTop(p) {
			start = "t"
		} else if legacyIsBottom(p) {
			start = "b"
		}
		slot := SideSlot{PerRow: 1, Rows: n, Start: start}
		switch {
		case legacyIsLeft(p):
			return "left", slot
		case legacyIsRight(p):
			return "right", slot
		case p == "tc":
			return "top", SideSlot{PerRow: 1, Rows: n, Start: "c"}
		default:
			return "bottom", SideSlot{PerRow: 1, Rows: n, Start: "c"}
		}
	}
	// Horizontal rows. One row keeps the badge count exact; above five the
	// row wraps once (the renderer scales an over-wide block down anyway).
	perRow, rows := n, int32(1)
	if n > 5 {
		rows = 2
		perRow = (n + 1) / 2
	}
	slot := SideSlot{PerRow: perRow, Rows: rows}
	switch {
	case legacyIsTop(p) || legacyIsBottom(p):
		slot.Start = "c"
		if legacyIsLeft(p) {
			slot.Start = "l"
		} else if legacyIsRight(p) {
			slot.Start = "r"
		}
		if legacyIsTop(p) {
			return "top", slot
		}
		return "bottom", slot
	case p == "l":
		slot.Start = "c"
		return "left", slot
	default: // "r"
		slot.Start = "c"
		return "right", slot
	}
}

// LayoutFromLegacy converts the Rust-era position / direction / split options
// and the kind's ratings limit into an ImageLayout. Unknown values fall back
// to the kind's Rust default.
func LayoutFromLegacy(kind, position, direction string, split bool, ratingsLimit int32) ImageLayout {
	def, ok := legacyDefaults[kind]
	if !ok {
		return DefaultLayout(kind)
	}
	if !legacyPositions[position] {
		position = def.position
	}
	if direction != "h" && direction != "v" && direction != "d" {
		direction = def.direction
	}
	if ratingsLimit > 10 {
		ratingsLimit = 10
	}
	if position == def.position && direction == def.direction && !split && ratingsLimit == def.limit {
		return DefaultLayout(kind)
	}

	var layout ImageLayout
	if ratingsLimit <= 0 {
		layout.Order = append([]string(nil), DefaultFillOrder...)
		return layout
	}

	vertical := direction == "v" || (direction == "d" && position != "bc" && position != "tc")

	var order []string
	place := func(p string, n int32) {
		side, slot := legacyGroupSlot(p, vertical, n)
		*layout.SideSlot(side) = slot
		order = append(order, side)
	}
	if split && ratingsLimit >= 2 {
		primary, opposite := legacySplitAnchors(position, !vertical)
		first := (ratingsLimit + 1) / 2
		place(primary, first)
		place(opposite, ratingsLimit-first)
	} else {
		place(position, ratingsLimit)
	}
	for _, s := range DefaultFillOrder {
		if !containsString(order, s) {
			order = append(order, s)
		}
	}
	layout.Order = order
	return layout
}

// AlphaFromLegacyBackground maps the Rust badge_background enum to a badge
// alpha: d (default look) → the default alpha, k (dark, 200/255) → 78,
// t (transparent, 120/255) → 47, n (none) → 0.
func AlphaFromLegacyBackground(bg string) (BadgeAlpha, bool) {
	switch bg {
	case "d":
		return DefaultBadgeAlpha(), true
	case "k":
		return 78, true
	case "t":
		return 47, true
	case "n":
		return 0, true
	}
	return 0, false
}

// legacySizeScale is the Rust BadgeSize::scale_factor.
var legacySizeScale = map[string]float64{"xs": 0.7, "s": 0.95, "m": 1.2, "l": 1.45, "xl": 1.7}

// PercentFromLegacySize maps the Rust badge_size enum to a badge size percent.
// The percent multiplies the kind's default scale (1.2, or 1.45 for episodes,
// see image/serve.go), so "m" is 100% for posters and "l" is 100% for
// episodes — the Rust defaults of each kind.
func PercentFromLegacySize(kind, size string) (ScalePercent, bool) {
	scale, ok := legacySizeScale[size]
	if !ok {
		return 0, false
	}
	base := 1.2
	if kind == "episode" {
		base = 1.45
	}
	return ClampScalePercent(int32(math.Round(scale / base * 100))), true
}

var legacyLimitColumn = map[string]string{
	"poster":   "ratings_limit",
	"logo":     "logo_ratings_limit",
	"backdrop": "backdrop_ratings_limit",
	"episode":  "episode_ratings_limit",
}

var badgeKinds = []string{"poster", "logo", "backdrop", "episode"}

// ConvertLegacyBadgeBackgroundsCtx writes *_badge_alpha from the legacy
// *_badge_background columns of api_key_settings. It must run before the
// migrations that drop those columns. Only rows whose alpha still holds the
// column default are touched, so it is idempotent and never overrides a value
// set in the current UI. Missing columns make it a no-op.
func ConvertLegacyBadgeBackgroundsCtx(ctx context.Context, db *sql.DB) error {
	cols, err := tableColumnTypes(ctx, db, "api_key_settings")
	if err != nil {
		return err
	}
	var converted int64
	for _, kind := range badgeKinds {
		bgCol, alphaCol := kind+"_badge_background", kind+"_badge_alpha"
		if _, ok := cols[bgCol]; !ok {
			continue
		}
		if _, ok := cols[alphaCol]; !ok {
			continue
		}
		for _, bg := range []string{"k", "t", "n"} {
			alpha, _ := AlphaFromLegacyBackground(bg)
			res, err := db.ExecContext(ctx,
				fmt.Sprintf("UPDATE api_key_settings SET %s = ? WHERE %s = ? AND %s = ?", alphaCol, bgCol, alphaCol),
				int32(alpha), bg, int32(DefaultBadgeAlpha()))
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			converted += n
		}
	}
	if converted > 0 {
		slog.Info("converted legacy badge backgrounds to alpha", "values", converted)
	}
	return nil
}

// ConvertLegacyBadgeLayoutsCtx writes *_layout from the legacy *_position,
// *_badge_direction and *_badge_split columns of api_key_settings. It must run
// after the *_layout columns are added and before the position/split columns
// are dropped. Rows whose layout is no longer the kind default are left alone.
func ConvertLegacyBadgeLayoutsCtx(ctx context.Context, db *sql.DB) error {
	cols, err := tableColumnTypes(ctx, db, "api_key_settings")
	if err != nil {
		return err
	}
	var converted int
	for _, kind := range badgeKinds {
		posCol, layoutCol, limitCol := kind+"_position", kind+"_layout", legacyLimitColumn[kind]
		_, hasPos := cols[posCol]
		_, hasLayout := cols[layoutCol]
		_, hasLimit := cols[limitCol]
		if !hasPos || !hasLayout || !hasLimit {
			continue
		}
		dirExpr := "'d'"
		if _, ok := cols[kind+"_badge_direction"]; ok {
			dirExpr = kind + "_badge_direction"
		}
		splitExpr := "0"
		if _, ok := cols[kind+"_badge_split"]; ok {
			splitExpr = kind + "_badge_split"
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(
			"SELECT api_key_id, %s, %s, %s, %s, %s FROM api_key_settings",
			posCol, dirExpr, splitExpr, limitCol, layoutCol))
		if err != nil {
			return err
		}
		type update struct {
			id     int64
			layout string
		}
		var updates []update
		def := DefaultLayout(kind)
		for rows.Next() {
			var id int64
			var pos, dir, layoutJSON string
			var split bool
			var limit int32
			if err := rows.Scan(&id, &pos, &dir, &split, &limit, &layoutJSON); err != nil {
				rows.Close()
				return err
			}
			current := UnmarshalLayout(layoutJSON, &def)
			if !sameLayout(&current, &def) {
				continue // already customised in the current model
			}
			conv := LayoutFromLegacy(kind, pos, dir, split, limit)
			if sameLayout(&conv, &def) {
				continue
			}
			s, err := MarshalLayout(&conv)
			if err != nil {
				rows.Close()
				return err
			}
			updates = append(updates, update{id, s})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, u := range updates {
			if _, err := db.ExecContext(ctx,
				fmt.Sprintf("UPDATE api_key_settings SET %s = ? WHERE api_key_id = ?", layoutCol), u.layout, u.id); err != nil {
				return err
			}
		}
		converted += len(updates)
	}
	if converted > 0 {
		slog.Info("converted legacy badge positions to layouts", "rows", converted)
	}
	return nil
}

// sameLayout compares layouts by what they render: a side that holds no
// badges matches any other empty side whatever its start anchor (the column
// defaults store "c" there, DefaultLayout leaves it empty).
func sameLayout(a, b *ImageLayout) bool {
	norm := func(l *ImageLayout) ImageLayout {
		out := ImageLayout{Order: l.OrderOrDefault()}
		for _, side := range SideNames {
			if slot := l.SideSlot(side); slot != nil && !slot.IsOff() {
				*out.SideSlot(side) = *slot
			}
		}
		return out
	}
	na, nb := norm(a), norm(b)
	return layoutEqual(&na, &nb)
}

// applyLegacyGlobals repairs render settings parsed from global_settings that
// still hold only Rust-era keys. Global settings are never dropped, so this
// also fixes installs that already moved to Go. Each conversion only applies
// when the current key is absent.
func applyLegacyGlobals(globals map[string]string, s *RenderSettings) {
	for _, kind := range badgeKinds {
		if _, ok := globals[kind+"_layout"]; !ok {
			pos, hasPos := globals[kind+"_position"]
			splitStr, hasSplit := globals[kind+"_badge_split"]
			if hasPos || hasSplit {
				def := legacyDefaults[kind]
				if !hasPos {
					pos = def.position
				}
				dir, ok := globals[kind+"_badge_direction"]
				if !ok {
					dir = def.direction
				}
				*s.layoutPtr(kind) = LayoutFromLegacy(kind, pos, dir, splitStr == "true" || splitStr == "1", s.ratingsLimitFor(kind))
			}
		}
		if _, ok := globals[kind+"_badge_alpha"]; !ok {
			if a, ok := AlphaFromLegacyBackground(globals[kind+"_badge_background"]); ok {
				*s.badgeAlphaPtr(kind) = a
			}
		}
		if p, ok := PercentFromLegacySize(kind, globals[kind+"_badge_size"]); ok {
			*s.badgeSizePtr(kind) = p
		}
	}
}

func (s *RenderSettings) layoutPtr(kind string) *ImageLayout {
	switch kind {
	case "logo":
		return &s.LogoLayout
	case "backdrop":
		return &s.BackdropLayout
	case "episode":
		return &s.EpisodeLayout
	}
	return &s.PosterLayout
}

func (s *RenderSettings) ratingsLimitFor(kind string) int32 {
	switch kind {
	case "logo":
		return s.LogoRatingsLimit
	case "backdrop":
		return s.BackdropRatingsLimit
	case "episode":
		return s.EpisodeRatingsLimit
	}
	return s.RatingsLimit
}

func (s *RenderSettings) badgeAlphaPtr(kind string) *BadgeAlpha {
	switch kind {
	case "logo":
		return &s.LogoBadgeAlpha
	case "backdrop":
		return &s.BackdropBadgeAlpha
	case "episode":
		return &s.EpisodeBadgeAlpha
	}
	return &s.PosterBadgeAlpha
}

func (s *RenderSettings) badgeSizePtr(kind string) *ScalePercent {
	switch kind {
	case "logo":
		return &s.LogoBadgeSize
	case "backdrop":
		return &s.BackdropBadgeSize
	case "episode":
		return &s.EpisodeBadgeSize
	}
	return &s.PosterBadgeSize
}
