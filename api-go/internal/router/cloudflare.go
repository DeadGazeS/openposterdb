package router

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"openposterdb/internal/config"
	"openposterdb/internal/services"
)

// Cloudflare range updates (Settings → General → Config: ticking
// "Cloudflare" asks the server to check cloudflare.com for a newer list).
//
// Cloudflare changes these ranges rarely and announces it well ahead, so a
// successful check is reused for cfCheckCooldown; a failed one is retried
// after cfRetryAfterFailure. The server-side cooldown is what stops repeated
// ticking (or several tabs/admins) from hitting cloudflare.com every time;
// the UI additionally debounces the checkbox.
const (
	cfCheckCooldown     = 24 * time.Hour
	cfRetryAfterFailure = 10 * time.Minute
	cfFetchTimeout      = 10 * time.Second
	cfRangesSetting     = "cloudflare_ranges"
	cfCheckedSetting    = "cloudflare_ranges_checked_at"
)

var cfSourceURLs = []string{"https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6"}

type cfUpdater struct {
	db     *sql.DB
	client *http.Client
	now    func() time.Time

	// mu is held for a whole check, so concurrent callers wait for the
	// running fetch and then see its result through the cooldown.
	mu          sync.Mutex
	lastSuccess time.Time
	lastAttempt time.Time
	lastErr     string
	source      string // "built-in" or "cloudflare.com"
}

type cfStatus struct {
	Ranges    int    `json:"ranges"`
	Source    string `json:"source"`
	CheckedAt int64  `json:"checked_at"` // unix seconds of the last successful check; 0 = never
	Error     string `json:"error,omitempty"`
}

type cfCheckResult struct {
	cfStatus
	Checked     bool  `json:"checked"` // false: skipped (checked recently)
	Changed     bool  `json:"changed"`
	NextCheckAt int64 `json:"next_check_at"`
}

// newCFUpdater restores ranges fetched by a previous run (so an update
// survives restarts); otherwise the built-in list stays in force.
func newCFUpdater(db *sql.DB, client *http.Client) *cfUpdater {
	if client == nil {
		client = &http.Client{}
	}
	u := &cfUpdater{db: db, client: client, now: time.Now, source: "built-in"}
	if db == nil {
		return u
	}
	ctx := context.Background()
	raw, err := services.GetGlobalSettingCtx(ctx, db, cfRangesSetting)
	if err != nil || raw == "" {
		return u
	}
	ranges, err := parseCloudflareList(strings.ReplaceAll(raw, ",", "\n"))
	if err != nil {
		slog.Warn("stored Cloudflare ranges are invalid; using the built-in list", "error", err)
		return u
	}
	config.SetCloudflareRanges(ranges)
	u.source = "cloudflare.com"
	if v, _ := services.GetGlobalSettingCtx(ctx, db, cfCheckedSetting); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			u.lastSuccess = time.Unix(secs, 0)
		}
	}
	return u
}

func (u *cfUpdater) status() cfStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.statusLocked()
}

func (u *cfUpdater) statusLocked() cfStatus {
	st := cfStatus{Ranges: len(config.CloudflareProxies()), Source: u.source, Error: u.lastErr}
	if !u.lastSuccess.IsZero() {
		st.CheckedAt = u.lastSuccess.Unix()
	}
	return st
}

// check fetches the published ranges unless a check ran recently, and
// applies + persists them when they differ from the current list.
func (u *cfUpdater) check(ctx context.Context) cfCheckResult {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.now()
	if !u.lastSuccess.IsZero() && now.Sub(u.lastSuccess) < cfCheckCooldown {
		return cfCheckResult{cfStatus: u.statusLocked(), NextCheckAt: u.lastSuccess.Add(cfCheckCooldown).Unix()}
	}
	if u.lastErr != "" && now.Sub(u.lastAttempt) < cfRetryAfterFailure {
		return cfCheckResult{cfStatus: u.statusLocked(), NextCheckAt: u.lastAttempt.Add(cfRetryAfterFailure).Unix()}
	}
	u.lastAttempt = now

	fetched, err := u.fetch(ctx)
	if err != nil {
		u.lastErr = err.Error()
		slog.Warn("Cloudflare range check failed; keeping the current list", "error", err)
		return cfCheckResult{cfStatus: u.statusLocked(), Checked: true, NextCheckAt: now.Add(cfRetryAfterFailure).Unix()}
	}
	u.lastErr = ""
	u.lastSuccess = now
	changed := !samePrefixes(fetched, config.CloudflareProxies())
	if changed {
		config.SetCloudflareRanges(fetched)
		slog.Info("Cloudflare IP ranges updated", "ranges", len(fetched))
	}
	u.source = "cloudflare.com"
	if u.db != nil {
		parts := make([]string, len(fetched))
		for i, p := range fetched {
			parts[i] = p.String()
		}
		if err := services.SetGlobalSettingsBatchCtx(ctx, u.db, map[string]string{
			cfRangesSetting:  strings.Join(parts, ","),
			cfCheckedSetting: strconv.FormatInt(now.Unix(), 10),
		}); err != nil {
			slog.Warn("storing the Cloudflare ranges failed", "error", err)
		}
	}
	return cfCheckResult{cfStatus: u.statusLocked(), Checked: true, Changed: changed, NextCheckAt: now.Add(cfCheckCooldown).Unix()}
}

func (u *cfUpdater) fetch(ctx context.Context) ([]netip.Prefix, error) {
	ctx, cancel := context.WithTimeout(ctx, cfFetchTimeout)
	defer cancel()
	var all bytes.Buffer
	for _, url := range cfSourceURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", services.UserAgent())
		resp, err := u.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", url, err)
		}
		body, err := services.ReadAllLimited(resp.Body, 64<<10)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", url, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s returned %d", url, resp.StatusCode)
		}
		all.Write(body)
		all.WriteByte('\n')
	}
	return parseCloudflareList(all.String())
}

// parseCloudflareList parses one CIDR per line and sanity-checks the result,
// so an error page or a truncated response never replaces a working list.
func parseCloudflareList(text string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p, err := netip.ParsePrefix(line)
		if err != nil {
			return nil, fmt.Errorf("unexpected line %q", line)
		}
		if p.Bits() < 8 {
			return nil, fmt.Errorf("implausibly wide range %s", p)
		}
		out = append(out, p.Masked())
	}
	if len(out) < 5 || len(out) > 200 {
		return nil, fmt.Errorf("unexpected number of ranges: %d", len(out))
	}
	return out, nil
}

func samePrefixes(a, b []netip.Prefix) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(p []netip.Prefix) []string {
		s := make([]string, len(p))
		for i, x := range p {
			s[i] = x.String()
		}
		slices.Sort(s)
		return s
	}
	return slices.Equal(key(a), key(b))
}
