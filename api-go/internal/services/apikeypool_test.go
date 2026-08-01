package services

import (
	"testing"
)

func TestSingleKeyNeverRotates(t *testing.T) {
	pool := NewAPIKeyPool([]string{"key1"})
	if pool.Len() != 1 {
		t.Fatal("expected 1 key")
	}

	k1 := pool.ActiveKeyRaw()
	pool.Report429()
	pool.Report429()
	pool.Report429()
	pool.Report429()
	k2 := pool.ActiveKeyRaw()

	if k1 != k2 {
		t.Error("single key pool should never rotate")
	}
}

func TestRotationAfterConsecutive429s(t *testing.T) {
	pool := NewAPIKeyPool([]string{"key1", "key2"})

	pool.Report429()
	pool.Report429()
	if pool.ActiveKeyRaw() != "key1" {
		t.Error("2 429s should not trigger rotation")
	}

	pool.Report429()
	if pool.ActiveKeyRaw() != "key2" {
		t.Error("3rd 429 should rotate to key2")
	}
}

func TestSuccessResetsConsecutiveCount(t *testing.T) {
	pool := NewAPIKeyPool([]string{"key1", "key2"})

	pool.Report429()
	pool.Report429()
	pool.ReportSuccess()
	pool.Report429()
	pool.Report429()

	if pool.ActiveKeyRaw() != "key1" {
		t.Error("success should reset 429 counter")
	}
}

func TestWrapsAround(t *testing.T) {
	pool := NewAPIKeyPool([]string{"key1", "key2", "key3"})

	for i := 0; i < 3; i++ {
		pool.Report429()
	}
	if pool.ActiveKeyRaw() != "key2" {
		t.Error("should rotate to key2")
	}

	for i := 0; i < 3; i++ {
		pool.Report429()
	}
	if pool.ActiveKeyRaw() != "key3" {
		t.Error("should rotate to key3")
	}

	for i := 0; i < 3; i++ {
		pool.Report429()
	}
	if pool.ActiveKeyRaw() != "key1" {
		t.Error("should wrap back to key1")
	}
}

func TestHashDeterministic(t *testing.T) {
	h1 := hashKey("testkey123")
	h2 := hashKey("testkey123")
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
}

func TestHashDifferentForDifferentKeys(t *testing.T) {
	h1 := hashKey("key_a")
	h2 := hashKey("key_b")
	if h1 == h2 {
		t.Error("different keys should produce different hashes")
	}
}

func TestHashLength(t *testing.T) {
	if len(hashKey("some_key")) != 12 {
		t.Error("hash should be 12 chars")
	}
}
