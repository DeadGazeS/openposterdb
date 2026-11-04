package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openposterdb/internal/config"
)

// cfStub serves the two cloudflare.com lists (or a failure), counting calls.
type cfStub struct {
	calls atomic.Int32
	v4    string
	v6    string
	fail  atomic.Bool
}

func (s *cfStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	body, status := s.v6, 200
	if strings.HasSuffix(req.URL.Path, "ips-v4") {
		body = s.v4
	}
	if s.fail.Load() {
		body, status = "<html>maintenance</html>", 200
	}
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func builtinLines(v6 bool) string {
	var lines []string
	for _, p := range config.BuiltinCloudflareProxies() {
		if p.Addr().Is6() == v6 {
			lines = append(lines, p.String())
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func resetCloudflare(t *testing.T) {
	t.Cleanup(func() { config.SetCloudflareRanges(config.BuiltinCloudflareProxies()) })
}

// Ticking "Cloudflare" checks cloudflare.com; a changed list is applied at
// once (the rate limiter's trust follows it) and stored; further checks
// within the cooldown don't contact cloudflare.com.
func TestCloudflareCheck_UpdatesAndCoolsDown(t *testing.T) {
	resetCloudflare(t)
	r := authTestRouter(t, 0)
	stub := &cfStub{v4: builtinLines(false) + "203.0.113.0/24\n", v6: builtinLines(true)}
	clock := time.Unix(1_800_000_000, 0)
	r.cloudflare = newCFUpdater(r.state.DB, &http.Client{Transport: stub})
	r.cloudflare.now = func() time.Time { return clock }

	// Before: 203.0.113.9 is not a Cloudflare edge, so it is taken as the client.
	r.trust = newProxyTrust(&config.Config{TrustedProxiesEnv: "private,cloudflare"}, r.state.DB)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.18.0.3:1"
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 203.0.113.9")
	if got := r.trust.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("before update: %q", got)
	}

	res := r.cloudflare.check(context.Background())
	if !res.Checked || !res.Changed || res.Source != "cloudflare.com" || res.Ranges != len(config.BuiltinCloudflareProxies())+1 {
		t.Fatalf("first check: %+v", res)
	}
	if got := r.trust.clientIP(req); got != "9.9.9.9" {
		t.Fatalf("after update the new edge range must be trusted, got %q", got)
	}

	calls := stub.calls.Load()
	clock = clock.Add(23 * time.Hour)
	if res := r.cloudflare.check(context.Background()); res.Checked {
		t.Fatalf("within the cooldown the check must be skipped: %+v", res)
	}
	if stub.calls.Load() != calls {
		t.Fatal("cloudflare.com contacted during the cooldown")
	}
	clock = clock.Add(2 * time.Hour)
	if res := r.cloudflare.check(context.Background()); !res.Checked || res.Changed {
		t.Fatalf("after the cooldown: %+v", res)
	}

	// A restart restores the fetched list from the database.
	config.SetCloudflareRanges(config.BuiltinCloudflareProxies())
	restarted := newCFUpdater(r.state.DB, &http.Client{Transport: stub})
	if st := restarted.status(); st.Source != "cloudflare.com" || st.Ranges != len(config.BuiltinCloudflareProxies())+1 || st.CheckedAt == 0 {
		t.Fatalf("after restart: %+v", st)
	}
}

// A broken response never replaces the list; failures are retried only after
// the short failure window.
func TestCloudflareCheck_FailureKeepsListAndBacksOff(t *testing.T) {
	resetCloudflare(t)
	r := authTestRouter(t, 0)
	stub := &cfStub{v4: builtinLines(false), v6: builtinLines(true)}
	stub.fail.Store(true)
	clock := time.Unix(1_800_000_000, 0)
	r.cloudflare = newCFUpdater(r.state.DB, &http.Client{Transport: stub})
	r.cloudflare.now = func() time.Time { return clock }

	res := r.cloudflare.check(context.Background())
	if !res.Checked || res.Error == "" || res.Source != "built-in" || res.Ranges != len(config.BuiltinCloudflareProxies()) {
		t.Fatalf("failed check: %+v", res)
	}
	calls := stub.calls.Load()
	clock = clock.Add(5 * time.Minute)
	if res := r.cloudflare.check(context.Background()); res.Checked || stub.calls.Load() != calls {
		t.Fatalf("retry inside the failure window: %+v", res)
	}
	stub.fail.Store(false)
	clock = clock.Add(6 * time.Minute)
	if res := r.cloudflare.check(context.Background()); !res.Checked || res.Error != "" || res.Changed {
		t.Fatalf("retry after the window: %+v", res)
	}
}

func TestParseCloudflareList_Rejects(t *testing.T) {
	for name, text := range map[string]string{
		"html":     "<html>error</html>",
		"too few":  "173.245.48.0/20\n",
		"catchall": strings.Repeat("173.245.48.0/20\n", 5) + "0.0.0.0/0\n",
	} {
		if _, err := parseCloudflareList(text); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := parseCloudflareList(builtinLines(false) + builtinLines(true)); err != nil {
		t.Errorf("built-in list rejected: %v", err)
	}
}
