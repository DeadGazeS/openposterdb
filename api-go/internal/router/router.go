package router

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"openposterdb/internal/config"
	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

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
	Caches        *services.MemCacheSet
	JWTSecret     []byte
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
	state     *AppState
	mux       *http.ServeMux
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

func (r *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return handlers.RequireAuth(r.jwtSecret())(next).ServeHTTP
}

func (r *Router) jwtSecret() []byte {
	return r.state.JWTSecret
}
