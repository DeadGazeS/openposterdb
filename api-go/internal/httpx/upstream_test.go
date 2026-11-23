package httpx

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	apperr "openposterdb/internal/errors"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// A provider that can't be reached is 502, one that is too slow 504; our own
// failures stay 500, and 4xx / capacity 503 are untouched.
func TestWriteAppError_UpstreamStatus(t *testing.T) {
	dns := &url.Error{Op: "Get", URL: "https://api.themoviedb.org/3/find/x", Err: &net.DNSError{Err: "no such host", Name: "api.themoviedb.org"}}
	refused := &url.Error{Op: "Get", URL: "https://api.themoviedb.org", Err: &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}}
	slow := &url.Error{Op: "Get", URL: "https://api.themoviedb.org", Err: timeoutErr{}}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"dns failure", apperr.NewAPIError(fmt.Errorf("request failed after 3 retries: %w", dns)), http.StatusBadGateway},
		{"connection refused", apperr.NewAPIError(refused), http.StatusBadGateway},
		{"timeout", apperr.NewAPIError(slow), http.StatusGatewayTimeout},
		{"deadline", apperr.NewAPIError(fmt.Errorf("tmdb: %w", context.DeadlineExceeded)), http.StatusGatewayTimeout},
		{"upstream 5xx", apperr.NewUpstreamStatus("TMDB API returned 503", 503), http.StatusBadGateway},
		{"wrapped upstream 5xx", fmt.Errorf("resolve: %w", apperr.NewUpstreamStatus("TVDB returned 500", 500)), http.StatusBadGateway},
		{"upstream 4xx", apperr.NewUpstreamStatus("TMDB API returned 401", 401), http.StatusInternalServerError},
		{"own failure", apperr.NewOther("render failed"), http.StatusInternalServerError},
		{"not found", apperr.NewIDNotFound("x"), http.StatusNotFound},
		{"busy", apperr.NewBusy("busy", 5), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		WriteAppError(rec, tc.err)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
}
