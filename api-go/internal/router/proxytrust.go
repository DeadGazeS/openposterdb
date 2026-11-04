package router

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"openposterdb/internal/config"
	"openposterdb/internal/services"
)

// trustedProxiesSetting is the global_settings key the admin UI writes
// (Settings → General → Config). Same syntax as TRUSTED_PROXIES.
const trustedProxiesSetting = "trusted_proxies"

// proxyTrust resolves which peers may set X-Forwarded-For: the
// TRUSTED_PROXIES env value when set (it locks the setting), otherwise the
// stored setting, read through the settings cache so a save or an import
// applies on the next request. Each distinct value is parsed once per
// Cloudflare range generation (the "cloudflare" keyword follows updates).
type proxyTrust struct {
	envRaw string // non-empty when TRUSTED_PROXIES is set
	db     *sql.DB

	mu      sync.Mutex
	memoRaw string
	memoGen uint64
	memo    []netip.Prefix
	memoOK  bool
}

func newProxyTrust(cfg *config.Config, db *sql.DB) *proxyTrust {
	t := &proxyTrust{db: db}
	if cfg != nil {
		t.envRaw = cfg.TrustedProxiesEnv
	}
	return t
}

func (t *proxyTrust) locked() bool { return t.envRaw != "" }

// storedRaw returns the stored setting ("" when unset or unreadable).
func (t *proxyTrust) storedRaw(ctx context.Context) string {
	if t.db == nil {
		return ""
	}
	return services.GlobalSettingCachedCtx(ctx, t.db, trustedProxiesSetting)
}

// effectiveRaw is the value in force: the env value, else the stored one.
func (t *proxyTrust) effectiveRaw(ctx context.Context) string {
	if t.locked() {
		return t.envRaw
	}
	return t.storedRaw(ctx)
}

func (t *proxyTrust) current(ctx context.Context) []netip.Prefix {
	raw := t.effectiveRaw(ctx)
	gen := config.CloudflareGeneration()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.memoOK && raw == t.memoRaw && gen == t.memoGen {
		return t.memo
	}
	parsed, err := config.ParseTrustedProxies(raw)
	if err != nil {
		// Only valid values are saved through the API and the env value is
		// validated at startup; anything else (a hand-edited row) falls back
		// to the default.
		slog.Warn("trusted_proxies value is invalid; using the default", "error", err)
		parsed = config.DefaultTrustedProxies()
	}
	t.memoRaw, t.memoGen, t.memo, t.memoOK = raw, gen, parsed, true
	return parsed
}

func (t *proxyTrust) clientIP(r *http.Request) string {
	return clientIPFor(r, t.current(r.Context()))
}

// proxySettings is the structured form the admin UI edits.
type proxySettings struct {
	TrustPrivate    bool     `json:"trust_private"`
	TrustCloudflare bool     `json:"trust_cloudflare"`
	Custom          []string `json:"custom"`
}

// parseProxySettings splits a TRUSTED_PROXIES-syntax value into the
// structured form ("" = the default: private only).
func parseProxySettings(raw string) proxySettings {
	raw = strings.TrimSpace(raw)
	s := proxySettings{Custom: []string{}}
	switch strings.ToLower(raw) {
	case "", "private":
		s.TrustPrivate = true
		return s
	case "none":
		return s
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.EqualFold(part, "private"):
			s.TrustPrivate = true
		case strings.EqualFold(part, "cloudflare"):
			s.TrustCloudflare = true
		default:
			s.Custom = append(s.Custom, part)
		}
	}
	return s
}

// raw serializes the structured form back to TRUSTED_PROXIES syntax.
func (s proxySettings) raw() string {
	var parts []string
	if s.TrustPrivate {
		parts = append(parts, "private")
	}
	if s.TrustCloudflare {
		parts = append(parts, "cloudflare")
	}
	for _, c := range s.Custom {
		if c = strings.TrimSpace(c); c != "" {
			parts = append(parts, c)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}
