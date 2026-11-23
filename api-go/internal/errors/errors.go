package errors

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Status  int
	Message string
	Err     error
	// RetryAfter, when > 0, is sent as the Retry-After header (seconds).
	RetryAfter int
}

// NewBusy is a 503 for a request the server declined to start because it is
// at capacity; the client may retry after retryAfter seconds.
func NewBusy(msg string, retryAfter int) *AppError {
	return &AppError{Status: http.StatusServiceUnavailable, Message: msg, RetryAfter: retryAfter}
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewInvalidIDType(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: msg}
}

func NewIDNotFound(msg string) *AppError {
	return &AppError{Status: http.StatusNotFound, Message: msg}
}

func NewBadRequest(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: msg}
}

func NewAPIError(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "API error", Err: err}
}

func NewImageError(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "image error", Err: err}
}

func NewOther(msg string) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: msg}
}

// UpstreamStatusError is the HTTP status a provider answered with when the
// caller has no mapping of its own for it (404 → NewIDNotFound).
type UpstreamStatusError struct {
	Status int
}

func (e *UpstreamStatusError) Error() string {
	return fmt.Sprintf("upstream status %d", e.Status)
}

// NewUpstreamStatus is a provider that answered with an unexpected status: a
// 500 internally, like NewAPIError, whose cause carries the status so
// httpx.WriteAppError can answer a provider 5xx with 502.
func NewUpstreamStatus(msg string, status int) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: msg, Err: &UpstreamStatusError{Status: status}}
}

// ClientMessage returns the error message safe for client exposure: 5xx
// errors are sanitized to "Internal server error" so internal details (SQL
// errors, provider errors, panic traces) don't leak to API callers. 4xx
// errors pass through verbatim because the caller needs the message to fix
// their request. Centralised here so httpx.WriteAppError produces identical
// sanitisation.
func (e *AppError) ClientMessage() string {
	if e.RetryAfter > 0 {
		return e.Message // capacity errors (NewBusy) carry no internal detail
	}
	if e.Status >= 500 {
		return "Internal server error"
	}
	return e.Message
}
