package services

import (
	"testing"
)

func TestSettingsHashDeterministic(t *testing.T) {
	a := &RenderSettings{
		ImageSource: "t", Lang: "en", Textless: false,
		RatingsLimit: 3, RatingsOrder: "imdb,rt", RatingsExclude: "",
		PosterTextSize: 100, PosterBadgeAlpha: 80,
	}
	b := &RenderSettings{
		ImageSource: "t", Lang: "en", Textless: false,
		RatingsLimit: 3, RatingsOrder: "imdb,rt", RatingsExclude: "",
		PosterTextSize: 100, PosterBadgeAlpha: 80,
	}
	if SettingsHash(a) != SettingsHash(b) {
		t.Fatal("same settings must produce the same hash")
	}
}

func TestSettingsHashDiffersOnChange(t *testing.T) {
	a := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 3}
	b := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 5}
	if SettingsHash(a) == SettingsHash(b) {
		t.Fatal("different ratings_limit must change the hash")
	}
}

func TestSettingsHashKindIndependent(t *testing.T) {
	// The hash must be stable across image kinds so a CDN edge sees one entry
	// per settings+image regardless of which kind URL was requested.
	a := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 3}
	b := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 3}
	if SettingsHash(a) != SettingsHash(b) {
		t.Fatal("hashes must be kind-independent")
	}
}

func TestSettingsHashLength(t *testing.T) {
	// The CDN URL embeds the hash; keep it short (32 hex chars = 128 bits).
	s := &RenderSettings{ImageSource: "t", Lang: "en"}
	if h := SettingsHash(s); len(h) != 32 {
		t.Errorf("hash length: got %d want 32", len(h))
	}
}

func TestSettingsHashNil(t *testing.T) {
	if SettingsHash(nil) != "" {
		t.Fatal("nil settings must yield empty hash")
	}
}

func TestHashRegistryRegisterReturnsSettingsHash(t *testing.T) {
	r := NewHashRegistry()
	s := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 3}
	if got := r.Register(s); got == "" || got != SettingsHash(s) {
		t.Fatalf("Register = %q, want SettingsHash %q", got, SettingsHash(s))
	}
}

func TestHashRegistryServedIndex(t *testing.T) {
	r := NewHashRegistry()
	r.RecordServed("k", CDNServed{CacheKey: "ck"})
	if got, ok := r.LookupServed("k"); !ok || got.CacheKey != "ck" {
		t.Fatalf("LookupServed: %v %+v", ok, got)
	}
	if _, ok := r.LookupServed("other"); ok {
		t.Fatal("unknown key must miss")
	}
}

func TestHashRegistryNilSafe(t *testing.T) {
	var r *HashRegistry
	if r.Register(&RenderSettings{}) != "" {
		t.Error("nil Register must return empty string")
	}
	if _, ok := r.LookupServed("anything"); ok {
		t.Error("nil LookupServed must miss")
	}
	if got := r.SweepExpired(); got != 0 {
		t.Errorf("nil SweepExpired: got %d want 0", got)
	}
}
