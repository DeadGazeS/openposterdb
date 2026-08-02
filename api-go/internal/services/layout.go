package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SideSlot describes the badges placed on one side of an image. PerRow is the
// number of badges in each row, Rows is how many rows the side holds. Start is
// the anchor along the side: "l"/"c"/"r" for the top and bottom sides,
// "t"/"c"/"b" for the left and right sides.
type SideSlot struct {
	PerRow int32  `json:"per_row"`
	Rows   int32  `json:"rows"`
	Start  string `json:"start"`
}

// Count returns the number of badges this side can hold.
func (s SideSlot) Count() int32 {
	return s.PerRow * s.Rows
}

// IsOff reports whether the side holds no badges.
func (s SideSlot) IsOff() bool {
	return s.PerRow <= 0 || s.Rows <= 0
}

// ImageLayout arranges rating badges around an image. Each of the four sides
// holds a block of badges (per-row × rows), all laid out in horizontal rows.
// Badges fill the sides in the order given by Order, each side consuming up to
// its capacity from the (already preference-ordered) list. The total number of
// ratings shown is the sum of all four side capacities.
type ImageLayout struct {
	Top    SideSlot `json:"top"`
	Right  SideSlot `json:"right"`
	Bottom SideSlot `json:"bottom"`
	Left   SideSlot `json:"left"`
	Order  []string `json:"order"`
}

// SideNames lists the four sides in a canonical order.
var SideNames = []string{"top", "right", "bottom", "left"}

// DefaultFillOrder is the order badges fill the sides when the layout doesn't
// specify one.
var DefaultFillOrder = []string{"bottom", "top", "left", "right"}

// SideSlot returns the slot for a side name.
func (l *ImageLayout) SideSlot(side string) *SideSlot {
	switch side {
	case "top":
		return &l.Top
	case "right":
		return &l.Right
	case "bottom":
		return &l.Bottom
	case "left":
		return &l.Left
	}
	return nil
}

// Total returns the number of ratings the layout displays (sum of all sides).
func (l *ImageLayout) Total() int32 {
	if l == nil {
		return 0
	}
	return l.Top.Count() + l.Right.Count() + l.Bottom.Count() + l.Left.Count()
}

// IsEmpty reports whether no side has any badges.
func (l *ImageLayout) IsEmpty() bool {
	return l == nil || l.Total() == 0
}

// OrderOrDefault returns the fill order, falling back to the default.
func (l *ImageLayout) OrderOrDefault() []string {
	if l == nil || len(l.Order) == 0 {
		return DefaultFillOrder
	}
	return l.Order
}

// FillOrder returns a valid order string (comma-separated side names).
func (l *ImageLayout) FillOrder() string {
	return strings.Join(l.OrderOrDefault(), ",")
}

// SetFillOrder parses a comma-separated side order, keeping only valid sides.
func (l *ImageLayout) SetFillOrder(order string) {
	var out []string
	for _, s := range strings.Split(order, ",") {
		s = strings.TrimSpace(s)
		if s != "" && containsString(SideNames, s) && !containsString(out, s) {
			out = append(out, s)
		}
	}
	l.Order = out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// MarshalLayout serializes a layout to JSON.
func MarshalLayout(l *ImageLayout) (string, error) {
	if l == nil {
		l = &ImageLayout{}
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalLayout parses a layout from JSON, falling back to a default layout
// on any error or empty input.
func UnmarshalLayout(s string, def *ImageLayout) ImageLayout {
	if s == "" {
		if def == nil {
			return ImageLayout{}
		}
		return *def
	}
	var l ImageLayout
	if err := json.Unmarshal([]byte(s), &l); err != nil {
		if def != nil {
			return *def
		}
		return ImageLayout{}
	}
	return l
}

// DefaultLayouts return per-kind layouts that reproduce the historical output.
func DefaultLayouts() map[string]ImageLayout {
	return map[string]ImageLayout{
		"poster": {
			Bottom: SideSlot{PerRow: 3, Rows: 1, Start: "c"},
			Order:  append([]string(nil), DefaultFillOrder...),
		},
		"logo": {
			Bottom: SideSlot{PerRow: 5, Rows: 1, Start: "c"},
			Order:  append([]string(nil), DefaultFillOrder...),
		},
		"backdrop": {
			Top:   SideSlot{PerRow: 5, Rows: 1, Start: "r"},
			Order: append([]string(nil), DefaultFillOrder...),
		},
		"episode": {
			Right: SideSlot{PerRow: 1, Rows: 1, Start: "t"},
			Order: append([]string(nil), DefaultFillOrder...),
		},
	}
}

// DefaultLayout returns the default layout for a kind.
func DefaultLayout(kind string) ImageLayout {
	if defs := DefaultLayouts(); defs != nil {
		if l, ok := defs[kind]; ok {
			return l
		}
	}
	return ImageLayout{Order: append([]string(nil), DefaultFillOrder...)}
}

// ValidateLayout returns an error if the layout has invalid values.
func ValidateLayout(l *ImageLayout) error {
	if l == nil {
		return nil
	}
	checks := []struct {
		side string
		slot SideSlot
	}{
		{"top", l.Top},
		{"right", l.Right},
		{"bottom", l.Bottom},
		{"left", l.Left},
	}
	for _, c := range checks {
		if c.slot.PerRow < 0 || c.slot.PerRow > 10 {
			return fmt.Errorf("%s badges-per-row must be between 0 and 10", c.side)
		}
		if c.slot.Rows < 0 || c.slot.Rows > 10 {
			return fmt.Errorf("%s row count must be between 0 and 10", c.side)
		}
		if !c.slot.IsOff() && !validStart(c.side, c.slot.Start) {
			return fmt.Errorf("invalid %s start position '%s'", c.side, c.slot.Start)
		}
	}
	return nil
}

func validStart(side, start string) bool {
	switch side {
	case "top", "bottom":
		return start == "l" || start == "c" || start == "r"
	case "left", "right":
		return start == "t" || start == "c" || start == "b"
	}
	return false
}
