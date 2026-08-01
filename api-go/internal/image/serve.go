package image

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	apperr "openposterdb/internal/errors"
	"openposterdb/internal/services"
)

var fontData []byte
var loadedFont *sfnt.Font

const (
	// labelFontFaceSize is the point size for rating source labels.
	labelFontFaceSize = 26.0
	// valueFontFaceSize is the point size for rating values, ~15% larger than the label.
	valueFontFaceSize = 30.0
)

func LoadFont(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fontData = data

	f, err := sfnt.Parse(data)
	if err != nil {
		return err
	}
	loadedFont = f
	return nil
}

// GetFontFace returns a fresh font.Face at the label size. opentype.Face is
// NOT safe for concurrent use — it owns a mutable sfnt.Buffer and rasterizer,
// so a single shared face corrupted its glyph data under parallel requests
// (panic: index out of range in sfnt.LoadGlyph). Each caller therefore gets
// its own face; the underlying *sfnt.Font is immutable and safe to share.
func GetFontFace() font.Face {
	return newFace(labelFontFaceSize)
}

// GetValueFontFace returns a fresh font.Face at the value size (~10% larger
// than the label size). See GetFontFace for the concurrency note.
func GetValueFontFace() font.Face {
	return newFace(valueFontFaceSize)
}

func newFace(size float64) font.Face {
	if loadedFont == nil {
		return nil
	}
	face, err := opentype.NewFace(loadedFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	return face
}

func loadFontFromData(data []byte) (font.Face, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    26,
		DPI:     72,
		Hinting: font.HintingNone,
	})
}

func GenerateImage(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize) ([]byte, error) {
	targetW := uint32(580)
	badgeScale := float32(1.0)

	if imageSize != nil {
		switch kind {
		case "poster":
			targetW = imageSize.PosterTargetWidth()
			badgeScale = imageSize.BadgeScale("poster")
		case "logo":
			targetW = imageSize.LogoTargetWidth()
			badgeScale = imageSize.BadgeScale("logo")
		case "backdrop":
			targetW = imageSize.BackdropTargetWidth()
			badgeScale = imageSize.BadgeScale("backdrop")
		case "episode":
			targetW = imageSize.EpisodeTargetWidth()
			badgeScale = imageSize.BadgeScale("episode")
		}
	}

	badgeSizeScale := services.BadgeSizeMedium.ScaleFactor()
	switch kind {
	case "poster":
		badgeSizeScale = settings.PosterBadgeSize.ScaleFactor()
	case "logo":
		badgeSizeScale = settings.LogoBadgeSize.ScaleFactor()
	case "backdrop":
		badgeSizeScale = settings.BackdropBadgeSize.ScaleFactor()
	case "episode":
		badgeSizeScale = settings.EpisodeBadgeSize.ScaleFactor()
	}
	badgeScale *= badgeSizeScale

	var labelStyle services.LabelStyle
	var badgeStyle services.BadgeStyle
	var badgeDirection services.BadgeDirection
	var appearance services.BadgeAppearance
	var position services.BadgePosition

	switch kind {
	case "poster":
		labelStyle = settings.PosterLabelStyle
		badgeStyle = settings.PosterBadgeStyle
		badgeDirection = settings.PosterBadgeDirection
		appearance = settings.PosterAppearance()
		position = settings.PosterPosition
	case "logo":
		labelStyle = settings.LogoLabelStyle
		badgeStyle = settings.LogoBadgeStyle
		badgeDirection = services.BadgeDirectionHorizontal
		appearance = settings.LogoAppearance()
		position = services.PositionBottomCenter
	case "backdrop":
		labelStyle = settings.BackdropLabelStyle
		badgeStyle = settings.BackdropBadgeStyle
		badgeDirection = settings.BackdropBadgeDirection
		appearance = settings.BackdropAppearance()
		position = settings.BackdropPosition
	case "episode":
		labelStyle = settings.EpisodeLabelStyle
		badgeStyle = settings.EpisodeBadgeStyle
		badgeDirection = settings.EpisodeBadgeDirection
		appearance = settings.EpisodeAppearance()
		position = settings.EpisodePosition
	}

	valueFace := GetValueFontFace()
	labelFace := GetFontFace()
	if valueFace == nil || labelFace == nil {
		return nil, fmt.Errorf("font not loaded")
	}

	switch kind {
	case "poster":
		return RenderPosterSync(imageBytes, badges, valueFace, labelFace, quality,
			position, badgeStyle, labelStyle, appearance, badgeDirection,
			targetW, badgeScale, settings.PosterBadgeSize,
			settings.PosterBadgeSplit, settings.PosterFit)

	case "logo":
		return RenderLogoSync(imageBytes, badges, valueFace, labelFace,
			badgeStyle, labelStyle, appearance, targetW, badgeScale)

	case "backdrop":
		return RenderBackdropSync(imageBytes, badges, valueFace, labelFace, quality,
			position, badgeStyle, labelStyle, appearance, badgeDirection,
			targetW, badgeScale, settings.BackdropBadgeSize,
			settings.BackdropEdgeInsetX, settings.BackdropEdgeInsetY)

	case "episode":
		return RenderEpisodeSync(imageBytes, badges, valueFace, labelFace, quality,
			position, badgeStyle, labelStyle, appearance, badgeDirection,
			targetW, badgeScale, settings.EpisodeBadgeSize, settings.EpisodeBlur)
	}

	return nil, fmt.Errorf("unknown image kind: %s", kind)
}

// --- CDN helpers ---

func SettingsHash(settings *services.RenderSettings, kind string, imageSizeStr *string) string {
	h := sha256.New()
	io.WriteString(h, kind+"\x00")
	suffix := services.SettingsCacheSuffix(settings, kind, imageSizeStr)
	io.WriteString(h, suffix+"\x00")
	io.WriteString(h, services.ExcludeCacheToken(settings.RatingsExclude)+"\x00")
	io.WriteString(h, string(settings.ImageSource)+"\x00")
	io.WriteString(h, settings.Lang+"\x00")
	if settings.Textless {
		io.WriteString(h, "1")
	} else {
		io.WriteString(h, "0")
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:32]
}

func ImageResponse(data []byte, contentType string) (int, map[string]string, []byte) {
	return 200, map[string]string{
		"Content-Type":  contentType,
		"Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
	}, data
}

func TmdbPosterVariant(lang string, textless bool) string {
	if lang == "en" {
		if textless {
			return "_t_tl"
		}
		return ""
	}
	if textless {
		return "_t_" + lang + "_tl"
	}
	return "_t_" + lang
}

func ImageExt(kind string) string {
	switch kind {
	case "logo":
		return "png"
	default:
		return "jpg"
	}
}

func ImageContentType(kind string) string {
	switch kind {
	case "logo":
		return "image/png"
	default:
		return "image/jpeg"
	}
}

func ImageSubdir(kind string) string {
	switch kind {
	case "logo":
		return "logos"
	case "backdrop":
		return "backdrops"
	case "episode":
		return "episodes"
	default:
		return "posters"
	}
}

func KindPrefix(kind string) string {
	switch kind {
	case "logo":
		return "_l"
	case "backdrop":
		return "_b"
	case "episode":
		return "_e"
	default:
		return ""
	}
}

func EnsureCacheDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

func ReadFileCache(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func WriteFileCache(path string, data []byte) error {
	if err := EnsureCacheDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func StripImageExt(idValue, kind string) string {
	switch kind {
	case "logo":
		return strings.TrimSuffix(idValue, ".png")
	default:
		return strings.TrimSuffix(idValue, ".jpg")
	}
}

func ImageDbValue(kind string) string {
	switch kind {
	case "logo":
		return "l"
	case "backdrop":
		return "b"
	case "episode":
		return "e"
	default:
		return "p"
	}
}

func computeCDNMaxAge(releaseDate *string, minStale, maxAge uint64) uint64 {
	return services.ComputeCDNMaxAge(releaseDate, minStale, maxAge)
}

func newFontFace(data []byte) font.Face {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 26, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	return face
}

// ServeImage is the full image-generation pipeline shared by the public image
// endpoint and the admin fetch endpoint. It resolves the ID, fetches ratings,
// downloads the base artwork, renders badges, and caches the result to the
// filesystem and the image_meta table.
//
// Returns (image bytes, content type, error).
func ServeImage(
	db *sql.DB,
	tmdb *services.TmdbClient,
	omdb *services.OmdbClient,
	mdblist *services.MdblistClient,
	trakt *services.TraktClient,
	fanart *services.FanartClient,
	idTypeStr, idValue string,
	kind string,
	settings *services.RenderSettings,
	cacheDir string,
	externalCacheOnly bool,
	ratingsMinStaleSecs uint64,
	ratingsMaxAgeSecs uint64,
	imageStaleSecs uint64,
	quality uint8,
	imageSizeStr *string,
) ([]byte, string, error) {
	idType, err := services.ParseIDType(idTypeStr)
	if err != nil {
		return nil, "", err
	}

	if err := services.ValidateIDValue(idValue); err != nil {
		return nil, "", err
	}

	if tmdb == nil {
		return nil, "", apperr.NewOther("TMDB API key not configured — image generation unavailable")
	}

	contentType := ImageContentType(kind)
	imageTypeChar := ImageDbValue(kind)
	imageSize := services.ImageSizeMedium
	if imageSizeStr != nil {
		imageSize = services.ParseImageSize(*imageSizeStr)
	}

	slog.Debug("image request", "kind", kind, "id", idTypeStr+"/"+idValue)

	// Resolve badge direction/style defaults before cache key construction.
	switch kind {
	case "poster":
		settings.PosterBadgeDirection = settings.PosterBadgeDirection.Resolve(settings.PosterPosition)
		settings.PosterBadgeStyle = settings.PosterBadgeStyle.Resolve(settings.PosterBadgeDirection)
	case "backdrop":
		settings.BackdropBadgeDirection = settings.BackdropBadgeDirection.Resolve(settings.BackdropPosition)
		settings.BackdropBadgeStyle = settings.BackdropBadgeStyle.Resolve(settings.BackdropBadgeDirection)
	case "episode":
		settings.EpisodeBadgeDirection = settings.EpisodeBadgeDirection.Resolve(settings.EpisodePosition)
		settings.EpisodeBadgeStyle = settings.EpisodeBadgeStyle.Resolve(settings.EpisodeBadgeDirection)
	}

	// Resolve the ID, uplifting episodes to their parent series for poster,
	// logo, and backdrop endpoints.
	resolved, err := services.ResolveID(idType, idValue, tmdb)
	if err != nil {
		return nil, "", err
	}

	if kind != "episode" && resolved.MediaType == services.MediaTypeEpisode {
		if resolved.Episode != nil {
			seriesID := services.FormatTMDbIDValue(resolved.Episode.ShowTMDbID, services.MediaTypeTV, nil)
			resolved, err = services.ResolveID(services.IDTypeTMDB, seriesID, tmdb)
			if err != nil {
				return nil, "", err
			}
		}
	}

	if kind == "episode" && resolved.MediaType != services.MediaTypeEpisode {
		return nil, "", apperr.NewBadRequest("not an episode - use poster/logo/backdrop endpoint")
	}

	// Fetch ratings.
	limit := settings.RatingsLimit
	switch kind {
	case "logo":
		limit = settings.LogoRatingsLimit
	case "backdrop":
		limit = settings.BackdropRatingsLimit
	case "episode":
		limit = settings.EpisodeRatingsLimit
	}

	var rawBadges []services.RatingBadge
	if limit > 0 {
		mediaType := "movie"
		switch resolved.MediaType {
		case services.MediaTypeTV:
			mediaType = "tv"
		case services.MediaTypeEpisode:
			mediaType = "episode"
		}

		var imdbID *string
		if resolved.IMDbID != nil && *resolved.IMDbID != "" {
			imdbID = resolved.IMDbID
		}

		var epShowID uint64
		var epSeason, epEpisode uint32
		if resolved.Episode != nil {
			epShowID = resolved.Episode.ShowTMDbID
			epSeason = resolved.Episode.SeasonNumber
			epEpisode = resolved.Episode.EpisodeNumber
		}

		_, _, _, _, rawBadges = services.FetchRatings(
			resolved.TMDbID, mediaType, imdbID,
			epShowID, epSeason, epEpisode,
			tmdb, omdb, mdblist, trakt,
		)
		slog.Debug("ratings fetched",
			"kind", kind,
			"id", idTypeStr+"/"+idValue,
			"badges", len(rawBadges),
			"sources", badgeSourceString(rawBadges),
		)
	} else {
		slog.Debug("ratings skipped", "kind", kind, "id", idTypeStr+"/"+idValue, "reason", "ratings_limit=0")
	}

	badges := services.ApplyRatingPreferences(rawBadges, settings.RatingsOrder, settings.RatingsExclude, limit)

	// Build the cache value (id value + variant + settings/ratings suffix).
	ratingsSuffix := services.BadgesCacheSuffix(badges)
	suffix := services.SettingsCacheSuffixWithRatings(settings, kind, imageSizeStr, ratingsSuffix)

	fanartPrimary := settings.ImageSource.IsFanart() && fanart != nil
	variant := cacheVariant(kind, settings, fanartPrimary)

	cacheValue := idValue + variant + suffix
	cacheKey := idTypeStr + "/" + cacheValue
	cachePath, err := services.TypedCachePath(cacheDir, ImageSubdir(kind), idTypeStr, cacheValue, ImageExt(kind))
	if err != nil {
		return nil, "", err
	}

	// Serve from filesystem cache if present and fresh.
	releaseDate := resolved.ReleaseDate
	staleSecs := services.ComputeStaleSecs(derefStr(releaseDate), ratingsMinStaleSecs, ratingsMaxAgeSecs)
	if !externalCacheOnly {
		if entry, err := services.ReadCache(cachePath, staleSecs); err == nil && !entry.IsStale {
			slog.Debug("image cache hit", "kind", kind, "cache_key", cacheKey)
			return entry.Bytes, contentType, nil
		}
	}

	// Fetch the base artwork (TMDB primary, fanart optional).
	var imageBytes []byte
	if fanartPrimary {
		imageBytes = fetchFanartArtwork(fanart, tmdb, resolved, kind, settings, cacheDir, externalCacheOnly)
	}
	if imageBytes == nil {
		imageBytes, err = fetchTmdbArtwork(tmdb, cacheDir, externalCacheOnly, imageStaleSecs, resolved, kind, settings, imageSize)
		if err != nil {
			return nil, "", err
		}
	}
	if imageBytes == nil {
		slog.Warn("no artwork available", "kind", kind, "id", idTypeStr+"/"+idValue)
		return nil, "", apperr.NewIDNotFound("no " + kind + " artwork available for this title")
	}

	// Render badges onto the artwork.
	rendered, err := GenerateImage(imageBytes, badges, settings, kind, quality, &imageSize)
	if err != nil {
		return nil, "", apperr.NewImageError(err)
	}

	// Persist: filesystem + metadata DB.
	if !externalCacheOnly {
		if err := services.WriteCache(cachePath, rendered); err != nil {
			slog.Warn("failed to write image cache", "cache_key", cacheKey, "error", err)
		}
	}
	if err := services.UpsertImageMeta(db, cacheKey, releaseDate, imageTypeChar); err != nil {
		slog.Warn("failed to upsert image meta", "cache_key", cacheKey, "error", err)
	}

	slog.Debug("image generated", "kind", kind, "cache_key", cacheKey, "badges", len(badges))
	return rendered, contentType, nil
}

// cacheVariant builds the language/source variant token that distinguishes the
// artwork source (and language) within a cache key. Empty for the poster
// default case so existing default cache keys stay stable.
func cacheVariant(kind string, settings *services.RenderSettings, fanartPrimary bool) string {
	switch kind {
	case "poster":
		if fanartPrimary {
			if settings.Textless {
				return "_f_tl"
			}
			return "_f_" + settings.Lang
		}
		return TmdbPosterVariant(settings.Lang, settings.Textless)
	case "logo":
		if fanartPrimary {
			return "_l_f_" + settings.Lang
		}
		return "_l_t_" + settings.Lang
	case "backdrop":
		if fanartPrimary {
			return "_b_f"
		}
		return "_b_t"
	case "episode":
		return ""
	}
	return ""
}

// fetchTmdbArtwork downloads the base artwork for the given kind from TMDB,
// preferring the on-disk base cache. Returns nil bytes when no artwork exists.
func fetchTmdbArtwork(tmdb *services.TmdbClient, cacheDir string, externalCacheOnly bool, imageStaleSecs uint64, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, imageSize services.ImageSize) ([]byte, error) {
	tmdbSize := imageSize.TmdbSize()

	var filePath string
	if kind == "episode" {
		if resolved.Episode != nil && resolved.Episode.StillPath != nil {
			filePath = *resolved.Episode.StillPath
		} else if resolved.PosterPath != nil {
			filePath = *resolved.PosterPath
		}
	} else if kind == "poster" && (settings.Lang == "en" && !settings.Textless) {
		if resolved.PosterPath != nil {
			filePath = *resolved.PosterPath
		}
	} else {
		mediaType := "tv"
		if resolved.MediaType == services.MediaTypeMovie {
			mediaType = "movie"
		}
		lang := settings.Lang
		if kind == "backdrop" {
			lang = ""
		}
		textless := settings.Textless && kind == "poster"
		images, err := tmdb.GetImages(mediaType, resolved.TMDbID, lang)
		if err != nil {
			return nil, err
		}
		var candidates []services.TmdbImage
		switch kind {
		case "poster":
			candidates = images.Posters
		case "logo":
			candidates = images.Logos
		case "backdrop":
			candidates = images.Backdrops
		}
		var selected *services.TmdbImage
		if kind == "poster" {
			selected = services.SelectPoster(candidates, lang, textless)
		} else {
			selected = services.SelectImage(candidates, lang, textless)
		}
		if selected != nil {
			filePath = selected.FilePath
		}
	}

	if filePath == "" {
		return nil, nil
	}

	if externalCacheOnly {
		return tmdb.FetchPosterBytes(filePath, tmdbSize)
	}

	basePath, err := services.BasePosterPath(cacheDir, filePath, tmdbSize)
	if err != nil {
		return nil, err
	}
	if entry, err := services.ReadCache(basePath, imageStaleSecs); err == nil {
		return entry.Bytes, nil
	}
	bytes, err := tmdb.FetchPosterBytes(filePath, tmdbSize)
	if err != nil {
		return nil, err
	}
	if err := services.WriteCache(basePath, bytes); err != nil {
		slog.Warn("failed to write base image cache", "path", basePath, "error", err)
	}
	return bytes, nil
}

// fetchFanartArtwork downloads the base artwork from fanart.tv when the
// user's image source is fanart. Returns nil bytes when unavailable so the
// caller falls through to TMDB.
func fetchFanartArtwork(fanart *services.FanartClient, tmdb *services.TmdbClient, resolved *services.ResolvedID, kind string, settings *services.RenderSettings, cacheDir string, externalCacheOnly bool) []byte {
	if kind == "episode" {
		return nil
	}

	var images *services.FanartImages
	var err error
	switch resolved.MediaType {
	case services.MediaTypeMovie:
		images, err = fanart.GetMovieImages(resolved.TMDbID)
	default:
		tvID := resolved.TMDbID
		if resolved.TVDBID != nil && *resolved.TVDBID != 0 {
			tvID = *resolved.TVDBID
		}
		images, err = fanart.GetTVImages(tvID)
	}
	if err != nil || images == nil {
		return nil
	}

	var candidates []services.FanartPoster
	switch kind {
	case "poster":
		candidates = images.Posters
	case "logo":
		candidates = images.Logos
	case "backdrop":
		candidates = images.Backdrops
	}
	lang := settings.Lang
	if kind == "backdrop" {
		lang = ""
	}
	selected, _, ok := services.SelectFanartImage(candidates, lang, settings.Textless && kind == "poster")
	if !ok {
		return nil
	}

	if !externalCacheOnly {
		if basePath, err := services.BaseFanartPath(cacheDir, selected.ID, ImageExt(kind)); err == nil {
			if entry, err := services.ReadCache(basePath, 0); err == nil {
				return entry.Bytes
			}
			if bytes, err := fanart.FetchPosterBytes(selected.URL); err == nil {
				_ = services.WriteCache(basePath, bytes)
				return bytes
			}
			return nil
		}
	}
	bytes, err := fanart.FetchPosterBytes(selected.URL)
	if err != nil {
		return nil
	}
	return bytes
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func badgeSourceString(badges []services.RatingBadge) string {
	keys := make([]string, len(badges))
	for i, b := range badges {
		keys[i] = b.Source.Key
	}
	return strings.Join(keys, ",")
}

func RenderImageWithFont(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize, fontData []byte) ([]byte, error) {
	return GenerateImage(imageBytes, badges, settings, kind, quality, imageSize)
}
