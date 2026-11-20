package image

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// WaitBackground waits for running work (with a deadline) and refuses new
// work afterwards. Restores the package state for the other tests.
func TestWaitBackground(t *testing.T) {
	t.Cleanup(func() { backgroundDraining.Store(false) })

	var finished atomic.Bool
	release := make(chan struct{})
	if !goBackground(func() { <-release; finished.Store(true) }) {
		t.Fatal("work refused before shutdown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if WaitBackground(ctx) {
		t.Fatal("reported done while work was still running")
	}
	if goBackground(func() {}) {
		t.Error("new work accepted while draining")
	}

	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if !WaitBackground(ctx2) || !finished.Load() {
		t.Error("running work was not waited for")
	}
}
