package image

import "sync"

// InflightSet deduplicates concurrent renders of the same cache key: the
// first caller becomes the leader and runs the work; every other caller with
// the same key waits on the leader's channel and receives the same result
// (bytes or error). It mirrors the Rust image_inflight (moka try_get_with)
// without a third-party dependency.
//
// A failed leader run propagates the error to all waiters and removes the
// entry, so the next request starts a fresh attempt. The set is safe for
// concurrent use.
type InflightSet struct {
	mu sync.Mutex
	m  map[string]*inflightEntry
}

type inflightEntry struct {
	ch     chan struct{}
	result []byte
	err    error
}

func NewInflightSet() *InflightSet {
	return &InflightSet{m: make(map[string]*inflightEntry)}
}

// RunCoalesced runs fn once for the given key. Concurrent callers with the
// same key wait and receive the leader's result. fn must be safe to call
// multiple times across different keys (concurrent leaders) — callers are
// responsible for any per-key side effects (e.g. cache writes) inside fn.
func (s *InflightSet) RunCoalesced(key string, fn func() ([]byte, error)) ([]byte, error) {
	s.mu.Lock()
	if e, ok := s.m[key]; ok {
		s.mu.Unlock()
		<-e.ch
		return e.result, e.err
	}
	e := &inflightEntry{ch: make(chan struct{})}
	s.m[key] = e
	s.mu.Unlock()

	b, err := fn()

	s.mu.Lock()
	delete(s.m, key)
	e.result, e.err = b, err
	close(e.ch)
	s.mu.Unlock()
	return b, err
}

// Len returns the number of in-flight keys (diagnostic/test helper).
func (s *InflightSet) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}
