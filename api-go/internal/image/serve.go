package image

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"

	"openposterdb/internal/services"
)

func GenerateImage(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize, fontData []byte) ([]byte, error) {
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

	f := newFontFace(fontData)
	if f == nil {
		return nil, fmt.Errorf("failed to load font")
	}
	labelFace := f
	valueFace := f

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

func newFontFace(fontData []byte) font.Face {
	return nil
}

// Helper: trim extension from id value

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

func CdnRedirectResponse(location string) (int, map[string]string) {
	return http.StatusFound, map[string]string{
		"Location":      location,
		"Cache-Control": "public, max-age=300, stale-while-revalidate=3600",
	}
}

func ImageResponse(data []byte, contentType string) (int, map[string]string, []byte) {
	return http.StatusOK, map[string]string{
		"Content-Type":  contentType,
		"Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
	}, data
}

func CdnImageResponse(data []byte, maxAge uint64, contentType string) (int, map[string]string, []byte) {
	swr := maxAge * 7
	return http.StatusOK, map[string]string{
		"Content-Type":  contentType,
		"Cache-Control": fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d", maxAge, swr),
	}, data
}

// --- TMDB poster variant ---

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

// --- File extension helpers ---

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

// --- Filesystem cache helpers ---

func EnsureCacheDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
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

func CacheFilePath(cacheDir, imageType, idType, idValue, ext string) (string, error) {
	return services.TypedCachePath(cacheDir, imageType, idType, idValue, ext)
}

// --- Image serving ---

func ServeImage(
	db interface{},
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
	imageStaleSecs uint64,
	ratingsMinStaleSecs uint64,
	ratingsMaxAgeSecs uint64,
	quality uint8,
	imageSizeStr *string,
	fontData []byte,
) ([]byte, string, error) {

	idType, err := services.ParseIDType(idTypeStr)
	if err != nil {
		return nil, "", err
	}

	if err := services.ValidateIDValue(idValue); err != nil {
		return nil, "", err
	}

	resolved, err := services.ResolveID(idType, idValue, tmdb)
	if err != nil {
		return nil, "", err
	}

	if resolved.MediaType == services.MediaTypeEpisode && kind != "episode" {
		if resolved.Episode != nil {
			seriesID := services.FormatTMDbIDValue(resolved.Episode.ShowTMDbID, services.MediaTypeTV, nil)
			resolved, err = services.ResolveID(services.IDTypeTMDB, seriesID, tmdb)
			if err != nil {
				return nil, "", err
			}
		}
	}

	if kind == "episode" && resolved.MediaType != services.MediaTypeEpisode {
		return nil, "", fmt.Errorf("not an episode")
	}

	limit := settings.RatingsLimit
	switch kind {
	case "logo":
		limit = settings.LogoRatingsLimit
	case "backdrop":
		limit = settings.BackdropRatingsLimit
	case "episode":
		limit = settings.EpisodeRatingsLimit
	}

	var imdbID *string
	if resolved.IMDbID != nil && *resolved.IMDbID != "" {
		imdbID = resolved.IMDbID
	}

	mediaType := "movie"
	switch resolved.MediaType {
	case services.MediaTypeTV:
		mediaType = "tv"
	case services.MediaTypeEpisode:
		mediaType = "episode"
	}

	var epShowID uint64
	var epSeason, epEpisode uint32
	if resolved.Episode != nil {
		epShowID = resolved.Episode.ShowTMDbID
		epSeason = resolved.Episode.SeasonNumber
		epEpisode = resolved.Episode.EpisodeNumber
	}

	_, _, _, _, rawBadges := services.FetchRatings(
		resolved.TMDbID, mediaType, imdbID,
		epShowID, epSeason, epEpisode,
		tmdb, omdb, mdblist, trakt,
	)

	badges := services.ApplyRatingPreferences(rawBadges, settings.RatingsOrder, settings.RatingsExclude, limit)

	if resolved.PosterPath == nil || *resolved.PosterPath == "" {
		desc := "unknown"
		if resolved.IMDbID != nil {
			desc = *resolved.IMDbID
		}
		return nil, "", fmt.Errorf("no poster available for %s / tmdb:%d", desc, resolved.TMDbID)
	}

	posterPath := *resolved.PosterPath

	tmdbSize := "w780"
	if imageSizeStr != nil {
		is := services.ParseImageSize(*imageSizeStr)
		tmdbSize = is.TmdbSize()
	}

	imageBytes, err := tmdb.FetchPosterBytes(posterPath, tmdbSize)
	if err != nil {
		return nil, "", err
	}

	result, err := GenerateImage(imageBytes, badges, settings, kind, quality, nil, fontData)
	if err != nil {
		return nil, "", err
	}

	releaseDate := ""
	if resolved.ReleaseDate != nil {
		releaseDate = *resolved.ReleaseDate
	}

	return result, releaseDate, nil
}

func parseImageSizeStub(raw *string) (*services.ImageSize, error) {
	if raw == nil {
		return nil, nil
	}
	s := services.ParseImageSize(*raw)
	return &s, nil
}

func computeCDNMaxAge(releaseDate *string, minStale, maxAge uint64) uint64 {
	return services.ComputeCDNMaxAge(releaseDate, minStale, maxAge)
}

// Helper: trim extension from id value
func StripImageExt(idValue, kind string) string {
	switch kind {
	case "logo":
		return strings.TrimSuffix(idValue, ".png")
	default:
		return strings.TrimSuffix(idValue, ".jpg")
	}
}

// Helper: get DB value for image type
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

// Placeholder for font interface
type fontFace interface {
	GetGlyphAdvance(r rune) (int, error)
}

func RenderImageWithFont(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize, fontData []byte) ([]byte, error) {
	return GenerateImage(imageBytes, badges, settings, kind, quality, imageSize, fontData)
}
