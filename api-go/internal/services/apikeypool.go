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
}

type APIKeyPool struct {
	keys  []apiKey
	mu    sync.Mutex
	state poolState
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
		keys: ak,
		state: poolState{
			activeIndex: 0,
		},
	}
}

func (p *APIKeyPool) Len() int {
	return len(p.keys)
}

func (p *APIKeyPool) ActiveKeyRaw() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maybeResetCooldown()
	return p.keys[p.state.activeIndex].raw
}

func (p *APIKeyPool) ActiveKeyHash() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.keys[p.state.activeIndex].hashed
}

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

func (p *APIKeyPool) ReportSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state.consecutive429s = 0

	if len(p.keys) > 1 {
		p.maybeResetCooldown()
	}
}

func (p *APIKeyPool) maybeResetCooldown() {
	if p.state.lastRotation == nil {
		return
	}
	if time.Since(*p.state.lastRotation) >= rotationCooldown {
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
	oldHash := p.keys[p.state.activeIndex].hashed
	p.state.activeIndex = (p.state.activeIndex + 1) % len(p.keys)
	now := time.Now()
	p.state.lastRotation = &now
	p.state.consecutive429s = 0
	slog.Warn("rotating API key after consecutive rate limits",
		"old_key", oldHash,
		"new_key", p.keys[p.state.activeIndex].hashed,
		"consecutive_429s", maxConsecutive429s,
	)
}
