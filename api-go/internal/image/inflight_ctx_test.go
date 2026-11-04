package image

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A waiter whose request was cancelled stops waiting (it used to block
// until the leader's render finished, up to the render timeout).
func TestRunCoalescedCtx_WaiterHonoursCancel(t *testing.T) {
	s := NewInflightSet()
	release := make(chan struct{})
	started := make(chan struct{})
	go s.RunCoalescedCtx(context.Background(), "k", func() ([]byte, error) {
		close(started)
		<-release
		return []byte("x"), nil
	})
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.RunCoalescedCtx(ctx, "k", func() ([]byte, error) { return nil, nil })
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter ignored cancellation")
	}
	close(release)
}
