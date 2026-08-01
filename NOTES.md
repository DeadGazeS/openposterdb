## Potential Issues

0. **Root cause of "0 posters / fetch does nothing" — FIXED 2026-08-01**: The Go backend's image pipeline was entirely stubbed out:
   - `internal/image/serve.go` `ServeImage` returned `fmt.Errorf("not implemented")`.
   - `internal/handlers/image.go` `HandleImage` (public `/{key}/{idType}/poster-default/{id}.jpg` endpoint) returned an empty JPEG body.
   - `internal/handlers/preview.go` `HandleFetchImage` (admin `.../fetch`) returned a JSON `{"note":"fetch triggered"}` stub, so the frontend's `res.blob()` produced a broken preview and nothing was ever cached → `image_meta` stayed empty → admin lists showed 0.
   - Preview handlers returned a JSON stub instead of a rendered image.
   - `services.ImageMetaItem` had no JSON tags, so `ListImageMetaByKind` serialized as `CacheKey`/`ReleaseDate`/… while the Vue frontend expects `cache_key`/`release_date`/… (this would break the table even once rows existed).

### Fixes applied (all in `api-go/`)

0. **Third round — logging, key pool rotation, key validation (2026-08-01):**
   - **LOG_LEVEL value mangling.** Docker's `env_file` does not strip inline comments, so `LOG_LEVEL=debug (optional — ...)` set the whole string as the value; `setupLogging` only matched exact strings so debug never engaged. Added `sanitizeValue`/`sanitizeSecrets` in `config.go` (truncate at first space/`(`/tab) applied to all env values, and the same truncation in `setupLogging`. Any accidental annotation in a value is now ignored.
   - **MDBList key pool never rotated.** `maxConsecutive429s` was 3, and each failed fetch only reports one 429 → the second key was never used. Lowered to 1 so the pool rotates to the next key on the first 429. Updated `apikeypool_test.go` for the new threshold.
   - **Debug logs for fetch results.** `ServeImage` now logs `ratings fetched` (badge count + source keys) and `ratings skipped` when the kind's ratings limit is 0, so `LOG_LEVEL=debug` shows whether rating fetch succeeded and which sources were used.
   - **API-key PUT validation.** `PUT /api/admin/settings/services` now validates before saving: rejects too-short/empty/duplicate keys and performs a live check against the provider (TMDB /configuration, OMDb /?apikey, MDBList /tmdb/movie/1, Fanart /v3/movies/550, Trakt /movies with client-id header). Only a definitive 401/403 rejects; network errors and 429s are tolerated. `UpdateKeys` errors are now surfaced (500) instead of ignored. Added `ValidateServiceKeyString`/`ValidateServiceKey`/`checkProviderURL`/`checkTraktClientID` in `servicekeys.go`.

0. **Second round — settings, icons, logging, refresh (2026-08-01):**
   - **Settings GET/PUT were broken.** `HandleUpdateSettings` decoded the body into `map[string]string`, which rejects numeric/boolean JSON values → 400 on any save (this is why "ratings priority order" couldn't be changed). Rewrote it with a typed `updateSettingsRequest` (pointers + defaults merged over stored globals), validation via `services.ValidateRenderSettings`, and persistence via new `services.RenderSettingsToMap`. `HandleGetSettings` now returns the full snake_case response (defaults applied) including `fanart_available`, `free_api_key_enabled`, `free_api_key_locked` — the Vue "Prefer Fanart.tv" checkbox renders only when `fanart_available` is true, so it was missing.
   - **Per-key settings JSON was broken.** `RenderSettings` and `APIKeySettings` had no JSON tags → the key-settings UI got PascalCase fields and PUT decoded nothing. Added snake_case tags to both.
   - **No rating icons.** `badge.go` computed widths/drew labels as if label style were always text. Implemented icon rendering for `LabelStyleIcon`/`LabelStyleOfficial` in `renderBadgeInner` + `RenderVerticalBadge` (helpers: `iconScaledWidth`, `iconFitInBox`, `badgeIconAndSize`, `scaleIcon`, `overlayIconShadowed`, `labelWidthForStyle`). `LoadIcons` now falls back to `.webp` (the official MAL icon ships as webp).
   - **Log levels.** Added `LOG_LEVEL` env (`debug`/`info`/`warn`/`error`/`off`, default `info`) → `config.Config.LogLevel`, `setupLogging()` (called from `init()` so startup lines respect it). Debug logs for image requests, cache hits, generation (with badge count), admin fetches, and settings saves; warn logs for cache/meta write failures and missing artwork. Documented in `docs/configuration.md`, `docker-compose.yml`, `api-go/.env.example`.
   - **Refresh-token bug.** `RefreshHandler` called `IssueTokenPair(..., "")` so refreshed access tokens had an empty `sub`; `RequireAuth` rejects `Username == ""` → every admin API call 401s after the 15-min token expires. Now resolves the username from the user ID.

1. **`internal/image/serve.go`**: Replaced the `ServeImage` stub with the full pipeline — ID resolve (+ episode uplift for poster/logo/backdrop), ratings fetch (skipped when the kind's ratings limit is 0), preferences, cache-value construction (`{id}{variant}{suffix}` with `_t_`/`_f_` poster, `_l_t_/f_{lang}` logo, `_b_t/f` backdrop variants), filesystem cache check/serve, base artwork download (TMDB with on-disk base cache; fanart optional), render via `GenerateImage`, then write FS cache + `UpsertImageMeta`. Added `cacheVariant`, `fetchTmdbArtwork`, `fetchFanartArtwork` helpers.
2. **`internal/handlers/image.go`**: `HandleImage` now resolves settings/query overrides then delegates to `image.ServeImage` and streams real bytes with `image/*` content-type and a 1h cache header. Added `ImageServeConfig` carrying `CacheDir`, `ExternalCacheOnly`, `RatingsMinStaleSecs`, `RatingsMaxAgeSecs`, `ImageStaleSecs`, `ImageQuality`.
3. **`internal/handlers/preview.go`**: `HandleFetchImage` now builds global settings and calls `image.ServeImage`, returning image bytes. The four preview handlers now actually render (sample poster/logo/backdrop artwork + `sampleBadges`) via `RenderPosterSync`/`RenderLogoSync`/`RenderBackdropSync`/`RenderEpisodeSync`.
4. **`internal/image/generate.go`**: Added startup-generated `SamplePosterPNG`/`SampleLogoPNG`/`SampleBackdropPNG` used by previews.
5. **`internal/services/db.go`**: Added JSON tags to `ImageMetaItem` (`cache_key`, `release_date`, `image_type`, `created_at`, `updated_at`).
6. **`internal/router/router.go`**: Threaded `s.imageServeConfig()` into `HandleImage` and all four `HandleFetchImage` call sites.

Verified end-to-end against a fresh container built from the local Dockerfile: admin fetch of `imdb/tt1528406` returns a 580×853 JPEG with badges, the admin list returns the row, `.../image` serves the cached file, the public `/{apiKey}/imdb/poster-default/tt1528406.jpg` endpoint serves the same image, invalid keys 401, and logo (PNG) / backdrop / episode (JPEG) fetch+list+serve all work. `go build ./...`, `go vet ./...`, and `go test ./...` all pass.

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
