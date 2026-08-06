package services

import (
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
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

func NewRetryConfig(maxRetries uint32, baseDelay, maxDelay time.Duration, serviceName string) RetryConfig {
	return RetryConfig{
		MaxRetries:  maxRetries,
		BaseDelay:   baseDelay,
		MaxDelay:    maxDelay,
		ServiceName: serviceName,
	}
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

func retryAfterDelay(resp *http.Response, config *RetryConfig, attempt uint32) time.Duration {
	if val := resp.Header.Get("Retry-After"); val != "" {
		if secs, err := strconv.ParseUint(val, 10, 64); err == nil {
			capped := min(time.Duration(secs)*time.Second, config.MaxDelay)
			return addJitter(capped)
		}
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
	for attempt := uint32(0); attempt <= config.MaxRetries; attempt++ {
		resp, err := requestFn()

		if err != nil {
			slog.Warn(fmt.Sprintf("%s connection error, retrying", config.ServiceName),
				"attempt", fmt.Sprintf("%d/%d", attempt+1, config.MaxRetries),
				"error", apperr.RedactURLSecrets(err),
			)
			if attempt == config.MaxRetries {
				return nil, fmt.Errorf("request failed after %d retries: %w", config.MaxRetries, err)
			}
			delay := backoffDelay(config, attempt)
			time.Sleep(delay)
			continue
		}

		status := resp.StatusCode

		if status == http.StatusTooManyRequests {
			if attempt == config.MaxRetries {
				slog.Warn(fmt.Sprintf("%s request failed after retries: 429 Too Many Requests", config.ServiceName),
					"attempt", config.MaxRetries,
				)
				return resp, nil
			}
			delay := retryAfterDelay(resp, config, attempt)
			slog.Warn(fmt.Sprintf("%s 429 Too Many Requests, retrying", config.ServiceName),
				"attempt", fmt.Sprintf("%d/%d", attempt+1, config.MaxRetries),
				"delay_ms", delay.Milliseconds(),
			)
			time.Sleep(delay)
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}

		if status >= 500 {
			if attempt == config.MaxRetries {
				slog.Warn(fmt.Sprintf("%s request failed after retries: %d", config.ServiceName, status),
					"attempt", config.MaxRetries,
				)
				return resp, nil
			}
			delay := backoffDelay(config, attempt)
			slog.Warn(fmt.Sprintf("%s %d, retrying", config.ServiceName, status),
				"attempt", fmt.Sprintf("%d/%d", attempt+1, config.MaxRetries),
				"delay_ms", delay.Milliseconds(),
			)
			time.Sleep(delay)
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("unreachable: retry loop exited without returning")
}
