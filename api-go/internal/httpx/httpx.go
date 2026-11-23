// Package httpx provides the small HTTP helpers shared by the router and the
// handlers package: JSON responses, JSON request decoding, and query params.
package httpx

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	apperr "openposterdb/internal/errors"
)

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// WriteError writes a JSON {"error": message} response with the given status.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// WriteAppError writes an *errors.AppError as a JSON {"error": message}
// response using the AppError's status + client-safe message. For non-AppError
// errors (or nil) it falls back to a 500 with a sanitized message — never
// leaks the raw err.Error() to API callers, mirroring the central sanitisation
// rule in errors.AppError.ClientMessage. Prefer this over WriteError(w, 500,
// err.Error()) wherever a handler returns an error it didn't construct itself.
func WriteAppError(w http.ResponseWriter, err error) {
	if err == nil {
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	var ae *apperr.AppError
	if stderrors.As(err, &ae) {
		if ae.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(ae.RetryAfter))
		}
		if ae.Status == http.StatusInternalServerError {
			if status, msg := upstreamFailure(err); status != 0 {
				WriteJSON(w, status, map[string]string{"error": msg})
				return
			}
		}
		WriteJSON(w, ae.Status, map[string]string{"error": ae.ClientMessage()})
		return
	}
	WriteError(w, http.StatusInternalServerError, "Internal server error")
}

// upstreamFailure tells a provider that could not be reached or answered
// with a server error (502), or did not answer in time (504), from a failure
// of our own (0: stays 500), by the network error or provider status the 500
// wraps.
func upstreamFailure(err error) (int, string) {
	var ne net.Error
	if stderrors.Is(err, context.DeadlineExceeded) || (stderrors.As(err, &ne) && ne.Timeout()) {
		return http.StatusGatewayTimeout, "Upstream provider did not answer in time"
	}
	var us *apperr.UpstreamStatusError
	if stderrors.As(err, &us) && us.Status >= 500 {
		return http.StatusBadGateway, "Upstream provider returned an error"
	}
	var opErr *net.OpError
	var dnsErr *net.DNSError
	if stderrors.As(err, &opErr) || stderrors.As(err, &dnsErr) || stderrors.As(err, &ne) {
		return http.StatusBadGateway, "Upstream provider could not be reached"
	}
	return 0, ""
}

// MaxJSONBody bounds a JSON request body. Every JSON body the API accepts
// (settings, keys, credentials) is a few KB; the settings import, which can
// be larger, reads its body itself.
const MaxJSONBody = 1 << 20

// DecodeJSON decodes the request body as JSON into v and closes the body.
// At most MaxJSONBody bytes are read; a larger body fails to decode.
func DecodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, MaxJSONBody)).Decode(v)
}

// ParseIntParam reads a positive integer query parameter, falling back to def
// when the parameter is missing, unparsable, or smaller than 1.
func ParseIntParam(r *http.Request, name string, def int64) int64 {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return def
	}
	return n
}
