package services

import (
	"context"
	"encoding/json"
	"fmt"
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
	return c.requestWithKey(ctx, url, c.KeyPool.ActiveKeyRaw())
}

func (c *TraktClient) requestWithKey(ctx context.Context, url, clientID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", clientID)
	req.Header.Set("User-Agent", UserAgent())
	return req, nil
}

func (c *TraktClient) request(url string) (*http.Request, error) {
	return c.requestCtx(context.Background(), url)
}

func (c *TraktClient) getRatingCtx(ctx context.Context, url string) (*TraktRatingsResponse, error) {
	start := time.Now()
	status, body, key, err := pooledDo(ctx, c.KeyPool, pooledCall{
		service: "Trakt",
		retry:   &TraktRetry,
		do: func(key string) (*http.Response, error) {
			req, err := c.requestWithKey(ctx, url, key)
			if err != nil {
				return nil, err
			}
			return c.HTTP.Do(req)
		},
		limited: is429,
	})
	logSlow("Trakt", time.Since(start).Milliseconds())
	if err != nil {
		return nil, withKey(err, key)
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, withKey(errors.NewOther(fmt.Sprintf("Trakt returned %d: %s", status, errSnippet(body))), key)
	}

	var result TraktRatingsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, withKey(errors.NewAPIError(err), key)
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
