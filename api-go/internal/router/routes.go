package router

import (
	"net/http"
	"path/filepath"
	"strings"

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
	s := r.state

	r.mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, req *http.Request) {
		status, resp := handlers.AuthStatus(s.DB, s.isFreeAPIKeyEnabled, s.Config.DisablePublicPages)
		httpx.WriteJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/setup", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = httpx.DecodeJSON(req, &body)

		status, resp, cookies := handlers.SetupHandler(s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		httpx.WriteJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = httpx.DecodeJSON(req, &body)

		status, resp, cookies := handlers.LoginHandler(s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
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
		status, resp, cookies := handlers.RefreshHandler(s.DB, r.jwtSecret(), s.SecureCookies, refreshToken)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		httpx.WriteJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		cookie, _ := req.Cookie("token")
		if cookie != nil {
			claims, err := handlers.ParseJWT(cookie.Value, r.jwtSecret())
			if err == nil && claims.Username != "" {
				handlers.LogoutHandler(s.DB, claims.Username)
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
		http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/", MaxAge: -1})
		httpx.WriteJSON(w, 200, map[string]bool{"success": true})
	})

	r.mux.HandleFunc("/api/auth/key-login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		_ = httpx.DecodeJSON(req, &body)
		status, resp := handlers.KeyLoginHandler(s.DB, r.jwtSecret(), body.APIKey)
		httpx.WriteJSON(w, status, resp)
	})
}

func (r *Router) registerKeyRoutes() {
	s := r.state

	r.mux.Handle("/api/keys", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleListKeys(s.DB)(w, req)
		case http.MethodPost:
			handlers.HandleCreateKey(s.DB)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/keys/{id}", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleDeleteKey(s.DB)(w, req)
	})))

	r.mux.Handle("/api/keys/{id}/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetKeySettings(s.DB, s.Fanart != nil)(w, req)
		case http.MethodPut:
			handlers.HandleUpdateKeySettings(s.DB)(w, req)
		case http.MethodDelete:
			handlers.HandleResetKeySettings(s.DB)(w, req)
		default:
			httpx.WriteError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/key/me", handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleSelfKeyInfo(s.DB)(w, req)
	})))

	r.mux.Handle("/api/key/me/settings", handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleSelfSettings(s.DB, s.Fanart != nil)(w, req)
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

	r.mux.Handle("/api/admin/stats", handlers.RequireAuth(r.jwtSecret())(handlers.HandleStats(s.DB, s.Config.CacheDir, s.Caches)))

	// Per-admin-user UI preferences (sidebar disclaimer state etc.).
	r.mux.Handle("/api/admin/prefs", handlers.RequireAuth(r.jwtSecret())(handlers.HandleUserPrefs(s.DB)))
	r.mux.Handle("/api/admin/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetSettings(s.DB, s.isFreeAPIKeyEnabled(), s.Config.FreeKeyEnabled != nil, s.Fanart != nil)(w, req)
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
				if err := services.ValidateServiceKeyString(v.name, *v.value, s.HTTPClient); err != nil {
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

	r.mux.HandleFunc("/api/admin/settings/export", r.requireAuth(handlers.HandleExportSettings(s.DB, s.ServiceKeys)))
	r.mux.HandleFunc("/api/admin/settings/import", r.requireAuth(handlers.HandleImportSettings(s.DB, s.ServiceKeys)))

	// Public rating-source logos used by the admin UI (plain brand icons, not
	// sensitive). Served without auth so <img> tags can load them.
	r.mux.Handle("/api/icons/{kind}/{key}", http.HandlerFunc(handlers.HandleIcon))
	r.mux.HandleFunc("/api/icons/badge", handlers.HandleBadgePreview(s.DB))
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
			preview := handlers.NewPreviewHandler(s.DB, s.previewConfig())
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
		r.mux.Handle("/api/key/me/preview/"+kind, handlers.RequireAPIKeyAuth(r.jwtSecret(), r.lastUsed())(http.HandlerFunc(serve)))
	}
}

func (r *Router) registerImageRoutes() {
	s := r.state

	// Per-IP rate limiters (0 disables). Constructed once per Router.
	imageRateLimit := RateLimit(newRateLimiter(s.Config.RateLimitRPM))
	cdnRateLimit := RateLimit(newRateLimiter(s.Config.RateLimitCDNRPM))

	// Content-addressed CDN route: /c/{hash}/{rest...}. Resolves the hash to
	// settings via the registry populated by HandleImage's redirect.
	cdnHandler := handlers.HandleCDNImage(s.imageDeps(), s.hashes())
	r.mux.Handle("/c/{hash}/{rest...}", cdnRateLimit(http.HandlerFunc(cdnHandler)))

	// Image/isValid routes via catch-all
	imageHandler := handlers.HandleImage(s.imageDeps(), s.isFreeAPIKeyEnabled)
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
