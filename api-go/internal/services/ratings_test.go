package services

import (
	"testing"
)

func TestRatingSourceKeys(t *testing.T) {
	sources := []*RatingSource{SourceMal, SourceImdb, SourceLetterboxd, SourceRt, SourceRtAudience,
		SourceMetacritic, SourceTmdb, SourceTrakt, SourceMdblist, SourceEbert}

	for _, s := range sources {
		if found := SourceFromKey(s.Key); found != s {
			t.Errorf("SourceFromKey(%q) returned wrong source", s.Key)
		}
		if found := SourceFromCacheChar(s.CacheChar); found != s {
			t.Errorf("SourceFromCacheChar(%c) returned wrong source", s.CacheChar)
		}
	}
}

func TestRatingSourceLabels(t *testing.T) {
	if SourceImdb.Label != "IMDb" {
		t.Error("wrong IMDb label")
	}
	if SourceTmdb.Label != "TMDB" {
		t.Error("wrong TMDB label")
	}
}

func TestCacheCharUnique(t *testing.T) {
	seen := make(map[byte]bool)
	for _, s := range allSources {
		if seen[s.CacheChar] {
			t.Errorf("duplicate cache char %c", s.CacheChar)
		}
		seen[s.CacheChar] = true
	}
}

func TestBadgesCacheSuffix(t *testing.T) {
	badges := []RatingBadge{
		{Source: SourceImdb, Value: "8.0"},
		{Source: SourceLetterboxd, Value: "4.2"},
		{Source: SourceRt, Value: "95%"},
	}
	if s := BadgesCacheSuffix(badges); s != "@ilr" {
		t.Errorf("expected @ilr, got %s", s)
	}
}

func TestBadgesCacheSuffixEmpty(t *testing.T) {
	if s := BadgesCacheSuffix(nil); s != "@" {
		t.Errorf("expected @, got %s", s)
	}
}

func TestRatingsCacheSuffixDefault(t *testing.T) {
	suffix := RatingsCacheSuffix("mal,imdb,lb,rt,rta,mc,tmdb,trakt", "", 3)
	if suffix != "@mil" {
		t.Errorf("expected @mil, got %s", suffix)
	}
}

func TestRatingsCacheSuffixCustomOrder(t *testing.T) {
	suffix := RatingsCacheSuffix("trakt,imdb,rt", "", 3)
	if suffix != "@kir" {
		t.Errorf("expected @kir, got %s", suffix)
	}
}

func TestRatingsCacheSuffixLimitZero(t *testing.T) {
	suffix := RatingsCacheSuffix("imdb,rt", "", 0)
	if suffix != "@" {
		t.Errorf("expected @, got %s", suffix)
	}
}

func TestRatingsCacheSuffixExcludeChangesSuffix(t *testing.T) {
	none := RatingsCacheSuffix("imdb,tmdb,rt", "", 3)
	excl := RatingsCacheSuffix("imdb,tmdb,rt", "rt", 3)
	if none == excl {
		t.Error("excluding a source must change the cache suffix")
	}
}

func TestApplyRatingPreferencesReorder(t *testing.T) {
	badges := []RatingBadge{
		{Source: SourceImdb, Value: "8.0"},
		{Source: SourceTmdb, Value: "75%"},
		{Source: SourceTrakt, Value: "80%"},
	}
	result := ApplyRatingPreferences(badges, "trakt,imdb", "", 8)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0].Source != SourceTrakt {
		t.Error("first should be Trakt")
	}
	if result[1].Source != SourceImdb {
		t.Error("second should be IMDb")
	}
}

func TestApplyRatingPreferencesLimit(t *testing.T) {
	badges := []RatingBadge{
		{Source: SourceImdb, Value: "8.0"},
		{Source: SourceTmdb, Value: "75%"},
		{Source: SourceTrakt, Value: "80%"},
	}
	result := ApplyRatingPreferences(badges, "", "", 2)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestApplyRatingPreferencesExcludesSource(t *testing.T) {
	badges := []RatingBadge{
		{Source: SourceImdb, Value: "8.0"},
		{Source: SourceRt, Value: "95%"},
		{Source: SourceTmdb, Value: "75%"},
	}
	result := ApplyRatingPreferences(badges, "imdb,rt,tmdb", "rt", 8)
	for _, b := range result {
		if b.Source == SourceRt {
			t.Error("RT should have been excluded")
		}
	}
}

func TestAvailableSourcesString(t *testing.T) {
	badges := []RatingBadge{
		{Source: SourceTmdb, Value: "7.5"},
		{Source: SourceImdb, Value: "8.0"},
		{Source: SourceRt, Value: "95%"},
	}
	// Canonical order: mal, imdb, lb, rt, rta, mc, tmdb, trakt
	if s := AvailableSourcesString(badges); s != "irt" {
		t.Errorf("expected irt, got %s", s)
	}
}

func TestExcludeCacheToken(t *testing.T) {
	if tkn := ExcludeCacheToken(""); tkn != "" {
		t.Errorf("expected empty, got %s", tkn)
	}
	if tkn := ExcludeCacheToken("trakt"); tkn != "k" {
		t.Errorf("expected k, got %s", tkn)
	}
	if tkn := ExcludeCacheToken("rt,tmdb"); tkn != ExcludeCacheToken("tmdb,rt") {
		t.Error("order-independent exclude tokens should match")
	}
}

func TestTraktBadge(t *testing.T) {
	b := TraktBadge(7.5, 1200)
	if b == nil {
		t.Fatal("expected badge")
	}
	if b.Value != "75%" {
		t.Errorf("expected 75%%, got %s", b.Value)
	}
}

func TestTraktBadgeSuppressesZero(t *testing.T) {
	if b := TraktBadge(0.0, 1000); b != nil {
		t.Error("zero rating should produce nil")
	}
	if b := TraktBadge(7.5, 0); b != nil {
		t.Error("zero votes should produce nil")
	}
}
