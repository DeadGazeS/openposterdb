package image

import (
	"context"
	"sync"
	"sync/atomic"
)

// Detached work (stale refreshes, cold-cache warm-ups, cross-id copies) runs
// past the request that started it. It is tracked so shutdown can wait for
// it — with a deadline — instead of cutting off image_meta upserts.
var (
	backgroundWG       sync.WaitGroup
	backgroundMu       sync.Mutex // orders Add against the drain in WaitBackground
	backgroundDraining atomic.Bool
)

// goBackground runs fn in a tracked goroutine. After WaitBackground started
// (shutdown), no new work is started and false is returned.
func goBackground(fn func()) bool {
	backgroundMu.Lock()
	if backgroundDraining.Load() {
		backgroundMu.Unlock()
		return false
	}
	backgroundWG.Add(1)
	backgroundMu.Unlock()
	go func() {
		defer backgroundWG.Done()
		fn()
	}()
	return true
}

// WaitBackground stops new background work and waits for the running work
// until ctx is done. Reports whether everything finished.
func WaitBackground(ctx context.Context) bool {
	backgroundMu.Lock()
	backgroundDraining.Store(true)
	backgroundMu.Unlock()
	return waitBackgroundWG(ctx)
}

// WaitIdle waits, until ctx is done, for the background work running now
// (including work it starts) without stopping new work, unlike
// WaitBackground, which drains for good. For tests only: they call it
// between requests, while nothing else starts background work, so detached
// work from one step cannot bleed into the next. Reports whether it finished.
func WaitIdle(ctx context.Context) bool {
	return waitBackgroundWG(ctx)
}

func waitBackgroundWG(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		backgroundWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}
