# Deploying to the Public Internet

If you plan to expose OpenPosterDB to external users, put it behind a reverse proxy with TLS and (optionally) a CDN. The sections below cover using Caddy as the reverse proxy and Cloudflare as the CDN.

- [Reverse proxy with Caddy](#reverse-proxy-with-caddy)
- [Deploying behind Cloudflare](#cloudflare)

## Reverse proxy with Caddy

The repository includes a [`Caddyfile.example`](../Caddyfile.example). Copy it, replace the domain, and add a Caddy service to your compose file:

```yaml
services:
  caddy:
    image: caddy:2
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data
    restart: unless-stopped

  openposterdb:
    image: ghcr.io/deadgazes/openposterdb:latest
    environment:
      # refer to docker-compose.yml
    volumes:
      - openposterdb-data:/data
    restart: unless-stopped

volumes:
  caddy-data:
  openposterdb-data:
```

Caddy automatically provisions TLS certificates via Let's Encrypt. No extra configuration is needed — just point your DNS A record to the server's IP.

## Cloudflare

When Cloudflare sits in front of your origin, `EXTERNAL_CACHE_ONLY=true` significantly reduces origin load:

- **`EXTERNAL_CACHE_ONLY`** skips image file writes to disk, relying on Cloudflare's edge cache for long-term storage and the in-memory cache for short-term deduplication. SQLite metadata (release dates, available rating sources) is still written so cache keys can be computed without external API calls. Image responses carry `Cache-Control: public, max-age=3600, stale-while-revalidate=86400`, so Cloudflare caches them at the edge for an hour and revalidates in the background
- Note: the Rust original's `ENABLE_CDN_REDIRECTS` (content-addressed `/c/` redirects) is **not implemented** in the Go port — a CDN caches per-URL, so identical settings under different API keys produce separate edge entries

Use the same Caddy + OpenPosterDB compose setup from the [reverse proxy section](#reverse-proxy-with-caddy), adding the flag and bumping the in-memory cache. The key changes to the `openposterdb` environment:

```yaml
      # Add these to your existing environment block
      EXTERNAL_CACHE_ONLY: "true"
      IMAGE_MEM_CACHE_MB: ${IMAGE_MEM_CACHE_MB:-1024}
```

> `CACHE_DIR` can be omitted because no images are written to disk. The volume is still needed for the SQLite database (`DB_DIR`). `IMAGE_MEM_CACHE_MB` is increased to 1024 because the in-memory cache is the only deduplication layer before Cloudflare — size it to fit your server's available RAM.

### Cloudflare configuration

1. **DNS**: Add an A record pointing to your server's IP with the orange cloud (Proxied) enabled.

2. **SSL/TLS**: Go to **SSL/TLS > Overview** and set the mode to **Full (strict)**. This encrypts traffic between Cloudflare and your origin. Caddy's auto-TLS handles the origin certificate, or you can use a [Cloudflare Origin CA certificate](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/).

3. **Cache Rules**: Cloudflare already caches `.jpg` and `.png` responses by default. The origin sets `Cache-Control: public, max-age=3600, stale-while-revalidate=86400` on image responses, so a cache rule is only needed to make Cloudflare cache them:
   - Go to **Caching > Cache Rules** and create a rule:
     - **When**: URI Path starts with `/{api_key}/` (or match `.jpg`/`.png` under your instance)
     - **Then**: **Eligible for cache**, set **Edge TTL** to **Respect origin**

4. **Tiered Cache**: Go to **Caching > Tiered Cache** and enable **Smart Tiered Caching**. This reduces origin hits by allowing Cloudflare's upper-tier data centers to serve cache hits to lower-tier ones.

5. **Static Assets**: The web UI's static assets (JS, CSS, fonts, images) are built by Vite with content hashes in their filenames (e.g. `assets/index-abc123.js`), making them safe to cache aggressively. Add a second cache rule:
   - **When**: URI Path starts with `/assets/`
   - **Then**: **Eligible for cache**, set **Edge TTL** to 1 year, **Browser TTL** to 1 year
   - These files are immutable — when the app is redeployed, Vite generates new filenames, so stale cache entries are never served.
   - The SPA's `index.html` is served without a file extension on all non-API routes, so Cloudflare will not cache it by default without a rule (see below).

6. **Web Console**: the SPA's `index.html` is served without a file extension on all non-API routes, so Cloudflare will not cache it by default without a rule. If you want the console available from the edge during origin outages, add a cache rule:
   - **When**: Hostname equals the domain **AND** URI Path does not start with `/api/` **AND** URI Path does not start with `/assets/`
   - **Then**: **Eligible for cache**, **Edge TTL** = 1 hour
   - Hashed `/assets/` files are already cached by the rule above.

7. **Browser TTL** (optional): Under **Caching > Configuration**, set **Browser Cache TTL** to **Respect Existing Headers** so the origin's `Cache-Control` headers are passed through to clients.

### How caching works

```
Client → Cloudflare edge
  → /{api_key}/imdb/poster-default/tt1234567.jpg
  → Cache HIT at edge (served from Cloudflare)
     or Cache MISS → Origin validates API key, renders the image → Cloudflare caches it → Response
```

After the first request, each image URL is served directly from Cloudflare's edge — the origin is not hit.
