package image

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"openposterdb/internal/services"
)

var fontData []byte
var loadedFontFace font.Face

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

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    26,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return err
	}
	loadedFontFace = face
	return nil
}

func GetFontFace() font.Face {
	return loadedFontFace
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

	f := GetFontFace()
	if f == nil {
		return nil, fmt.Errorf("font not loaded")
	}

	switch kind {
	case "poster":
		return RenderPosterSync(imageBytes, badges, f, f, quality,
			position, badgeStyle, labelStyle, appearance, badgeDirection,
			targetW, badgeScale, settings.PosterBadgeSize,
			settings.PosterBadgeSplit, settings.PosterFit)

	case "logo":
		return RenderLogoSync(imageBytes, badges, f, f,
			badgeStyle, labelStyle, appearance, targetW, badgeScale)

	case "backdrop":
		return RenderBackdropSync(imageBytes, badges, f, f, quality,
			position, badgeStyle, labelStyle, appearance, badgeDirection,
			targetW, badgeScale, settings.BackdropBadgeSize,
			settings.BackdropEdgeInsetX, settings.BackdropEdgeInsetY)

	case "episode":
		return RenderEpisodeSync(imageBytes, badges, f, f, quality,
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
	ratingsMinStaleSecs uint64,
	ratingsMaxAgeSecs uint64,
	quality uint8,
	imageSizeStr *string,
) ([]byte, string, error) {

	idType, err := services.ParseIDType(idTypeStr)
	if err != nil {
		return nil, "", err
	}
	_ = idType

	if err := services.ValidateIDValue(idValue); err != nil {
		return nil, "", err
	}

	if tmdb == nil {
		return nil, "", fmt.Errorf("TMDB not configured")
	}

	resolved, err := services.ResolveID(idType, idValue, tmdb)
	if err != nil {
		return nil, "", err
	}
	_ = resolved

	return nil, "", fmt.Errorf("not implemented")
}

func RenderImageWithFont(imageBytes []byte, badges []services.RatingBadge, settings *services.RenderSettings, kind string, quality uint8, imageSize *services.ImageSize, fontData []byte) ([]byte, error) {
	return GenerateImage(imageBytes, badges, settings, kind, quality, imageSize)
}
