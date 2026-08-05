# Architecture

Internal reference for how OpenPosterDB caches and renders images. You don't need any of this to self-host — it's here for contributors and anyone debugging cache behaviour.

- [Tech stack](#tech-stack)
- [Cache architecture](#cache-architecture)
- [Clearing the cache](#clearing-the-cache)

## Tech stack

- **API**: Go, net/http + SQLite, image for rendering
- **Web**: Vue 3, TypeScript, Tailwind CSS, Vite

## Cache architecture

Images are cached in three layers: in-memory (MemCache), filesystem, and SQLite metadata. Cache keys encode all the settings that affect the rendered output so that different configurations produce separate cached files.

### Filesystem layout

```
{CACHE_DIR}/
├── base/
│   ├── posters/{tmdb_size}/  # Raw TMDB poster downloads (grouped by CDN size: w500, w780, original)
│   └── fanart/           # Raw fanart.tv downloads ({fanart_id}.{ext})
├── posters/{id_type}/    # Rendered poster JPEGs
├── logos/{id_type}/       # Rendered logo PNGs
├── backdrops/{id_type}/   # Rendered backdrop JPEGs
├── episodes/{id_type}/   # Rendered episode JPEGs
└── preview/{subdir}/      # Preview images for the settings UI
```

### Cache key format

Cache keys uniquely identify a rendered image. They are used as keys in the in-memory cache and stored in the `image_meta` SQLite table.

All kinds share one shape — `{id_type}/{id_value}{variant}{suffixes}` — the suffixes are appended in a fixed per-kind order:

**Poster:**
```
{id_type}/{id_value}{variant}{ratings}{badge_style}{label_style}{badge_direction}{layout}{text_size}{badge_size}{badge_width}{badge_height}{logo_size}{badge_shape}{badge_alpha}{fit}{image_size}{colors}
```

**Logo:**
```
{id_type}/{id_value}{variant}{ratings}{badge_style}{label_style}{layout}{text_size}{badge_size}{badge_width}{badge_height}{logo_size}{badge_shape}{badge_alpha}{image_size}{colors}
```

**Backdrop:**
```
{id_type}/{id_value}{variant}{ratings}{badge_style}{label_style}{badge_direction}{layout}{text_size}{badge_size}{badge_width}{badge_height}{logo_size}{badge_shape}{badge_alpha}{edge_inset}{image_size}{colors}
```

**Episode:**
```
{id_type}/{id_value}{variant}{ratings}{badge_style}{label_style}{badge_direction}{layout}{text_size}{badge_size}{badge_width}{badge_height}{logo_size}{badge_shape}{badge_alpha}{blur}{image_size}{colors}
```

Suffix order comes from `SettingsCacheSuffixWithRatings` (`internal/services/cachesuffix.go`); every suffix is emitted unconditionally except the ones marked "only when…" in the table below (defaults produce no token so existing keys stay stable).

### Suffix reference

| Suffix | Format | Example | Description |
|---|---|---|---|
| Ratings | `@{chars}` | `@mil` | Single-char per source, no commas (`m`=MAL, `i`=IMDb, `l`=Letterboxd, `r`=RT, `a`=RT Audience, `c`=Metacritic, `t`=TMDB, `k`=Trakt, `d`=MDBList score, `e`=Roger Ebert) |
| Badge style | `.s{style}` | `.slr`, `.stb` | `lr` = logo left, `rl` = value left, `tb` = logo top, `bt` = value top (legacy aliases `h`/`v` accepted); resolved via `ForShape` |
| Label style | `.l{style}` | `.lt`, `.li`, `.lo`, `.lh` | `t` = text labels, `i` = icon labels, `o` = official provider logos, `h` = high-resolution provider logos (rasterized from `highRes` SVGs) |
| Badge direction | `.d{dir}` | `.dh`, `.dv` | `h` = horizontal, `v` = vertical, `d` = default (layout positions are set via the per-kind layout grid; the UI no longer exposes direction) |
| Layout | `.ly{token}` | `.lya1b2c3` | 8-char hash of the per-side badge layout (per-row × rows × start per side + fill order); only present when non-default |
| Text size | `.ts{n}` | `.ts145` | Rating text size as a percentage of the default (only when ≠ 100) |
| Badge size | `.bz{n}` | `.bz120` | Overall badge size as a percentage of the default (only when ≠ 100) |
| Badge width | `.bw{n}` | `.bw150` | Badge width scale as a percentage of the default (only when ≠ 100) |
| Badge height | `.bh{n}` | `.bh75` | Badge height scale as a percentage of the default (only when ≠ 100) |
| Logo size | `.ls{n}` | `.ls110` | Rating source logo size as a percentage of the default (only when ≠ 100) |
| Badge shape | `.sh{shape}` | `.shr`, `.shp` | `r` = rounded (default), `p` = pill (the `sh` prefix distinguishes it from the `.s{style}` token above) |
| Badge alpha | `.ba{n}` | `.ba80` | Badge background opacity (0–100); the badge background is black at this alpha |
| Poster fit | `.f{fit}` | `.fc`, `.fp`, `.fb` | `c` = cover, `p` = pad, `b` = blur — `native` (default) emits no token |
| Edge inset (backdrop) | `.eh{n}` / `.ev{n}` | `.eh8`, `.ev3` | Backdrop ratings inset from the edge by `n`% — `eh` horizontal, `ev` vertical; only when non-zero |
| Blur (episode) | `.blur` | `.blur` | Episode spoiler blur; only when enabled |
| Image size | `.z{size}` | `.zm`, `.zl` | `s` = small, `m` = medium (default), `l` = large, `vl` = very-large |
| Colors | `.col{token}` | `.colab12cd34` | 8-char hash of the per-source color overrides; only when any override is set |

### Source variant markers

The variant marker encodes the source and the language, and carries the kind letter for logo/backdrop keys (`cacheVariant` in `internal/image/serve.go`). Posters use a separate scheme — the default case (English, non-textless) has no marker for backward compatibility:

| Image type | Variant | Marker | Description |
|---|---|---|---|
| Poster | TMDB default | *(none)* | Default English poster from TMDB (backward-compatible) |
| Poster | TMDB language | `_t_{lang}` | Language-specific TMDB poster (e.g. `_t_de`) |
| Poster | TMDB textless | `_t_tl` | Textless TMDB poster |
| Poster | Fanart textless | `_f_tl` | Fanart image with no text overlay |
| Poster | Fanart language | `_f_{lang}` | Fanart image matching language (e.g. `_f_en`) |
| Logo | TMDB | `_l_t_{lang}` | Logo sourced from TMDB (e.g. `_l_t_en`) |
| Logo | Fanart | `_l_f_{lang}` | Logo sourced from Fanart.tv |
| Backdrop | TMDB | `_b_t` | Backdrop sourced from TMDB |
| Backdrop | Fanart | `_b_f` | Backdrop sourced from Fanart.tv |
| Episode | *(none)* | — | Episodes have no variant marker |

### Database values

The `image_meta` table tracks metadata for cached images:

| Field | Short Value | Meaning |
|---|---|---|
| `image_type` | `p` | Poster |
| `image_type` | `l` | Logo |
| `image_type` | `b` | Backdrop |
| `image_type` | `e` | Episode |

### Settings short values

Settings are stored as short single-character or two-character codes:

| Setting | Values | Meaning |
|---|---|---|
| `image_source` | `t`, `f` | TMDB, Fanart.tv |
| `badge_style` | `d`, `lr`, `rl`, `tb`, `bt` | Default (logo left), logo left, value left, logo top, value top |
| `label_style` | `t`, `i`, `o`, `h` | Text, Icon, Official, High Res |
| `badge_direction` | `d`, `h`, `v` | Default, Horizontal, Vertical (legacy; the UI no longer exposes it) |
| `layout` | JSON | Per-side badge layout (per kind: `poster_layout`, `logo_layout`, `backdrop_layout`, `episode_layout`). Each of the four sides holds `per_row` (badges per row), `rows` (row count), and `start` (anchor — `l`/`c`/`r` for top/bottom, `t`/`c`/`b` for left/right), plus an `order` array of side names (fill order). Badges are laid out in horizontal rows on every side. The total number of ratings shown is the sum of the four side capacities (`per_row × rows`). Defaults preserve old behaviour: poster bottom 3×1 centre, logo bottom 5×1 centre, backdrop top 5×1 right, episode right 1×1 top |
| `text_size` | `50`–`400` | Rating text font size as a percentage of the default (100 = default) |
| `badge_size` | `50`–`400` | Overall badge size as a percentage of the default (100 = default) |
| `badge_width` / `badge_height` | `50`–`400` | Badge frame scale on a single axis as a percentage of the default |
| `logo_size` | `50`–`400` | Rating source logo size as a percentage of the default (100 = default) |
| `badge_shape` | `r`, `p` | Rounded (default), Pill |
| `badge_alpha` | `0`–`100` | Badge background opacity (default 80); the badge background is black at this alpha |

### Example cache keys

```
# TMDB poster, 3 ratings (MAL, IMDb, Letterboxd), default everything, medium image
imdb/tt0111161@mil.slr.lt.dd.shp.ba80.zm

# Same poster at large image size with larger text (145%)
imdb/tt0111161@mil.slr.lt.dd.ts145.zl

# Fanart textless poster
imdb/tt0111161_f_tl@mil.slr.lt.dd.zm

# Logo from TMDB with English language, 3 ratings, icon labels
imdb/tt0111161_l_t_en@mil.slr.li.zm

# Logo from Fanart.tv with English language
imdb/tt0111161_l_f_en@mil.slr.lt.zm

# Backdrop from TMDB, text labels, 150% text size, edge insets, large image
imdb/tt0111161_b_t@mil.slr.lt.dd.ts150.eh8.ev3.zl

# Episode with 1 rating, blur enabled, default text size, medium image
imdb/tt0959621@i.slr.lt.dd.blur.zm
```

### Cross-ID cache

**Not implemented in the Go port.** The Rust original wrote rendered images to the filesystem cache under all resolved alternate IDs (IMDB, TMDB, TVDB) so the same content requested via a different ID type skipped regeneration. The Go implementation caches only under the requested ID; a request via another ID type re-resolves and regenerates. (Listed in the repo notes as an open parity gap.)

### Staleness and background refresh

Rendered-image cache entries are checked for staleness based on the film's release date (`ComputeStaleSecs` in `internal/services/cache.go`):
- **Unreleased / unknown**: uses `RATINGS_STALE_SECS` (default 24h)
- **Recent films**: linearly increasing stale time from `RATINGS_STALE_SECS` to `RATINGS_MAX_AGE_SECS`
- **Old films** (age > `RATINGS_MAX_AGE_SECS`): never stale (ratings are stable)

When a cached entry is stale, the Go implementation **re-fetches and re-renders inline** on the next request (the request blocks until the fresh image is ready). The Rust original spawned a background refresh and coalesced concurrent identical requests — **neither is implemented in the Go port** (open parity gaps).

### CDN caching

**Not implemented in the Go port.** The Rust original redirected authenticated poster requests to content-addressed `/c/{settings_hash}/...` URLs so a CDN could deduplicate cache entries across users with identical settings. The Go implementation serves images directly at the requested URL (the `ENABLE_CDN_REDIRECTS` flag did nothing in Go and has been removed), so a CDN caches per-URL — two users with identical settings but different API keys produce separate edge cache entries.

Image responses carry a fixed `Cache-Control: public, max-age=3600, stale-while-revalidate=86400` header; preview responses use `public, max-age=60`. A CDN in front can still cache and serve these normally (see [Deployment](deployment.md#cloudflare)).

### External cache only

When `EXTERNAL_CACHE_ONLY=true`, the server skips image file writes to disk (rendered posters and base source images from TMDB/Fanart.tv). This is useful when deployed behind a CDN like Cloudflare that caches responses at the edge.

- The in-memory (MemCache) image cache still handles short-term request deduplication
- The cache directory is not created on startup
- Filesystem reads naturally return misses (no files on disk), so every request either hits the in-memory cache or regenerates the image
- Best used behind a CDN (e.g. Cloudflare) so the edge absorbs the vast majority of traffic
- SQLite metadata is **always** written, even with this flag — `image_meta` stores release dates (for CDN TTL computation) and `available_ratings` records which rating sources have data for each movie (so cache keys can be reconstructed without external API calls on cache hits)
- The Docker volume is still required for the SQLite database (`DB_DIR`), even when image caching is fully external

## Clearing the cache

The admin panel can purge cached images without touching the database volume or restarting the container — useful when a poster rendered from a bad source image, when global render settings changed and left orphaned variants behind, or for general cache hygiene.

- **Clear everything** — the **Clear cache** button on the dashboard (and on the **Settings** page) wipes all rendered images, raw downloads, and settings-preview thumbnails on disk, every `image_meta` / `available_ratings` row, and every in-memory image cache (including the settings-preview cache and the upstream TMDB/Fanart.tv image-list and ratings caches). Images regenerate from scratch on the next request, so the first load of each title afterwards is slower. This is the path that guarantees a fully clean re-fetch. The on-disk wipe is **instant regardless of cache size** — the cache directories are atomically renamed aside and the (potentially slow) recursive delete runs in the background — so the request returns immediately even with hundreds of thousands of files, and an interrupted delete is swept on the next startup.
- **Clear one image type** — the **Clear posters / logos / backdrops / episodes** button at the top of each list view removes all cached images of just that kind (its rendered directory + `image_meta` rows), leaving the other kinds and the shared `available_ratings` index untouched. Like clear-all, the on-disk wipe is staged aside and removed in the background.
- **Purge one title, or one variant** — the trash button on a row in the poster/logo/backdrop/episode lists opens a dialog with two choices:
  - **Entire title** removes *every* cached variant of that title for that image kind. One title maps to many cache entries (the key encodes ratings, layout, style, size, language, …), so this prefix-matches the title id rather than deleting a single key.
  - **This variant** removes only the single rendered entry the row represents (one exact cache key), leaving the title's other variants and its shared `available_ratings` index untouched.

Each purge clears the relevant layers consistently: the in-memory render caches, the rendered files on disk, and the SQLite metadata (`image_meta` plus the title's `available_ratings` index, so the next request re-resolves its sources).

A per-title purge is scoped to **rendered output**. A couple of things it deliberately does not reach, because they self-heal:

- **Upstream source caches** (the TMDB/Fanart.tv image lists and aggregated ratings) are keyed by the resolved TMDB id and expire on their own short TTL (~30–60 min). A re-render right after a per-title purge may briefly reuse them, so for an immediate clean re-fetch of changed upstream art/ratings use **Clear cache** instead.
- **Cross-ID copies** — the same title cached under a different id form (e.g. an `imdb` request and a `tmdb` request resolving to the same movie) live under a different `id_type`. A purge targets the id form you pass; the alternate copy regenerates or expires on its own.

The matching API endpoints (behind the admin auth middleware) are `POST /api/admin/cache/purge` (clear everything), `DELETE /api/admin/{posters,logos,backdrops,episodes}` (clear one kind), and `DELETE /api/admin/{posters,logos,backdrops,episodes}/{id_type}/{id_value}` (purge one title — or a single variant with `?scope=variant`, in which case `{id_value}` is the full cache value rather than the bare title id).

Under `EXTERNAL_CACHE_ONLY`, there are no files on disk to remove and the CDN's cached copies cannot be purged from here, so a purge clears only the in-memory caches and SQLite metadata. The admin UI surfaces this as a partial purge.
