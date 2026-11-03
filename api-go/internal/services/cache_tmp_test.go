package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepTempFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "posters", "imdb")
	os.MkdirAll(sub, 0o755)
	old := filepath.Join(sub, ".tmp-123")
	recent := filepath.Join(sub, ".tmp-456")
	keep := filepath.Join(sub, "tt1.jpg")
	for _, p := range []string{old, recent, keep} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, past, past)
	if n := SweepTempFiles(dir); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old temp file must be removed")
	}
	for _, p := range []string{recent, keep} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must survive: %v", p, err)
		}
	}
}
