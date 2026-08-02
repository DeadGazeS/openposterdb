package services

import (
	"testing"
)

func TestValidateIDValueRejectsEmpty(t *testing.T) {
	if ValidateIDValue("") == nil {
		t.Error("empty should fail")
	}
	if ValidateIDValue(".") == nil {
		t.Error("dot should fail")
	}
	if ValidateIDValue("..") == nil {
		t.Error("dot-dot should fail")
	}
}

func TestValidateIDValueRejectsTraversal(t *testing.T) {
	if ValidateIDValue("../../etc/passwd") == nil {
		t.Error("path traversal should fail")
	}
	if ValidateIDValue("foo/bar") == nil {
		t.Error("slash should fail")
	}
	if ValidateIDValue("foo\\bar") == nil {
		t.Error("backslash should fail")
	}
	if ValidateIDValue("foo\x00bar") == nil {
		t.Error("null byte should fail")
	}
}

func TestValidateIDValueAcceptsValid(t *testing.T) {
	if ValidateIDValue("tt1234567") != nil {
		t.Error("valid id should pass")
	}
	if ValidateIDValue("episode-1396-S1E1") != nil {
		t.Error("episode id should pass")
	}
	if ValidateIDValue("movie-550") != nil {
		t.Error("movie id should pass")
	}
}

func TestTitleFileMatchAnchorsOnDelimiter(t *testing.T) {
	if !TitleFileMatch("tt123@imc", "tt123") {
		t.Error("ratings suffix match")
	}
	if !TitleFileMatch("tt123_t_de@imc", "tt123") {
		t.Error("variant match")
	}
	if !TitleFileMatch("tt123_l_t_en@i", "tt123") {
		t.Error("logo variant match")
	}
	if !TitleFileMatch("tt123_b_t@i.p1", "tt123") {
		t.Error("backdrop variant match")
	}
	if !TitleFileMatch("tt123", "tt123") {
		t.Error("bare match")
	}
}

func TestTitleFileMatchRejectsSiblingPrefix(t *testing.T) {
	if TitleFileMatch("tt1234567@imc", "tt123") {
		t.Error("should not match longer id")
	}
	if TitleFileMatch("movie-123@i", "movie-12") {
		t.Error("should not match shorter id")
	}
}

func TestComputeStaleSecsNoReleaseDate(t *testing.T) {
	result := ComputeStaleSecs("", 86400, 31536000)
	if result != 86400 {
		t.Errorf("expected 86400, got %d", result)
	}
}

func TestComputeStaleSecsInvalidDate(t *testing.T) {
	result := ComputeStaleSecs("not-a-date", 86400, 31536000)
	if result != 86400 {
		t.Errorf("expected 86400, got %d", result)
	}
}

func TestComputeStaleSecsFutureFilm(t *testing.T) {
	result := ComputeStaleSecs("2099-01-01", 86400, 31536000)
	if result != 86400 {
		t.Errorf("expected 86400, got %d", result)
	}
}

func TestComputeStaleSecsOldFilm(t *testing.T) {
	result := ComputeStaleSecs("2000-01-01", 86400, 31536000)
	if result != 0 {
		t.Errorf("expected 0 (never stale), got %d", result)
	}
}

func TestDateStrToEpochKnownValue(t *testing.T) {
	if dateStrToEpoch("1970-01-02") != 86400 {
		t.Error("1970-01-02 should be 86400")
	}
	if dateStrToEpoch("1970-01-01") != 0 {
		t.Error("1970-01-01 should be 0")
	}
}

func TestDateStrToEpochInvalidMonth(t *testing.T) {
	if dateStrToEpoch("2020-13-01") != 0 {
		t.Error("invalid month should return 0")
	}
}

func TestDateStrToEpochInvalidDay(t *testing.T) {
	if dateStrToEpoch("2020-01-32") != 0 {
		t.Error("invalid day should return 0")
	}
	if dateStrToEpoch("2020-02-30") != 0 {
		t.Error("feb 30 should return 0")
	}
}

func TestDateStrToEpochLeapDay(t *testing.T) {
	if dateStrToEpoch("2020-02-29") == 0 {
		t.Error("2020 leap day should be valid")
	}
	if dateStrToEpoch("2023-02-29") != 0 {
		t.Error("2023 feb 29 should be invalid")
	}
	if dateStrToEpoch("2020-04-31") != 0 {
		t.Error("apr 31 should be invalid")
	}
}

func TestIsLeap(t *testing.T) {
	if !isLeap(2000) {
		t.Error("2000 should be leap")
	}
	if !isLeap(2024) {
		t.Error("2024 should be leap")
	}
	if isLeap(1900) {
		t.Error("1900 should not be leap")
	}
	if isLeap(2023) {
		t.Error("2023 should not be leap")
	}
}

func TestComputeCDNMaxAge(t *testing.T) {
	rd := "2024-01-01"
	age := ComputeCDNMaxAge(&rd, 86400, 31536000)
	if age == 0 {
		t.Error("recent film should have non-zero max-age")
	}

	old := "2000-01-01"
	ageOld := ComputeCDNMaxAge(&old, 86400, 31536000)
	if ageOld != 365*24*3600 {
		t.Errorf("old film should have 1-year max-age, got %d", ageOld)
	}
}

func TestLayoutCacheSuffix(t *testing.T) {
	def := DefaultLayout("poster")
	if LayoutCacheSuffix(&def, "poster") != "" {
		t.Error("default layout should add no cache suffix")
	}
	mod := def
	mod.Top = SideSlot{PerRow: 2, Rows: 1, Start: "l"}
	if LayoutCacheSuffix(&mod, "poster") == "" {
		t.Error("non-default layout should add a suffix")
	}
}
