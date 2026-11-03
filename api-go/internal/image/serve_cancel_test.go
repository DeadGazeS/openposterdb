package image

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// gatedPosterStub is tmdbStub whose artwork download blocks until gate is
// closed and honours request cancellation (as a real transport does).
type gatedPosterStub struct {
	tmdbStub
	gate    chan struct{}
	started chan struct{}
	once    sync.Once
}

func (s *gatedPosterStub) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "image.tmdb.org" {
		s.once.Do(func() { close(s.started) })
		select {
		case <-s.gate:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       ioNopCloser(bytes.NewReader(s.poster)),
		}, nil
	}
	return s.tmdbStub.RoundTrip(req)
}

// TestServeImage_LeaderCancelDoesNotFailWaiters: concurrent requests for one
// cache key share the leader's render. The render used to run under the
// leader's request context, so the leader's client disconnecting failed every
// waiter with "context canceled".
func TestServeImage_LeaderCancelDoesNotFailWaiters(t *testing.T) {
	loadTestFont(t)
	db := newServeTestDB(t)
	stub := &gatedPosterStub{tmdbStub: tmdbStub{poster: SamplePosterPNG}, gate: make(chan struct{}), started: make(chan struct{})}
	base := serveParams(t, db, t.TempDir())
	base.TMDB.HTTP = &http.Client{Transport: stub}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leader := base
	leader.Context = leaderCtx
	leaderDone := make(chan struct{})
	go func() { defer close(leaderDone); ServeImage(leader) }()

	select {
	case <-stub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("leader never reached the artwork download")
	}

	waiter := base
	waiter.Context = context.Background()
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, _, err := ServeImage(waiter)
		done <- result{b, err}
	}()
	// Wait until the waiter has joined the leader's in-flight render.
	deadline := time.Now().Add(5 * time.Second)
	for base.Inflight.TotalCalls() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("waiter never joined the in-flight render")
		}
		time.Sleep(time.Millisecond)
	}

	cancelLeader()
	time.Sleep(20 * time.Millisecond) // let a ctx-bound fetch observe the cancel
	close(stub.gate)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("waiter failed because the leader cancelled: %v", r.err)
		}
		if len(r.b) == 0 {
			t.Fatal("waiter got an empty image")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waiter never returned")
	}

	// Drain the async work (leader + cross-ID write, which ends with the
	// image_meta upsert) before TempDir cleanup.
	<-leaderDone
	var n int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err := db.QueryRow(`SELECT COUNT(*) FROM image_meta`).Scan(&n); err == nil && n == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
