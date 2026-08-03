package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// layoutFields returns the four layout fields of s for table-driven checks.
func layoutFields(s *APIKeySettings) map[string]string {
	return map[string]string{
		"poster_layout":   s.PosterLayout,
		"logo_layout":     s.LogoLayout,
		"backdrop_layout": s.BackdropLayout,
		"episode_layout":  s.EpisodeLayout,
	}
}

func TestAPIKeySettingsUnmarshalLayoutObjects(t *testing.T) {
	payload := `{
		"image_source": "t",
		"lang": "de",
		"ratings_limit": 5,
		"poster_layout":   {"top": {"per_row": 2, "rows": 1, "start": "c"}, "order": ["top", "left"]},
		"logo_layout":     {"bottom": {"per_row": 4, "rows": 2, "start": "l"}},
		"backdrop_layout": {"right": {"per_row": 1, "rows": 3, "start": "t"}},
		"episode_layout":  {"left": {"per_row": 2, "rows": 1, "start": "b"}}
	}`

	var s APIKeySettings
	if err := json.Unmarshal([]byte(payload), &s); err != nil {
		t.Fatalf("unmarshal object-form payload: %v", err)
	}
	if s.ImageSource != "t" || s.Lang != "de" || s.RatingsLimit != 5 {
		t.Errorf("scalar fields not decoded: %+v", s)
	}
	for name, layout := range layoutFields(&s) {
		if layout == "" {
			t.Errorf("%s should be non-empty", name)
			continue
		}
		var l ImageLayout
		if err := json.Unmarshal([]byte(layout), &l); err != nil {
			t.Errorf("%s value %q does not unmarshal into ImageLayout: %v", name, layout, err)
		}
	}

	// The stored JSON must be a valid layout preserving full object detail.
	var poster ImageLayout
	if err := json.Unmarshal([]byte(s.PosterLayout), &poster); err != nil {
		t.Fatalf("poster_layout invalid: %v", err)
	}
	if poster.Top.PerRow != 2 || poster.Top.Rows != 1 || poster.Top.Start != "c" {
		t.Errorf("poster top slot not preserved: %+v", poster.Top)
	}
	if len(poster.Order) != 2 || poster.Order[0] != "top" || poster.Order[1] != "left" {
		t.Errorf("poster order not preserved: %v", poster.Order)
	}
}

func TestAPIKeySettingsUnmarshalLegacyStringLayouts(t *testing.T) {
	poster := `{"top":{"per_row":3,"rows":1,"start":"l"}}`
	logo := `{"bottom":{"per_row":5,"rows":1,"start":"c"}}`
	backdrop := `{"right":{"per_row":2,"rows":1,"start":"t"}}`
	episode := `{"left":{"per_row":1,"rows":2,"start":"b"}}`

	payload, err := json.Marshal(map[string]interface{}{
		"image_source":    "f",
		"poster_layout":   poster,
		"logo_layout":     logo,
		"backdrop_layout": backdrop,
		"episode_layout":  episode,
	})
	if err != nil {
		t.Fatal(err)
	}

	var s APIKeySettings
	if err := json.Unmarshal(payload, &s); err != nil {
		t.Fatalf("unmarshal legacy string-form payload: %v", err)
	}
	if s.ImageSource != "f" {
		t.Errorf("image_source not decoded: %+v", s)
	}
	want := map[string]string{
		"poster_layout":   poster,
		"logo_layout":     logo,
		"backdrop_layout": backdrop,
		"episode_layout":  episode,
	}
	for name, layout := range layoutFields(&s) {
		if layout != want[name] {
			t.Errorf("%s = %q, want %q", name, layout, want[name])
		}
	}
}

func TestAPIKeySettingsUnmarshalWithoutLayouts(t *testing.T) {
	var s APIKeySettings
	if err := json.Unmarshal([]byte(`{"image_source": "t", "lang": "de"}`), &s); err != nil {
		t.Fatalf("unmarshal without layout fields: %v", err)
	}
	if s.ImageSource != "t" || s.Lang != "de" {
		t.Errorf("scalar fields not decoded: %+v", s)
	}
	for name, layout := range layoutFields(&s) {
		if layout != "" {
			t.Errorf("%s should be empty when absent, got %q", name, layout)
		}
	}
}

// TestExportPayloadAcceptsObjectFormKeyLayouts exercises the settings
// export/import decode path with object-form layouts inside ExportedAPIKey
// settings — a fresh export that somehow emitted objects must still import.
func TestExportPayloadAcceptsObjectFormKeyLayouts(t *testing.T) {
	raw := `{
		"kind": "openposterdb/settings",
		"version": 1,
		"settings": {},
		"api_keys": [{
			"name": "k1",
			"settings": {
				"image_source": "t",
				"poster_layout":   {"bottom": {"per_row": 3, "rows": 1, "start": "c"}},
				"logo_layout":     {"bottom": {"per_row": 5, "rows": 1, "start": "c"}},
				"backdrop_layout": {"top": {"per_row": 5, "rows": 1, "start": "r"}},
				"episode_layout":  {"right": {"per_row": 1, "rows": 1, "start": "t"}}
			}
		}]
	}`

	var p ExportPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("export payload with object layouts: %v", err)
	}
	if len(p.APIKeys) != 1 || p.APIKeys[0].Settings == nil {
		t.Fatalf("api key settings not decoded: %+v", p.APIKeys)
	}
	s := p.APIKeys[0].Settings
	for name, layout := range layoutFields(s) {
		if layout == "" {
			t.Errorf("%s should be non-empty", name)
			continue
		}
		var l ImageLayout
		if err := json.Unmarshal([]byte(layout), &l); err != nil {
			t.Errorf("%s value %q does not unmarshal into ImageLayout: %v", name, layout, err)
		}
	}

	// And the legacy string form must still import through the same path.
	legacy := strings.ReplaceAll(raw,
		`"poster_layout":   {"bottom": {"per_row": 3, "rows": 1, "start": "c"}}`,
		`"poster_layout": "{\"bottom\":{\"per_row\":3,\"rows\":1,\"start\":\"c\"}}"`)
	var p2 ExportPayload
	if err := json.Unmarshal([]byte(legacy), &p2); err != nil {
		t.Fatalf("export payload with legacy string layout: %v", err)
	}
	if p2.APIKeys[0].Settings == nil {
		t.Fatal("settings nil after legacy decode")
	}
	if p2.APIKeys[0].Settings.PosterLayout != `{"bottom":{"per_row":3,"rows":1,"start":"c"}}` {
		t.Errorf("legacy poster_layout not preserved: %q", p2.APIKeys[0].Settings.PosterLayout)
	}
}
