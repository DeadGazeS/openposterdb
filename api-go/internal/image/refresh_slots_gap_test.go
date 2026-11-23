package image

import (
	"context"
	"testing"
	"time"

	"openposterdb/internal/services"
)

// runRefresh calls refreshStale and reports whether it went past the slot
// check: the ServeParams have no Settings, so a refresh that actually starts
// rendering panics at its first settings read.
func runRefresh(p ServeParams, key string) (started bool) {
	defer func() {
		if recover() != nil {
			started = true
		}
	}()
	p.refreshStale(&services.ResolvedID{}, services.ImageSize(""), key)
	return false
}

// With all refreshSlotsMax background-refresh slots taken, a refresh is
// skipped on the spot (returns without rendering, recording no back-off and
// leaving nothing waiting for a slot); a later stale request tries again.
func TestRefreshStale_SkippedWhenSlotsFull(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !WaitIdle(ctx) { // earlier tests' refreshes must not hold slots
		t.Fatal("background work still running")
	}
	var releases []func()
	defer func() {
		for _, r := range releases {
			r()
		}
	}()
	for range refreshSlotsMax {
		r, ok := refreshSlots.tryAcquire()
		if !ok {
			t.Fatal("refresh slot unexpectedly busy")
		}
		releases = append(releases, r)
	}

	const key = "gap/refresh-slots-full"
	done := make(chan bool, 1)
	go func() { done <- runRefresh(ServeParams{}, key) }()
	select {
	case started := <-done:
		if started {
			t.Fatal("refresh ran with all refresh slots busy")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh waited for a slot instead of being skipped")
	}
	if refreshFailures.blocked(key, time.Now()) {
		t.Fatal("skipped refresh recorded a back-off")
	}

	// Freeing the slots runs nothing queued: no slot gets taken.
	for _, r := range releases {
		r()
	}
	releases = nil
	time.Sleep(50 * time.Millisecond)
	if n := len(refreshSlots.slots); n != 0 {
		t.Fatalf("%d refresh slots taken after release: a skipped refresh was queued", n)
	}

	// Control: with a free slot the same call does start the refresh.
	if !runRefresh(ServeParams{}, key) {
		t.Fatal("refresh with a free slot did not start; the test cannot tell a skip apart")
	}
	if n := len(refreshSlots.slots); n != 0 {
		t.Fatalf("refresh slot leaked by the panicking refresh: %d taken", n)
	}
}
