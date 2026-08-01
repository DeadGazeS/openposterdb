package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"openposterdb/internal/config"
	"openposterdb/internal/handlers"
	"openposterdb/internal/router"
	"openposterdb/internal/services"
)

var jwtSecret []byte
var secureCookies bool

func init() {
	jwtSecret = loadJWTSecret()
	secureCookies = loadSecureCookies()
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.FromEnv()
	logConfig(cfg)

	dbPath, dbDir := getDBPaths()
	os.MkdirAll(dbDir, 0755)

	db, err := setupDatabase(dbPath)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := runSchema(db); err != nil {
		slog.Error("failed to apply schema", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if count, err := services.DeleteExpiredRefreshTokens(db); err != nil {
		slog.Warn("failed to clean expired tokens", "error", err)
	} else if count > 0 {
		slog.Info("Cleaned up expired refresh tokens", "count", count)
	}

	if err := runUpgrades(db, cfg); err != nil {
		slog.Warn("data upgrades failed", "error", err)
	}

	seedAdminIfNeeded(db, cfg)

	httpClient := buildHTTPClient()

	tmdbClient := services.NewTmdbClient(cfg.TMDBAPIKey, httpClient)

	mgr := services.NewServiceKeyManager(db, jwtSecret, httpClient,
		cfg.MDBListAPIKeys, cfg.OMDBAPIKey, cfg.FanartAPIKey, cfg.TraktClientID)
	mgr.Init()

	state := &router.AppState{
		Config:        cfg,
		DB:            db,
		HTTPClient:    httpClient,
		TMDB:          tmdbClient,
		ServiceKeys:   mgr,
		SecureCookies: secureCookies,
	}

	state.SetupOMDB(mgr.OMDBKeys())
	state.SetupMDBList(mgr.MDBListKeys())
	state.SetupFanart(mgr.FanartKeys())
	state.SetupTrakt(mgr.TraktClientIDs())

	if cfg.ExternalCacheOnly && !cfg.EnableCDNRedirects {
		slog.Warn("EXTERNAL_CACHE_ONLY is enabled without ENABLE_CDN_REDIRECTS — " +
			"every request after the in-memory cache expires will regenerate the image. " +
			"Consider enabling CDN redirects so a CDN can absorb repeat traffic.")
	}

	if !cfg.ExternalCacheOnly {
		os.MkdirAll(cfg.CacheDir, 0755)
	}
	os.MkdirAll(cfg.DBDir, 0755)

	pendingLastUsed := &sync.Map{}
	go startFlushWorker(db, pendingLastUsed, 60*time.Second)

	r := router.New(state)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: r,
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

	slog.Info("shutdown signal received, flushing pending last_used updates")

	flushPendingKeys(db, pendingLastUsed)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}

func loadJWTSecret() []byte {
	hexStr := os.Getenv("JWT_SECRET")
	if hexStr == "" {
		slog.Error("JWT_SECRET is not set. This is required.\n" +
			"Generate one with: openssl rand -hex 32\n" +
			"Then add it to your .env file.")
		os.Exit(1)
	}
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		slog.Error("JWT_SECRET is not valid hex", "error", err)
		os.Exit(1)
	}
	if len(bytes) != 32 {
		slog.Error("JWT_SECRET must be 32 bytes (64 hex chars)", "got", len(bytes))
		os.Exit(1)
	}
	slog.Info("JWT_SECRET loaded from environment")
	return bytes
}

func loadSecureCookies() bool {
	val := os.Getenv("COOKIE_SECURE")
	if val == "" {
		return true
	}
	return val != "false" && val != "0"
}

func getDBPaths() (string, string) {
	dbDir := os.Getenv("DB_DIR")
	if dbDir == "" {
		dbDir = "./db"
	}
	abs, _ := filepath.Abs(dbDir)
	return filepath.Join(abs, "openposterdb.db"), abs
}

func setupDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(4)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-8000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		db.Exec(p)
	}
	return db, nil
}

func runSchema(db *sql.DB) error {
	for _, query := range schemaSQL {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

func runMigrations(db *sql.DB) error {
	for _, m := range migrations {
		_, err := db.Exec(m.SQL)
		if err != nil {
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, m.ExpectedError) {
				continue
			}
			return err
		}
	}
	return nil
}

func runUpgrades(db *sql.DB, cfg *config.Config) error {
	return services.RunUpgrades(db, cfg.CacheDir, cfg.ExternalCacheOnly)
}

func seedAdminIfNeeded(db *sql.DB, cfg *config.Config) {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		return
	}
	count, err := services.CountAdminUsers(db)
	if err != nil {
		slog.Error("Failed to check admin users", "error", err)
		return
	}
	if count > 0 {
		slog.Debug("Admin user already exists, skipping seed")
		return
	}
	hash, err := handlers.HashPassword(password)
	if err != nil {
		slog.Error("Failed to hash admin password", "error", err)
		return
	}
	if _, err := services.CreateAdminUser(db, username, hash); err != nil {
		slog.Error("Failed to seed admin user", "error", err)
		return
	}
	slog.Info("Seeded admin user from environment", "username", username)
}

func buildHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

func logConfig(cfg *config.Config) {
	slog.Info("rating providers configured",
		"mdblist", !isEmptySlice(cfg.MDBListAPIKeys),
		"omdb", cfg.OMDBAPIKey != "",
		"fanart", cfg.FanartAPIKey != "",
		"trakt", cfg.TraktClientID != "",
	)
	slog.Info("server configuration",
		"cache_dir", cfg.CacheDir,
		"db_dir", cfg.DBDir,
		"image_quality", cfg.ImageQuality,
		"mem_cache_mb", cfg.ImageMemCacheMB,
		"secure_cookies", secureCookies,
		"cdn_redirects", cfg.EnableCDNRedirects,
		"external_cache_only", cfg.ExternalCacheOnly,
		"free_key_enabled", cfg.FreeKeyEnabled,
	)
}

func isEmptySlice(s []string) bool {
	return len(s) == 0
}

func startFlushWorker(db *sql.DB, pending *sync.Map, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			var ids []int64
			pending.Range(func(key, value any) bool {
				id, ok := key.(int64)
				if ok {
					ids = append(ids, id)
				}
				pending.Delete(key)
				return true
			})
			if len(ids) > 0 {
				if err := services.BatchUpdateLastUsed(db, ids); err != nil {
					slog.Warn("failed to batch update last_used_at", "error", err)
				}
			}
		}
	}()
}

func flushPendingKeys(db *sql.DB, pending *sync.Map) {
	var ids []int64
	pending.Range(func(key, value any) bool {
		id, ok := key.(int64)
		if ok {
			ids = append(ids, id)
		}
		pending.Delete(key)
		return true
	})
	if len(ids) > 0 {
		if err := services.BatchUpdateLastUsed(db, ids); err != nil {
			slog.Warn("failed to flush last_used_at on shutdown", "error", err)
		}
	}
}
