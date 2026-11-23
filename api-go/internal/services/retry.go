package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperr "openposterdb/internal/errors"
)

type RetryConfig struct {
	MaxRetries  uint32
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	ServiceName string
}

var TMDBAPIRetry = RetryConfig{
	MaxRetries:  2,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    4 * time.Second,
	ServiceName: "tmdb",
}

var TMDBCDNRetry = RetryConfig{
	MaxRetries:  2,
	BaseDelay:   1 * time.Second,
	MaxDelay:    8 * time.Second,
	ServiceName: "tmdb-cdn",
}

var FanartRetry = RetryConfig{
	MaxRetries:  3,
	BaseDelay:   1 * time.Second,
	MaxDelay:    8 * time.Second,
	ServiceName: "fanart",
}

var OMDBRetry = RetryConfig{
	MaxRetries:  1,
	BaseDelay:   2 * time.Second,
	MaxDelay:    2 * time.Second,
	ServiceName: "omdb",
}

var MDBListRetry = RetryConfig{
	MaxRetries:  1,
	BaseDelay:   2 * time.Second,
	MaxDelay:    2 * time.Second,
	ServiceName: "mdblist",
}

var TraktRetry = RetryConfig{
	MaxRetries:  1,
	BaseDelay:   2 * time.Second,
	MaxDelay:    2 * time.Second,
	ServiceName: "trakt",
}

var KitsuRetry = RetryConfig{
	MaxRetries:  1,
	BaseDelay:   2 * time.Second,
	MaxDelay:    4 * time.Second,
	ServiceName: "kitsu",
}

var MALRetry = RetryConfig{
	MaxRetries:  1,
	BaseDelay:   2 * time.Second,
	MaxDelay:    4 * time.Second,
	ServiceName: "anilist",
}

// RetryAfter parses a Retry-After header (delay in seconds or an HTTP date)
// on a 429 or 503 response. ok is false when there is none.
func RetryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable) {
		return 0, false
	}
	val := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if val == "" {
		return 0, false
	}
	if secs, err := strconv.ParseUint(val, 10, 32); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(val); err == nil {
		return max(time.Until(t), 0), true
	}
	return 0, false
}

func retryAfterDelay(resp *http.Response, config *RetryConfig, attempt uint32) time.Duration {
	if d, ok := RetryAfter(resp); ok {
		return addJitter(min(d, config.MaxDelay))
	}
	return backoffDelay(config, attempt)
}

func backoffDelay(config *RetryConfig, attempt uint32) time.Duration {
	delay := min(config.BaseDelay*(1<<attempt), config.MaxDelay)
	return addJitter(delay)
}

func addJitter(delay time.Duration) time.Duration {
	jitterMs := int64(delay/time.Millisecond) / 4
	if jitterMs > 0 {
		ms := rand.Int63n(jitterMs + 1)
		return delay + time.Duration(ms)*time.Millisecond
	}
	return delay
}

type RequestFunc func() (*http.Response, error)

func SendWithRetry(config *RetryConfig, requestFn RequestFunc) (*http.Response, error) {
	return SendWithRetryCtx(context.Background(), config, requestFn)
}

// SendWithRetryCtx is SendWithRetry bound to ctx: once ctx is done it neither
// sleeps through the backoff nor issues another attempt, and returns an error
// wrapping ctx.Err().
func SendWithRetryCtx(ctx context.Context, config *RetryConfig, requestFn RequestFunc) (*http.Response, error) {
	for attempt := uint32(0); attempt <= config.MaxRetries; attempt++ {
		resp, err := requestFn()

		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("%s request cancelled: %w", config.ServiceName, ctxErr)
			}
			slog.Warn(fmt.Sprintf("%s connection error, retrying", config.ServiceName),
				"attempt", fmt.Sprintf("%d/%d", attempt+1, config.MaxRetries),
				"error", apperr.RedactURLSecrets(err),
			)
			if attempt == config.MaxRetries {
				return nil, fmt.Errorf("request failed after %d retries: %w", config.MaxRetries, err)
			}
			if err := sleepCtx(ctx, backoffDelay(config, attempt)); err != nil {
				return nil, fmt.Errorf("%s request cancelled: %w", config.ServiceName, err)
			}
			continue
		}

		status := resp.StatusCode
		retryable := status == http.StatusTooManyRequests || status >= 500
		if !retryable {
			return resp, nil
		}
		if attempt == config.MaxRetries {
			slog.Warn(fmt.Sprintf("%s request failed after retries: %d", config.ServiceName, status),
				"attempt", config.MaxRetries,
			)
			return resp, nil
		}
		// The provider asks for a longer pause than this policy waits:
		// retrying now would only be refused again (and count against the
		// key). Hand the response back so the caller can block the key.
		if d, ok := RetryAfter(resp); ok && d > config.MaxDelay {
			slog.Warn(fmt.Sprintf("%s %d with Retry-After beyond the retry policy, not retrying", config.ServiceName, status),
				"retry_after_s", int64(d.Seconds()))
			return resp, nil
		}
		var delay time.Duration
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			delay = retryAfterDelay(resp, config, attempt)
		} else {
			delay = backoffDelay(config, attempt)
		}
		slog.Warn(fmt.Sprintf("%s %d, retrying", config.ServiceName, status),
			"attempt", fmt.Sprintf("%d/%d", attempt+1, config.MaxRetries),
			"delay_ms", delay.Milliseconds(),
		)
		// Release the connection before waiting, not after.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err := sleepCtx(ctx, delay); err != nil {
			return nil, fmt.Errorf("%s request cancelled: %w", config.ServiceName, err)
		}
	}

	return nil, fmt.Errorf("unreachable: retry loop exited without returning")
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
