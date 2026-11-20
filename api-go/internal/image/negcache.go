package image

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync"
	"time"

	apperr "openposterdb/internal/errors"
)

// Negative cache of render failures, keyed by cache key: a title without
// artwork, or an upstream that is down, would otherwise be fetched again on
// every request (media servers re-request missing images constantly). Only
// ID 404s used to be remembered (services.ResolveIDCachedCtx).
const (
	renderNoArtworkTTL = 10 * time.Minute // 404: no artwork for this title
	renderFailureTTL   = time.Minute      // 5xx / timeout: upstream trouble
	renderFailuresMax  = 5000
)

type renderFailure struct {
	err   error
	until time.Time
}

type renderFailureCache struct {
	mu      sync.Mutex
	entries map[string]renderFailure
	now     func() time.Time
}

var renderFailures = &renderFailureCache{entries: map[string]renderFailure{}, now: time.Now}

// recent returns the remembered failure of key, if still current.
func (c *renderFailureCache) recent(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil
	}
	if !c.now().Before(e.until) {
		delete(c.entries, key)
		return nil
	}
	return e.err
}

// remember records a render failure when it is worth remembering: a 404
// (no artwork) for renderNoArtworkTTL, a 5xx or timeout for
// renderFailureTTL. Never a capacity refusal (503 + Retry-After, the client
// retries), a cancellation (the request went away mid-delivery, not the
// render) or a client error.
func (c *renderFailureCache) remember(key string, err error) {
	ttl := renderFailureTTL
	var appErr *apperr.AppError
	switch {
	case err == nil, stderrors.Is(err, context.Canceled):
		return
	case stderrors.As(err, &appErr):
		switch {
		case appErr.RetryAfter > 0:
			return
		case appErr.Status == http.StatusNotFound:
			ttl = renderNoArtworkTTL
		case appErr.Status < 500:
			return
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.entries[key] = renderFailure{err: err, until: now.Add(ttl)}
	if len(c.entries) <= renderFailuresMax {
		return
	}
	for k, e := range c.entries {
		if !now.Before(e.until) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries { // still full: drop arbitrary entries
		if len(c.entries) <= renderFailuresMax {
			break
		}
		delete(c.entries, k)
	}
}

// ResetRenderFailures forgets every remembered failure. Called when provider
// keys change or the cache is cleared: a "no artwork" caused by a missing or
// wrong key must not outlive the fix by up to renderNoArtworkTTL.
func ResetRenderFailures() {
	renderFailures.mu.Lock()
	clear(renderFailures.entries)
	renderFailures.mu.Unlock()
}

// forget drops a remembered failure (after a successful render).
func (c *renderFailureCache) forget(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}
