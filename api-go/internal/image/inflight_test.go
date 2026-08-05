package image

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
	time.Sleep(100 * time.Millisecond) // give every worker time to join
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
	time.Sleep(100 * time.Millisecond) // let the follower join the in-flight run
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
	time.Sleep(100 * time.Millisecond)
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
