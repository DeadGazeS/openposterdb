package image

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitForTotalCalls polls until the inflight set has been entered at least
// `want` times, or fails the test after `deadline`. Replaces a fixed
// time.Sleep that pessimistically waited for followers to enter
// RunCoalesced before the leader released.
func waitForTotalCalls(t *testing.T, set *InflightSet, want int64, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if set.TotalCalls() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("inflight total calls %d < %d after %v", set.TotalCalls(), want, deadline)
}

func TestInflightSet_DedupsConcurrentCalls(t *testing.T) {
	set := NewInflightSet()
	var runs atomic.Int64
	const workers = 20

	started := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = set.RunCoalesced("key-a", func() ([]byte, error) {
				if runs.Add(1) == 1 {
					close(started)
				}
				<-release // hold the leader inside fn so followers can join
				return []byte("rendered"), nil
			})
		}(i)
	}

	<-started
	waitForTotalCalls(t, set, 20, 2*time.Second) // all 20 workers entered RunCoalesced
	close(release)
	wg.Wait()

	if got := runs.Load(); got != 1 {
		t.Fatalf("fn ran %d times, want exactly 1", got)
	}
	for i, err := range results {
		if err != nil {
			t.Errorf("worker %d: unexpected error %v", i, err)
		}
	}
	if set.Len() != 0 {
		t.Fatalf("set should be empty after completion, Len=%d", set.Len())
	}
}

func TestInflightSet_PropagatesLeaderResult(t *testing.T) {
	set := NewInflightSet()
	var runs atomic.Int64

	started := make(chan struct{})
	release := make(chan struct{})

	var leaderBytes []byte
	var leaderErr error
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		leaderBytes, leaderErr = set.RunCoalesced("k", func() ([]byte, error) {
			runs.Add(1)
			close(started)
			<-release // hold the leader inside fn
			return []byte("leader-result"), nil
		})
	}()

	<-started
	var followerBytes []byte
	var followerErr error
	followerDone := make(chan struct{})
	go func() {
		defer close(followerDone)
		followerBytes, followerErr = set.RunCoalesced("k", func() ([]byte, error) {
			return []byte("never-called"), nil
		})
	}()
	waitForTotalCalls(t, set, 2, 2*time.Second) // leader + follower both entered
	close(release)
	<-followerDone
	<-leaderDone

	if leaderErr != nil || followerErr != nil {
		t.Fatalf("errors: leader=%v follower=%v", leaderErr, followerErr)
	}
	if string(followerBytes) != "leader-result" {
		t.Fatalf("follower got %q, want leader-result", followerBytes)
	}
	if string(leaderBytes) != "leader-result" {
		t.Fatalf("leader got %q", leaderBytes)
	}
	if runs.Load() != 1 {
		t.Fatalf("fn ran %d times, want 1", runs.Load())
	}
}

func TestInflightSet_PropagatesLeaderError(t *testing.T) {
	set := NewInflightSet()
	boom := errors.New("render failed")

	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int64

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = set.RunCoalesced("k", func() ([]byte, error) {
				if runs.Add(1) == 1 {
					close(started)
				}
				<-release
				return nil, boom
			})
		}(i)
	}
	<-started
	waitForTotalCalls(t, set, 4, 2*time.Second) // all 4 workers entered
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != boom {
			t.Errorf("worker %d: got %v, want the leader error", i, err)
		}
	}
	if set.Len() != 0 {
		t.Fatalf("set must be empty after a failed run, Len=%d", set.Len())
	}
}

func TestInflightSet_RetriesAfterFailure(t *testing.T) {
	set := NewInflightSet()
	var runs atomic.Int64
	// First run fails; the next request must re-run fn (entry removed).
	_, err := set.RunCoalesced("k", func() ([]byte, error) {
		runs.Add(1)
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected first run to fail")
	}
	b, err := set.RunCoalesced("k", func() ([]byte, error) {
		runs.Add(1)
		return []byte("second-attempt"), nil
	})
	if err != nil || string(b) != "second-attempt" {
		t.Fatalf("second run: bytes=%q err=%v", b, err)
	}
	if runs.Load() != 2 {
		t.Fatalf("fn ran %d times, want 2", runs.Load())
	}
}

func TestInflightSet_IndependentKeys(t *testing.T) {
	set := NewInflightSet()
	var wg sync.WaitGroup
	for _, k := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _ = set.RunCoalesced(key, func() ([]byte, error) { return []byte(key), nil })
		}(k)
	}
	wg.Wait()
	if set.Len() != 0 {
		t.Fatalf("set should drain, Len=%d", set.Len())
	}
}

func TestRunCoalescedPanicRecovers(t *testing.T) {
	set := NewInflightSet()

	started := make(chan struct{})
	release := make(chan struct{})

	leaderDone := make(chan error, 1)
	go func() {
		_, err := set.RunCoalesced("panic-key", func() ([]byte, error) {
			close(started)
			<-release // hold the leader inside fn so the waiter can join
			panic("boom: rasterizer blew up")
		})
		leaderDone <- err
	}()

	// Wait until the leader is inside fn (entry is in the map) before
	// spawning the waiter, so the waiter is guaranteed to join the
	// in-flight run rather than racing to become the leader itself.
	<-started

	// A waiter joining while the leader is inside fn must receive an error
	// instead of blocking on <-e.ch forever.
	waiterDone := make(chan error, 1)
	go func() {
		_, err := set.RunCoalesced("panic-key", func() ([]byte, error) {
			return []byte("never-called"), nil
		})
		waiterDone <- err
	}()

	waitForTotalCalls(t, set, 2, 2*time.Second) // leader + waiter both entered
	close(release)                              // trigger the leader's panic

	timeout := time.After(2 * time.Second)
	collected := 0
	for collected < 2 {
		select {
		case err := <-leaderDone:
			collected++
			if err == nil {
				t.Fatal("leader got nil error; want a non-nil error containing 'panic'")
			}
			if !strings.Contains(err.Error(), "panic") {
				t.Fatalf("leader error %q does not contain 'panic'", err)
			}
		case err := <-waiterDone:
			collected++
			if err == nil {
				t.Fatal("waiter got nil error; want a non-nil error containing 'panic'")
			}
			if !strings.Contains(err.Error(), "panic") {
				t.Fatalf("waiter error %q does not contain 'panic'", err)
			}
		case <-timeout:
			t.Fatal("timed out waiting for panicked leader and waiter; a caller blocked forever")
		}
	}

	if set.Len() != 0 {
		t.Fatalf("set must be empty after a panicked run, Len=%d", set.Len())
	}
}

func TestRunCoalescedPanicRemovesEntry(t *testing.T) {
	set := NewInflightSet()

	_, err := set.RunCoalesced("panic-key", func() ([]byte, error) {
		panic("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("first run: got err=%v, want non-nil error containing 'panic'", err)
	}
	if set.Len() != 0 {
		t.Fatalf("entry not removed after panic, Len=%d", set.Len())
	}

	// The panic must not leave the key wedged: a fresh call re-runs fn.
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := set.RunCoalesced("panic-key", func() ([]byte, error) {
			return []byte("recovered"), nil
		})
		done <- result{b, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("fresh call after panic: unexpected error %v", r.err)
		}
		if string(r.b) != "recovered" {
			t.Fatalf("fresh call after panic: got %q, want %q", r.b, "recovered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on fresh call after panic; entry was not removed")
	}
}
