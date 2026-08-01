# OpenPosterDB (OPDB)

A self-hosted, drop-in replacement for [RPDB (Rating Poster Database)](https://ratingposterdb.com). Generates movie and TV posters, logos, backdrops, and episode stills with rating overlays — pulling artwork from TMDB (or optionally Fanart.tv) and aggregating ratings from IMDb, Rotten Tomatoes, Metacritic, Trakt, Letterboxd, MyAnimeList, MDBList, and Roger Ebert.

[Website](https://openposterdb.com) · [GitHub](https://github.com/PNRxA/openposterdb) · [Docker Hub](https://hub.docker.com/r/pnrxa/openposterdb) · [Documentation](docs/README.md)

## Self-hosting

```bash
cp api/.env.example .env
# Set TMDB_API_KEY, MDBLIST_API_KEY (or OMDB_API_KEY), and JWT_SECRET in .env
docker compose up -d
```

Then open **http://localhost:3000** and create your admin account.

## API Keys

TMDB is required for artwork. MDBList is recommended (one key covers all 9 rating sources). For Fanart.tv, OMDb, and Trakt, keys can be set via the admin UI without restarting — each supports **comma-separated multi-key pools** with automatic rotation and hashed logging.

## Features

- **Multi-source ratings** — MDBList, OMDb, Trakt, Fanart.tv (optional). Supports IMDb, RT Critics, RT Audience, Metacritic, Trakt, Letterboxd, MAL, MDBList score, and Roger Ebert
- **Posters** — configurable badge position, direction, style, shape, size, background, aspect ratio fit, split badges, textless
- **Logos** — badges horizontally below with configurable style, size, shape, background
- **Backdrops** — vertical badges with configurable position, edge inset, direction
- **Episodes** — horizontal badges with optional blur (spoiler protection), configurable position
- **Per-key settings** — override image source, language, textless, badge settings per API key
- **Multi-key pools** — comma-separated keys with automatic rotation and hashed logging (MDBList, Fanart.tv, OMDb, Trakt)
- **Encrypted key storage** — service keys encrypted at rest in SQLite
- **Multi-layer caching** — in-memory (moka), filesystem, and SQLite with background refresh and request coalescing
- **Admin UI** — Vue 3 panel for dashboard, key management, per-key settings, cache purging (all/kind/single-row)
- **Auth** — Argon2, JWT, rotating refresh tokens, API key access
- **CDN redirects, external-cache-only mode, disable-public-pages** — see [Configuration](docs/configuration.md)

## Connect your media server

Grab an API key from the admin UI:

- **[aiometadata](docs/media-servers.md#aiometadata)** — poster/backdrop/logo/episode URL templates
- **[Jellyfin](docs/media-servers.md#jellyfin)** — dedicated remote image provider plugin
- **[Plex](docs/media-servers.md#plex)** — Custom Metadata Provider (PMS 1.43+)

Any client fetching images by URL works — see the **[API Reference](docs/api.md)**.

## Documentation

- **[Connecting a Media Server](docs/media-servers.md)** — aiometadata, Jellyfin, Plex
- **[Configuration](docs/configuration.md)** — all environment variables
- **[API Reference](docs/api.md)** — endpoints, parameters, image sizes
- **[Deployment](docs/deployment.md)** — reverse proxy and CDN
- **[Architecture](docs/architecture.md)** — caching internals

## Acknowledgments

**Data & image providers**: [TMDB](https://www.themoviedb.org/) (this product uses the TMDB API but is not endorsed or certified by TMDB), [MDBList](https://mdblist.com/), [OMDb](https://www.omdbapi.com/), [Fanart.tv](https://fanart.tv/), [RPDB](https://ratingposterdb.com/) (API design inspiration), [Simple Icons](https://simpleicons.org/).

**Rating sources**: [IMDb](https://www.imdb.com/), [Rotten Tomatoes](https://www.rottentomatoes.com/), [Metacritic](https://www.metacritic.com/), [Trakt](https://trakt.tv/), [Letterboxd](https://letterboxd.com/), [MyAnimeList](https://myanimelist.net/).

## License

[MIT](LICENSE)
