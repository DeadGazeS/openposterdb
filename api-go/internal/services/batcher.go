package services

import (
	"context"
	"sync"
	"time"
)

// batcher gathers single-key lookups for a short window and answers them
// with one upstream call of up to max keys (providers that meter requests,
// not rows). Concurrent lookups of the same key share one slot. The batch
// call runs detached from the callers' contexts, bounded by timeout; a
// caller that gives up just stops waiting.
type batcher[K comparable, V any] struct {
	window  time.Duration
	max     int
	timeout time.Duration
	fetch   func(ctx context.Context, keys []K) (map[K]V, error)

	mu      sync.Mutex
	pending map[K][]chan batchResult[V]
	order   []K
	timer   *time.Timer
}

type batchResult[V any] struct {
	v   V
	ok  bool
	err error
}

func newBatcher[K comparable, V any](window time.Duration, max int, timeout time.Duration, fetch func(context.Context, []K) (map[K]V, error)) *batcher[K, V] {
	return &batcher[K, V]{window: window, max: max, timeout: timeout, fetch: fetch, pending: map[K][]chan batchResult[V]{}}
}

// get returns key's value (ok=false: the upstream has none) or the batch's
// error.
func (b *batcher[K, V]) get(ctx context.Context, key K) (V, bool, error) {
	ch := make(chan batchResult[V], 1)
	b.mu.Lock()
	if _, queued := b.pending[key]; !queued {
		b.order = append(b.order, key)
	}
	b.pending[key] = append(b.pending[key], ch)
	var full []K
	var waiters map[K][]chan batchResult[V]
	if len(b.order) >= b.max {
		full, waiters = b.takeLocked()
	} else if b.timer == nil {
		b.timer = time.AfterFunc(b.window, b.flush)
	}
	b.mu.Unlock()
	if full != nil {
		go b.run(full, waiters)
	}
	select {
	case r := <-ch:
		return r.v, r.ok, r.err
	case <-ctx.Done():
		var zero V
		return zero, false, ctx.Err()
	}
}

// flush sends everything pending (in batches of max).
func (b *batcher[K, V]) flush() {
	for {
		b.mu.Lock()
		b.timer = nil
		if len(b.order) == 0 {
			b.mu.Unlock()
			return
		}
		keys, waiters := b.takeLocked()
		b.mu.Unlock()
		go b.run(keys, waiters)
	}
}

// takeLocked removes up to max pending keys with their waiters.
func (b *batcher[K, V]) takeLocked() ([]K, map[K][]chan batchResult[V]) {
	n := min(len(b.order), b.max)
	keys := append([]K(nil), b.order[:n]...)
	b.order = b.order[n:]
	waiters := make(map[K][]chan batchResult[V], n)
	for _, k := range keys {
		waiters[k] = b.pending[k]
		delete(b.pending, k)
	}
	if len(b.order) == 0 && b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	return keys, waiters
}

func (b *batcher[K, V]) run(keys []K, waiters map[K][]chan batchResult[V]) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	res, err := b.fetch(ctx, keys)
	for _, k := range keys {
		var r batchResult[V]
		if err != nil {
			r.err = err
		} else {
			r.v, r.ok = res[k]
		}
		for _, ch := range waiters[k] {
			ch <- r
		}
	}
}
