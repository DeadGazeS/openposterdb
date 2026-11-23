package router

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"openposterdb/internal/config"
	"openposterdb/internal/httpx"
)

// rlEntry pairs a token-bucket limiter with the last time it was touched.
// SweepIdle uses lastSeen to evict entries from clients that haven't made a
// request in a long time, bounding the map's memory footprint.
type rlEntry struct {
	lim      *rate.Limiter
	lastSeen atomic.Int64 // unix nanos
	subIP    string       // the IP of a sub-key bucket ("" for a per-IP one)
}

// rateLimiter holds per-IP token-bucket limiters. Created lazily on first
// sight of an IP and reaped by SweepIdle after idleTimeout without traffic.
type rateLimiter struct {
	perMinute   uint64
	burst       int
	limiters    sync.Map // map[string | subKeyBucket]*rlEntry
	count       atomic.Int64
	idleTimeout time.Duration
	overflow    *rate.Limiter // shared bucket once maxLimiterEntries is reached
	// clientIP keys the buckets; the Router sets it to its live
	// trusted-proxy resolver (Settings → General → Config / TRUSTED_PROXIES).
	clientIP func(*http.Request) string
	// subKey, when set, splits an IP's bucket by a request value (the API
	// key prefix on the image routes), up to maxKeyBucketsPerIP per IP.
	subKey    func(*http.Request) string
	mu        sync.Mutex     // guards keysPerIP and sub-key bucket creation
	keysPerIP map[string]int // sub-key buckets per IP
}

// subKeyBucket keys a sub-key bucket; plain per-IP buckets are keyed by the
// IP string, so the two can't collide.
type subKeyBucket struct{ sub, ip string }

// maxLimiterEntries bounds the per-IP map; beyond it new IPs share one
// bucket until idle entries are swept.
const maxLimiterEntries = 100_000

// maxKeyBucketsPerIP caps the sub-key buckets one IP can create, so a client
// sending random API keys can't fill the table (and push every other client
// into the overflow bucket); further keys from that IP share its own bucket.
const maxKeyBucketsPerIP = 8

// Rust image-route limits: 300 requests a minute with a burst of 240 per
// API-key prefix + IP; isValid 10 a second with a burst of 30.
const (
	imageBurst       = 240
	isValidPerMinute = 600
	isValidBurst     = 30
)

// newRateLimiter allows perMinute requests a minute, the whole minute's
// quota at once.
func newRateLimiter(perMinute uint64) *rateLimiter {
	return newRateLimiterBurst(perMinute, int(perMinute))
}

// newRateLimiterBurst allows perMinute requests a minute with bursts of up to
// burst (capped at perMinute, at least 1).
func newRateLimiterBurst(perMinute uint64, burst int) *rateLimiter {
	burst = max(min(burst, int(perMinute)), 1)
	return &rateLimiter{
		perMinute:   perMinute,
		burst:       burst,
		idleTimeout: 5 * time.Minute,
		overflow:    rate.NewLimiter(rate.Limit(float64(perMinute)/60.0), burst),
		clientIP:    defaultClientIP,
	}
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
	burst := max(r.burst, 1)
	if r.count.Load() >= maxLimiterEntries {
		return r.overflow
	}
	e := &rlEntry{lim: rate.NewLimiter(rate.Limit(perSec), burst)}
	e.lastSeen.Store(now)
	actual, loaded := r.limiters.LoadOrStore(ip, e)
	if !loaded {
		r.count.Add(1)
	}
	ae := actual.(*rlEntry)
	ae.lastSeen.Store(now)
	return ae.lim
}

// requestLimiter returns the bucket for a request: per IP, or per sub-key +
// IP when subKey is set. An IP past maxKeyBucketsPerIP sub-key buckets gets
// its own per-IP bucket for any further key.
func (r *rateLimiter) requestLimiter(req *http.Request) *rate.Limiter {
	ip := r.clientIP(req)
	if r.subKey == nil {
		return r.limiter(ip)
	}
	key := subKeyBucket{sub: r.subKey(req), ip: ip}
	now := time.Now().UnixNano()
	if v, ok := r.limiters.Load(key); ok {
		e := v.(*rlEntry)
		e.lastSeen.Store(now)
		return e.lim
	}
	if r.count.Load() >= maxLimiterEntries {
		return r.overflow
	}
	r.mu.Lock()
	if r.keysPerIP[ip] >= maxKeyBucketsPerIP {
		r.mu.Unlock()
		return r.limiter(ip)
	}
	e := &rlEntry{lim: rate.NewLimiter(rate.Limit(float64(r.perMinute)/60.0), max(r.burst, 1)), subIP: ip}
	e.lastSeen.Store(now)
	actual, loaded := r.limiters.LoadOrStore(key, e)
	if !loaded {
		r.count.Add(1)
		if r.keysPerIP == nil {
			r.keysPerIP = make(map[string]int)
		}
		r.keysPerIP[ip]++
	}
	r.mu.Unlock()
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
			if e.subIP != "" {
				r.mu.Lock()
				if _, ok := r.limiters.LoadAndDelete(key); ok {
					if r.keysPerIP[e.subIP]--; r.keysPerIP[e.subIP] <= 0 {
						delete(r.keysPerIP, e.subIP)
					}
					r.count.Add(-1)
					removed++
				}
				r.mu.Unlock()
				return true
			}
			r.limiters.Delete(key)
			r.count.Add(-1)
			removed++
		}
		return true
	})
	return removed
}

func isTrustedProxy(a netip.Addr, trusted []netip.Prefix) bool {
	a = a.Unmap()
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientIPFor returns the address rate limits are keyed on, given the peers
// trusted to set X-Forwarded-For. XFF is only honoured when the connection
// comes from a trusted proxy, and then walked from the right, skipping
// trusted hops: the first untrusted entry is the client. The leftmost
// entries are whatever the client sent, so taking the first one let anyone
// pick a fresh identity per request.
func clientIPFor(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !isTrustedProxy(peer, trusted) {
		return host
	}
	xff := r.Header.Values("X-Forwarded-For")
	hops := strings.Split(strings.Join(xff, ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" {
			continue
		}
		a, err := netip.ParseAddr(hop)
		if err != nil {
			return host // malformed chain: fall back to the proxy itself
		}
		if !isTrustedProxy(a, trusted) {
			return a.Unmap().String()
		}
	}
	return host
}

// defaultClientIP resolves with the default trust (loopback + private);
// limiters created outside a Router use it.
func defaultClientIP(r *http.Request) string {
	return clientIPFor(r, config.DefaultTrustedProxies())
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
			lim := rl.requestLimiter(r)
			r2 := lim.Reserve()
			if !r2.OK() {
				// Should not happen with a non-zero rate; defensively 429.
				httpx.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			if delay := r2.Delay(); delay > 0 {
				r2.Cancel()
				w.Header().Set("Retry-After", formatRetryAfter(delay))
				httpx.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
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
