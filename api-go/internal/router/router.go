package router

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"openposterdb/internal/config"
	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

const freeAPIKey = "t0-free-rpdb"

type AppState struct {
	Config        *config.Config
	DB            *sql.DB
	HTTPClient    *http.Client
	TMDB          *services.TmdbClient
	OMDB          *services.OmdbClient
	MDBList       *services.MdblistClient
	Fanart        *services.FanartClient
	Trakt         *services.TraktClient
	ServiceKeys   *services.ServiceKeyManager
	SecureCookies bool
	jwtSecret     []byte
}

func New(state *AppState) *Router {
	r := &Router{
		state: state,
		mux:   http.NewServeMux(),
	}
	r.registerRoutes()
	return r
}

type Router struct {
	state *AppState
	mux   *http.ServeMux
	staticDir string
	fs        http.Handler
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if r.state.SecureCookies {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	}

	// Check if this is an API or image route before going to mux
	path := req.URL.Path
	if !isAPIPath(path) && r.fs != nil {
		fullPath := filepath.Join(r.staticDir, path)
		if _, err := os.Stat(fullPath); err == nil {
			r.fs.ServeHTTP(w, req)
			return
		}
		// SPA fallback for non-API routes
		http.ServeFile(w, req, filepath.Join(r.staticDir, "index.html"))
		return
	}

	r.mux.ServeHTTP(w, req)
}

func isAPIPath(path string) bool {
	if strings.HasPrefix(path, "/api/") || path == "/api" {
		return true
	}
	if strings.HasPrefix(path, "/c/") {
		return true
	}
	// Image route: /{64-char-hex-key}/...
	rest := path
	if strings.HasPrefix(rest, "/") {
		rest = rest[1:]
	}
	parts := strings.SplitN(rest, "/", 2)
	first := parts[0]
	if len(first) == 64 && isHex(first) {
		return true
	}
	return false
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func (r *Router) registerRoutes() {
	s := r.state

	r.mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, req *http.Request) {
		status, resp := handlers.AuthStatus(s.DB, s.isFreeAPIKeyEnabled, s.Config.DisablePublicPages)
		writeJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/setup", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		decodeJSON(req, &body)

		status, resp, cookies := handlers.SetupHandler(s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		writeJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		decodeJSON(req, &body)

		status, resp, cookies := handlers.LoginHandler(s.DB, r.jwtSecret(), s.SecureCookies, body.Username, body.Password)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		writeJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
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
		writeJSON(w, status, resp)
	})

	r.mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
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
		writeJSON(w, 200, map[string]bool{"success": true})
	})

	r.mux.HandleFunc("/api/auth/key-login", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			writeError(w, 405, "Method not allowed")
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		decodeJSON(req, &body)
		status, resp := handlers.KeyLoginHandler(s.DB, r.jwtSecret(), body.APIKey)
		writeJSON(w, status, resp)
	})

	r.mux.Handle("/api/keys", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleListKeys(s.DB)(w, req)
		case http.MethodPost:
			handlers.HandleCreateKey(s.DB)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/keys/{id}", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleDeleteKey(s.DB)(w, req)
	})))

	r.mux.Handle("/api/keys/{id}/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetKeySettings(s.DB)(w, req)
		case http.MethodPut:
			handlers.HandleUpdateKeySettings(s.DB)(w, req)
		case http.MethodDelete:
			handlers.HandleResetKeySettings(s.DB)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/key/me", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleSelfKeyInfo(s.DB)(w, req)
	})))

	r.mux.Handle("/api/key/me/settings", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleSelfSettings(s.DB)(w, req)
		case http.MethodPut:
			handlers.HandleUpdateSelfSettings(s.DB)(w, req)
		case http.MethodDelete:
			handlers.HandleResetSelfSettings(s.DB)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/admin/stats", handlers.RequireAuth(r.jwtSecret())(handlers.HandleStats(s.DB)))
	r.mux.Handle("/api/admin/settings", handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			handlers.HandleGetSettings(s.DB, s.isFreeAPIKeyEnabled(), s.Config.FreeKeyEnabled != nil, s.Fanart != nil)(w, req)
		case http.MethodPut:
			handlers.HandleUpdateSettings(s.DB, s.Config.FreeKeyEnabled != nil)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	})))

	r.mux.Handle("/api/admin/cache/purge", handlers.RequireAuth(r.jwtSecret())(
		handlers.HandlePurgeAll(s.DB, s.Config.CacheDir, s.Config.ExternalCacheOnly)))

	// Admin preview routes
	r.mux.Handle("/api/admin/preview/poster", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandlePoster(w, req)
	}))
	r.mux.Handle("/api/admin/preview/logo", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleLogo(w, req)
	}))
	r.mux.Handle("/api/admin/preview/backdrop", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleBackdrop(w, req)
	}))
	r.mux.Handle("/api/admin/preview/episode", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleEpisode(w, req)
	}))

	// Clear-by-kind routes
	r.mux.Handle("/api/admin/posters", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			handlers.HandleClearKind(s.DB, s.Config.CacheDir, "p", s.Config.ExternalCacheOnly)(w, req)
		} else {
			handlers.HandleListImages(s.DB, "p")(w, req)
		}
	}))
	r.mux.Handle("/api/admin/logos", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			handlers.HandleClearKind(s.DB, s.Config.CacheDir, "l", s.Config.ExternalCacheOnly)(w, req)
		} else {
			handlers.HandleListImages(s.DB, "l")(w, req)
		}
	}))
	r.mux.Handle("/api/admin/backdrops", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			handlers.HandleClearKind(s.DB, s.Config.CacheDir, "b", s.Config.ExternalCacheOnly)(w, req)
		} else {
			handlers.HandleListImages(s.DB, "b")(w, req)
		}
	}))
	r.mux.Handle("/api/admin/episodes", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodDelete {
			handlers.HandleClearKind(s.DB, s.Config.CacheDir, "e", s.Config.ExternalCacheOnly)(w, req)
		} else {
			handlers.HandleListImages(s.DB, "e")(w, req)
		}
	}))

	// Per-title purge + image serve + fetch
	r.mux.Handle("/api/admin/posters/{id_type}/{id_value}", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		idT, idV := req.PathValue("id_type"), req.PathValue("id_value")
		if req.Method == http.MethodDelete {
			handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, "p", idT, idV, s.Config.ExternalCacheOnly)(w, req)
		} else {
			writeError(w, 405, "Method not allowed")
		}
	}))
	r.mux.Handle("/api/admin/posters/{id_type}/{id_value}/image", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleImageFile(s.DB, s.Config.CacheDir, "p", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))
	r.mux.Handle("/api/admin/posters/{id_type}/{id_value}/fetch", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleFetchImage(s.DB, s.imageServeConfig(), s.TMDB, s.OMDB, s.MDBList, s.Trakt, s.Fanart, "p", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))
	r.mux.Handle("/api/admin/logos/{id_type}/{id_value}", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		idT, idV := req.PathValue("id_type"), req.PathValue("id_value")
		switch req.Method {
		case http.MethodGet:
			handlers.HandleImageFile(s.DB, s.Config.CacheDir, "l", idT, idV)(w, req)
		case http.MethodDelete:
			handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, "l", idT, idV, s.Config.ExternalCacheOnly)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	}))
	r.mux.Handle("/api/admin/logos/{id_type}/{id_value}/fetch", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleFetchImage(s.DB, s.imageServeConfig(), s.TMDB, s.OMDB, s.MDBList, s.Trakt, s.Fanart, "l", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))
	r.mux.Handle("/api/admin/backdrops/{id_type}/{id_value}", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		idT, idV := req.PathValue("id_type"), req.PathValue("id_value")
		switch req.Method {
		case http.MethodGet:
			handlers.HandleImageFile(s.DB, s.Config.CacheDir, "b", idT, idV)(w, req)
		case http.MethodDelete:
			handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, "b", idT, idV, s.Config.ExternalCacheOnly)(w, req)
		default:
			writeError(w, 405, "Method not allowed")
		}
	}))
	r.mux.Handle("/api/admin/backdrops/{id_type}/{id_value}/fetch", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleFetchImage(s.DB, s.imageServeConfig(), s.TMDB, s.OMDB, s.MDBList, s.Trakt, s.Fanart, "b", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))
	r.mux.Handle("/api/admin/episodes/{id_type}/{id_value}", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		idT, idV := req.PathValue("id_type"), req.PathValue("id_value")
		if req.Method == http.MethodDelete {
			handlers.HandlePurgeTitle(s.DB, s.Config.CacheDir, "e", idT, idV, s.Config.ExternalCacheOnly)(w, req)
		} else {
			writeError(w, 405, "Method not allowed")
		}
	}))
	r.mux.Handle("/api/admin/episodes/{id_type}/{id_value}/image", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleImageFile(s.DB, s.Config.CacheDir, "e", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))
	r.mux.Handle("/api/admin/episodes/{id_type}/{id_value}/fetch", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		handlers.HandleFetchImage(s.DB, s.imageServeConfig(), s.TMDB, s.OMDB, s.MDBList, s.Trakt, s.Fanart, "e", req.PathValue("id_type"), req.PathValue("id_value"))(w, req)
	}))

	// Key self-service preview routes
	r.mux.Handle("/api/key/me/preview/poster", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandlePoster(w, req)
	})))
	r.mux.Handle("/api/key/me/preview/logo", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleLogo(w, req)
	})))
	r.mux.Handle("/api/key/me/preview/backdrop", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleBackdrop(w, req)
	})))
	r.mux.Handle("/api/key/me/preview/episode", handlers.RequireAPIKeyAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		preview := handlers.NewPreviewHandler(s.DB, &handlers.PreviewConfig{CacheDir: s.Config.CacheDir, ExternalCacheOnly: s.Config.ExternalCacheOnly, ImageQuality: s.Config.ImageQuality})
		preview.HandleEpisode(w, req)
	})))

	r.mux.HandleFunc("/api/admin/settings/services", r.requireAuth(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			writeJSON(w, 200, s.ServiceKeys.GetStatus())
		case http.MethodPut:
			var update services.ServiceKeysUpdate
			decodeJSON(req, &update)

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
					writeError(w, 400, v.name+": "+err.Error())
					return
				}
			}

			if err := s.ServiceKeys.UpdateKeys(&update); err != nil {
				writeError(w, 500, "failed to update keys: "+err.Error())
				return
			}
			s.SetupTMDB(s.ServiceKeys.TMDBKey())
			s.SetupOMDB(s.ServiceKeys.OMDBKeys())
			s.SetupMDBList(s.ServiceKeys.MDBListKeys())
			s.SetupFanart(s.ServiceKeys.FanartKeys())
			s.SetupTrakt(s.ServiceKeys.TraktClientIDs())
			writeJSON(w, 200, s.ServiceKeys.GetStatus())
		default:
			writeError(w, 405, "Method not allowed")
		}
	}))

	r.mux.HandleFunc("/api/free-key/settings", handlers.HandleFreeKeySettings(s.DB, s.isFreeAPIKeyEnabled))

	// Image/isValid routes via catch-all
	imageHandler := handlers.HandleImage(s.DB, s.imageServeConfig(), s.TMDB, s.OMDB, s.MDBList, s.Trakt, s.Fanart, s.isFreeAPIKeyEnabled)
	isValidHandler := handlers.HandleIsValid(s.DB, s.isFreeAPIKeyEnabled)
	r.mux.HandleFunc("/{apiKey}/{rest...}", func(w http.ResponseWriter, req *http.Request) {
		rest := req.PathValue("rest")
		if rest == "isValid" {
			isValidHandler(w, req)
			return
		}
		imageHandler(w, req)
	})

	// Setup SPA serving
	if s.Config.StaticDir != "" {
		abs, _ := filepath.Abs(s.Config.StaticDir)
		r.staticDir = abs
		r.fs = http.FileServer(http.Dir(abs))
	}
}

func (s *AppState) SetupTMDB(key string) {
	if key != "" {
		s.TMDB = services.NewTmdbClient(key, s.HTTPClient)
	} else {
		s.TMDB = nil
	}
}

func (s *AppState) SetupOMDB(keys []string) {
	if len(keys) > 0 {
		s.OMDB = services.NewOmdbClient(keys, s.HTTPClient)
	} else {
		s.OMDB = nil
	}
}

func (s *AppState) SetupMDBList(keys []string) {
	if len(keys) > 0 {
		s.MDBList = services.NewMdblistClient(keys, s.HTTPClient)
	} else {
		s.MDBList = nil
	}
}

func (s *AppState) SetupFanart(keys []string) {
	if len(keys) > 0 {
		s.Fanart = services.NewFanartClient(keys, s.HTTPClient)
	} else {
		s.Fanart = nil
	}
}

func (s *AppState) SetupTrakt(keys []string) {
	if len(keys) > 0 {
		s.Trakt = services.NewTraktClient(keys, s.HTTPClient)
	} else {
		s.Trakt = nil
	}
}

func (s *AppState) isFreeAPIKeyEnabled() bool {
	if s.Config.FreeKeyEnabled != nil {
		return *s.Config.FreeKeyEnabled
	}
	val, _ := services.GetGlobalSetting(s.DB, "free_api_key_enabled")
	return val == "true"
}

func (s *AppState) imageServeConfig() *handlers.ImageServeConfig {
	return &handlers.ImageServeConfig{
		CacheDir:            s.Config.CacheDir,
		ExternalCacheOnly:   s.Config.ExternalCacheOnly,
		RatingsMinStaleSecs: s.Config.RatingsMinStaleSecs,
		RatingsMaxAgeSecs:   s.Config.RatingsMaxAgeSecs,
		ImageStaleSecs:      s.Config.ImageStaleSecs,
		ImageQuality:        s.Config.ImageQuality,
	}
}

func (r *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		handler := handlers.RequireAuth(r.jwtSecret())(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next(w, req)
		}))
		handler.ServeHTTP(w, req)
	}
}

func (r *Router) jwtSecret() []byte {
	return r.state.jwtSecret
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(req *http.Request, v interface{}) {
	defer req.Body.Close()
	json.NewDecoder(req.Body).Decode(v)
}
