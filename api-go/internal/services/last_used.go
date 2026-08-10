package services

import (
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// LastUsedFlusher batches api_keys.last_used_at updates: handlers record key
// IDs in Pending, and the flusher persists them in one UPDATE per interval
// instead of one write per request.
type LastUsedFlusher struct {
	db       *sql.DB
	interval time.Duration
	done     chan struct{} // closed by Stop() to signal the worker to exit

	// Pending holds the api_keys IDs (int64 keys, values ignored) that still
	// need a last_used_at update.
	Pending sync.Map
}

func NewLastUsedFlusher(db *sql.DB, interval time.Duration) *LastUsedFlusher {
	return &LastUsedFlusher{db: db, interval: interval, done: make(chan struct{})}
}

// Record marks the key as recently used; the next Flush will persist it to
// the database. Safe to call from any goroutine (sync.Map).
func (f *LastUsedFlusher) Record(keyID int64) {
	if f == nil {
		return
	}
	f.Pending.Store(keyID, struct{}{})
}

// Start launches the periodic flush worker; it runs until Stop is called.
func (f *LastUsedFlusher) Start() {
	go func() {
		ticker := time.NewTicker(f.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				f.flush()
			case <-f.done:
				return
			}
		}
	}()
}

// Stop signals the worker to exit. Safe to call multiple times.
func (f *LastUsedFlusher) Stop() {
	if f == nil {
		return
	}
	select {
	case <-f.done:
		// already closed
	default:
		close(f.done)
	}
}

// Flush persists any remaining pending IDs (called on shutdown).
func (f *LastUsedFlusher) Flush() {
	f.flush()
}

func (f *LastUsedFlusher) flush() {
	var ids []int64
	f.Pending.Range(func(key, value any) bool {
		id, ok := key.(int64)
		if ok {
			ids = append(ids, id)
		}
		f.Pending.Delete(key)
		return true
	})
	if len(ids) > 0 {
		if err := BatchUpdateLastUsed(f.db, ids); err != nil {
			slog.Warn("failed to batch update last_used_at", "error", err)
		}
	}
}
