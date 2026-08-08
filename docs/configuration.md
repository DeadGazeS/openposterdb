# Configuration

OpenPosterDB is configured entirely through environment variables. When using Docker Compose, set them in a `.env` file at the project root (copy `.env.example` to get started); when using `docker run`, pass them with `-e`.

## API keys

At minimum you need a **TMDB key** (for artwork), a **JWT secret** (for auth), and **one ratings source**. MDBList is recommended because a single key covers all nine rating sources.

| Variable | Default | Description |
|---|---|---|
| `TMDB_API_KEY` | — | [TMDB](https://www.themoviedb.org/settings/api) API v3 key. Recommended for artwork; without it, posters/badges/logos/etc. fall back to Fanart.tv or MDBList image sources and some rating fetches are skipped |
| `JWT_SECRET` | *required* | 32-byte hex string (`openssl rand -hex 32`) |
| `MDBLIST_API_KEY` | — | [MDBList](https://mdblist.com/preferences/) key — preferred, covers all 9 rating sources (IMDb, RT Critics, RT Audience, Metacritic, Trakt, Letterboxd, MAL, MDBList score, Roger Ebert) |
| `OMDB_API_KEY` | — | [OMDb](https://www.omdbapi.com/apikey.aspx) key (IMDb, RT Critics, Metacritic only). Also required for IMDB episode ratings |
| `TRAKT_CLIENT_ID` | — | [Trakt](https://trakt.tv/oauth/applications) Client ID — Trakt community ratings for movies, shows, and episodes |
| `FANART_API_KEY` | — | [Fanart.tv](https://fanart.tv/get-an-api-key/) key — enables Fanart.tv as an alternative or preferred image source for posters, logos, and backdrops |

## Server & storage

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `0.0.0.0:3000` | Server bind address |
| `CACHE_DIR` | `./cache` | Poster and metadata cache directory |
| `DB_DIR` | `./db` | SQLite database directory |
| `IMAGE_QUALITY` | `85` | JPEG output quality (1-100) |
| `IMAGE_MEM_CACHE_MB` | `512` | In-memory cache size in MB |
| `STATIC_DIR` | — | Directory of the built web UI (SPA) to serve; empty disables static serving |

## Auth & access

| Variable | Default | Description |
|---|---|---|
| `COOKIE_SECURE` | `true` | HTTPS-only cookies. Set to `false` if you access the admin UI over plain HTTP (no TLS) — otherwise the browser drops auth cookies and login appears broken |
| `CORS_ORIGIN` | — | Allowed origin for admin requests |
| `ADMIN_USERNAME` | — | Seed admin username on first run |
| `ADMIN_PASSWORD` | — | Seed admin password on first run |
| `FREE_KEY_ENABLED` | — | Force-enable (`true`) or force-disable (`false`) the free API key, overriding the admin UI toggle. When set, the UI toggle is locked. Omit to let admins control it from the settings page |
| `DISABLE_PUBLIC_PAGES` | `false` | Hide the landing page, docs, legal, and all unauthenticated routes, redirecting visitors to the login page. All pages remain accessible to authenticated users. Useful for private instances |

## Caching & refresh

| Variable | Default | Description |
|---|---|---|
| `RATINGS_STALE_SECS` | `86400` | Min ratings cache lifetime |
| `RATINGS_MAX_AGE_SECS` | `31536000` | Film age after which ratings stop refreshing |
| `IMAGE_STALE_SECS` | `0` | Base image cache lifetime (0 = never re-fetch) |
| `EXTERNAL_CACHE_ONLY` | `false` | Skip image file writes to disk; rely on a CDN for caching. SQLite metadata is still written (see [Architecture](architecture.md#external-cache-only)) |
| `ENABLE_CDN_REDIRECTS` | `false` | When `true`, the public image endpoint responds with a 302 to `/c/{settings_hash}/...` so a CDN can dedupe cache entries across all users with identical settings (only the hash matters for the cache key, not the API key). Skipped for the free key (settings are public and would leak). |

## Rate limiting

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_RPM` | `60` | Per-key requests-per-minute cap on the admin routes (all `/api/admin/*` + per-key `/api/key/me/*`). |
| `RATE_LIMIT_CDN_RPM` | `240` | Per-key requests-per-minute cap on the CDN redirect endpoint (`/c/...`). Higher default than the admin cap because the CDN edge retries on transient failures. |

## Logging

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `info` | Log verbosity: `debug` (shows actions — image requests, cache hits, generation, settings saves), `info` (default), `warn` (only warnings and errors), `error`, `off`. |

*(The original Rust backend used `RUST_LOG`; the Go backend uses `LOG_LEVEL`.)*
