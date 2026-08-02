## Potential Issues

0. **Badge layout / size / layout-grid round (2026-08-02)**:
   - **Badge box is FIXED by badge_size — oversized content clips, centred** (user requirement: "I didn't want badge size to change if text size or logo gets too big; it should just be cut off, cut evenly/centred"). Reverted the earlier "box grows to fit" behaviour in `renderBadgeInner` (badge.go) back to the original fixed `badgeH = badgeHeight + pillPadV` and `totalW` from base widths. `labelWidthForStyle` again returns the base (100%) label width independent of `logoScale`, and `RenderBadge`/`RenderBadgesUniform` use `baseTextWidth` (measured width / textScale) so the box is identical at any text/logo size. Icon is centred in the left section, value text centred in the right section, so oversized content is clipped evenly on both sides. `RenderVerticalBadge` restored to the original fixed-box version (vertBadgeW/labelH/valueH fixed by badge_size, content centred).
   - **Per-kind 4-side badge layout grid replaces position/split/badges-per-row** (user: split shared the row budget wrong for top/bottom; wanted a per-side grid where "left+right/top+bottom" badges are set independently). New `ImageLayout` per kind (`poster_layout`, `logo_layout`, `backdrop_layout`, `episode_layout`): each of the four sides holds `per_row` (badges per row), `rows` (row count) and `start` (anchor — `l`/`c`/`r` for top/bottom, `t`/`c`/`b` for left/right), plus an `order` array of side names (fill order). Badges are laid out in **horizontal rows on every side**; the total number of ratings shown is the sum of the four side capacities (`per_row × rows`). Defaults preserve old output: poster bottom 3×1 centre, logo bottom 5×1 centre, backdrop top 5×1 right, episode right 1×1 top. Stored as JSON strings in `global_settings` + `api_key_settings` TEXT columns. Plumbed through `RenderSettings`/`APIKeySettings` (struct, defaults, parse, toMap, Get/Update/Upsert, effective settings), schema CREATE TABLE + 4 migrations, cache suffix `.ly{hash}` (`LayoutCacheSuffix`, only when non-default), public `?layout={json}` query override, preview `layout` param, admin settings GET/PUT, free-key response, and frontend `LayoutEditor` grid component per kind.
   - **Renderers rewritten to layout placement** (badge.go `renderSideBlock`/`overlaySideBlock`/`overlayLayoutOnCanvas`, generate.go `composeLogoLayout` for the logo's outside-blocks): badges are distributed to sides in `order`, each side's block is a Rows×PerRow horizontal grid anchored per `start`. `BadgeDirection.Resolve(position)` removed → `ResolveDefault()` (default resolves to horizontal since every side uses horizontal rows). Old `overlayPosterGroup`/`overlayHorizontalRows`/`overlayVerticalStack`/`buildBadgeBlock`/`composeLogoBadges`/split budget logic removed.
   - **Admin GET now returns `poster_layout`/`logo_layout`/`backdrop_layout`/`episode_layout`** (was missing the per-row fields, so the frontend reset to defaults on reload — the "Badges per row resets on reload" bug). Free-key response + public query mapping updated to `?layout=`.
   - **Follow-up (2026-08-02, user feedback):**
     - **Save "invalid JSON" fixed** — the frontend sent `poster_layout` etc. as a JSON **string** in the admin PUT body, but `updateSettingsRequest.PosterLayout *services.ImageLayout` expects an **object**. `SaveSettingsPayload` now uses `ImageLayout` and the form's `save()` sends the layout object directly (GET still returns a JSON string).
     - **General max ratings removed from the UI** — the per-kind "Max ratings" number inputs are gone; the number of ratings shown is now driven by the layout grid (the sum of per-side `per_row × rows`). `serve.go` derives the fetch limit from the layout total per kind; previews default the limit to `layout.Total()` (the `?ratings_limit` query override still caps it when provided). The `LayoutEditor` shows "N ratings shown".
     - **Badge blocks no longer clipped at the image edge** — `overlaySideBlock` now scales a side block down (preserving aspect ratio) when it would exceed the canvas width/height, so the outermost badges are never cut off.
     - **Live preview on grid edits** — `LayoutEditor` previously mutated nested objects in place (via `v-model="sideSlot(side).value.per_row"`), so the parent's layout ref never got a new object and the preview watcher never fired. Rewritten to emit fresh layout objects on every change (`setSlot`/`setField` + `:model-value`/`@update:model-value`).
     - **Settings tabs unified** — all four image-type tabs now share the same structure: a larger preview + `LayoutEditor` side-by-side, then a consistent settings grid order (badge style → label style → text/badge/logo size → badge shape → background opacity → direction → type-specific extras). Rating Source Badges moved beside Rating Colours in a two-column layout.
   - Verified: `go build/vet/test` all pass; vitest 325/325 pass; `vue-tsc --noEmit` clean; `vite build` succeeds. Render checks: poster 5-badge row at 250% text now scales down to fit the 400px canvas with margins (no edge clipping).

0. **Per-source color save round-trip broken — root cause found 2026-08-01**:
   - Symptom: colors "don't always save", and the header **Discard changes** button stays visible after a successful Save.
   - Root cause: `SetGlobalSettingsBatch` (api-go/internal/services/db.go:1222) is UPSERT-only — it never deletes keys. `NormalizeSourceColors` (colors.go:88) strips overrides that equal the default, so `colorsToMap` (db.go:894) omits them from the batch. Any previously-stored `color_<key>_<attr>` row therefore **stays in `global_settings` forever** once set.
   - Consequence: reset a color to its default (or toggle a border off), save → the batch has no `color_imdb_accent`/`color_imdb_border` → the old row is still there → `HandleGetSettings` → `EffectiveSourceColors` returns the *old* color → the form reloads an old value ≠ the user's edit → `dirty` stays true → **Discard changes never disappears**, and the color appears to "not save".
   - Fix plan: when persisting global settings, also DELETE any `color_%` rows that are no longer present in the normalized set (targeted prune, not a blanket settings wipe).

0. **Badge background/colors "not rendered well" — 2026-08-01**:
   - The badge geometry is hand-rolled and aliased: `fillRect` (badge.go:262) writes hard pixel edges, `roundCorners` (badge.go:235) zeroes corner pixels producing jagged corners and *cuts into the border* (border is drawn, then corners are erased), and `drawRoundedBorder` (badge.go:222) is an aliased stroked rect. On a real photo the badges look crude.
   - Preview path already passes per-source colors through `parsePreviewColors` (handlers/preview.go:659) → `RenderBadgesUniform` → `renderBadgeInner` → `sectionColors` (badge.go:32), so color *application* works; the visible quality problem is aliasing, not color application.

0. **Previews use black built-in sample art + the wrong title — 2026-08-01**:
   - `demoPosterBytes` (handlers/preview.go:118) resolves IMDb `tt1528406` (the OLD demo title) and only for the poster. Logo/backdrop/episode previews use built-in `SampleLogoPNG`/`SampleBackdropPNG` (dark gradient / gray box) → "black preview with normal (sample) ratings".
   - Desired: all four previews render **tt12637874** (TV show) artwork; episode preview = S01E01 still.

0. **Browser console errors (h1-check / detectStore / file:// Security Error) — NOT app bugs**:
   - `h1-check.js` + `detectStore()` come from a Firefox **browser extension** content script (injected via `ExtensionContent.sys.mjs` into an isolated world), and the `Security Error … may not load or link to file:///` is the same extension trying to touch `file://`. None of these are produced by the app; no app-side fix applies.

0. **Root cause of "0 posters / fetch does nothing" — FIXED 2026-08-01**: The Go backend's image pipeline was entirely stubbed out:
   - `internal/image/serve.go` `ServeImage` returned `fmt.Errorf("not implemented")`.
   - `internal/handlers/image.go` `HandleImage` (public `/{key}/{idType}/poster-default/{id}.jpg` endpoint) returned an empty JPEG body.
   - `internal/handlers/preview.go` `HandleFetchImage` (admin `.../fetch`) returned a JSON `{"note":"fetch triggered"}` stub, so the frontend's `res.blob()` produced a broken preview and nothing was ever cached → `image_meta` stayed empty → admin lists showed 0.
   - Preview handlers returned a JSON stub instead of a rendered image.
   - `services.ImageMetaItem` had no JSON tags, so `ListImageMetaByKind` serialized as `CacheKey`/`ReleaseDate`/… while the Vue frontend expects `cache_key`/`release_date`/… (this would break the table even once rows existed).

### Fixes applied (all in `api-go/`)

0. **Fifth round — per-source rating colors (2026-08-01):**
   - Per-source color settings (accent / value background / border / text), stored in `global_settings` (`color_<key>_<attr>`), global-only. **Border is opt-in and defaults to none** (no border drawn unless a source has a border color set).
   - **Later refined**: the four per-source color pickers were reduced to **border + text** only. The badge background is now **black** with an **opacity slider** (0–100) that replaces the old "Badge background" dropdown (`BadgeBackground` enum → per-kind `BadgeAlpha`). New fields: `poster_badge_alpha`/`logo_badge_alpha`/`backdrop_badge_alpha`/`episode_badge_alpha` (default 80). Schema migration converts the old `*_badge_background` columns.
   - `services/colors.go`: `SourceColorSet` = {border, text}, defaults (border empty = none), `EffectiveSourceColors`, `NormalizeSourceColors`, `ParseHexColor`, validation.
   - `RenderSettings.Colors` + `BadgeAlpha` flow through parse/store, cache keys (`.col<hash>` / `.ba<alpha>` tokens), and badge rendering (black box at alpha; border only drawn when set).
   - **Bug fixed**: `overlayHorizontalRows`/`overlayVerticalStack` uint32 underflow drew badges off-canvas when a row exceeded the canvas width → now int + clamped.
   - Frontend: "Rating Colors" section (global only) with border on/off + color and text color per source; per-kind background-opacity sliders; no autosave — **Save** next to Refresh in the page header, **Discard changes** to its left shown only when dirty (form exposes `save`/`discard`/`dirty` via `defineExpose`; `showActions` prop). Free API key toggle folded into Save. Fixed a bug where `dirty` was always true after load (editColors wasn't initialized from settings.colors).
   - Tests updated for the Save-button flow and the badge-alpha change.

0. **Fourth round — font concurrency crash (2026-08-01):**
   - **Crash**: under parallel image requests (e.g. a Plex library grid), the server panicked with `runtime error: index out of range [15] with length 0` in `golang.org/x/image/font/sfnt.(*Font).LoadGlyph`, killing one poster request per occurrence.
   - **Root cause**: a single global `opentype.Face` (`loadedFontFace`) was shared by every concurrent request. Per the x/image docs, "A Face is not safe to use concurrently" — it owns a mutable `sfnt.Buffer` + `vector.Rasterizer`, so parallel `Glyph` calls corrupted the buffer and produced a bogus glyph index. (It was NOT a bad character in the font.)
   - **Fix**: `LoadFont` now keeps the immutable `*sfnt.Font` (documented thread-safe) and `GetFontFace()` returns a **fresh** `opentype.Face` per caller. Each render (GenerateImage + the four preview handlers) gets its own face, so no shared mutable state. Also made `drawText`/`textWidth` panic-safe per rune as defense-in-depth. Added `api-go/internal/image/serve_test.go` (`TestConcurrentRendering`) which runs clean under `-race`; verified with a 20-way concurrent fetch burst (0 panics).

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

## Research Notes (2026-08-01) — tools for better badge/color rendering

| Link | Reasoning | Decision |
|------|-----------|----------|
| https://pkg.go.dev/golang.org/x/image/vector | Anti-aliased 2-D vector rasterizer (`vector.Rasterizer`, floats, `MoveTo/LineTo/QuadTo/ClosePath`, draws a mask onto any `draw.Image`). **Already a direct dependency** (go.mod has `golang.org/x/image v0.44.0`). Perfect for smooth rounded-rect fills + border strokes for badges with zero new dependencies. | **USE for badge geometry**: replace aliased `fillRect`/`roundCorners`/`drawRoundedBorder` with anti-aliased vector paths. Primary choice. |
| https://pkg.go.dev/github.com/fogleman/gg | Full 2-D canvas (rounded rects, circles, text, gradients, `SetHexColor`, `DrawImage`), MIT, ~1.9k importers. Would also give anti-aliased shapes + easier stroke/radius. | **Fallback only** — last release v1.3.0 (2019), pulls in `golang.org/x/freetype`, and per RULES.md new deps need care. Not needed since `x/image/vector` covers the requirement. |
| https://pkg.go.dev/github.com/disintegration/imaging | High-quality resize, Gaussian blur, fit/crop helpers. Would upgrade `resizeToFill`/`resizeExact`/blur past the current `ApproxBiLinear` downscale-upscale. | Optional polish; adds a dependency. Defer unless blur/scale quality is explicitly requested. |
| https://pkg.go.dev/github.com/lucasb-eyer/go-colorful | Perceptual color math (HCL/Lab/OkLab), implements `color.Color`, hex parse/format, blend/distance. Could drive an auto text color (white vs black) from background luminance/contrast so text stays readable on any badge background. | Nice-to-have; WCAG relative-luminance contrast is ~10 lines by hand, so only add if richer color features are wanted. Auto-contrast text color can be implemented without it. |

### Open questions / decisions pending approval:
1. **Preview badges**: keep the current demo badge set (all 10 sources, so every source color is testable instantly) on the real tt12637874 artwork, or fetch the show's *real* ratings per preview (extra external API calls per slider tick — heavier and only sources with ratings appear)?
2. Whether to also add the auto text-color-from-background contrast tweak in the same pass.

### Decisions (user-confirmed 2026-08-01):
1. **Preview badges = REAL ratings for tt12637874, fetched ONCE and persisted.** Use the existing (currently unused) `available_ratings` table (`ReadAvailableRatings`/`UpsertAvailableRatings`, db.go:1461/1472) keyed by `imdb/tt12637874` for show previews and `imdb/episode-tt12637874-S1E1` for the episode preview. Persist as compact JSON `{key: value}` (rebuild `RatingBadge` via `SourceFromKey`). First preview request fetches via `FetchRatings`, writes to DB; later requests read the cache. Preview handler gains OMDB/MDBList/Trakt clients (extend `PreviewConfig`, wire all 8 router call sites).
2. **No auto text-contrast tweak** — text colour stays fully manual.

## Implementation Plan (approved steps, in order)

1. **Fix stale color keys on save** (`api-go/internal/services/db.go`, `api-go/internal/handlers/admin.go`):
   - In `HandleUpdateSettings`, after building `batch`, compute the existing `color_%` keys from the current `globals` that are NOT in `batch` and delete them (targeted prune). Add `services.RemoveGlobalSettings(db, keys []string) error`. Keeps Reset-a-color/toggle-border-off from resurrecting old values; this also makes `dirty` go false after save → **Discard changes disappears**.
2. **Anti-aliased badge geometry** (`api-go/internal/image/badge.go`):
   - Replace aliased `fillRect` + `roundCorners` + `drawRoundedBorder` with `golang.org/x/image/vector` rasterizer paths (already a dependency):
     - border = outer rounded-rect stroke (rasterize outer fill in border color, then inner rounded-rect on top in the section colors — inset by border thickness).
     - label section = full badge rounded rect in `labelBG`; value section = right-hand rounded region in `valueBG`.
   - No new dependencies. `GenerateImage` callers unchanged.
3. **Previews → tt12637874 artwork, real cached ratings** (`api-go/internal/handlers/preview.go`, `api-go/internal/router/router.go`, `api-go/internal/image/serve.go`, `api-go/internal/services/ratings.go`):
   - `demoPosterBytes` → `demoArtworkBytes(kind)`; resolve `imdb/tt12637874` (poster/logo/backdrop) and `imdb/episode-tt12637874-S1E1` (episode). Reuse the TMDB artwork fetch (poster path / `GetImages` logos+backdrops / episode still) with the on-disk base cache; add `image.DemoArtwork(...)` exported helper.
   - `sampleBadges()` → per-handler cached real badges via `available_ratings` (show key for poster/logo/backdrop, episode key for episode), fetched once on first miss.
   - Frontend: no changes required.
4. **Verify**: `go build ./...`, `go vet ./...`, `go test ./...` in api-go; frontend `npm test:unit` unaffected (no web changes); manual check of settings save + previews against the running app.

## Completed work (2026-08-01)

- **Stale color key prune** — added `services.RemoveGlobalSettings` + `services.PruneStaleColorSettings` (db.go) and wired it into `HandleUpdateSettings` (admin.go) so `color_*` rows absent from the saved batch are deleted. This fixes "colors don't always save" and the Discard-changes button staying after save.
- **Anti-aliased badges** — replaced the aliased `fillRect`/`roundCorners`/`drawRoundedBorder` with `golang.org/x/image/vector` paths (`roundedRectPath`/`roundedRectMask`/`fillRoundedRect` with a per-corner bitmask). Border is now an inset ring drawn under the section colors; corners are smooth. Both horizontal and vertical badges updated. No new dependencies.
  - **REVISED (same pass)**: the border ring is now stroked **on top** of the sections (`ringMask` = outer rounded-rect mask minus inset inner mask) so the badge center stays semi-transparent over the artwork, matching the old look. Sections are drawn first.
  - **Second round bug found & fixed**: `color.RGBA` is a **non-premultiplied** color, but `image/draw`'s Over blending expects premultiplied values; `(dr*a + sr*ma)/m >> 8` overflows past 8 bits and `uint8` wraps → semi-transparent colored fills rendered as dark blobs (the "two cut off circles" + "background colours not rendered well"). `sectionColors` now returns `color.NRGBA` (whose `RGBA()` premultiplies), so colored badge sections composite correctly. Opaque fills (white borders, opaque text) are unaffected. This also explains why only the *black* default ever looked right before.
- **Previews use tt12637874** — new `image.DemoArtwork` resolves `imdb/tt12637874` (S01E01 for episode) and fetches poster/logo/backdrop/still artwork via the existing TMDB fetch + base cache. Preview handlers use it per-kind, falling back to the sample gradients only when TMDB is down.
- **Real ratings, fetched once** — `services.MarshalRatingBadges`/`UnmarshalRatingBadges` persist badge sets as compact JSON (`{key:value}`). `PreviewHandler.demoBadges(kind)` reads the demo title's ratings from `available_ratings` (`imdb/tt12637874` or `imdb/episode-tt12637874-S1E1`), fetches once on miss via `FetchRatings` (needs OMDB/MDBList/Trakt clients — added to `PreviewConfig` and wired at all 8 preview routes via `AppState.previewConfig()`), and falls back to the sample badge set only when nothing can be fetched.
- **Verification**: `go build ./...`, `go vet ./...`, `go test ./...` all pass inside the repo's `golang:1.25-bookworm` toolchain; full `docker compose build` (Containerfile, Go + web) succeeds. Running container still uses the old image until restarted.

## Settings export / import (2026-08-01)

- **Backend**: `services/export.go` — `ExportPayload` (kind `openposterdb/settings`, `settings` = all `global_settings` except `service_key_*`, plus optional decrypted `service_keys` and optional `api_keys` [name + per-key settings]). `BuildExportPayload` / `ApplyImportPayload` (upserts global settings, re-encrypts & stores service keys for non-env-locked services, matches API keys by name or recreates them with a fresh value — raw values can't be recovered from their hash, so new ones are returned in `ImportResult.regenerated_keys`). `ServiceKeyManager.PlaintextKeys()` decrypts DB-stored keys. `FindAPIKeyByName` added. Handlers `handlers/settings_export.go`: `GET /api/admin/settings/export?include_service_keys=&include_api_keys=` and `POST /api/admin/settings/import` (both admin-auth).
- **Fixed latent bug**: `UpsertAPIKeySettings` had **42 `?` placeholders for 41 columns** → any per-key settings save failed with "42 values for 41 columns". Removed the extra placeholder (verified by the new `TestExportImportRoundTrip`, which exports settings + a service key + an API key and imports into a fresh DB).
- **Frontend**: `adminApi.exportSettings/importSettings`. New "Backup & Restore" box at the bottom of the Settings page with an **Export** button that opens a dialog (checkboxes for "External API keys" and "API keys", both default on) that downloads `openposterdb-settings-<date>.json`, and an **Import** button that reads a JSON file, restores it, refreshes settings, and shows any newly-regenerated API key values.

## Build / deployment notes (2026-08-01)

- **Version string**: the sidebar reads `version` from `web/package.json`, which the Containerfile rewrites to `APP_VERSION`. When building locally you must pass the full version including the commit, e.g. `docker compose build --build-arg APP_VERSION=1.2.1-dev-$(git rev-parse --short HEAD)` (the Makefile's `build`/`up` targets already do this). Passing only `1.2.1-dev` drops the commit from the displayed version.
- **UI (settings page)**: the form is now a stack of bordered boxes matching Poster/Logo/Backdrop/Episode — `Image Settings` (heading + Save/Discard + Language + Fanart), `Rating Display`, and one combined `Rating Colours` + `Rating Source Badges` box (both headings inside a single `rounded-md border p-4` container). All box headings use `text-sm font-semibold`. "Backdrop (series/movie)" renamed to "Backdrop (movie/series)".

## ebert highRes SVG (created 2026-08-02)

- **Request**: create an SVG from the ebert icon so the High Res label style has a real Roger Ebert logo (it was the only source missing a `highRes` SVG, falling back to the 48px official PNG).
- **Approach (user-approved)**: trace the 48px `official/ebert.png` into an SVG. No tracing tool is installed and RULES forbid installing software, so I wrote a pure-Python tracer (`/tmp/opencode/trace.py`): binarize at native 48px (alpha > 128), upscale the mask 10× with nearest-neighbour, extract boundary segments of filled cells, connect into loops, `collapse_collinear` + `simplify` (RDP), then emit `viewBox="0 0 250 250"` with the artwork scaled to preserve the source's relative footprint (content was 34×31 in 48 → fills ~250×(34/48)×(31/48), centred) and the hole loop wound opposite to the outer loop so the default `nonzero` fill rule punches the notch.
- **Renderer caveats handled**: `tdewolff/canvas` ignores `fill-rule="evenodd"`, so the inner loop winding is reversed instead (verified: notch renders transparent). The anti-aliased diagonals in the 48px source can't be reproduced exactly by hard-edge tracing — ~5% of pixels differ at 48px (all on the slanted edges); the silhouette (E shape, diagonal top, notched stem) matches.
- **Result**: `api-go/assets/icons/highRes/ebert.svg` (fill `#e8590c`, matching the source). Verified end-to-end: `loadHighResIcons` now loads 14/14 icons including ebert, and a `LabelStyleHighRes` ebert badge renders icon pixels (`TestHighResIconLoad` covers it). Go build/vet/test pass.
- **Smoothed (user requested "smoothen borders a lot")**: rewrote `smooth_stairs` to also collapse staircases with comparable-but-unequal step lengths (the left diagonal's H/V steps are ~5.2/10.4 units, which the old equal-length check skipped).
  - **REVISED (user: "outer smooth, inner box must stay a box")**: the previous pass applied Chaikin to every loop, rounding the notch into an ellipse. The final `ebert.svg` applies **Chaikin (3 iterations, 0.25) to only the outer outline** (identified by largest area) while the notch loop is left as an exact 4-point box. Outer boundary is smoothly curved (boundary deltas 0–2px per 2px of travel in tdewolff renders), the notch renders as a constant-width transparent rectangle (left x=47, right x=77, top y=123, bottom y=189).
  - Also fixed two `smooth_stairs` bugs found while debugging: greedy segments now handle wrap-around index ranges correctly (ring rotated so no chosen segment crosses the 0 boundary; start index picked outside covered + endpoint sets), and runs stop at steps much longer than the staircase so they don't swallow the long edge after a diagonal.
  - **REVISED again (user: "you cut the finger — it's supposed to look like a thumbs up")**: two bugs were cutting the thumb tip:
    1. The default binarize threshold (`alpha > 128`) excluded the thumb-tip pixels (alpha 104–109 at row 6), so the trace started at row 7. Lowered to `alpha > 100` (content bbox becomes y[6,37] and the tip is captured).
    2. The fit **centered** the artwork, but the source has asymmetric margins (tip sits 6px from top, 11px from bottom), so centering pushed the thumb down 2px. Fit now reproduces the source's exact top/left margin fractions instead of centering (`tx = 250*mx - minx*s`, `ty = 250*my - miny*s`, with `mx`/`my` from the source content bbox).
    - Also made `smooth_stairs` require **constant horizontal and vertical step lengths** within a run, so it no longer merges the thumb's 2:1 diagonal into the 1:1 body diagonal (a slope change). Final: `TRACE_ALPHA=100 CHAIT=3` — thumb tip renders at the correct top margin (topmost pixel y=31 in 250-space, matching 48px row 6), notch stays an exact box, 48px hard-diff vs source drops to ~21 px (<1%).
  - **REVISED again (user: "make the thumb top point bigger to the right so it doesn't look like a dent")**: the traced tip was a narrow spike (cap only ~1.2 units wide at x 118–119, right edge a flat "shelf" at y 33–35 that then jumped from R=125 to R=130). Manually reshaped the tip cap in the SVG path to a broad rounded arc extending right (cap now ~121.5–128.8 at the top, widening smoothly). Verified in tdewolff: tip renders as a round cap (no spike/dent), notch still a constant 29px box, 48px hard-diff ~22 px.

## High Resolution label style `h` (IMPLEMENTED, 2026-08-02)

- **Request**: new "high resolution" icon label style that uses the `highRes/` SVG files, rendering crisp provider logos in badges.
- **Design (user-approved)**: add a 4th `LabelStyle` value `h` (High Res) that rasterizes the `assets/icons/highRes/*.svg` source art at 250px and uses it like the `o` (Official) style. Chosen over pre-rasterizing to PNGs: added a Go SVG renderer dependency.
- **SVG renderer**: `github.com/tdewolff/canvas` (+ transitive deps pulled in by `go get`/`go mod tidy`). Evaluated `srwiley/oksvg` first — it cannot process SVG `<mask>` elements and rendered 12/13 icons blank/black, so it was rejected.
- **Fidelity work** (verified vs ksvgtopng5, all 13 icons <0.3% pixel diff after fixes):
  - `tmdb.svg`: gradient `<stop>`s used `style="stop-color:#…"`, which tdewolff ignores → logo rendered black. Changed to `stop-color="#…"` attributes (equivalent, renderer-compatible).
  - `Rotten_Tomatoes_critic_certified_fresh.svg`: full-canvas `<rect fill="#000000" opacity="0">` (invisible in browsers) rendered as opaque black in tdewolff → removed the rect.
  - 3 more RT SVGs use `<mask>` bounding-rect clips; tdewolff renders mask content literally (white boxes). Since these masks only clip to the artwork bounds, `loadSVG` strips `<mask>` blocks and `mask="url(#…)"` refs at load time (visually lossless, keeps source files intact).
- **Implementation**:
  - `services/db.go`: `LabelStyleHighRes = "h"` in const, `ParseLabelStyle`, `UsesIcon`.
  - `image/icons.go`: `highresCache`; `loadSVG` (ParseSVG → stripMasks → `rasterizer.Draw` at 250px, aspect-fit); `loadHighResIcons()` loads all 14 `officialIconKeys` (ebert has no SVG → skipped, falls back to official PNG); `HighResIconForBadge` mirrors `OfficialIconForBadge` from `highresCache`.
  - `image/badge.go`: `iconForBadge`, `badgeIconAndSize`, `labelWidthForStyle` treat `h` like `o` (fit-in-box).
  - Handlers pass `label_style=h` through unchanged (all cast strings; no validator restricts it).
  - Frontend: "High Res" option added to all 4 label-style selects (`RenderSettingsForm.vue`) + `FreeApiKeyCard.vue`; `LABEL_STYLE_LABELS` gains `h: 'High Res'` (constants.ts). The `.webp` frontend refs are generated from PNGs by the vite build (`icons total` log), so no web/public changes were needed.
  - Docs: `docs/api.md`, `docs/architecture.md` updated for `label_style={t|i|o|h}` / `.lh` cache suffix (cache suffix is generic `.l{style}`, no change needed).
- **Tests**: `TestHighResIconLoad` (serve_test.go) asserts all sources with SVGs load distinct icons and `h`-style badges render icon pixels. `go build/vet/test` pass; frontend `vitest run` 329/329 pass (in `node:22-bookworm` container); `vue-tsc` still reports the same 75 pre-existing test-file errors (none new); `vite build` succeeds.
- **Runtime note**: highRes SVGs are loaded once at startup via `LoadIcons()` (`main.go:119`); asset path `assets/icons/highRes` is relative to the working dir (`/app` in the container), matching the existing font/icon loading.

## TMDB icon centering (analysis, 2026-08-02)- **Request**: centre the TMDB logo in `api-go/assets/icons/highRes/tmdb.svg`.
- **Measurement** (rendered via ksvgtopng5, bright-pixel bbox): logo native bounds x `0–511`, y `71–440` (512×370), native center `(255.5, 255.5)`. Background card (rounded rect + stroke) spans `32–480` on both axes → card center `256`.
- **Current state**: `translate(73, 64.08) scale(0.75)` → rendered logo `x 73–457` (center **265**, i.e. 9px right of 256), `y ~117–394` (center ~255.7, already centered). Horizontal is off; vertical is fine.
- **Proposed fix** (1-line change, no other behaviour change): `translate(73, 64.08)` → `translate(64.375, 64.375)` keeps `scale(0.75)`; logo center lands exactly at `(256, 256)` with symmetric 32.375px margins inside the card.
- **Awaiting approval** (RULES.md waiting state) before editing the file.
- **DONE (user approved 2026-08-02)**: `translate(73, 64.08)` → `translate(64.375, 64.375)` in `tmdb.svg:31`. Verified by render: logo bbox `x 64–448`, `y 118–394`, center `(256.0, 256.0)`, symmetric 32px horizontal / 86px vertical margins to the card.
- **All 13 highRes SVGs normalized to 250×250 (user approved 2026-08-02)**: every icon in `api-go/assets/icons/highRes/` was rewritten onto a `viewBox="0 0 250 250"` canvas with its artwork scaled to fill (aspect-preserving, `translate(...) scale(...)` wrapper `<g>`), so artwork is centered within each icon. This also fixed `Rotten-Tomatoes-Rotten.svg`, whose tomato sat ~4% left of center (its art spanned x `0–73.8` of an 80-wide viewBox; now centered). Verified by rendering all 13 at 250px: content center within ±0.5px of (125,125), XML parses cleanly. These SVGs are source art only — the runtime loads 48×48 PNGs from `icons/white`/`icons/official` (icons.go), so no app change. Backups of the originals: `/tmp/opencode/backup_*.svg`.
- **highRes SVG filenames unified to runtime `official/` keys (user approved 2026-08-02)**: the user first renamed the 6 RT SVGs into `critics-*`/`audience-*`; I then renamed all 13 to exactly match the `icons/official/` keys — `imdb`, `letterboxd`, `metacritic`, `mal`, `mdblist`, `tmdb`, `trakt`, `Rotten_Tomatoes_critic_certified_fresh`, `Rotten_Tomatoes_critic_positive`, `Rotten_Tomatoes_critic_rotten`, `Rotten_Tomatoes_positive_audience`, `Rotten_Tomatoes_negative_audience`, `Rotten_Tomatoes_verified_hot_audience` (all `.svg`). Content untouched (verified: all still render at 250px); source-art only, no code references the old names.

## Per-kind text size settings (analysis, 2026-08-02)

- **Request**: add a per-kind setting to change the text size (rating-badge text) in the UI, separately for poster / logo / backdrop / episode.
- **Context**: the generated images render rating badges. Font faces are created at fixed sizes in `image/serve.go` (`labelFontFaceSize = 26.0`, `valueFontFaceSize = 32.0`) and the badge geometry scales via `badgeScale` = `imageSize.BadgeScale(kind) * badgeSize.ScaleFactor()` (db.go:432, badge.go `newScaledDims`). The existing "Badge size" dropdown (xs–xl) is a coarse whole-badge scale; there is currently no per-kind control over the text/font itself.
- **Proposed design**: four new integer settings `poster_text_size` / `logo_text_size` / `backdrop_text_size` / `episode_text_size` (percent, default 100, clamped 50–200). At 100% output is byte-identical to today. A kind's text scale multiplies both the badge font faces (`GetValueFontFaceAt`/`GetFontFaceAt` at `32*scale` / `26*scale`) and `badgeScale`, so the badge box stays proportional to its text (no overflow). Cache key gains a `.ts<N>` token (only when ≠ 100, keeping existing keys stable). Flow-through: `RenderSettings`/`APIKeySettings` (db.go) + defaults/parse/toMap/effective, 4 schema migrations for `api_key_settings`, `HandleGetSettings`/`updateSettingsRequest` (admin.go), `FreeKeySettingsResponse` (image.go), preview `text_size` query param (preview.go), `text_size` query override on the public endpoint (image.go), frontend form sliders + snapshots + save + preview params (RenderSettingsForm.vue, api.ts).
- **Files to touch**: api-go/internal/services/{db.go, cachesuffix.go}, api-go/cmd/server/schema.go, api-go/internal/handlers/{admin.go, image.go, preview.go}, api-go/internal/image/{serve.go, badge.go}, web/src/components/RenderSettingsForm.vue, web/src/lib/api.ts, tests (db_test.go, export_test.go, RenderSettingsForm.spec.ts, SettingsView.spec.ts).
- **Awaiting approval** (RULES.md waiting state) before implementation.

## Per-kind text size settings + preview fix + tabbed settings page (IMPLEMENTED, 2026-08-02)

### Text size replaces Badge size (user chose option C: replace the dropdown with a slider)- **New per-kind settings** `poster_text_size` / `logo_text_size` / `backdrop_text_size` / `episode_text_size` (int, percent). `TextSize` type (db.go): `DefaultTextSize() = 100`, `ClampTextSize` (50–200), `ScaleFactor(kind)` returns the badge multiplier (100 = the kind's historical default: 1.2 poster/logo/backdrop, 1.45 episode, so default output is byte-identical to before), `Percent()` for the font faces, `CacheSuffix()` = `.ts<N>` (empty at 100 so existing default keys stay clean). The `BadgeSize` enum (`xs`–`xl`) was **removed** from settings/API.
- **Rendering** (`image/serve.go`): `GetFontFacesAt(textSizePct)` builds label/value faces at `26·pct/100` / `32·pct/100`; `GenerateImage` multiplies `badgeScale` by `TextSize.ScaleFactor(kind)`. `RenderPosterSync/RenderBackdropSync/RenderEpisodeSync` now take `textScale float32` instead of `BadgeSize`, and the row-cap logic keys off `textScale >= 1.45` (matches the old Large/XL threshold). `RenderLogoSync` accepts `textScale` too.
- **API**: settings GET/PUT now carry `*_text_size` (admin.go); public image endpoint + free-key settings use `text_size` query param (image.go, `text_size` parsed as int, legacy `badge_size` still accepted as an int alias); the four preview handlers read `text_size` (preview.go, `previewTextSize` helper).
- **DB**: 4 schema migrations add `*_text_size` columns and drop the old `*_badge_size` columns from `api_key_settings`; `GetAPIKeySettings`/`UpsertAPIKeySettings`/`GetEffectiveRenderSettings` updated.
- **Frontend**: `RenderSettingsForm.vue` replaced the four "Badge size" selects with "Text size" sliders (50–200%, step 1) plus snapshot/save/preview plumbing (`editXTextSize`, `text_size` preview args). `api.ts` preview fns + `SaveSettingsPayload` use `text_size`. `FreeApiKeyCard.vue` replaced the badge-size select with a number input (`text_size`). `auth-api.ts` `FreeKeyDefaults` updated. `BADGE_SIZE_LABELS` removed from `constants.ts`. Docs (`docs/api.md`, `docs/architecture.md`) and dev scripts (`scripts/visual-report.sh`, `scripts/regenerate-examples.sh`) updated.

### Preview staleness bug — FIXED
1. `RenderSettingsForm.vue`: the `props.settings` watcher now snapshots the edits before applying, and after `nextTick` (syncing=false) calls `updateAllPreviews()` when the applied values actually changed — so externally-refreshed settings immediately re-render all previews.
2. `SettingsView.vue`: `saveSettings` now calls `refetch()` after a successful PUT so the react-query cache is fresh (no stale pre-save data on re-visit).

### Settings page restructured into sidebar tabs
- New shadcn-style `Tabs` UI components (`web/src/components/ui/tabs/`) built on reka-ui, matching the existing shadcn pattern.
- `SettingsView.vue` now uses a `grid grid-cols-1 lg:grid-cols-[300px,1fr]` layout with a left section nav (General / Image / Cache / Backup) and right-hand tab panels, mirroring the reference design. All four panels are force-mounted (like the reference HTML keeps every tabpanel in the DOM) so the Save/Discard header actions and the form stay live regardless of the active tab. Each card got the reference-style uppercase title bar.

### Verification
- `go build ./...`, `go vet ./...`, `go test ./...` all pass (host Go 1.26).
- Frontend `vitest run`: **329/329 pass** (run in the project's `node:22-bookworm` container because `node_modules` is root-owned from the docker build; no passwordless sudo). `vite build` succeeds.
- Lint (oxlint) clean on all changed files. `vue-tsc --build` reports the **same 75 pre-existing errors** as the untouched baseline (all in `__tests__`, e.g. strict-null `findCurlCode(...)` usages, `FreeKeyDefaults` badge_background mismatch) — none introduced by this work. The pre-existing single-word `vue/multi-word-component-names` eslint warnings also apply to every existing ui component (Card, Select, …).

## Preview bug: saved logo badge style shows vertical after revisiting (2026-08-02)

- **Symptom**: set Logo → Badge style = Horizontal, save, navigate away and back → the logo preview renders vertical; only re-selecting Horizontal manually corrects it.
- **Root cause (two interacting issues in the Vue settings page)**:
  1. `SettingsView.vue` loads settings via `useQuery(['global-settings'])`. Saving goes through `RenderSettingsForm.save()` → `saveSettings` → `adminApi.updateSettings` (PUT) — this never updates the react-query cache, and the form's `loadSettings()` GET also bypasses it. On revisiting the page, react-query serves the **stale pre-save cache** first (vertical), then refetches the fresh value in the background.
  2. `RenderSettingsForm.vue`'s `watch(() => props.settings, …)` sets `syncing = true`, calls `applySettings(s)` (which changes `editLogoBadgeStyle` to the fresh value), and flips `syncing = false` inside `nextTick` — but **never refreshes the previews**. Every preview watcher early-returns while `syncing` is true, so when the fresh settings arrive the logo preview stays on the stale (vertical) render. Manually re-selecting the style triggers the watcher outside `syncing`, fixing it.
- **Fix plan**:
  1. `RenderSettingsForm.vue`: after the settings-sync `nextTick`, call `updateAllPreviews()` so any externally-updated settings immediately re-render all previews.
  2. `SettingsView.vue`: after a successful `adminApi.updateSettings`, call `refetch()` so the query cache is fresh and revisits don't briefly mount with stale data.
- **Verified by reading**: `RenderSettingsForm.vue` watchers (lines 300-307, 513-551), `SettingsView.vue` `saveSettings`/`useQuery` (lines 45-112), `api.ts` `adminApi.updateSettings`, backend save/GET paths (admin.go `HandleUpdateSettings`/`HandleGetSettings` store and return `logo_badge_style` correctly).

## Follow-up round: tabs fix + three independent size controls (2026-08-02)

### Settings page tabs
- **Bug**: clicking the section tabs did nothing and all four panels were visible. Root cause: reka-ui's `TabsContent` computes `hidden = !(forceMount || isSelected)`, so the `force-mount` I used forced `hidden=false` on every panel → all rendered.
- **Fix**: `SettingsView.vue` now sets `:unmount-on-hide="false"` on the `Tabs` root and removed `force-mount` from the panels. With `unmount-on-hide=false` reka-ui keeps all panels mounted (so the form/previews stay alive and tests still find everything) but applies the `hidden` attribute to inactive ones — only the active section is shown, and clicking switches. Matches the reference markup (all tabpanels in the DOM, inactive ones `hidden`).

### Three independent size controls (text / badge / logo)
- The user asked for **text size, badge size and logo size**, each adjustable per image type, with **no overflow outside the badge**.
- Backend: replaced the single `TextSize` type with a shared `ScalePercent` (int 50–200, default 100; `DefaultScalePercent`/`ClampScalePercent`/`Percent`/`ScaleCacheSuffix(prefix, …)` emitting `.tsN`/`.bzN`/`.lsN`). Each kind now has three settings: `*_text_size`, `*_badge_size`, `*_logo_size` (12 total), plumbed through `RenderSettings`, `APIKeySettings` (+ 8 new `api_key_settings` schema migrations), parse/store/effective, admin GET/PUT, public image query params (`text_size`/`badge_size`/`logo_size`), free-key response, preview handlers, and cache suffixes.
- **Rendering** (`image/badge.go`): the badge box now **auto-sizes to fit its content** — the height grows from its base to the taller of the value text, the label text, or the (logo-scaled) icon; widths already follow `textWidth`. Logo height = `48 · badgeScale · logo_size/100`. No clipping at any setting.
- **Scales** (`image/serve.go`, `generate.go`): `badgeMultiplier = kindDefault · badge_size/100` (kindDefault 1.2 poster/logo/backdrop, 1.45 episode) drives the frame scale + the per-row cap (`>= 1.45`); font faces come from `GetFontFacesAt(text_size)`; `logoScale = logo_size/100` flows into the badge renderers. At 100/100/100 output matches the historical defaults.
- Frontend: each of Poster/Logo/Backdrop/Episode now has three sliders (Text size / Badge size / Logo size, 50–200%); `FreeApiKeyCard` gained Badge size / Logo size number inputs; `SaveSettingsPayload`/`FreeKeyDefaults`/`api.ts` preview fns updated.

### Verification
- `go build/vet/test`, `vitest run` **329/329**, `vite build`, and full `docker compose build` all pass. `vue-tsc --build` still reports the same 75 pre-existing test-file errors (none new). oxlint clean on changed files.



- Each of the 6 RT logo variants now has its **own** color settings (accent/value/border/text) instead of sharing one per source:
  - critics: `rt_cf` (Certified Fresh), `rt_pos` (Fresh), `rt_rot` (Rotten)
  - audience: `rta_hot` (Verified Hot), `rta_pos` (Fresh), `rta_neg` (Rotten)
- Backend (`services/colors.go`): `RatingVariant` + `rtVariants` (thresholds mirror `OfficialIconForBadge`), `RatingColorKey(badge)` maps an RT badge to its variant by score, `AllColorKeys()`/`IsColorKey`/`DefaultColorSetForKey`/`RTVariantByKey`. `EffectiveSourceColors`, `NormalizeSourceColors`, `ValidateSourceColors`, and `parseSourceColors` all operate over source keys + variant keys. Storage keys are `color_rt_cf_accent` etc. — no migration needed.
- `image/badge.go`: `colorOverride(colors, badge)` now looks up the variant key first, then falls back to the parent source key (`rt`/`rta`) so colors saved before the split still apply. `ratings_order`/`ratings_exclude` still use the logical `rt`/`rta` keys (unchanged).
- Frontend (`constants.ts`): `RT_COLOR_VARIANTS` (6) + `RATING_COLOR_ROWS` (8 non-RT sources + 6 variants). The Rating Colors section iterates `RATING_COLOR_ROWS` (labels widened to w-44). The badge gallery keeps showing RT's 3 variants, now coloured per-variant.
- Verified: `rt` 90% badge renders with the `rt_cf` green accent, `rt` 40% with the `rt_rot` red accent, distinct logos and sizes. Go build/vet/test + 330 frontend tests pass.

## Rating Source Logos in the UI (2026-08-01)

- New public endpoint `GET /api/icons/{kind}/{key}` serves rating-source logo files from `assets/icons` (kind `default`) and `assets/icons/official` (kind `official`), preferring `.png` then `.webp`. Keys are whitelisted (`iconSourceKeys`/`officialIconKeys` in `icons.go`); unknown keys → 404. No auth required so `<img>` tags can load them (brand logos only, not sensitive). Handler: `handlers/icons.go` → `HandleIcon`; route registered in `router.go`.
- **New public endpoint `GET /api/icons/badge?source={key}&value={value}`** renders a single badge (official logo + value + background) as PNG using the current global color settings (`HandleBadgePreview`, `handlers/icons.go`). Rotten Tomatoes values pick the logo variant (e.g. 40% → rotten, 90% → certified fresh), matching real behaviour.
  - **REVISED (round 3)**: the badge endpoint now accepts the same live render params as the preview endpoint (`colors`, `badge_alpha`, `badge_shape`, `label_style`, `ratings_order`, `ratings_exclude`, `ratings_limit`) and falls back to saved settings. The frontend gallery passes the current form state (debounced 300ms) so **unsaved** border/colour changes show up immediately, matching the poster preview. Fixed a test break: `RenderSettingsForm.spec.ts` mocks `@/lib/api` to `{}`, so the component inlines the colors-JSON serialization instead of importing `colorsQuery` at runtime.
  - **REVISED (round 4)**: section opacity bug — the label fill covered the whole badge and the value fill composited over it, so the value (text) section was effectively more opaque than the label (logo) section. Both sections are now drawn as **non-overlapping tiles** (label `[0,valueX]` TL|BL, value `[valueX,totalW]` TR|BR; vertical: label top TL|TR, value bottom BL|BR). Verified both sections render at alpha 204 for 80%.
  - **REVISED (round 5)**: V-notch bug in `roundedRectPath` — the square-corner branch did `LineTo(tx,ty)` (the point *after* the corner, e.g. `(x1, y0+r)`), so every "square" corner became a **bevel**, cutting a V-shaped notch out of the badge at the label/value seam (visible as transparent wedge holes at the top/bottom of the seam). The else branch now does `LineTo(cx,cy)` (the actual corner point). Verified: no interior transparent holes remain.
- Frontend: `SOURCE_BADGE_SAMPLES` in `web/src/lib/constants.ts` (per-source sample values; Rotten Tomatoes lists 3 variants). `RenderSettingsForm.vue` shows a "Rating Source Badges" gallery below the global-only "Rating Colors" section: one rendered badge per source (3 for Rotten Tomatoes critics/audience — one per logo variant). The separate logo column was removed at the user's request; only the badge previews remain. The `/api/icons/{kind}/{key}` logo endpoint stays (still used by nothing in the UI after the change, but harmless and whitelisted).
- Verified: `go build/vet/test` pass; frontend `npm run test:unit` 330/330 pass; badge endpoint returns distinct images per RT value and 400 for unknown sources.
