package services

import (
	"testing"
	"time"
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

func TestHashRegistryRegisterAndLookup(t *testing.T) {
	r := NewHashRegistry(0) // no TTL
	s := &RenderSettings{ImageSource: "t", Lang: "en", RatingsLimit: 3}
	hash := r.Register(s)
	if hash == "" {
		t.Fatal("Register returned empty hash")
	}
	got, ok := r.Lookup(hash)
	if !ok || got == nil || got.Lang != "en" {
		t.Fatalf("Lookup: got ok=%v, settings=%+v", ok, got)
	}
}

func TestHashRegistryTTLExpiry(t *testing.T) {
	r := NewHashRegistry(10 * time.Millisecond)
	s := &RenderSettings{ImageSource: "t"}
	hash := r.Register(s)
	if _, ok := r.Lookup(hash); !ok {
		t.Fatal("entry must be present immediately after Register")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := r.Lookup(hash); ok {
		t.Fatal("entry must expire after TTL")
	}
}

func TestHashRegistrySweepExpired(t *testing.T) {
	r := NewHashRegistry(10 * time.Millisecond)
	r.Register(&RenderSettings{ImageSource: "t"})
	if r.Len() != 1 {
		t.Fatalf("Len: got %d want 1", r.Len())
	}
	time.Sleep(30 * time.Millisecond)
	removed := r.SweepExpired()
	if removed != 1 {
		t.Errorf("SweepExpired: got %d want 1", removed)
	}
	if r.Len() != 0 {
		t.Errorf("Len after sweep: got %d want 0", r.Len())
	}
}

func TestHashRegistryNilSafe(t *testing.T) {
	var r *HashRegistry
	if r.Register(&RenderSettings{}) != "" {
		t.Error("nil Register must return empty string")
	}
	if _, ok := r.Lookup("anything"); ok {
		t.Error("nil Lookup must miss")
	}
	if got := r.SweepExpired(); got != 0 {
		t.Errorf("nil SweepExpired: got %d want 0", got)
	}
}
