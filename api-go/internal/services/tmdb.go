package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openposterdb/internal/errors"

	"log/slog"
)

type TmdbClient struct {
	APIKey string
	HTTP   *http.Client
}

func NewTmdbClient(apiKey string, httpClient *http.Client) *TmdbClient {
	return &TmdbClient{
		APIKey: apiKey,
		HTTP:   httpClient,
	}
}

func (c *TmdbClient) GetCtx(ctx context.Context, path string, params map[string]string, target any) error {
	var url strings.Builder
	url.WriteString(fmt.Sprintf("https://api.themoviedb.org/3%s?api_key=%s", path, c.APIKey))
	for k, v := range params {
		url.WriteString(fmt.Sprintf("&%s=%s", k, v))
	}

	start := time.Now()
	resp, err := httpGetCtx(ctx, c.HTTP, &TMDBAPIRetry, url.String())
	logSlow("TMDB API", time.Since(start).Milliseconds())
	if err != nil {
		return errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.NewOther(fmt.Sprintf("TMDB API returned %d: %s", resp.StatusCode, string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
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
		return nil, errors.NewOther(fmt.Sprintf("TMDB CDN returned %d for %s", resp.StatusCode, filePath))
	}
	return io.ReadAll(resp.Body)
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

func (c *TmdbClient) FetchPosterBytesConditionalCtx(ctx context.Context, posterPath, tmdbSize string, ifModifiedSince *time.Time) ([]byte, bool, error) {
	url := fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", tmdbSize, posterPath)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false, errors.NewAPIError(err)
	}

	if ifModifiedSince != nil {
		req.Header.Set("If-Modified-Since", ifModifiedSince.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, false, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, errors.NewOther(fmt.Sprintf("TMDB CDN returned %d", resp.StatusCode))
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, errors.NewAPIError(err)
	}
	return bytes, false, nil
}

func (c *TmdbClient) FetchPosterBytesConditional(posterPath, tmdbSize string, ifModifiedSince *time.Time) ([]byte, bool, error) {
	return c.FetchPosterBytesConditionalCtx(context.Background(), posterPath, tmdbSize, ifModifiedSince)
}

func (c *TmdbClient) GetImagesCtx(ctx context.Context, mediaType string, tmdbID uint64, lang string) (*TmdbImagesResponse, error) {
	base := LangBase(lang)
	includeLang := "null"
	if base != "" {
		includeLang = base + ",null"
	}
	path := fmt.Sprintf("/%s/%d/images", mediaType, tmdbID)
	var result TmdbImagesResponse
	if err := c.GetCtx(ctx, path, map[string]string{"include_image_language": includeLang}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *TmdbClient) GetImages(mediaType string, tmdbID uint64, lang string) (*TmdbImagesResponse, error) {
	return c.GetImagesCtx(context.Background(), mediaType, tmdbID, lang)
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

func SelectPoster(images []TmdbImage, lang string, textless bool) *TmdbImage {
	return selectImageRanked(images, lang, textless, true)
}

func SelectImage(images []TmdbImage, lang string, textless bool) *TmdbImage {
	return selectImageRanked(images, lang, textless, false)
}

func selectImageRanked(images []TmdbImage, lang string, textless bool, hasTargetRatio bool) *TmdbImage {
	if len(images) == 0 {
		return nil
	}

	rank := func(img *TmdbImage) (bool, float64) {
		standard := false
		if hasTargetRatio && img.AspectRatio > 0 {
			diff := img.AspectRatio - posterTargetRatio
			if diff < 0 {
				diff = -diff
			}
			standard = diff <= posterRatioTol
		}
		return standard, img.VoteAverage
	}

	findBest := func(target *string) *TmdbImage {
		var best *TmdbImage
		var bestRank bool
		var bestVote float64
		for i := range images {
			img := &images[i]
			imgTarget := img.Iso639_1
			if (target == nil && imgTarget == nil) || (target != nil && imgTarget != nil && *imgTarget == *target) {
				r, v := rank(img)
				if best == nil || r && !bestRank || (r == bestRank && v > bestVote) {
					best = img
					bestRank = r
					bestVote = v
				}
			}
		}
		return best
	}

	if textless {
		return findBest(nil)
	}

	if lang == "" {
		return findBest(nil)
	}

	base := LangBase(lang)
	region := LangRegion(lang)

	if region != "" {
		for i := range images {
			img := &images[i]
			imgLang := ""
			imgRegion := ""
			if img.Iso639_1 != nil {
				imgLang = *img.Iso639_1
			}
			if img.Iso3166_1 != nil {
				imgRegion = *img.Iso3166_1
			}
			if imgLang == base && imgRegion == region {
				return img
			}
		}
	}

	if img := findBest(&base); img != nil {
		return img
	}

	if base != "en" {
		en := "en"
		if img := findBest(&en); img != nil {
			return img
		}
	}

	return nil
}

func httpGetCtx(ctx context.Context, client *http.Client, config *RetryConfig, url string) (*http.Response, error) {
	return SendWithRetry(config, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "openposterdb/1.2.1")
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

// logSlow logs a warning when an operation takes longer than 2 seconds. It's
// not currently called from anywhere — kept around as a useful per-call
// timing helper. Wire it up at the relevant call sites (TmdbClient.Get +
// FetchImageBytes, for instance) if you want slow-TMDB warnings back in
// the logs.
func logSlow(label string, ms int64) {
	if ms > 2000 {
		slog.Warn("slow "+label, "ms", ms)
	}
}
