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

	// Pending holds the api_keys IDs (int64 keys, values ignored) that still
	// need a last_used_at update.
	Pending sync.Map
}

func NewLastUsedFlusher(db *sql.DB, interval time.Duration) *LastUsedFlusher {
	return &LastUsedFlusher{db: db, interval: interval}
}

// Record marks the key as recently used; the next Flush will persist it to
// the database. Safe to call from any goroutine (sync.Map).
func (f *LastUsedFlusher) Record(keyID int64) {
	if f == nil {
		return
	}
	f.Pending.Store(keyID, struct{}{})
}

// Start launches the periodic flush worker; it runs until the process exits.
func (f *LastUsedFlusher) Start() {
	ticker := time.NewTicker(f.interval)
	go func() {
		for range ticker.C {
			f.flush()
		}
	}()
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
