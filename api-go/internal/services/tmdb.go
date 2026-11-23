package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"slices"
	"strings"
	"time"

	"openposterdb/internal/errors"

	"log/slog"
)

type TmdbClient struct {
	APIKey string
	HTTP   *http.Client
	// images caches /images responses: every settings variant of a title
	// re-rendered the same list. nil (a struct literal) disables caching.
	images *MemCache
}

// upstreamListTTL bounds how long an artwork list (TMDB /images, fanart.tv)
// is reused across renders before it is fetched again.
const upstreamListTTL = 30 * time.Minute

func NewTmdbClient(apiKey string, httpClient *http.Client) *TmdbClient {
	return &TmdbClient{
		APIKey: apiKey,
		HTTP:   httpClient,
		images: NewMemCache(0, 5000, upstreamListTTL, 0),
	}
}

func (c *TmdbClient) GetCtx(ctx context.Context, path string, params map[string]string, target any) error {
	q := neturl.Values{"api_key": {c.APIKey}}
	for k, v := range params {
		q.Set(k, v) // escaped: a value can't add parameters
	}
	var url strings.Builder
	url.WriteString("https://api.themoviedb.org/3" + path + "?" + q.Encode())

	start := time.Now()
	resp, err := httpGetCtx(ctx, c.HTTP, &TMDBAPIRetry, url.String())
	logSlow("TMDB API", time.Since(start).Milliseconds())
	if err != nil {
		return errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Unknown/deleted id: a not-found (cached briefly by the ID
		// resolver), not a server error retried on every request.
		return errors.NewIDNotFound(fmt.Sprintf("TMDB: %s not found", path))
	}
	if resp.StatusCode != http.StatusOK {
		return errors.NewUpstreamStatus(fmt.Sprintf("TMDB API returned %d: %s", resp.StatusCode, upstreamErrorSnippet(resp.Body)), resp.StatusCode)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUpstreamJSON)).Decode(target); err != nil {
		return errors.NewAPIError(err)
	}
	return nil
}

func (c *TmdbClient) Get(path string, params map[string]string, target any) error {
	return c.GetCtx(context.Background(), path, params, target)
}

func (c *TmdbClient) FetchImageBytesCtx(ctx context.Context, filePath, size string) ([]byte, error) {
	url := fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, filePath)
	start := time.Now()
	resp, err := httpGetCtx(ctx, c.HTTP, &TMDBCDNRetry, url)
	logSlow("TMDB CDN", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewUpstreamStatus(fmt.Sprintf("TMDB CDN returned %d for %s", resp.StatusCode, filePath), resp.StatusCode)
	}
	return ReadAllLimited(resp.Body, MaxArtworkBytes)
}

func (c *TmdbClient) FetchImageBytes(filePath, size string) ([]byte, error) {
	return c.FetchImageBytesCtx(context.Background(), filePath, size)
}

func (c *TmdbClient) FetchPosterBytes(posterPath, tmdbSize string) ([]byte, error) {
	return c.FetchPosterBytesCtx(context.Background(), posterPath, tmdbSize)
}

func (c *TmdbClient) FetchPosterBytesCtx(ctx context.Context, posterPath, tmdbSize string) ([]byte, error) {
	return c.FetchImageBytesCtx(ctx, posterPath, tmdbSize)
}

// FetchPosterBytesConditionalCtx re-fetches an image only if it changed:
// If-Modified-Since / If-None-Match are sent when given, and a 304 returns
// notModified with no bytes. Uses the same retry policy and User-Agent as
// the other CDN calls. etag is the response's ETag (when one was sent).
func (c *TmdbClient) FetchPosterBytesConditionalCtx(ctx context.Context, posterPath, tmdbSize string, ifModifiedSince *time.Time, ifNoneMatch string) (bytes []byte, etag string, notModified bool, err error) {
	url := fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", tmdbSize, posterPath)
	start := time.Now()
	resp, err := SendWithRetryCtx(ctx, &TMDBCDNRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent())
		if ifModifiedSince != nil {
			req.Header.Set("If-Modified-Since", ifModifiedSince.UTC().Format(http.TimeFormat))
		}
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		return c.HTTP.Do(req)
	})
	logSlow("TMDB CDN", time.Since(start).Milliseconds())
	if err != nil {
		return nil, "", false, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, resp.Header.Get("ETag"), true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, errors.NewUpstreamStatus(fmt.Sprintf("TMDB CDN returned %d for %s", resp.StatusCode, posterPath), resp.StatusCode)
	}
	bytes, err = ReadAllLimited(resp.Body, MaxArtworkBytes)
	if err != nil {
		return nil, "", false, errors.NewAPIError(err)
	}
	return bytes, resp.Header.Get("ETag"), false, nil
}

// baseFlight shares one base-image download between concurrent renders of
// the same artwork (different settings render different cache keys but read
// the same base file).
var baseFlight flightGroup[[]byte]

// FetchBaseImageCtx returns the TMDB image filePath at size, using the
// on-disk base cache (base/posters/<size>/...) unless externalCacheOnly.
//   - fresh copy: served from disk;
//   - copy older than staleSecs (0 = never stale): revalidated with a
//     conditional request (If-Modified-Since = file mtime, If-None-Match =
//     the stored ETag). 304 bumps the mtime; any failure serves the stale
//     copy;
//   - no copy: downloaded and stored.
//
// Concurrent callers for the same base file share one download.
func (c *TmdbClient) FetchBaseImageCtx(ctx context.Context, cacheDir, filePath, size string, staleSecs uint64, externalCacheOnly bool) ([]byte, error) {
	if externalCacheOnly {
		return baseFlight.do(ctx, "ext|"+size+filePath, func(fctx context.Context) ([]byte, error) {
			return c.FetchPosterBytesCtx(fctx, filePath, size)
		})
	}
	basePath, err := BasePosterPath(cacheDir, filePath, size)
	if err != nil {
		return nil, err
	}
	NoteBaseAccess(basePath) // the cache cap's LRU order (mtime is the staleness clock)
	if entry, err := ReadCache(basePath, staleSecs); err == nil && !entry.IsStale {
		return entry.Bytes, nil
	}
	return baseFlight.do(ctx, basePath, func(fctx context.Context) ([]byte, error) {
		return c.refreshBaseImage(fctx, basePath, filePath, size, staleSecs)
	})
}

func (c *TmdbClient) refreshBaseImage(ctx context.Context, basePath, filePath, size string, staleSecs uint64) ([]byte, error) {
	entry, readErr := ReadCache(basePath, staleSecs)
	if readErr == nil && !entry.IsStale {
		return entry.Bytes, nil // another caller refreshed it meanwhile
	}
	etagPath := basePath + ".etag"
	if readErr != nil {
		bytes, etag, _, err := c.FetchPosterBytesConditionalCtx(ctx, filePath, size, nil, "")
		if err != nil {
			return nil, err
		}
		storeBaseImage(basePath, etagPath, bytes, etag)
		return bytes, nil
	}

	var since *time.Time
	if info, err := os.Stat(basePath); err == nil {
		t := info.ModTime()
		since = &t
	}
	storedETag := ""
	if b, err := os.ReadFile(etagPath); err == nil {
		storedETag = strings.TrimSpace(string(b))
	}
	bytes, etag, notModified, err := c.FetchPosterBytesConditionalCtx(ctx, filePath, size, since, storedETag)
	switch {
	case err != nil:
		slog.Debug("base image revalidation failed, serving the stale copy", "path", filePath, "error", err)
		return entry.Bytes, nil
	case notModified:
		now := time.Now()
		if err := os.Chtimes(basePath, now, now); err != nil {
			slog.Debug("base image touch failed", "path", basePath, "error", err)
		}
		return entry.Bytes, nil
	}
	storeBaseImage(basePath, etagPath, bytes, etag)
	return bytes, nil
}

func storeBaseImage(basePath, etagPath string, bytes []byte, etag string) {
	if err := WriteCache(basePath, bytes); err != nil {
		slog.Warn("failed to write base image cache", "path", basePath, "error", err)
		return
	}
	if etag == "" {
		os.Remove(etagPath)
		return
	}
	if err := WriteCache(etagPath, []byte(etag)); err != nil {
		slog.Debug("failed to store base image etag", "path", etagPath, "error", err)
	}
}

// imageLanguages is the include_image_language list for one /images call:
// the requested language, English, untagged images ("null" and "xx") and the title's original
// language, so every selection tier can match without a second request.
func imageLanguages(lang, originalLang string) string {
	var out []string
	add := func(l string) {
		if l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	add(LangBase(lang))
	add("en")
	add("null")
	add("xx") // TMDB's explicit "no language" tag (isUntaggedImage)
	add(LangBase(originalLang))
	return strings.Join(out, ",")
}

func (c *TmdbClient) GetImagesCtx(ctx context.Context, mediaType string, tmdbID uint64, lang, originalLang string) (*TmdbImagesResponse, error) {
	includeLang := imageLanguages(lang, originalLang)
	path := fmt.Sprintf("/%s/%d/images", mediaType, tmdbID)
	key := path + "?" + includeLang
	if c.images != nil {
		if v, ok := upstreamGet(c.images, key); ok {
			return v.(*TmdbImagesResponse), nil // shared: callers only read it
		}
	}
	gen := upstreamGenNow()
	var result TmdbImagesResponse
	if err := c.GetCtx(ctx, path, map[string]string{"include_image_language": includeLang}, &result); err != nil {
		return nil, err
	}
	if c.images != nil {
		upstreamSet(c.images, key, gen, &result)
	}
	return &result, nil
}

func (c *TmdbClient) GetImages(mediaType string, tmdbID uint64, lang, originalLang string) (*TmdbImagesResponse, error) {
	return c.GetImagesCtx(context.Background(), mediaType, tmdbID, lang, originalLang)
}

type TmdbImage struct {
	FilePath    string  `json:"file_path"`
	Iso639_1    *string `json:"iso_639_1"`
	Iso3166_1   *string `json:"iso_3166_1"`
	VoteAverage float64 `json:"vote_average"`
	AspectRatio float64 `json:"aspect_ratio"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
}

type TmdbImagesResponse struct {
	Backdrops []TmdbImage `json:"backdrops"`
	Logos     []TmdbImage `json:"logos"`
	Posters   []TmdbImage `json:"posters"`
}

const posterTargetRatio = 2.0 / 3.0
const posterRatioTol = 0.05

// SelectPoster picks a poster: see selectImageRanked for the tiers; within a
// tier, 2:3 posters beat other shapes, then the higher vote average wins.
func SelectPoster(images []TmdbImage, lang, originalLang string, textless bool) *TmdbImage {
	return selectImageRanked(images, lang, originalLang, textless, true)
}

// SelectImage picks a logo (same tiers as posters, no aspect preference).
func SelectImage(images []TmdbImage, lang, originalLang string, textless bool) *TmdbImage {
	return selectImageRanked(images, lang, originalLang, textless, false)
}

// SelectBackdrop picks a backdrop: untagged ("null"/"xx", i.e. without text)
// first, then the requested language, then any.
func SelectBackdrop(images []TmdbImage, lang string) *TmdbImage {
	best := func(match func(*TmdbImage) bool) *TmdbImage { return bestImage(images, match, false) }
	if img := best(isUntaggedImage); img != nil {
		return img
	}
	if base := LangBase(lang); base != "" {
		if img := best(func(i *TmdbImage) bool { return imageLang(i) == base }); img != nil {
			return img
		}
	}
	return best(func(*TmdbImage) bool { return true })
}

func imageLang(img *TmdbImage) string {
	if img.Iso639_1 == nil {
		return ""
	}
	return *img.Iso639_1
}

// isUntaggedImage: no language ("null") or TMDB's "xx" (no language) tag.
func isUntaggedImage(img *TmdbImage) bool {
	l := imageLang(img)
	return l == "" || l == "xx"
}

// bestImage returns the best-ranked image matching match: with
// hasTargetRatio, images within posterRatioTol of 2:3 beat the rest; ties go
// to the higher vote average.
func bestImage(images []TmdbImage, match func(*TmdbImage) bool, hasTargetRatio bool) *TmdbImage {
	var best *TmdbImage
	var bestStd bool
	var bestVote float64
	for i := range images {
		img := &images[i]
		if !match(img) {
			continue
		}
		std := false
		if hasTargetRatio && img.AspectRatio > 0 {
			diff := img.AspectRatio - posterTargetRatio
			if diff < 0 {
				diff = -diff
			}
			std = diff <= posterRatioTol
		}
		if best == nil || std && !bestStd || (std == bestStd && img.VoteAverage > bestVote) {
			best, bestStd, bestVote = img, std, img.VoteAverage
		}
	}
	return best
}

// selectImageRanked walks the language tiers and returns the best image of the
// first tier that has one:
//
//	textless: untagged ("null"/"xx") only
//	otherwise: lang+region → lang → en → original language → untagged → any
//
// Every tier ranks by bestImage (aspect ratio, then votes).
func selectImageRanked(images []TmdbImage, lang, originalLang string, textless bool, hasTargetRatio bool) *TmdbImage {
	if len(images) == 0 {
		return nil
	}
	best := func(match func(*TmdbImage) bool) *TmdbImage { return bestImage(images, match, hasTargetRatio) }
	if textless {
		return best(isUntaggedImage)
	}
	base := LangBase(lang)
	if region := LangRegion(lang); base != "" && region != "" {
		if img := best(func(i *TmdbImage) bool {
			return imageLang(i) == base && i.Iso3166_1 != nil && strings.EqualFold(*i.Iso3166_1, region)
		}); img != nil {
			return img
		}
	}
	for _, l := range []string{base, "en", LangBase(originalLang)} {
		if l == "" {
			continue
		}
		if img := best(func(i *TmdbImage) bool { return imageLang(i) == l }); img != nil {
			return img
		}
	}
	if img := best(isUntaggedImage); img != nil {
		return img
	}
	return best(func(*TmdbImage) bool { return true })
}

func httpGetCtx(ctx context.Context, client *http.Client, config *RetryConfig, url string) (*http.Response, error) {
	return SendWithRetryCtx(ctx, config, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent())
		resp, err := client.Do(req)
		if err != nil {
			// The url.Error carries the full URL, including the api_key query
			// param — redact it so keys never reach logs.
			return nil, errors.RedactURLSecrets(err)
		}
		return resp, nil
	})
}

func httpGet(client *http.Client, config *RetryConfig, url string) (*http.Response, error) {
	return httpGetCtx(context.Background(), client, config, url)
}

// logSlow logs a warning when an operation takes longer than 2 seconds
// (provider calls, ratings fetches, renders, whole image requests).
func logSlow(label string, ms int64, attrs ...any) {
	if ms > 2000 {
		slog.Warn("slow "+label, append([]any{"ms", ms}, attrs...)...)
	}
}

// LogSlow is logSlow for other packages.
func LogSlow(label string, ms int64, attrs ...any) { logSlow(label, ms, attrs...) }
