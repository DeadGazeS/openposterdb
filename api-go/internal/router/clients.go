package router

import (
	"openposterdb/internal/handlers"
	"openposterdb/internal/services"
)

func (s *AppState) SetupTMDB(key string) {
	if key != "" {
		s.TMDB = services.NewTmdbClient(key, s.HTTPClient)
	} else {
		s.TMDB = nil
	}
}

// previewConfig builds the shared config for the admin/key preview handlers.
func (s *AppState) previewConfig() *handlers.PreviewConfig {
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

func (s *AppState) SetupOMDB(keys []string) {
	if len(keys) > 0 {
		s.OMDB = services.NewOmdbClient(keys, s.HTTPClient)
	} else {
		s.OMDB = nil
	}
}

func (s *AppState) SetupMDBList(keys []string) {
	if len(keys) > 0 {
		s.MDBList = services.NewMdblistClient(keys, s.HTTPClient)
	} else {
		s.MDBList = nil
	}
}

func (s *AppState) SetupFanart(keys []string) {
	if len(keys) > 0 {
		s.Fanart = services.NewFanartClient(keys, s.HTTPClient)
	} else {
		s.Fanart = nil
	}
}

func (s *AppState) SetupTrakt(keys []string) {
	if len(keys) > 0 {
		s.Trakt = services.NewTraktClient(keys, s.HTTPClient)
	} else {
		s.Trakt = nil
	}
}

// RefreshClientsFromKeys rebuilds all rating-provider clients from the
// current ServiceKeyManager values (after a settings update).
func (s *AppState) RefreshClientsFromKeys() {
	s.SetupTMDB(s.ServiceKeys.TMDBKey())
	s.SetupOMDB(s.ServiceKeys.OMDBKeys())
	s.SetupMDBList(s.ServiceKeys.MDBListKeys())
	s.SetupFanart(s.ServiceKeys.FanartKeys())
	s.SetupTrakt(s.ServiceKeys.TraktClientIDs())
}

// imageDeps bundles the dependencies shared by the public image and admin
// fetch handlers.
func (s *AppState) imageDeps() handlers.ImageDeps {
	return handlers.ImageDeps{
		DB:      s.DB,
		Config:  s.imageServeConfig(),
		TMDB:    s.TMDB,
		OMDB:    s.OMDB,
		MDBList: s.MDBList,
		Trakt:   s.Trakt,
		Fanart:  s.Fanart,
	}
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
		RatingsMinStaleSecs: s.Config.RatingsMinStaleSecs,
		RatingsMaxAgeSecs:   s.Config.RatingsMaxAgeSecs,
		ImageStaleSecs:      s.Config.ImageStaleSecs,
		ImageQuality:        s.Config.ImageQuality,
		Caches:              s.Caches,
	}
}
