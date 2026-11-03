package services

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSendWithRetryCtx_StopsOnCancel: a cancelled request must not sleep
// through the backoff and retry — it used to time.Sleep regardless.
func TestSendWithRetryCtx_StopsOnCancel(t *testing.T) {
	cfg := &RetryConfig{MaxRetries: 3, BaseDelay: 2 * time.Second, MaxDelay: 4 * time.Second, ServiceName: "test"}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	start := time.Now()
	_, err := SendWithRetryCtx(ctx, cfg, func() (*http.Response, error) {
		calls++
		cancel()
		return nil, errors.New("connection reset")
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should wrap context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry after cancel)", calls)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("took %v; must not sleep the backoff after cancel", d)
	}
}

// TestSendWithRetryCtx_429CancelledDuringWait closes the 429 body and returns
// promptly when the context ends mid-backoff.
func TestSendWithRetryCtx_429CancelledDuringWait(t *testing.T) {
	cfg := &RetryConfig{MaxRetries: 2, BaseDelay: 2 * time.Second, MaxDelay: 4 * time.Second, ServiceName: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	body := &closeTracker{Reader: strings.NewReader("slow down")}
	start := time.Now()
	_, err := SendWithRetryCtx(ctx, cfg, func() (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: body}, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
	if !body.closed {
		t.Error("429 body must be closed")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v; must return when ctx ends", d)
	}
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }
