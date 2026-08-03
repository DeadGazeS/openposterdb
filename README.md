# OpenPosterDB

OpenPosterDB is a self-hosted service that generates custom movie and TV posters, logos, backdrops, and episode stills with rating overlays. It's designed as a drop-in replacement for RPDB (Rating Poster Database) and plugs directly into your media server.

Pull artwork from TMDB (or optionally Fanart.tv) and combine it with ratings from IMDb, Rotten Tomatoes, Metacritic, Trakt, Letterboxd, MyAnimeList, MDBList, and Roger Ebert — all rendered into clean, branded image overlays you control.

- [Website](https://openposterdb.com)
- [GitHub](https://github.com/PNRxA/openposterdb)
- [Docker Hub](https://hub.docker.com/r/pnrxa/openposterdb)
- [Documentation](docs/README.md)

---

## Highlights

- **Everything self-hosted** — your keys, your data, no third-party dependency for image serving.
- **Drop-in RPDB compatibility** — same API shape, so existing Plex/Jellyfin/aiometadata setups can switch over.
- **Fully customisable overlays** — per-source badge colours, badge style, shape, size, and opacity.
- **Works with the tools you already use** — Plex, Jellyfin, and aiometadata.

## Features

### Image generation
- **Posters, logos, backdrops, and episode stills** — generated on demand with rating overlays.
- **Per-source badge colours** — logo background, text background, border, and text colour for every rating source. Rotten Tomatoes gets separate settings for each logo variant (Certified Fresh, Fresh, Rotten, Verified Hot, and more).
- **Anti-aliased rendering** — smooth rounded and pill badge shapes with crisp border rings.
- **Flexible layouts** — badge style (logo/number arrangement: left-right, right-left, top-bottom, or bottom-top), shape, size, and aspect-ratio fitting are all configurable, plus a per-kind **badge layout grid**: set how many badges (per row × rows) sit on each of the four sides (top/right/bottom/left), each anchored to a start position, with a configurable fill order. The total number of ratings shown equals the sum of the four sides.
- **Independent sizing up to 400%** — text, badge, and logo size sliders per image type (50–400%).
- **High-resolution label style** — a `High Res` label option renders each rating source's crisp, source-SVG logo (instead of the 48px official PNGs).
- **Aspect-ratio fitting** — poster fit modes (`native`/`cover`/`pad`/`blur`) keep non-2:3 posters from being cropped.
- **Episode blur** — optional blur for spoiler protection.
- **Background opacity** — independent opacity slider for posters, logos, backdrops, and episodes.

### Ratings
- Aggregates ratings from **MDBList, OMDb, Trakt, and TMDB** across IMDb, Rotten Tomatoes (critics and audience), Metacritic, Trakt, Letterboxd, MyAnimeList, MDBList, and Roger Ebert.
- Configurable **rating order and per-source exclusion** — reorder with the arrows, hide a source with the eye toggle. The number of ratings shown is driven by the layout grid.
- Ratings are cached so repeated requests are fast and don't hammer the providers.

### Configuration & keys
- **Per-key settings** — override image source, language, textless, and badge settings for each API key.
- **Multi-key pools** — comma-separated keys with automatic rotation and 429 back-off for MDBList, Fanart.tv, OMDb, and Trakt.
- **Encrypted at rest** — service keys are encrypted in the SQLite database.
- **Settings backup & restore** — export and import your full configuration, optionally including external service keys and API keys, from the admin panel.

### Operations
- **Multi-layer caching** — filesystem and SQLite with staleness-aware refresh.
- **Full admin UI** — a Vue 3 dashboard covering stats, API key management, per-key settings, and cache purging (all / per-kind / per-title).
- **Secure auth** — Argon2 password hashing, JWT access tokens, rotating refresh tokens, and API-key auth.

## Self-hosting

```bash
cp api-go/.env.example .env
# Set a JWT_SECRET (openssl rand -hex 32). Add TMDB_API_KEY for artwork and
# MDBLIST_API_KEY (or OMDB_API_KEY) for ratings — or add them later in the admin UI.
docker compose up -d
```

Then open **http://localhost:3000** and create your admin account.

TMDB is required for artwork — it can be set in `.env` or later via the admin UI (Settings → External API Keys) without restarting. MDBList is recommended (a single key covers all nine rating sources). Fanart.tv, OMDb, and Trakt keys can also be added through the admin UI without restarting.

## Connect your media server

Grab an API key from the admin UI and point your media server at it:

- **[aiometadata](docs/media-servers.md#aiometadata)** — poster/backdrop/logo/episode URL templates
- **[Jellyfin](docs/media-servers.md#jellyfin)** — dedicated remote image provider plugin
- **[Plex](docs/media-servers.md#plex)** — Custom Metadata Provider (PMS 1.43+)

Any client that fetches images by URL works — see the [API Reference](docs/api.md).

## Documentation

- [Connecting a Media Server](docs/media-servers.md) — aiometadata, Jellyfin, Plex
- [Configuration](docs/configuration.md) — all environment variables
- [API Reference](docs/api.md) — endpoints, parameters, image sizes
- [Deployment](docs/deployment.md) — reverse proxy and CDN
- [Architecture](docs/architecture.md) — caching internals

## Acknowledgments

**Data & image providers**: [TMDB](https://www.themoviedb.org/) (this product uses the TMDB API but is not endorsed or certified by TMDB), [MDBList](https://mdblist.com/), [OMDb](https://www.omdbapi.com/), [Fanart.tv](https://fanart.tv/), [RPDB](https://ratingposterdb.com/) (API design inspiration), [Simple Icons](https://simpleicons.org/).

**Rating sources**: [IMDb](https://www.imdb.com/), [Rotten Tomatoes](https://www.rottentomatoes.com/), [Metacritic](https://www.metacritic.com/), [Trakt](https://trakt.tv/), [Letterboxd](https://letterboxd.com/), [MyAnimeList](https://myanimelist.net/).

## License

[MIT](LICENSE)
