package services

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const rotationCooldown = 24 * time.Hour
const maxConsecutive429s = 1

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)[:12]
}

type apiKey struct {
	raw    string
	hashed string
}

type poolState struct {
	activeIndex     int
	consecutive429s uint32
	lastRotation    *time.Time
	// blockedUntil holds keys an upstream asked us to leave alone
	// (Retry-After longer than the retry policy waits), by index.
	blockedUntil map[int]time.Time
}

type APIKeyPool struct {
	keys  []apiKey
	mu    sync.Mutex
	state poolState
	now   func() time.Time // tests
}

func NewAPIKeyPool(keys []string) *APIKeyPool {
	if len(keys) == 0 {
		panic("APIKeyPool requires at least one key")
	}
	ak := make([]apiKey, len(keys))
	for i, k := range keys {
		ak[i] = apiKey{raw: k, hashed: hashKey(k)}
	}
	return &APIKeyPool{
		keys:  ak,
		state: poolState{activeIndex: 0, blockedUntil: map[int]time.Time{}},
		now:   time.Now,
	}
}

func (p *APIKeyPool) Len() int {
	return len(p.keys)
}

// ActiveKeyRaw returns the key to use. When every key is blocked it still
// returns the active one; callers that honour blocks use ActiveKey.
func (p *APIKeyPool) ActiveKeyRaw() string {
	raw, _ := p.ActiveKey()
	if raw == "" {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.keys[p.state.activeIndex].raw
	}
	return raw
}

// ActiveKey returns the key to use and true, skipping keys that are blocked
// (BlockKey). With every key blocked it returns ("", false): the caller
// should not call the upstream until a block expires.
func (p *APIKeyPool) ActiveKey() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maybeResetCooldown()
	now := p.now()
	for i := range p.keys {
		idx := (p.state.activeIndex + i) % len(p.keys)
		if until, ok := p.state.blockedUntil[idx]; ok {
			if now.Before(until) {
				continue
			}
			delete(p.state.blockedUntil, idx)
		}
		if idx != p.state.activeIndex {
			p.switchTo(idx, "blocked key skipped")
		}
		return p.keys[idx].raw, true
	}
	return "", false
}

// BlockedUntil returns when the earliest block ends if every key is blocked,
// else the zero time.
func (p *APIKeyPool) BlockedUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	var earliest time.Time
	for i := range p.keys {
		until, ok := p.state.blockedUntil[i]
		if !ok || !now.Before(until) {
			return time.Time{}
		}
		if earliest.IsZero() || until.Before(earliest) {
			earliest = until
		}
	}
	return earliest
}

func (p *APIKeyPool) ActiveKeyHash() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.keys[p.state.activeIndex].hashed
}

// KeyHash returns the log-safe hash of a raw key.
func KeyHash(raw string) string { return hashKey(raw) }

func (p *APIKeyPool) Report429() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.keys) <= 1 {
		return
	}

	p.maybeResetCooldown()

	p.state.consecutive429s++
	if p.state.consecutive429s >= maxConsecutive429s {
		p.rotate()
	}
}

// Report429For records a rate limit on key raw: the key is blocked until
// `until` (zero = no explicit block) and, when it is the active key, the pool
// rotates to the next one. Unlike Report429 it names the key, so a 429 seen
// after another goroutine already rotated doesn't rotate past a good key.
func (p *APIKeyPool) Report429For(raw string, until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.indexOf(raw)
	if idx < 0 {
		return
	}
	if !until.IsZero() && p.now().Before(until) {
		p.state.blockedUntil[idx] = until
		slog.Warn("API key blocked by the provider's Retry-After", "key", p.keys[idx].hashed, "until", until.UTC().Format(time.RFC3339))
	}
	if idx != p.state.activeIndex || len(p.keys) <= 1 {
		return
	}
	p.maybeResetCooldown()
	p.state.consecutive429s++
	if p.state.consecutive429s >= maxConsecutive429s {
		p.rotate()
	}
}

func (p *APIKeyPool) ReportSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state.consecutive429s = 0

	if len(p.keys) > 1 {
		p.maybeResetCooldown()
	}
}

func (p *APIKeyPool) indexOf(raw string) int {
	for i, k := range p.keys {
		if k.raw == raw {
			return i
		}
	}
	return -1
}

func (p *APIKeyPool) maybeResetCooldown() {
	if p.state.lastRotation == nil {
		return
	}
	if p.now().Sub(*p.state.lastRotation) >= rotationCooldown {
		oldHash := p.keys[p.state.activeIndex].hashed
		p.state.activeIndex = 0
		p.state.lastRotation = nil
		p.state.consecutive429s = 0
		slog.Info("API key cooldown expired, returning to primary key",
			"old_key", oldHash,
			"new_key", p.keys[0].hashed,
		)
	}
}

func (p *APIKeyPool) rotate() {
	p.switchTo((p.state.activeIndex+1)%len(p.keys), "rotating API key after consecutive rate limits")
}

func (p *APIKeyPool) switchTo(idx int, msg string) {
	oldHash := p.keys[p.state.activeIndex].hashed
	p.state.activeIndex = idx
	now := p.now()
	p.state.lastRotation = &now
	p.state.consecutive429s = 0
	slog.Warn(msg,
		"old_key", oldHash,
		"new_key", p.keys[idx].hashed,
	)
}
