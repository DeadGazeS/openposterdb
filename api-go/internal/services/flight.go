package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// flightTimeout bounds a shared (deduplicated) upstream call, which runs
// detached from any single caller's cancellation.
const flightTimeout = 60 * time.Second

// flightGroup deduplicates concurrent calls with the same key (a minimal,
// generic singleflight; golang.org/x/sync is not a dependency).
type flightGroup[T any] struct {
	mu sync.Mutex
	m  map[string]*flightCall[T]
}

type flightCall[T any] struct {
	done chan struct{}
	val  T
	err  error
}

// do runs fn once for all concurrent callers of key. fn gets a context
// detached from the first caller's cancellation (bounded by flightTimeout),
// so one caller going away doesn't fail the others; every caller still stops
// waiting when its own ctx ends.
func (g *flightGroup[T]) do(ctx context.Context, key string, fn func(context.Context) (T, error)) (T, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*flightCall[T])
	}
	c, ok := g.m[key]
	if !ok {
		c = &flightCall[T]{done: make(chan struct{})}
		g.m[key] = c
		go func() {
			defer func() {
				if r := recover(); r != nil {
					c.err = fmt.Errorf("panic in shared call: %v", r)
					slog.Error("flightGroup call panicked", "key", key, "panic", r)
				}
				g.mu.Lock()
				delete(g.m, key)
				g.mu.Unlock()
				close(c.done)
			}()
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flightTimeout)
			defer cancel()
			c.val, c.err = fn(fctx)
		}()
	}
	g.mu.Unlock()
	select {
	case <-c.done:
		return c.val, c.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
