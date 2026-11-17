package services

import (
	"context"
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
// status are returned; the caller interprets non-limit statuses.
func pooledDo(ctx context.Context, pool *APIKeyPool, c pooledCall) (int, []byte, string, error) {
	var lastStatus int
	var lastBody []byte
	for range pool.Len() {
		key, ok := pool.ActiveKey()
		if !ok {
			return 0, nil, "", errors.NewOther(fmt.Sprintf("%s: every API key is rate limited until %s", c.service, pool.BlockedUntil().UTC().Format(time.RFC3339)))
		}
		resp, err := SendWithRetryCtx(ctx, c.retry, func() (*http.Response, error) { return c.do(key) })
		if err != nil {
			return 0, nil, key, errors.NewAPIError(errors.RedactURLSecrets(err))
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
		resp.Body.Close()
		if !c.limited(resp.StatusCode, body) {
			pool.ReportSuccess()
			return resp.StatusCode, body, key, nil
		}
		var until time.Time
		if d, ok := RetryAfter(resp); ok {
			until = time.Now().Add(d)
		} else if c.limitBlock > 0 {
			until = time.Now().Add(c.limitBlock)
		}
		pool.Report429For(key, until)
		slog.Warn(c.service+" API key rate limited", "key", KeyHash(key), "status", resp.StatusCode)
		lastStatus, lastBody = resp.StatusCode, body
	}
	return lastStatus, lastBody, "", errors.NewOther(fmt.Sprintf("%s rate limited (%d): %s", c.service, lastStatus, errSnippet(lastBody)))
}

func is429(status int, _ []byte) bool { return status == http.StatusTooManyRequests }
