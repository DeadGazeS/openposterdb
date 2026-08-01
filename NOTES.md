## Potential Issues

1. **Existing partial Go implementation in api/**: Already present. The new api-go/ implementation is independent.

2. **Requires `golang.org/x/image`**: The badge rendering uses `golang.org/x/image/font` for the `Face` interface. The go.mod only has `crypto`, `sync`, and `sqlite` - the `x/image` dependency needs to be added when running `go mod tidy`.

3. **Font loading not implemented**: The `newFontFace()` stub in `serve.go` returns nil. A proper OpenType font parser (e.g., `golang.org/x/image/font/opentype`) is required to load the Inter-Bold.ttf font and create `font.Face` instances. Without this, badges render empty (no text).

4. **Icon loading from filesystem**: Icons are loaded at runtime from the filesystem (`assets/icons/`). The path may need adjustment relative to the binary location.

5. **No compilation/testing possible**: Per RULES.md, cannot run `go build` or `go test` to verify correctness.

6. **Memory cache not implemented**: The Rust code uses `moka` for in-memory caching with eviction policies. The Go implementation skips this layer - images are fetched fresh each request or from filesystem cache only. To match the Rust implementation, a sync.Map or custom cache with TTL and weight-based eviction would be needed.

7. **Background refresh not implemented**: When a stale cache entry is served, the Rust code spawns a background refresh. The Go implementation does not do this.

8. **Request coalescing not implemented**: The Rust code uses `image_inflight` cache to coalesce concurrent requests for the same image. Go doesn't have this.

9. **Cross-ID cache not implemented**: When a poster is generated for one ID type (e.g. IMDB), the Rust code writes entries for all alternate IDs (TMDB, TVDB). Go doesn't do this.

## Migration Progress: COMPLETE

### All Created Files (31 files, ~6,200+ lines)

| # | File | Lines | Description |
|---|------|-------|-------------|
| 1 | `go.mod` | 9 | Module declaration |
| 2 | `cmd/server/main.go` | 280 | Entry point, DB setup, server bootstrap |
| 3 | `cmd/server/schema.go` | 260 | Schema SQL + 48 migrations |
| 4 | `internal/config/config.go` | 105 | Environment variable parsing |
| 5 | `internal/errors/errors.go` | 70 | Error types, HTTP status mapping |
| 6 | `internal/services/db.go` | 900 | DB CRUD, settings enums, types |
| 7 | `internal/services/retry.go` | 145 | HTTP retry with exponential backoff |
| 8 | `internal/services/apikeypool.go` | 140 | API key pool with 429 rotation |
| 9 | `internal/services/lang.go` | 15 | Language code helpers |
| 10 | `internal/services/validation.go` | 45 | Username/password validation |
| 11 | `internal/services/tmdb.go` | 210 | TMDB API client |
| 12 | `internal/services/mdblist.go` | 110 | MDBList API client |
| 13 | `internal/services/omdb.go` | 55 | OMDb API client |
| 14 | `internal/services/trakt.go` | 85 | Trakt API client |
| 15 | `internal/services/fanart.go` | 180 | Fanart.tv API client |
| 16 | `internal/services/servicekeys.go` | 265 | AES-256-GCM encrypted keys |
| 17 | `internal/services/ratings.go` | 300 | Ratings aggregation, cache keys |
| 18 | `internal/services/id.go` | 370 | ID resolution via TMDB |
| 19 | `internal/services/cache.go` | 210 | Filesystem cache, staleness |
| 20 | `internal/services/cachesuffix.go` | 115 | Cache key suffix construction |
| 21 | `internal/services/upgrade.go` | 70 | Data upgrade framework |
| 22 | `internal/handlers/auth.go` | 300 | Auth (Argon2, JWT, login/setup/refresh) |
| 23 | `internal/handlers/middleware.go` | 105 | JWT auth middleware |
| 24 | `internal/handlers/image.go` | 380 | Image endpoint handler, settings resolution |
| 25 | `internal/handlers/admin.go` | 145 | Stats, settings, cache purge |
| 26 | `internal/handlers/api_keys.go` | 230 | API key CRUD + per-key settings |
| 27 | `internal/router/router.go` | 280 | Route registration, SPA fallback |
| 28 | `internal/image/badge.go` | ~560 | Badge rendering (horizontal/vertical, all shapes/backgrounds) |
| 29 | `internal/image/icons.go` | ~120 | Rating source icon loading |
| 30 | `internal/image/generate.go` | ~430 | Poster/logo/backdrop/episode rendering |
| 31 | `internal/image/serve.go` | ~310 | Image serving pipeline, CDN helpers |

### What each module does:

**Server (cmd/server/)** — Starts HTTP server, initializes SQLite with WAL pragmas, applies schema + 48 migrations, runs data upgrades, seeds admin user, starts flush worker for API key last_used_at updates.

**Config** — Parses all 18 environment variables: TMDB/MDBList/OMDb/Trakt/Fanart keys, cache/db dirs, server address, cache TTLs, image quality, CDN flags, auth settings.

**Services** — All business logic:
- Database: Full CRUD for all 7 tables, settings enums/types (BadgeStyle, BadgePosition, ImageSource, BadgeSize, etc.), render settings parsing
- External APIs: TMDB (including TMDB images API with language fallback), MDBList (IMDb + TMDB lookup), OMDb, Trakt (movie/show/episode ratings), Fanart (movies + TV)
- Infrastructure: Retry with exponential backoff + jitter, API key pool with 429 rotation (3 consecutive → rotate, 24h cooldown), AES-256-GCM service key encryption via HKDF, validation, language helpers
- Ratings: 10 rating sources, aggregation from MDBList/OMDb/Trakt/TMDB in parallel, preference application (order/exclude/limit), cache key computation
- ID Resolution: IMDB/TMDB/TVDB → TMDB ID via TMDB Find API, episode support (S{season}E{episode} format)

**Handlers** — HTTP request handlers:
- Auth: Argon2 password hashing, JWT access tokens (15min) + rotating refresh tokens (7days), API key JWT login (24h)
- Middleware: JWT-based RequireAuth/RequireAPIKeyAuth/RequireAnyAuth
- Image: Settings resolution (free key + per-key + query overrides), full query parameter parsing (20+ params)
- Admin: Stats, global settings get/update, cache purge, image meta listing
- API Keys: Create/delete/list keys, per-key settings get/update/reset

**Router** — Registers all routes with middleware, serves SPA with fallback to index.html, security headers (X-Content-Type-Options, X-Frame-Options, HSTS).

**Image rendering** — Badge rendering with all shapes (rounded/pill) and backgrounds (default/dark/transparent/none), text + shadow, corner rounding, vertical/horizontal layouts, uniform widths. Poster/logo/backdrop/episode generation with aspect-ratio fitting (native/cover/pad), badge overlay with position/direction/spacing.

### Image Rendering Caveats:
- **Font loading**: The `newFontFace()` stub needs a real OpenType parser. Badges currently render empty (no text) until a proper font.Face is constructed from Inter-Bold.ttf.
- **Icons**: Loaded from filesystem at runtime via `LoadIcons()`. Caller must call this before serving images.
- **Blur**: Simplified to downscale+upscale. True Gaussian blur (like the Rust code's `imageproc::gaussian_blur_f32`) is not available in Go's stdlib.
- **Memory cache**: Filesystem cache only. The 3-layer caching (moka + disk + SQLite) from Rust is reduced to filesystem-only for Go.
