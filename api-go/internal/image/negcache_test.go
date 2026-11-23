package image

import (
	"context"
	"fmt"
	"testing"
	"time"

	apperr "openposterdb/internal/errors"
)

func TestRenderFailureCache(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := &renderFailureCache{entries: map[string]renderFailure{}, now: func() time.Time { return now }}

	c.remember("noart", apperr.NewIDNotFound("no poster artwork available"))
	c.remember("upstream", apperr.NewAPIError(fmt.Errorf("tmdb 502")))
	c.remember("timeout", fmt.Errorf("render: %w", context.DeadlineExceeded))
	c.remember("busy", apperr.NewBusy("server busy", 5))
	c.remember("gone", fmt.Errorf("client left: %w", context.Canceled))
	c.remember("bad", apperr.NewBadRequest("not an episode"))

	for key, want := range map[string]bool{"noart": true, "upstream": true, "timeout": true, "busy": false, "gone": false, "bad": false} {
		if got := c.recent(key) != nil; got != want {
			t.Errorf("%s remembered = %v, want %v", key, got, want)
		}
	}

	now = now.Add(2 * time.Minute) // 5xx/timeout expire after a minute, 404 after ten
	if c.recent("upstream") != nil || c.recent("timeout") != nil {
		t.Error("5xx/timeout failure outlived its minute")
	}
	if c.recent("noart") == nil {
		t.Error("no-artwork failure expired early")
	}
	now = now.Add(10 * time.Minute)
	if c.recent("noart") != nil {
		t.Error("no-artwork failure outlived ten minutes")
	}

	c.remember("x", apperr.NewIDNotFound("none"))
	c.forget("x")
	if c.recent("x") != nil {
		t.Error("forget kept the entry")
	}
}

func TestRenderFailureCache_Bounded(t *testing.T) {
	c := &renderFailureCache{entries: map[string]renderFailure{}, now: time.Now}
	for i := range renderFailuresMax + 100 {
		c.remember(fmt.Sprintf("k%d", i), apperr.NewIDNotFound("none"))
	}
	if n := len(c.entries); n > renderFailuresMax {
		t.Errorf("entries = %d, want ≤ %d", n, renderFailuresMax)
	}
}

// A key fix or cache clear must not leave a "no artwork" from before it in
// place for up to renderNoArtworkTTL.
func TestResetRenderFailures(t *testing.T) {
	renderFailures.remember("tmdb:movie-1:poster", apperr.NewIDNotFound("no poster artwork available"))
	if renderFailures.recent("tmdb:movie-1:poster") == nil {
		t.Fatal("failure not remembered")
	}
	ResetRenderFailures()
	if err := renderFailures.recent("tmdb:movie-1:poster"); err != nil {
		t.Fatalf("after reset: %v", err)
	}
}
