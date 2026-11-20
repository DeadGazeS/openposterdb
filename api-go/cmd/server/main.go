package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"openposterdb/internal/app"
	"openposterdb/internal/config"
	"openposterdb/internal/image"
	"openposterdb/internal/router"
	"openposterdb/internal/services"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	app.SetupLogging(cfg.LogLevel)
	slog.Info("JWT_SECRET loaded from environment")
	slog.Info("SECRETS_KEY loaded from environment")
	logConfig(cfg)

	dbPath, dbDir := app.DBPath(cfg.DBDir)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		slog.Error("cannot create the database directory", "dir", dbDir, "error", err)
		os.Exit(1)
	}

	db, err := app.OpenDatabase(dbPath)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := app.RunSchema(db, schemaSQL); err != nil {
		slog.Error("failed to apply schema", "error", err)
		os.Exit(1)
	}

	if err := app.RunMigrations(db, migrations); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if count, err := services.DeleteExpiredRefreshTokens(db); err != nil {
		slog.Warn("failed to clean expired tokens", "error", err)
	} else if count > 0 {
		slog.Info("Cleaned up expired refresh tokens", "count", count)
	}

	if err := services.RunUpgrades(db, cfg.CacheDir, cfg.ExternalCacheOnly); err != nil {
		// Surface at ERROR (#10.5): a failing runOnce upgrade leaves the server
		// running with a broken feature until someone hits it; the previous
		// slog.Warn blended into the noise. Schema-coupled upgrades (e.g. v004
		// badge_size int columns, the 2026-08-14 outage class) are especially
		// load-bearing — operators should see this in the error stream.
		slog.Error("data upgrades failed — feature may be broken until manually repaired", "error", err)
	}

	app.SeedAdminIfNeeded(db, cfg.AdminUsername, cfg.AdminPassword)
	image.ConfigureLimits(cfg.RenderConcurrency, cfg.CrossIDConcurrency)

	// One shared transport for every upstream (TMDB API + image CDN, OMDb,
	// MDBList, Trakt, Fanart, Kitsu, AniList). The default keeps only 2 idle
	// connections per host, so concurrent renders kept re-handshaking TLS.
	// A connection that hangs is cut by the dial / TLS / response-header
	// timeouts, so the retries still get a chance within the 30 s total
	// (which used to be the only bound: one hung connection ate all of it).
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.MaxIdleConns = 256
	transport.MaxIdleConnsPerHost = 64
	transport.MaxConnsPerHost = 128
	transport.IdleConnTimeout = 90 * time.Second
	httpClient := &http.Client{Timeout: 30 * time.Second, Transport: transport}

	mgr := services.NewServiceKeyManager(db, cfg.SecretsKey, cfg.JWTSecret, httpClient,
		cfg.TMDBAPIKey, cfg.MDBListAPIKeys, cfg.OMDBAPIKey, cfg.FanartAPIKey, cfg.TraktClientID, cfg.TVDBAPIKey)
	mgr.Init()

	var tmdbClient *services.TmdbClient
	tmdbKey := cfg.TMDBAPIKey
	if tmdbKey == "" {
		tmdbKey = mgr.TMDBKey()
	}
	if tmdbKey != "" {
		tmdbClient = services.NewTmdbClient(tmdbKey, httpClient)
	} else {
		slog.Warn("TMDB_API_KEY is not set — image endpoints will return 503 until a key is configured in the admin UI")
	}

	// Offline id mapping lists (IMDb↔TMDB↔TVDB and the anime list): loaded
	// from the disk snapshot, then downloaded and refreshed every 24 h in the
	// background. Lookups simply miss until a list is loaded.
	// The lists are tens of MB: a longer total timeout than API calls get.
	idMappings := services.NewIDMappings(&http.Client{Timeout: 5 * time.Minute, Transport: transport}, filepath.Join(cfg.DBDir, "mappings"))
	idMappings.Start(context.Background())

	// IMDb's own ratings dataset (opt-in: IMDb licenses it for
	// non-commercial use only): the first IMDb rating source when loaded.
	if cfg.IMDbRatingsDataset {
		imdbRatings := services.NewIMDbRatings(&http.Client{Timeout: 10 * time.Minute, Transport: transport}, filepath.Join(cfg.DBDir, "mappings", "imdb-ratings.bin"))
		services.SetIMDbRatingsDataset(imdbRatings)
		imdbRatings.Start(context.Background())
	}

	if cfg.PublicMetaDBRatings {
		services.SetPublicMetaDB(services.NewPublicMetaDBClient(httpClient))
	}

	state := &router.AppState{
		Config:          cfg,
		DB:              db,
		HTTPClient:      httpClient,
		TMDB:            tmdbClient,
		Kitsu:           services.NewKitsuClient(httpClient),
		AniList:         services.NewAniListClient(httpClient),
		Mappings:        idMappings,
		ServiceKeys:     mgr,
		SecureCookies:   cfg.SecureCookies,
		JWTSecret:       cfg.JWTSecret,
		SecretsKey:      cfg.SecretsKey,
	}

	// Process-wide in-memory caches (mirroring the Rust moka caches): rendered
	// images capped by IMAGE_MEM_CACHE_MB (1h TTL, 30min idle), ID resolutions
	// (50k entries, 24h TTL), and fetched ratings (50k entries, 72h TTL).
	state.Caches = &services.MemCacheSet{
		ImageMem: services.NewMemCache(
			int64(cfg.ImageMemCacheMB)*1024*1024, 0,
			1*time.Hour, 30*time.Minute,
		),
		// Hard limit; resolutions older than services.idRefreshAfter (1h)
		// are served while a background refresh runs.
		IDs: services.NewMemCache(
			0, 50_000, 24*time.Hour, 0,
		),
		// Hard limit, past the 24 h refresh of older titles (see
		// services.ratingsRefreshFor): due badges are served while a
		// background refresh runs.
		Ratings: services.NewMemCache(
			0, 50_000, 72*time.Hour, 0,
		),
	}
	// Mirror the Rust image_mem_cache: re-check release-date staleness of
	// in-memory entries every 60s so fresh ratings propagate through hot
	// cached images within about a minute.
	state.Caches.ImageMem.SetRevalidateAfter(60 * time.Second)

	state.SetupOMDB(mgr.OMDBKeys())
	state.SetupMDBList(mgr.MDBListKeys())
	state.SetupFanart(mgr.FanartKeys())
	state.SetupTrakt(mgr.TraktClientIDs())
	state.SetupTVDB(mgr.TVDBKey())

	if !cfg.ExternalCacheOnly {
		if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
			slog.Error("cannot create the cache directory", "dir", cfg.CacheDir, "error", err)
			os.Exit(1)
		}
	}
	// Background startup housekeeping: finish deletes of cache dirs staged
	// by a clear that the previous run didn't complete, and drop temp files
	// left by a crash mid-write.
	go func() {
		if err := services.CleanupStagedDirs(cfg.CacheDir); err != nil {
			slog.Warn("cleanup of staged cache dirs failed", "error", err)
		}
		if n := services.SweepTempFiles(cfg.CacheDir); n > 0 {
			slog.Info("removed leftover cache temp files", "count", n)
		}
	}()
	if !cfg.ExternalCacheOnly {
		services.StartCacheSizeCap(context.Background(), db, cfg.CacheDir, cfg.CacheMaxBytes)
	}

	if err := image.LoadFont("assets/fonts/Inter-Bold.ttf"); err != nil {
		slog.Warn("failed to load font, image previews will not render", "error", err)
	} else {
		slog.Info("font loaded")
	}
	image.LoadIcons()

	flusher := services.NewLastUsedFlusher(db, 60*time.Second)
	state.LastUsedFlusher = flusher
	flusher.Start()

	// CDN content-addressed settings hash registry. Entries expire after 5 min
	// (matches the documented settings-hash TTL); a janitor sweeps them up.
	state.CDNHashes = services.NewHashRegistry()
	cdnSweeperDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				state.CDNHashes.SweepExpired()
				// MemCache.Set only trims from the LRU back; drop expired
				// entries elsewhere so they don't sit in memory.
				state.Caches.SweepExpired()
			case <-cdnSweeperDone:
				return
			}
		}
	}()

	// In-flight render dedup: concurrent requests for the same cache key
	// share a single render (mirrors the Rust image_inflight).
	state.Inflight = image.NewInflightSet()

	r := router.New(state)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("server listening", "addr", cfg.ListenAddr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown signal received, draining requests")
	state.Draining.Store(true) // /api/ready → 503

	close(cdnSweeperDone)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	// Detached renders, refreshes and cross-id copies started by requests:
	// let them finish (bounded) so their cache files and image_meta rows
	// aren't cut off half-way.
	bgCtx, bgCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer bgCancel()
	if !image.WaitBackground(bgCtx) {
		slog.Warn("shutdown: background image work still running after the deadline; stopping anyway")
	}
	// After the server stopped accepting and drained requests, so uses
	// recorded by in-flight requests aren't lost: persist last_used and the
	// batched image access times (services.TouchImageAccessCtx).
	flusher.Stop()
	flusher.Flush()
	services.FlushImageAccess(db)

	slog.Info("server stopped")
}

func logConfig(cfg *config.Config) {
	slog.Info("rating providers configured",
		"tmdb", cfg.TMDBAPIKey != "",
		"mdblist", len(cfg.MDBListAPIKeys) > 0,
		"omdb", cfg.OMDBAPIKey != "",
		"fanart", cfg.FanartAPIKey != "",
		"trakt", cfg.TraktClientID != "",
		"tvdb", cfg.TVDBAPIKey != "",
	)
	slog.Info("server configuration",
		"cache_dir", cfg.CacheDir,
		"db_dir", cfg.DBDir,
		"image_quality", cfg.ImageQuality,
		"mem_cache_mb", cfg.ImageMemCacheMB,
		"secure_cookies", cfg.SecureCookies,
		"external_cache_only", cfg.ExternalCacheOnly,
		"enable_cdn_redirects", cfg.EnableCDNRedirects,
		"rate_limit_rpm", cfg.RateLimitRPM,
		"rate_limit_cdn_rpm", cfg.RateLimitCDNRPM,
		"free_key_enabled", cfg.FreeKeyEnabled,
		"log_level", cfg.LogLevel,
	)
}
