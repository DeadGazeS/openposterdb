package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

func (c *TraktClient) request(url string) (*http.Request, error) {
	clientID := c.KeyPool.ActiveKeyRaw()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", clientID)
	req.Header.Set("User-Agent", "openposterdb/1.2.1")
	return req, nil
}

func (c *TraktClient) getRating(url string) (*TraktRatingsResponse, error) {
	req, err := c.request(url)
	if err != nil {
		return nil, errors.NewAPIError(err)
	}

	resp, err := SendWithRetry(&TraktRetry, func() (*http.Response, error) {
		return c.HTTP.Do(req)
	})
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

func (c *TraktClient) GetMovieRating(imdbID string) (*TraktRatingsResponse, error) {
	return c.getRating(fmt.Sprintf("https://api.trakt.tv/movies/%s/ratings", imdbID))
}

func (c *TraktClient) GetShowRating(imdbID string) (*TraktRatingsResponse, error) {
	return c.getRating(fmt.Sprintf("https://api.trakt.tv/shows/%s/ratings", imdbID))
}

func (c *TraktClient) GetEpisodeRating(showID string, season, episode uint32) (*TraktRatingsResponse, error) {
	return c.getRating(fmt.Sprintf("https://api.trakt.tv/shows/%s/seasons/%d/episodes/%d/ratings", showID, season, episode))
}
