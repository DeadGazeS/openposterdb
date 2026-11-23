# Configuration

OpenPosterDB is configured entirely through environment variables. When using Docker Compose, set them in a `.env` file at the project root (copy `.env.example` to get started); when using `docker run`, pass them with `-e`.

## API keys

The server needs two secrets to start: **`JWT_SECRET`** and **`SECRETS_KEY`** (`make env` generates both). For useful output you also want a **TMDB key** (artwork) and **one ratings source**; MDBList is recommended because a single key covers all nine rating sources. Provider keys can also be entered in the admin UI (Settings → Source API Keys) instead of here.

| Variable | Default | Description |
|---|---|---|
| `TMDB_API_KEY` | — | [TMDB](https://www.themoviedb.org/settings/api) API v3 key. Recommended for artwork; without it, posters/badges/logos/etc. fall back to Fanart.tv or MDBList image sources and some rating fetches are skipped |
| `JWT_SECRET` | *required* | 32-byte hex string (`openssl rand -hex 32`); signs login sessions |
| `SECRETS_KEY` | *required* | 32-byte hex string (`openssl rand -hex 32`); encrypts the stored API keys and provider keys. Keep it stable and back it up — changing it makes the stored keys unreadable (keys then have to be re-entered / regenerated) |
| `MDBLIST_API_KEY` | — | [MDBList](https://mdblist.com/preferences/) key (or a comma-separated list of keys, used in turn) — preferred, covers all 9 rating sources (IMDb, RT Critics, RT Audience, Metacritic, Trakt, Letterboxd, MAL, MDBList score, Roger Ebert) |
| `OMDB_API_KEY` | — | [OMDb](https://www.omdbapi.com/apikey.aspx) key (IMDb, RT Critics, Metacritic only). Also required for IMDB episode ratings |
| `TRAKT_CLIENT_ID` | — | [Trakt](https://trakt.tv/oauth/applications) Client ID — Trakt community ratings for movies, shows, and episodes |
| `FANART_API_KEY` | — | [Fanart.tv](https://fanart.tv/get-an-api-key/) key — enables Fanart.tv as an alternative or preferred image source for posters, logos, and backdrops |
| `FANART_CLIENT_KEY` | — | Optional personal [Fanart.tv](https://fanart.tv/get-an-api-key/) key, sent alongside the project key above so newer images are returned. Env only |
| `TVDB_API_KEY` | — | [TheTVDB](https://thetvdb.com/api-information) v4 key — resolves `tvdb` ids TMDB has no cross-link for (often movies), finds the TVDB id Fanart.tv needs for a series, and is the last artwork fallback |
| `IMDB_RATINGS_DATASET` | `false` | Use [IMDb's ratings dataset](https://developer.imdb.com/non-commercial-datasets/) (`title.ratings.tsv.gz`, ~1.5 M titles, episodes included) as the first IMDb rating source — no API call per title. Downloaded at startup, kept in `DB_DIR/mappings/imdb-ratings.bin` and refreshed daily (titles with fewer than 20 votes are skipped). **IMDb licenses this dataset for personal and non-commercial use only** — enable it only if that applies to your instance |
| `PUBLICMETADB_RATINGS` | `false` | Use [PublicMetaDB](https://publicmetadb.com) (no key needed) to fill Rotten Tomatoes (critics and audience), Metacritic, Letterboxd and Roger Ebert ratings that MDBList / OMDb didn't provide. Lookups send TMDB ids to that service, batched up to 150 titles per request |

ID cross-references (IMDb ↔ TMDB ↔ TVDB, and MAL / Kitsu / AniList ↔ IMDb / TMDB for anime) come from public mapping lists ([dins](https://github.com/0xConstant1/dins-mappings), [Wikidata](https://github.com/0xConstant1/Wikidata-Fetcher), [Fribb's anime-lists](https://github.com/Fribb/anime-lists)). They are downloaded at startup, kept as a snapshot in `DB_DIR/mappings` (so a restart works offline) and refreshed every 24 hours.

## Server & storage

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `0.0.0.0:3000` | Server bind address |
| `CACHE_DIR` | `./cache` | Poster and metadata cache directory |
| `CACHE_MAX_SIZE` | — (unlimited) | Size cap for the disk cache, e.g. `20GB` or `500MB` (binary units). Checked every 10 minutes; when the cache is over the cap, the least recently served images (and the oldest downloaded artwork) are deleted until it is at 90 % of the cap. Renders orphaned by settings changes and downloaded artwork otherwise accumulate forever |
| `DB_DIR` | `./db` | SQLite database directory |
| `IMAGE_QUALITY` | `85` | JPEG output quality (1-100; values outside are clamped) |
| `IMAGE_MEM_CACHE_MB` | `512` | In-memory cache size in MB |
| `RENDER_CONCURRENCY` | CPUs × 2 | Images rendered at the same time. A request that can't get a slot within ~3 s (or finds the wait queue full) gets `503` with `Retry-After`. `0` is rejected |
| `CROSS_ID_CONCURRENCY` | CPUs | Background writers copying a render under the title's other ids (imdb/tmdb/tvdb); copies are skipped while all are busy. `0` is rejected |
| `STATIC_DIR` | — | Directory of the built web UI (SPA) to serve; empty disables static serving |
| `SHUTDOWN_DRAIN_DELAY_SECS` | `0` | On a stop signal, keep serving this many seconds with `/api/ready` answering `503` before closing the listener, so a load balancer or orchestrator polling readiness stops routing here first (a second stop signal ends the wait). The shutdown then drains requests (up to 10 s) and background renders (up to 20 s), so the container's stop grace period must cover the delay plus that. `0` shuts down at once; invalid values fall back to `0` with a warning |

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
| `RATE_LIMIT_RPM` | `300` | Requests-per-minute cap on the public image routes (`/{api_key}/...`, including the free key), counted per API key (its first 16 characters) and client IP, with bursts of up to 240 so a library scan isn't throttled. `/{api_key}/isValid` has its own bucket (10 a second, bursts of 30). `0` disables both. |
| `RATE_LIMIT_CDN_RPM` | `300` | Per-IP requests-per-minute cap on the CDN endpoint (`/c/...`), bursts of up to 240. `0` disables it. |
| `RATE_LIMIT_AUTH_RPM` | `10` | Per-IP requests-per-minute cap on the credential endpoints (`/api/auth/login`, `/api/auth/setup`, `/api/auth/key-login`). Session refresh (`/api/auth/refresh`) has its own per-IP bucket of 60 a minute (burst 20), since the web UI refreshes on every page load. `0` disables both. |
| `TRUSTED_PROXIES` | `private` | Which peers may set `X-Forwarded-For` (used to key the per-IP rate limits). Usually set in the admin UI instead: **Settings → General → Config** (reverse proxy on this host, Cloudflare, other IPs/ranges, plus a readout of how the server sees your IP). When this variable is set it overrides the UI, which then shows it read-only. Syntax: `private` = loopback and private ranges; `none` = always use the connection address; `cloudflare` = Cloudflare's edge ranges; or a comma list of IPs/CIDRs and these keywords. If port 3000 is published directly to the internet, use `none` (with Docker Desktop or the userland proxy, outside clients can appear to come from a private address). Behind Cloudflare and a reverse proxy on the same host, use `private,cloudflare`. The Cloudflare ranges are built in; ticking Cloudflare in the UI checks cloudflare.com for a newer list (at most once a day; retried after 10 minutes if it fails) and applies and stores it automatically. Ranges wider than /8 (IPv4) or /7 (IPv6) are rejected. Note: Cloudflare Workers (anyone's) also connect from Cloudflare's ranges and can set their own `X-Forwarded-For`, so with `cloudflare` trusted a Worker can pick which rate-limit bucket it lands in; this only affects rate limiting, never authentication. |

## Logging

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `info` | Log verbosity: `debug` (shows actions — image requests, cache hits, generation, settings saves), `info` (default), `warn` (only warnings and errors), `error`, `off`. |

*(The original Rust backend used `RUST_LOG`; the Go backend uses `LOG_LEVEL`.)*
