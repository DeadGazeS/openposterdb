package router

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// rlEntry pairs a token-bucket limiter with the last time it was touched.
// SweepIdle uses lastSeen to evict entries from clients that haven't made a
// request in a long time, bounding the map's memory footprint.
type rlEntry struct {
	lim      *rate.Limiter
	lastSeen atomic.Int64 // unix nanos
}

// rateLimiter holds per-IP token-bucket limiters. Created lazily on first
// sight of an IP and reaped by SweepIdle after idleTimeout without traffic.
type rateLimiter struct {
	perMinute    uint64
	limiters     sync.Map // map[string]*rlEntry
	idleTimeout  time.Duration
}

func newRateLimiter(perMinute uint64) *rateLimiter {
	return &rateLimiter{perMinute: perMinute, idleTimeout: 5 * time.Minute}
}

// limiter returns (creating on first sight) the per-IP token bucket. The
// rate is `perMinute / 60` events per second with a burst of `perMinute` so
// that a freshly seen IP can spend the whole minute's quota in a short burst
// before being throttled. The lastSeen stamp is updated on every call so
// active IPs are not reaped.
func (r *rateLimiter) limiter(ip string) *rate.Limiter {
	now := time.Now().UnixNano()
	if v, ok := r.limiters.Load(ip); ok {
		e := v.(*rlEntry)
		e.lastSeen.Store(now)
		return e.lim
	}
	perSec := float64(r.perMinute) / 60.0
	burst := int(r.perMinute)
	if burst < 1 {
		burst = 1
	}
	e := &rlEntry{lim: rate.NewLimiter(rate.Limit(perSec), burst)}
	e.lastSeen.Store(now)
	actual, _ := r.limiters.LoadOrStore(ip, e)
	ae := actual.(*rlEntry)
	ae.lastSeen.Store(now)
	return ae.lim
}

// SweepIdle evicts per-IP entries whose lastSeen is older than the idle
// timeout. Returns the number of entries removed. Safe to call concurrently.
func (r *rateLimiter) SweepIdle() int {
	cutoff := time.Now().Add(-r.idleTimeout).UnixNano()
	removed := 0
	r.limiters.Range(func(key, value any) bool {
		e := value.(*rlEntry)
		if e.lastSeen.Load() < cutoff {
			r.limiters.Delete(key)
			removed++
		}
		return true
	})
	return removed
}

// clientIP extracts the best-effort client IP from the request. It honours
// X-Forwarded-For's first comma-separated entry (when present) — deployments
// behind a trusted reverse proxy can pass the real client. Otherwise it
// falls back to RemoteAddr's host portion.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimit returns a middleware that admits requests only when the client's
// IP has tokens available. perMinute=0 disables the limit (pass-through) so
// tests/dev environments don't need to opt out explicitly.
//
// On a deny it returns 429 with a Retry-After header derived from the
// limiter's reserve delay (rounded up to the next whole second).
func RateLimit(rl *rateLimiter) func(http.Handler) http.Handler {
	if rl == nil || rl.perMinute == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			lim := rl.limiter(ip)
			r2 := lim.Reserve()
			if !r2.OK() {
				// Should not happen with a non-zero rate; defensively 429.
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			if delay := r2.Delay(); delay > 0 {
				r2.Cancel()
				w.Header().Set("Retry-After", formatRetryAfter(delay))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// formatRetryAfter converts a duration to the integer seconds form
// Retry-After accepts (per RFC 7231).
func formatRetryAfter(d interface{ Seconds() float64 }) string {
	secs := int(d.Seconds() + 0.999)
	if secs < 1 {
		secs = 1
	}
	return itoa(secs)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}
