package router

import (
	"log/slog"
	"net/http"
	"openposterdb/internal/config"
	"path/filepath"
	"strings"
	"time"

	"openposterdb/internal/handlers"
	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// adminImageKinds describes the four image kinds managed under /api/admin:
// the single-letter kind used by the cache layer, the plural URL segment, and
// whether the image file is served on the item path itself (logos/backdrops)
// or on a /image subroute (posters/episodes).
var adminImageKinds = []struct {
	kind        string
	plural      string
	imageOnBase bool
}{
	{"p", "posters", false},
	{"l", "logos", true},
	{"b", "backdrops", true},
	{"e", "episodes", false},
}

// previewKinds are the four admin/self-service preview endpoints.
var previewKinds = []string{"poster", "logo", "backdrop", "episode"}

func (r *Router) registerRoutes() {
	r.registerAuthRoutes()
	r.registerKeyRoutes()
	r.registerAdminRoutes()
	r.registerKindRoutes()
	r.registerPreviewRoutes()
	r.registerOpenAPIRoute()
	r.registerImageRoutes()
	r.setupStatic()
}

// registerOpenAPIRoute wires GET /api/openapi.json. The spec is public by
// default so a CDN edge can cache it; when DISABLE_PUBLIC_PAGES=true the
// route is gated behind admin auth.
func (r *Router) registerOpenAPIRoute() {
	openapiHandler := http.HandlerFunc(handlers.HandleOpenAPISpec)
	if r.state.Config.DisablePublicPages {
		r.mux.Handle("/api/openapi.json", handlers.RequireAuth(r.jwtSecret())(openapiHandler))
		return
	}
	r.mux.Handle("/api/openapi.json", openapiHandler)
}

func (r *Router) registerAuthRoutes() {
	// Per-IP limit on the credential endpoints (login/setup/key-login): they
	// were unthrottled, allowing unlimited password guessing.
	authLimiter := newRateLimiter(r.state.Config.RateLimitAuthRPM)
	authLimiter.clientIP = r.trust.clientIP
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			authLimiter.SweepIdle()
		}
	}()
	authRateLimit := RateLimit(authLimiter)
	throttled := func(pattern string, h http.HandlerFunc) {
		r.mux.Handle(pattern, authRateLimit(h))
	}
	s := r.state

	r.mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, req *http.Request) {
		status, resp := handlers.AuthStatus(req.Context(), s.DB, s.isFreeAPIKeyEnabled, s.Config.DisablePublicPages)
		httpx.WriteJSON(w, status, resp)
	})

	throttled("/api/auth/setup", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = httpx.DecodeJSON(req, &body)

		status, resp, cookies := handlers.SetupHandler(req.Context(), s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		httpx.WriteJSON(w, status, resp)
	})

	throttled("/api/auth/login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = httpx.DecodeJSON(req, &body)

		status, resp, cookies := handlers.LoginHandler(req.Context(), s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		httpx.WriteJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		cookie, _ := req.Cookie("refresh_token")
		refreshToken := ""
		if cookie != nil {
			refreshToken = cookie.Value
		}
		status, resp, cookies := handlers.RefreshHandler(req.Context(), s.DB, r.jwtSecret(), s.SecureCookies, refreshToken)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		httpx.WriteJSON(w, status, resp)
	})

	// Logout. The refresh cookie is path-scoped to /api/auth/refresh, so the
	// browser only sends it to paths under that prefix: the SPA logs out via
	// /api/auth/refresh/logout, which revokes exactly this refresh token.
	// Both endpoints also accept the bearer access token and then revoke all
	// of that user's refresh tokens, and both clear the cookie on its own
	// path (clearing it on "/" left the real cookie in place).
	logout := func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		if c, _ := req.Cookie("refresh_token"); c != nil && c.Value != "" {
			if err := services.DeleteRefreshTokenByHashCtx(req.Context(), s.DB, handlers.HashRefreshToken(c.Value)); err != nil {
				slog.Warn("logout: revoking refresh token failed", "error", err)
			}
		}
		if token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer "); ok {
			if claims, err := handlers.ParseJWT(token, r.jwtSecret()); err == nil && claims.Username != "" {
				handlers.LogoutHandler(req.Context(), s.DB, claims.Username)
			}
		}
		http.SetCookie(w, handlers.ClearRefreshCookie(s.SecureCookies))
		httpx.WriteJSON(w, 200, map[string]bool{"success": true})
	}
	r.mux.HandleFunc("/api/auth/logout", logout)
	r.mux.HandleFunc("/api/auth/refresh/logout", logout)

	throttled("/api/auth/key-login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		_ = httpx.DecodeJSON(req, &body)
		status, resp := handlers.KeyLoginHandler(req.Context(), s.DB, r.jwtSecret(), body.APIKey)
		httpx.WriteJSON(w, status, resp)
	})
}

func (r *Router) registerKeyRoutes() {
	s := r.state

	r.mux.Handle("/api/keys", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleListKeys(s.DB, s.SecretsKey)(w, req)
		case http.MethodPost:
			handlers.HandleCreateKey(s.DB, s.SecretsKey)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/keys/{id}", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleDeleteKey(s.DB)(w, req)
	})))

	r.mux.Handle("/api/keys/{id}/reveal", handlers.RequireAuth(r.jwtSecret())(handlers.HandleRevealKey(s.DB, s.SecretsKey)))

	r.mux.Handle("/api/keys/{id}/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetKeySettings(s.DB, s.fanartAvailable())(w, req)
		case http.MethodPut:
			handlers.HandleUpdateKeySettings(s.DB)(w, req)
		case http.MethodDelete:
			handlers.HandleResetKeySettings(s.DB)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/key/me", handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed(), r.state.DB)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleSelfKeyInfo(s.DB, s.SecretsKey)(w, req)
	})))

	r.mux.Handle("/api/key/me/settings", handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed(), r.state.DB)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleSelfSettings(s.DB, s.fanartAvailable())(w, req)
		case http.MethodPut:
			handlers.HandleUpdateSelfSettings(s.DB)(w, req)
		case http.MethodDelete:
			handlers.HandleResetSelfSettings(s.DB)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.HandleFunc("/api/free-key/settings", handlers.HandleFreeKeySettings(s.DB, s.isFreeAPIKeyEnabled))
}

func (r *Router) registerAdminRoutes() {
	s := r.state

	r.mux.Handle("/api/admin/stats", handlers.RequireAuth(r.jwtSecret())(handlers.HandleStats(s.DB, s.Config.CacheDir, s.Config.ExternalCacheOnly, s.Caches)))

	// Per-admin-user UI preferences (sidebar disclaimer state etc.).
	r.mux.Handle("/api/admin/prefs", handlers.RequireAuth(r.jwtSecret())(handlers.HandleUserPrefs(s.DB)))
	r.mux.Handle("/api/admin/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetSettings(s.DB, s.isFreeAPIKeyEnabled(), s.Config.FreeKeyEnabled != nil, s.fanartAvailable())(w, req)
		case http.MethodPut:
			handlers.HandleUpdateSettings(s.DB, s.Config.FreeKeyEnabled != nil)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/admin/cache/purge", handlers.RequireAuth(r.jwtSecret())(
		handlers.HandlePurgeAll(s.DB, s.Config.CacheDir, s.Config.ExternalCacheOnly, s.Caches)))

	r.mux.HandleFunc("/api/admin/settings/services", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			httpx.WriteJSON(w, 200, s.ServiceKeys.GetStatus())
		case http.MethodPut:
			var update services.ServiceKeysUpdate
			_ = httpx.DecodeJSON(req, &update)
			// The UI keeps existing keys as "ref:<fingerprint>" (it never
			// receives their plaintext); expand them before validating.
			for _, f := range []struct {
				name  string
				value **string
			}{{"tmdb", &update.TMDB}, {"mdblist", &update.MDBList}, {"omdb", &update.OMDB}, {"fanart", &update.Fanart}, {"trakt", &update.Trakt}} {
				if *f.value == nil {
					continue
				}
				expanded, err := s.ServiceKeys.ExpandKeyRefs(f.name, **f.value)
				if err != nil {
					httpx.WriteError(w, 400, f.name+": "+err.Error())
					return
				}
				*f.value = &expanded
			}

			validators := []struct {
				name  string
				value *string
			}{
				{"tmdb", update.TMDB},
				{"mdblist", update.MDBList},
				{"omdb", update.OMDB},
				{"fanart", update.Fanart},
				{"trakt", update.Trakt},
			}
			for _, v := range validators {
				if v.value == nil || strings.TrimSpace(*v.value) == "" {
					continue
				}
				if err := services.ValidateServiceKeyStringCtx(req.Context(), v.name, *v.value, s.HTTPClient); err != nil {
					httpx.WriteError(w, 400, v.name+": "+err.Error())
					return
				}
			}

			if err := s.ServiceKeys.UpdateKeys(&update); err != nil {
				httpx.WriteError(w, 500, "failed to update keys: "+err.Error())
				return
			}
			s.RefreshClientsFromKeys()
			httpx.WriteJSON(w, 200, s.ServiceKeys.GetStatus())
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	}))

	// Client IP / reverse proxy settings (Settings → General → Config):
	// which peers may set X-Forwarded-For. TRUSTED_PROXIES in the
	// environment locks it. The response includes how this very request's
	// address was resolved, so the admin can check the setting.
	r.mux.HandleFunc("/api/admin/settings/network", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
		case http.MethodPut:
			if r.trust.locked() {
				httpx.WriteError(w, 409, "set by the TRUSTED_PROXIES environment variable")
				return
			}
			var body proxySettings
			if err := httpx.DecodeJSON(req, &body); err != nil {
				httpx.WriteError(w, 400, "invalid request body")
				return
			}
			raw := body.raw()
			if _, err := config.ParseTrustedProxies(raw); err != nil {
				httpx.WriteError(w, 400, strings.TrimPrefix(err.Error(), "TRUSTED_PROXIES: "))
				return
			}
			if err := services.SetGlobalSettingCtx(req.Context(), s.DB, trustedProxiesSetting, raw); err != nil {
				httpx.WriteError(w, 500, "Failed to save")
				return
			}
		default:
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{
			"locked":     r.trust.locked(),
			"settings":   parseProxySettings(r.trust.effectiveRaw(req.Context())),
			"cloudflare": r.cloudflare.status(),
			"detected": map[string]string{
				"client_ip":     r.trust.clientIP(req),
				"remote_addr":   req.RemoteAddr,
				"forwarded_for": strings.Join(req.Header.Values("X-Forwarded-For"), ", "),
			},
		})
	}))

	// Check cloudflare.com for newer edge ranges (the UI calls this when the
	// Cloudflare box is ticked). Rate-limited server-side: see cfCheckCooldown.
	r.mux.HandleFunc("/api/admin/settings/network/cloudflare-check", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		httpx.WriteJSON(w, 200, r.cloudflare.check(req.Context()))
	}))

	// Reveal one stored provider key (the list only carries masked forms).
	r.mux.HandleFunc("/api/admin/settings/services/reveal", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Service string `json:"service"`
			ID      string `json:"id"`
		}
		if err := httpx.DecodeJSON(req, &body); err != nil {
			httpx.WriteError(w, 400, "invalid request body")
			return
		}
		key, ok := s.ServiceKeys.RevealKey(body.Service, body.ID)
		if !ok {
			httpx.WriteError(w, 404, "key not found")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, 200, map[string]string{"key": key})
	}))

	r.mux.HandleFunc("/api/admin/settings/export", r.requireAuth(handlers.HandleExportSettings(s.DB, s.ServiceKeys)))
	r.mux.HandleFunc("/api/admin/settings/import", r.requireAuth(handlers.HandleImportSettings(s.DB, s.ServiceKeys, s.RefreshClientsFromKeys)))

	// Public rating-source logos used by the admin UI (plain brand icons, not
	// sensitive). Served without auth so <img> tags can load them.
	r.mux.Handle("/api/icons/{kind}/{key}", http.HandlerFunc(handlers.HandleIcon))
	// Unauthenticated and renders on every request: per-IP limit. Generous,
	// since the settings page loads a row of badges per colour change.
	badgeLimiter := newRateLimiter(600)
	badgeLimiter.clientIP = r.trust.clientIP
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			badgeLimiter.SweepIdle()
		}
	}()
	r.mux.Handle("/api/icons/badge", RateLimit(badgeLimiter)(handlers.HandleBadgePreview(s.DB)))
}

// registerKindRoutes wires the list/clear, per-title purge/image/fetch routes
// for all four image kinds from the adminImageKinds table.
func (r *Router) registerKindRoutes() {
	s := r.state

	for _, k := range adminImageKinds {
		base := "/api/admin/" + k.plural
		r.mux.Handle(base, r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodDelete {
				handlers.HandleClearKind(s.DB, s.Config.CacheDir, k.kind, s.Config.ExternalCacheOnly, s.Caches)(w, req)
			} else {
				handlers.HandleListImages(s.DB, k.kind)(w, req)
			}
		}))

		item := base + "/{id_type}/{id_value}"
		if k.imageOnBase {
			r.mux.Handle(item, r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
				idT, idV := req.PathValue("id_type"), req.PathValue("id_value")
				switch req.Method {
				case http.MethodGet:
					handlers.HandleImageFile(s.DB, s.Config.CacheDir, k.kind, idT, idV)(w, req)
				case http.MethodDelete:
					handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, k.kind, idT, idV, s.Config.ExternalCacheOnly, s.Caches)(w, req)
				default:
					httpx.WriteError(w, 405, "Method not allowed")
				}
			}))
		} else {
			r.mux.Handle(item, r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodDelete {
					handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, k.kind, req.PathValue("id_type"), req.PathValue("id_value"), s.Config.ExternalCacheOnly, s.Caches)(w, req)
				} else {
					httpx.WriteError(w, 405, "Method not allowed")
				}
			}))
			r.mux.Handle(item+"/image", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
				handlers.HandleImageFile(s.DB, s.Config.CacheDir, k.kind, req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
			}))
		}

		r.mux.Handle(item+"/fetch", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
			handlers.HandleFetchImage(s.imageDeps(), k.kind, req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
		}))
	}
}

func (r *Router) registerPreviewRoutes() {
	s := r.state

	for _, kind := range previewKinds {
		serve := func(w http.ResponseWriter, req *http.Request) {
			preview := handlers.NewPreviewHandler(req.Context(), s.DB, s.previewConfig())
			switch kind {
			case "poster":
				preview.HandlePoster(w, req)
			case "logo":
				preview.HandleLogo(w, req)
			case "backdrop":
				preview.HandleBackdrop(w, req)
			case "episode":
				preview.HandleEpisode(w, req)
			}
		}
		r.mux.Handle("/api/admin/preview/"+kind, r.requireAuth(serve))
		r.mux.Handle("/api/key/me/preview/"+kind, handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed(), r.state.DB)(http.HandlerFunc(serve)))
	}
}

func (r *Router) registerImageRoutes() {
	s := r.state

	// Per-IP rate limiters (0 disables). Constructed once per Router; janitor
	// sweeps idle IPs every minute so long-running deployments don't grow
	// the limiters map unboundedly.
	imageLimiter := newRateLimiter(s.Config.RateLimitRPM)
	imageLimiter.clientIP = r.trust.clientIP
	cdnLimiter := newRateLimiter(s.Config.RateLimitCDNRPM)
	cdnLimiter.clientIP = r.trust.clientIP
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			imageLimiter.SweepIdle()
			cdnLimiter.SweepIdle()
		}
	}()
	imageRateLimit := RateLimit(imageLimiter)
	cdnRateLimit := RateLimit(cdnLimiter)

	// Content-addressed CDN route: /c/{hash}/{rest...}. Resolves the hash to
	// settings via the registry populated by HandleImage's redirect.
	cdnHandler := handlers.HandleCDNImage(s.imageDeps, s.hashes())
	r.mux.Handle("/c/{hash}/{rest...}", cdnRateLimit(http.HandlerFunc(cdnHandler)))

	// Image/isValid routes via catch-all
	imageHandler := handlers.HandleImage(s.imageDeps, s.isFreeAPIKeyEnabled)
	isValidHandler := handlers.HandleIsValid(s.DB, s.isFreeAPIKeyEnabled)
	r.mux.Handle("/{apiKey}/{rest...}", imageRateLimit(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rest := req.PathValue("rest")
		if rest == "isValid" {
			isValidHandler(w, req)
			return
		}
		imageHandler(w, req)
	})))
}

// setupStatic configures SPA serving from Config.StaticDir (if set).
func (r *Router) setupStatic() {
	if r.state.Config.StaticDir != "" {
		abs, _ := filepath.Abs(r.state.Config.StaticDir)
		r.staticDir = abs
		r.fs = http.FileServer(http.Dir(abs))
	}
}
