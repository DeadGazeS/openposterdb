package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"openposterdb/internal/errors"
)

type OmdbClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
}

func NewOmdbClient(keys []string, httpClient *http.Client) *OmdbClient {
	return &OmdbClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
	}
}

type OmdbResponse struct {
	Ratings    []OmdbRating `json:"Ratings"`
	IMDBRating *string      `json:"imdbRating"`
	Metascore  *string      `json:"Metascore"`
}

type OmdbRating struct {
	Source string `json:"Source"`
	Value  string `json:"Value"`
}

func (c *OmdbClient) GetRatings(imdbID string) (*OmdbResponse, error) {
	apiKey := c.KeyPool.ActiveKeyRaw()
	url := fmt.Sprintf("https://www.omdbapi.com/?apikey=%s&i=%s", apiKey, imdbID)

	resp, err := SendWithRetry(&OMDBRetry, func() (*http.Response, error) {
		return c.HTTP.Get(url)
	})
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("OMDb returned %d: %s", resp.StatusCode, string(body)))
	}

	var result OmdbResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.NewAPIError(err)
	}
	return &result, nil
}
