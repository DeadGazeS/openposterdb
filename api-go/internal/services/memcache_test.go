package services

import (
	"testing"
	"time"
)

func TestMemCacheGetSetDelete(t *testing.T) {
	c := NewMemCache(0, 0, time.Hour, 0)
	c.Set("a", []byte("hello"), 5)
	if v, ok := c.Get("a"); !ok || string(v.([]byte)) != "hello" {
		t.Fatalf("Get(a) = %v, %v; want hello, true", v, ok)
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Error("entry should be deleted")
	}
	if c.Len() != 0 {
		t.Errorf("Len after delete = %d, want 0", c.Len())
	}
}

func TestMemCacheWeightCapEvictsLRU(t *testing.T) {
	// Capacity 10 bytes; three 4-byte entries must evict the oldest.
	c := NewMemCache(10, 0, time.Hour, 0)
	c.Set("a", []byte("aaaa"), 4)
	c.Set("b", []byte("bbbb"), 4)
	c.Set("c", []byte("cccc"), 4)
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted (LRU)")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("b should still be cached")
	}
	if c.WeightedBytes() != 8 {
		t.Errorf("WeightedBytes = %d, want 8", c.WeightedBytes())
	}
}

func TestMemCacheAccessRefreshesLRU(t *testing.T) {
	c := NewMemCache(10, 0, time.Hour, 0)
	c.Set("a", []byte("aaaa"), 4)
	c.Set("b", []byte("bbbb"), 4)
	// Touch a so b becomes the LRU victim.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	c.Set("c", []byte("cccc"), 4)
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted (a was refreshed)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should still be cached")
	}
}

func TestMemCacheEntryCap(t *testing.T) {
	c := NewMemCache(0, 2, time.Hour, 0)
	c.Set("a", 1, 1)
	c.Set("b", 2, 1)
	c.Set("c", 3, 1)
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2 (entry cap)", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted")
	}
}

func TestMemCacheTTLExpiry(t *testing.T) {
	c := NewMemCache(0, 0, 50*time.Millisecond, 0)
	c.Set("a", 1, 1)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("entry missing before TTL")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Error("entry should have expired")
	}
	if c.Len() != 0 {
		t.Errorf("Len after expiry = %d, want 0", c.Len())
	}
}

func TestMemCacheIdleTTL(t *testing.T) {
	c := NewMemCache(0, 0, time.Hour, 50*time.Millisecond)
	c.Set("a", 1, 1)
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Error("entry should have expired via idle TTL")
	}
}

func TestMemCacheDeletePrefixAndClear(t *testing.T) {
	c := NewMemCache(0, 0, time.Hour, 0)
	c.Set("imdb/tt1", 1, 1)
	c.Set("imdb/tt2", 2, 1)
	c.Set("tmdb/99", 3, 1)
	c.DeletePrefix("imdb/")
	if c.Len() != 1 {
		t.Errorf("Len after prefix delete = %d, want 1", c.Len())
	}
	if _, ok := c.Get("tmdb/99"); !ok {
		t.Error("tmdb/99 should remain")
	}
	c.Clear()
	if c.Len() != 0 || c.WeightedBytes() != 0 {
		t.Errorf("after Clear: Len=%d WeightedBytes=%d, want 0/0", c.Len(), c.WeightedBytes())
	}
}

func TestMemCacheSetReplacesWeight(t *testing.T) {
	c := NewMemCache(0, 0, time.Hour, 0)
	c.Set("a", []byte("small"), 5)
	c.Set("a", []byte("much larger value"), 17)
	if c.WeightedBytes() != 17 {
		t.Errorf("WeightedBytes = %d, want 17 (replaced)", c.WeightedBytes())
	}
	if v, ok := c.Get("a"); !ok || string(v.([]byte)) != "much larger value" {
		t.Errorf("Get(a) should return the new value")
	}
}
