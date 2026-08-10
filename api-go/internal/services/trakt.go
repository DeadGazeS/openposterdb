package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"openposterdb/internal/errors"
)

type TraktClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
}

func NewTraktClient(keys []string, httpClient *http.Client) *TraktClient {
	return &TraktClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
	}
}

type TraktRatingsResponse struct {
	Rating float64 `json:"rating"`
	Votes  uint64  `json:"votes"`
}

func (c *TraktClient) requestCtx(ctx context.Context, url string) (*http.Request, error) {
	clientID := c.KeyPool.ActiveKeyRaw()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", clientID)
	req.Header.Set("User-Agent", "openposterdb/1.2.1")
	return req, nil
}

func (c *TraktClient) request(url string) (*http.Request, error) {
	return c.requestCtx(context.Background(), url)
}

func (c *TraktClient) getRatingCtx(ctx context.Context, url string) (*TraktRatingsResponse, error) {
	req, err := c.requestCtx(ctx, url)
	if err != nil {
		return nil, errors.NewAPIError(err)
	}

	start := time.Now()
	resp, err := SendWithRetry(&TraktRetry, func() (*http.Response, error) {
		return c.HTTP.Do(req)
	})
	logSlow("Trakt", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Trakt returned %d: %s", resp.StatusCode, string(body)))
	}

	var result TraktRatingsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.NewAPIError(err)
	}
	return &result, nil
}

func (c *TraktClient) getRating(url string) (*TraktRatingsResponse, error) {
	return c.getRatingCtx(context.Background(), url)
}

func (c *TraktClient) GetMovieRatingCtx(ctx context.Context, imdbID string) (*TraktRatingsResponse, error) {
	return c.getRatingCtx(ctx, fmt.Sprintf("https://api.trakt.tv/movies/%s/ratings", imdbID))
}

func (c *TraktClient) GetMovieRating(imdbID string) (*TraktRatingsResponse, error) {
	return c.GetMovieRatingCtx(context.Background(), imdbID)
}

func (c *TraktClient) GetShowRatingCtx(ctx context.Context, imdbID string) (*TraktRatingsResponse, error) {
	return c.getRatingCtx(ctx, fmt.Sprintf("https://api.trakt.tv/shows/%s/ratings", imdbID))
}

func (c *TraktClient) GetShowRating(imdbID string) (*TraktRatingsResponse, error) {
	return c.GetShowRatingCtx(context.Background(), imdbID)
}

func (c *TraktClient) GetEpisodeRatingCtx(ctx context.Context, showID string, season, episode uint32) (*TraktRatingsResponse, error) {
	return c.getRatingCtx(ctx, fmt.Sprintf("https://api.trakt.tv/shows/%s/seasons/%d/episodes/%d/ratings", showID, season, episode))
}

func (c *TraktClient) GetEpisodeRating(showID string, season, episode uint32) (*TraktRatingsResponse, error) {
	return c.GetEpisodeRatingCtx(context.Background(), showID, season, episode)
}
