package router

import (
	"database/sql"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"

	"openposterdb/internal/config"
	"openposterdb/internal/handlers"
	"openposterdb/internal/image"
	"openposterdb/internal/services"
)

type AppState struct {
	Config          *config.Config
	DB              *sql.DB
	HTTPClient      *http.Client
	TMDB            *services.TmdbClient
	OMDB            *services.OmdbClient
	MDBList         *services.MdblistClient
	Fanart          *services.FanartClient
	Trakt           *services.TraktClient
	Kitsu           *services.KitsuClient
	AniList         *services.AniListClient
	KitsuIMDbMapper *services.KitsuIMDbMapper
	ServiceKeys     *services.ServiceKeyManager
	SecureCookies   bool
	Caches          *services.MemCacheSet
	CDNHashes       *services.HashRegistry
	Inflight        *image.InflightSet
	JWTSecret       []byte
	SecretsKey      []byte
	LastUsedFlusher *services.LastUsedFlusher

	// clientsMu protects the rating-provider client fields (TMDB/OMDB/MDBList/
	// Fanart/Trakt). Setup* and RefreshClientsFromKeys write; imageDeps() and
	// previewConfig() read. Read+write never block each other across readers.
	clientsMu sync.RWMutex
}

func New(state *AppState) *Router {
	if state.Config != nil && state.Config.TrustedProxies != nil {
		trustedProxies = *state.Config.TrustedProxies
	}
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
		Gzip(http.HandlerFunc(r.serveStatic)).ServeHTTP(w, req)
		return
	}

	CORS(r.state.Config.CORSOrigin)(Gzip(r.mux)).ServeHTTP(w, req)
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

func (r *Router) lastUsed() handlers.KeyRecorder {
	return r.state.LastUsedFlusher
}

// serveStatic serves the built SPA. Vite's /assets/* files carry a content
// hash in their name, so they are immutable; index.html (and the SPA
// fallback) must be revalidated so a deploy is picked up immediately instead
// of a heuristically-cached page pointing at chunks that no longer exist.
// Directories never produce an http.FileServer listing.
func (r *Router) serveStatic(w http.ResponseWriter, req *http.Request) {
	// Clean before touching the filesystem: a raw "/../../etc/passwd"
	// (curl --path-as-is) otherwise reached os.Stat outside staticDir, and
	// the 404-vs-index.html difference revealed which host files exist.
	path := pathpkg.Clean("/" + req.URL.Path)
	w.Header().Set("Content-Security-Policy", spaCSP)
	if path != "/" {
		if fi, err := os.Stat(filepath.Join(r.staticDir, filepath.FromSlash(path))); err == nil && !fi.IsDir() {
			switch {
			case strings.HasPrefix(path, "/assets/"):
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			default:
				w.Header().Set("Cache-Control", "public, max-age=86400")
			}
			r.fs.ServeHTTP(w, req)
			return
		}
	}
	// "/" and the SPA fallback for unknown non-API routes.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, req, filepath.Join(r.staticDir, "index.html"))
}

// spaCSP is the Content-Security-Policy for the web UI: scripts only from
// this origin (no inline/eval), so an injected script can't run and read the
// admin session. Inline styles are allowed (Vue/the UI kit set style
// attributes); images may be blob:/data: (rendered previews); the only
// third-party request is the GitHub release check in the sidebar.
const spaCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' https://api.github.com; " +
	"worker-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
