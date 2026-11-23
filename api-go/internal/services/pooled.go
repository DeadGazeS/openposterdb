package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"openposterdb/internal/errors"
)

// pooledCall describes one upstream call made with a key from an APIKeyPool.
type pooledCall struct {
	service string
	retry   *RetryConfig
	// do sends the request with the given key.
	do func(key string) (*http.Response, error)
	// limited reports whether a response means "this key is rate limited"
	// (429, or a provider-specific equivalent such as OMDb's 401).
	limited func(status int, body []byte) bool
	// limitBlock is how long a limited key is left alone when the response
	// carries no Retry-After (0 = only rotate, don't block).
	limitBlock time.Duration
}

// pooledDo runs c with the pool's active key. A rate-limited key is reported
// (blocked until Retry-After, rotated away from) and the call is retried once
// per remaining key. While every key is blocked, no request is made and an
// error is returned. On success the body (bounded by maxUpstreamJSON) and the
// status are returned; the caller interprets non-limit statuses. A 503 whose
// Retry-After is longer than the retry policy waits counts as a rate limit
// too. The key returned is the last one used ("" when none was).
func pooledDo(ctx context.Context, pool *APIKeyPool, c pooledCall) (int, []byte, string, error) {
	var lastStatus int
	var lastBody []byte
	var lastKey string
	for range pool.Len() {
		key, ok := pool.ActiveKey()
		if !ok {
			return 0, nil, lastKey, errors.NewOther(fmt.Sprintf("%s: every API key is rate limited until %s", c.service, pool.BlockedUntil().UTC().Format(time.RFC3339)))
		}
		lastKey = key
		resp, err := SendWithRetryCtx(ctx, c.retry, func() (*http.Response, error) { return c.do(key) })
		if err != nil {
			return 0, nil, key, errors.NewAPIError(errors.RedactURLSecrets(err))
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
		resp.Body.Close()
		retryAfter, hasRetryAfter := RetryAfter(resp)
		limited := c.limited(resp.StatusCode, body) ||
			(resp.StatusCode == http.StatusServiceUnavailable && hasRetryAfter && retryAfter > c.retry.MaxDelay)
		if !limited {
			pool.ReportSuccess()
			return resp.StatusCode, body, key, nil
		}
		var until time.Time
		if hasRetryAfter {
			until = time.Now().Add(retryAfter)
		} else if c.limitBlock > 0 {
			until = time.Now().Add(c.limitBlock)
		}
		pool.Report429For(key, until)
		slog.Warn(c.service+" API key rate limited", "key", KeyHash(key), "status", resp.StatusCode)
		lastStatus, lastBody = resp.StatusCode, body
	}
	return lastStatus, lastBody, lastKey, errors.NewOther(fmt.Sprintf("%s rate limited (%d): %s", c.service, lastStatus, errSnippet(lastBody)))
}

// keyedError tags a provider error with the hash of the API key that made
// the failed call: the pool may have rotated to another key since.
type keyedError struct {
	err     error
	keyHash string
}

func (e *keyedError) Error() string { return e.err.Error() }
func (e *keyedError) Unwrap() error { return e.err }

// withKey tags err with the hash of the raw key that made the call (nil and
// a call without a key are left as they are).
func withKey(err error, raw string) error {
	if err == nil || raw == "" {
		return err
	}
	return &keyedError{err: err, keyHash: KeyHash(raw)}
}

// failedKeyHash returns the key hash withKey attached to err, or "".
func failedKeyHash(err error) string {
	var ke *keyedError
	if stderrors.As(err, &ke) {
		return ke.keyHash
	}
	return ""
}

func is429(status int, _ []byte) bool { return status == http.StatusTooManyRequests }
