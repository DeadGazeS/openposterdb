package handlers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

// distinctSettings fills every APIKeySettings field (except api_key_id)
// with a value derived from its json tag and a salt, so any field the merge
// takes from the wrong side shows up.
func distinctSettings(t *testing.T, salt string, n int64) services.APIKeySettings {
	t.Helper()
	var s services.APIKeySettings
	v := reflect.ValueOf(&s).Elem()
	for i := 0; i < v.NumField(); i++ {
		tag := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
		if tag == "api_key_id" {
			continue
		}
		switch f := v.Field(i); f.Kind() {
		case reflect.String:
			f.SetString(salt + "_" + tag)
		case reflect.Int32, reflect.Int64:
			f.SetInt(n + int64(i))
		case reflect.Bool:
			f.SetBool(n%2 == 1)
		default:
			t.Fatalf("unhandled kind %s for %s", f.Kind(), tag)
		}
	}
	return s
}

// TestMergeKeySettings_EveryField: for every per-key field, an omitted key
// keeps the stored value and a sent key (even 0 / false / "") wins — no
// per-field code, so new fields are covered automatically.
func TestMergeKeySettings_EveryField(t *testing.T) {
	base := distinctSettings(t, "base", 1)

	// Payload omits everything → the stored row comes back unchanged.
	var empty keySettingsUpdate
	if err := json.Unmarshal([]byte(`{}`), &empty); err != nil {
		t.Fatal(err)
	}
	if got := mergeKeySettingsUpdate(&base, &empty); !reflect.DeepEqual(got, base) {
		t.Errorf("empty payload changed the stored row:\n got %+v\nwant %+v", got, base)
	}

	// Payload sends everything → every field takes the payload value.
	sent := distinctSettings(t, "sent", 2)
	data, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	var full keySettingsUpdate
	if err := json.Unmarshal(data, &full); err != nil {
		t.Fatal(err)
	}
	if got := mergeKeySettingsUpdate(&base, &full); !reflect.DeepEqual(got, sent) {
		t.Errorf("full payload not applied:\n got %+v\nwant %+v", got, sent)
	}

	// Explicit zero values are honoured, not treated as omitted.
	var zeros keySettingsUpdate
	if err := json.Unmarshal([]byte(`{"ratings_limit":0,"textless":false,"lang":""}`), &zeros); err != nil {
		t.Fatal(err)
	}
	got := mergeKeySettingsUpdate(&base, &zeros)
	if got.RatingsLimit != 0 || got.Textless || got.Lang != "" {
		t.Errorf("explicit zeros not honoured: limit=%d textless=%v lang=%q", got.RatingsLimit, got.Textless, got.Lang)
	}
	if got.PosterBadgeStyle != base.PosterBadgeStyle {
		t.Errorf("omitted poster_badge_style = %q, want stored %q", got.PosterBadgeStyle, base.PosterBadgeStyle)
	}
}
