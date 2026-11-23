package image

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	apperr "openposterdb/internal/errors"
)

// slotLimiter caps concurrent work. Callers wait at most maxWait and only
// while fewer than maxWaiting others are queued; past that the request is
// declined (503 + Retry-After) instead of piling up behind the CPU.
type slotLimiter struct {
	slots      chan struct{}
	waiting    atomic.Int64
	maxWaiting int64
	maxWait    time.Duration
}

func newSlotLimiter(slots, maxWaiting int, maxWait time.Duration) *slotLimiter {
	return &slotLimiter{slots: make(chan struct{}, max(slots, 1)), maxWaiting: int64(maxWaiting), maxWait: maxWait}
}

// busyRetryAfter is the Retry-After (seconds) sent when renders are declined.
const busyRetryAfter = 5

var errRenderBusy = apperr.NewBusy("server busy rendering images, retry shortly", busyRetryAfter)

// acquire waits for a slot; the returned func releases it.
func (l *slotLimiter) acquire(ctx context.Context) (func(), error) {
	select {
	case l.slots <- struct{}{}:
		return l.release, nil
	default:
	}
	if l.waiting.Add(1) > l.maxWaiting {
		l.waiting.Add(-1)
		return nil, errRenderBusy
	}
	defer l.waiting.Add(-1)
	timer := time.NewTimer(l.maxWait)
	defer timer.Stop()
	select {
	case l.slots <- struct{}{}:
		return l.release, nil
	case <-timer.C:
		return nil, errRenderBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// tryAcquire takes a slot only if one is free right now.
func (l *slotLimiter) tryAcquire() (func(), bool) {
	select {
	case l.slots <- struct{}{}:
		return l.release, true
	default:
		return nil, false
	}
}

func (l *slotLimiter) release() { <-l.slots }

const (
	renderMaxWait = 3 * time.Second
	// renderQueuePerSlot bounds the wait queue relative to the slot count.
	renderQueuePerSlot = 8
	// refreshSlotsMax: background refreshes at a time (the rest are skipped
	// and picked up by a later request).
	refreshSlotsMax = 3
)

var (
	limitsMu     sync.RWMutex
	renderSlots  = newSlotLimiter(runtime.NumCPU()*2, runtime.NumCPU()*2*renderQueuePerSlot, renderMaxWait)
	crossIDSlots = newSlotLimiter(runtime.NumCPU(), 0, 0)
	refreshSlots = newSlotLimiter(refreshSlotsMax, 0, 0)
)

// ConfigureLimits sets RENDER_CONCURRENCY (renders at a time) and
// CROSS_ID_CONCURRENCY (alternate-id cache writers at a time). Call once at
// startup; values < 1 keep the defaults (config validation rejects them).
func ConfigureLimits(render, crossID int) {
	limitsMu.Lock()
	defer limitsMu.Unlock()
	if render > 0 {
		renderSlots = newSlotLimiter(render, render*renderQueuePerSlot, renderMaxWait)
	}
	if crossID > 0 {
		crossIDSlots = newSlotLimiter(crossID, 0, 0)
	}
}

func currentRenderSlots() *slotLimiter {
	limitsMu.RLock()
	defer limitsMu.RUnlock()
	return renderSlots
}

func currentCrossIDSlots() *slotLimiter {
	limitsMu.RLock()
	defer limitsMu.RUnlock()
	return crossIDSlots
}

// refreshBackoff keeps a failing background refresh from being retried on
// every request that sees the stale entry: after a failure the key waits
// refreshBackoffMin, doubling up to refreshBackoffMax; a success clears it.
type refreshBackoff struct {
	mu sync.Mutex
	m  map[string]backoffEntry
}

type backoffEntry struct {
	until time.Time
	delay time.Duration
}

const (
	refreshBackoffMin  = 5 * time.Minute
	refreshBackoffMax  = 6 * time.Hour
	refreshBackoffKeep = 10000 // entries kept before expired ones are pruned
)

var refreshFailures = &refreshBackoff{m: map[string]backoffEntry{}}

func (b *refreshBackoff) blocked(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.m[key]
	return ok && now.Before(e.until)
}

func (b *refreshBackoff) failed(key string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := refreshBackoffMin
	if e, ok := b.m[key]; ok {
		d = min(e.delay*2, refreshBackoffMax)
	}
	if len(b.m) >= refreshBackoffKeep {
		for k, e := range b.m {
			if now.After(e.until) {
				delete(b.m, k)
			}
		}
		for k := range b.m { // still full: drop arbitrary entries
			if len(b.m) < refreshBackoffKeep {
				break
			}
			delete(b.m, k)
		}
	}
	b.m[key] = backoffEntry{until: now.Add(d), delay: d}
}

func (b *refreshBackoff) succeeded(key string) {
	b.mu.Lock()
	delete(b.m, key)
	b.mu.Unlock()
}
