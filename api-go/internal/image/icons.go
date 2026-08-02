package image

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/webp"

	"openposterdb/internal/services"
)

var (
	iconCache     = make(map[services.RatingSource]*image.RGBA)
	officialCache = make(map[string]*image.RGBA)
	iconCacheMu   sync.RWMutex
	iconsLoaded   = false
)

// iconSourceKeys and officialIconKeys are the whitelisted keys served by
// ServeIcon. They are also the keys LoadIcons loads at startup.
var iconSourceKeys = []string{"imdb", "tmdb", "rt", "rta", "mc", "trakt", "lb", "mal", "mdblist", "ebert"}

var officialIconKeys = []string{
	"imdb", "tmdb", "metacritic", "trakt", "letterboxd", "mal", "mdblist", "ebert",
	"Rotten_Tomatoes_critic_positive", "Rotten_Tomatoes_critic_rotten",
	"Rotten_Tomatoes_critic_certified_fresh",
	"Rotten_Tomatoes_positive_audience", "Rotten_Tomatoes_negative_audience",
	"Rotten_Tomatoes_verified_hot_audience",
}

func iconKeyAllowed(kind, key string) bool {
	switch kind {
	case "white", "default":
		for _, k := range iconSourceKeys {
			if k == key {
				return true
			}
		}
	case "official":
		for _, k := range officialIconKeys {
			if k == key {
				return true
			}
		}
	}
	return false
}

// ServeIcon writes the bytes of a whitelisted icon to w. kind is "white"
// (assets/icons/white) or "official" (assets/icons/official). It prefers the
// .png file and falls back to .webp. Returns false when the icon is unknown.
func ServeIcon(w http.ResponseWriter, kind, key string) bool {
	if !iconKeyAllowed(kind, key) {
		return false
	}
	dir := "assets/icons/white"
	if kind == "official" {
		dir = "assets/icons/official"
	}
	for _, ext := range []string{".png", ".webp"} {
		path := dir + "/" + key + ext
		if _, err := os.Stat(path); err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		if ext == ".webp" {
			w.Header().Set("Content-Type", "image/webp")
		} else {
			w.Header().Set("Content-Type", "image/png")
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(data)
		return true
	}
	return false
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}

// loadIcon loads a single icon file, trying PNG first and falling back to WebP.
func loadIcon(path string) (*image.RGBA, error) {
	if img, err := loadPNG(path); err == nil {
		return img, nil
	}
	if strings.HasSuffix(path, ".png") {
		webpPath := strings.TrimSuffix(path, ".png") + ".webp"
		f, err := os.Open(webpPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, err := webp.Decode(f)
		if err != nil {
			return nil, err
		}
		bounds := img.Bounds()
		rgba := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
		return rgba, nil
	}
	return nil, fmt.Errorf("unsupported icon format")
}

func LoadIcons() {
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()

	if iconsLoaded {
		return
	}

	sources := []struct {
		source services.RatingSource
		key    string
	}{
		{*services.SourceImdb, "imdb"},
		{*services.SourceTmdb, "tmdb"},
		{*services.SourceRt, "rt"},
		{*services.SourceRtAudience, "rta"},
		{*services.SourceMetacritic, "mc"},
		{*services.SourceTrakt, "trakt"},
		{*services.SourceLetterboxd, "lb"},
		{*services.SourceMal, "mal"},
		{*services.SourceMdblist, "mdblist"},
		{*services.SourceEbert, "ebert"},
	}

	for _, s := range sources {
		path := "assets/icons/white/" + s.key + ".png"
		if img, err := loadIcon(path); err == nil {
			iconCache[s.source] = img
		}
	}

	for _, key := range officialIconKeys {
		path := "assets/icons/official/" + key + ".png"
		if img, err := loadIcon(path); err == nil {
			officialCache[key] = img
		}
	}

	iconsLoaded = true
}

func IconForSource(source *services.RatingSource) *image.RGBA {
	iconCacheMu.RLock()
	defer iconCacheMu.RUnlock()
	return iconCache[*source]
}

func parsePercent(value string) uint32 {
	var result float64
	n, _ := fmt.Sscanf(value, "%f%%", &result)
	if n == 1 {
		return uint32(result)
	}
	return 0
}

func OfficialIconForBadge(badge *services.RatingBadge) *image.RGBA {
	iconCacheMu.RLock()
	defer iconCacheMu.RUnlock()

	switch badge.Source {
	case services.SourceImdb:
		return officialCache["imdb"]
	case services.SourceTmdb:
		return officialCache["tmdb"]
	case services.SourceMetacritic:
		return officialCache["metacritic"]
	case services.SourceTrakt:
		return officialCache["trakt"]
	case services.SourceLetterboxd:
		return officialCache["letterboxd"]
	case services.SourceMal:
		return officialCache["mal"]
	case services.SourceMdblist:
		return officialCache["mdblist"]
	case services.SourceEbert:
		return officialCache["ebert"]
	case services.SourceRt:
		score := parsePercent(badge.Value)
		if score >= 75 {
			return officialCache["Rotten_Tomatoes_critic_certified_fresh"]
		} else if score >= 60 {
			return officialCache["Rotten_Tomatoes_critic_positive"]
		}
		return officialCache["Rotten_Tomatoes_critic_rotten"]
	case services.SourceRtAudience:
		score := parsePercent(badge.Value)
		if score >= 75 {
			return officialCache["Rotten_Tomatoes_verified_hot_audience"]
		} else if score >= 60 {
			return officialCache["Rotten_Tomatoes_positive_audience"]
		}
		return officialCache["Rotten_Tomatoes_negative_audience"]
	}
	return nil
}
