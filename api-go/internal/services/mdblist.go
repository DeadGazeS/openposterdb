package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"openposterdb/internal/errors"

	"log/slog"
)

type MdblistClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
}

func NewMdblistClient(keys []string, httpClient *http.Client) *MdblistClient {
	return &MdblistClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
	}
}

func (c *MdblistClient) ActiveKeyHash() string {
	return c.KeyPool.ActiveKeyHash()
}

type MdblistResponse struct {
	Ratings []MdblistRating `json:"ratings"`
	IDs     MdblistIDs      `json:"ids"`
	Score   *float64        `json:"score"`
}

type MdblistIDs struct {
	IMDB *string `json:"imdb"`
	TMDB *uint64 `json:"tmdb"`
	TVDB *uint64 `json:"tvdb"`
}

type MdblistRating struct {
	Source string   `json:"source"`
	Value  *float64 `json:"value"`
	Score  *float64 `json:"score"`
	Votes  *int64   `json:"votes"`
}

func (c *MdblistClient) GetRatings(imdbID, mediaType string) (*MdblistResponse, error) {
	kind, err := mdblistKind(mediaType)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.mdblist.com/imdb/%s/%s", kind, imdbID)
	return c.fetch(url)
}

func (c *MdblistClient) GetRatingsByTMDB(tmdbID uint64, mediaType string) (*MdblistResponse, error) {
	kind, err := mdblistKind(mediaType)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.mdblist.com/tmdb/%s/%d", kind, tmdbID)
	return c.fetch(url)
}

func (c *MdblistClient) fetch(url string) (*MdblistResponse, error) {
	apiKey := c.KeyPool.ActiveKeyRaw()
	keyHash := c.KeyPool.ActiveKeyHash()

	fullURL := fmt.Sprintf("%s?apikey=%s", url, apiKey)

	resp, err := SendWithRetry(&MDBListRetry, func() (*http.Response, error) {
		req, _ := http.NewRequest("GET", fullURL, nil)
		req.Header.Set("User-Agent", "openposterdb/1.2.1")
		return c.HTTP.Do(req)
	})

	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		c.KeyPool.Report429()
		slog.Warn("mdblist API key rate limited (429)", "key", keyHash)
		return nil, errors.NewOther(fmt.Sprintf("MDBList 429: %s", string(body)))
	}

	c.KeyPool.ReportSuccess()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("MDBList returned %d: %s", resp.StatusCode, string(body)))
	}

	var result MdblistResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.NewAPIError(err)
	}
	return &result, nil
}

func mdblistKind(mediaType string) (string, error) {
	switch mediaType {
	case "movie":
		return "movie", nil
	case "tv":
		return "show", nil
	default:
		return "", errors.NewOther("mdblist does not support episode ratings")
	}
}
