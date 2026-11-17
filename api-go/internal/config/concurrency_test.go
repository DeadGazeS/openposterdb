package config

import (
	"runtime"
	"testing"
)

func TestFromEnv_ConcurrencyDefaults(t *testing.T) {
	t.Setenv("RENDER_CONCURRENCY", "")
	t.Setenv("CROSS_ID_CONCURRENCY", "")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.RenderConcurrency != runtime.NumCPU()*2 || c.CrossIDConcurrency != runtime.NumCPU() {
		t.Errorf("defaults: render %d cross-id %d", c.RenderConcurrency, c.CrossIDConcurrency)
	}
}

func TestFromEnv_ConcurrencyValues(t *testing.T) {
	t.Setenv("RENDER_CONCURRENCY", "6")
	t.Setenv("CROSS_ID_CONCURRENCY", " 3 ")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.RenderConcurrency != 6 || c.CrossIDConcurrency != 3 {
		t.Errorf("got render %d cross-id %d, want 6/3", c.RenderConcurrency, c.CrossIDConcurrency)
	}
}

// 0 would block every render, so it is a startup error, not a default.
func TestFromEnv_ConcurrencyZeroRejected(t *testing.T) {
	for _, key := range []string{"RENDER_CONCURRENCY", "CROSS_ID_CONCURRENCY"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "0")
			if _, err := FromEnv(); err == nil {
				t.Errorf("%s=0 accepted", key)
			}
		})
	}
}

// An unparsable value falls back to the default (and is logged).
func TestFromEnv_ConcurrencyInvalidUsesDefault(t *testing.T) {
	t.Setenv("RENDER_CONCURRENCY", "lots")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.RenderConcurrency != runtime.NumCPU()*2 {
		t.Errorf("got %d", c.RenderConcurrency)
	}
}
