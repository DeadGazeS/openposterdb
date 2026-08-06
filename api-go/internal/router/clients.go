package router

import (
	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

// The setupX helpers mutate the rating-provider client fields; the caller
// is responsible for holding clientsMu (writer lock). SetupX wraps each in
// withClientsLock; RefreshClientsFromKeys holds the lock once for the whole
// batch — readers either see the pre-refresh snapshot or the post-refresh
// snapshot, never a torn mix.

// withClientsLock runs fn while holding the clientsMu writer lock.
func (s *AppState) withClientsLock(fn func()) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	fn()
}

func (s *AppState) setTMDB(key string) {
	if key != "" {
		s.TMDB = services.NewTmdbClient(key, s.HTTPClient)
	} else {
		s.TMDB = nil
	}
}

func (s *AppState) SetupTMDB(key string) {
	s.withClientsLock(func() { s.setTMDB(key) })
}

// previewConfig builds the shared config for the admin/key preview handlers.
// Re-resolved on every request via the routes' closure — see routes.go.
func (s *AppState) previewConfig() *handlers.PreviewConfig {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return &handlers.PreviewConfig{
		CacheDir:          s.Config.CacheDir,
		ExternalCacheOnly: s.Config.ExternalCacheOnly,
		ImageQuality:      s.Config.ImageQuality,
		TMDB:              s.TMDB,
		OMDB:              s.OMDB,
		MDBList:           s.MDBList,
		Trakt:             s.Trakt,
	}
}

func (s *AppState) setOMDB(keys []string) {
	if len(keys) > 0 {
		s.OMDB = services.NewOmdbClient(keys, s.HTTPClient)
	} else {
		s.OMDB = nil
	}
}

func (s *AppState) SetupOMDB(keys []string) {
	s.withClientsLock(func() { s.setOMDB(keys) })
}

func (s *AppState) setMDBList(keys []string) {
	if len(keys) > 0 {
		s.MDBList = services.NewMdblistClient(keys, s.HTTPClient)
	} else {
		s.MDBList = nil
	}
}

func (s *AppState) SetupMDBList(keys []string) {
	s.withClientsLock(func() { s.setMDBList(keys) })
}

func (s *AppState) setFanart(keys []string) {
	if len(keys) > 0 {
		s.Fanart = services.NewFanartClient(keys, s.HTTPClient)
	} else {
		s.Fanart = nil
	}
}

func (s *AppState) SetupFanart(keys []string) {
	s.withClientsLock(func() { s.setFanart(keys) })
}

func (s *AppState) setTrakt(keys []string) {
	if len(keys) > 0 {
		s.Trakt = services.NewTraktClient(keys, s.HTTPClient)
	} else {
		s.Trakt = nil
	}
}

func (s *AppState) SetupTrakt(keys []string) {
	s.withClientsLock(func() { s.setTrakt(keys) })
}

// RefreshClientsFromKeys rebuilds all rating-provider clients from the
// current ServiceKeyManager values (after a settings update). Holds
// clientsMu once for the whole batch so handlers see one consistent
// pre-refresh or post-refresh snapshot — never a torn mix.
func (s *AppState) RefreshClientsFromKeys() {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	s.setTMDB(s.ServiceKeys.TMDBKey())
	s.setOMDB(s.ServiceKeys.OMDBKeys())
	s.setMDBList(s.ServiceKeys.MDBListKeys())
	s.setFanart(s.ServiceKeys.FanartKeys())
	s.setTrakt(s.ServiceKeys.TraktClientIDs())
}

// imageDeps bundles the dependencies shared by the public image and admin
// fetch handlers. Must be re-called per request (passed as a method value
// to HandleImage/HandleCDNImage) so a refresh is visible to in-flight
// handlers — without per-request resolution the handler closure captures
// one snapshot and never sees the new clients.
func (s *AppState) imageDeps() handlers.ImageDeps {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return handlers.ImageDeps{
		DB:        s.DB,
		Config:    s.imageServeConfig(),
		TMDB:      s.TMDB,
		OMDB:      s.OMDB,
		MDBList:   s.MDBList,
		Trakt:     s.Trakt,
		Fanart:    s.Fanart,
		CDNHashes: s.CDNHashes,
	}
}

// hashes returns the settings-hash registry used to map /c/{hash}/... URLs back
// to the effective render settings. May be nil (the CDN redirect path is
// skipped in that case).
func (s *AppState) hashes() handlers.CDNLookup {
	return s.CDNHashes
}

func (s *AppState) isFreeAPIKeyEnabled() bool {
	if s.Config.FreeKeyEnabled != nil {
		return *s.Config.FreeKeyEnabled
	}
	val, _ := services.GetGlobalSetting(s.DB, "free_api_key_enabled")
	return val == "true"
}

func (s *AppState) imageServeConfig() *handlers.ImageServeConfig {
	return &handlers.ImageServeConfig{
		CacheDir:            s.Config.CacheDir,
		ExternalCacheOnly:   s.Config.ExternalCacheOnly,
		EnableCDNRedirects:  s.Config.EnableCDNRedirects,
		RatingsMinStaleSecs: s.Config.RatingsMinStaleSecs,
		RatingsMaxAgeSecs:   s.Config.RatingsMaxAgeSecs,
		ImageStaleSecs:      s.Config.ImageStaleSecs,
		ImageQuality:        s.Config.ImageQuality,
		Caches:              s.Caches,
		Inflight:            s.Inflight,
	}
}
