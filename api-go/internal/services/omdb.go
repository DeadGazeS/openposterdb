package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"time"

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

// omdbLimitBlock: OMDb's daily limit answers 401 "Request limit reached!"
// without a Retry-After; the key is left alone this long before it is tried
// again.
const omdbLimitBlock = time.Hour

// omdbLimited: OMDb reports its daily request limit as a 401 with
// "Request limit reached!" (an invalid key is a 401 too, but not a limit).
func omdbLimited(status int, body []byte) bool {
	return status == http.StatusTooManyRequests ||
		(status == http.StatusUnauthorized && bytes.Contains(body, []byte("Request limit reached")))
}

func (c *OmdbClient) GetRatingsCtx(ctx context.Context, imdbID string) (*OmdbResponse, error) {
	start := time.Now()
	status, body, key, err := pooledDo(ctx, c.KeyPool, pooledCall{
		service: "OMDb",
		retry:   &OMDBRetry,
		do: func(key string) (*http.Response, error) {
			url := fmt.Sprintf("https://www.omdbapi.com/?apikey=%s&i=%s", neturl.QueryEscape(key), neturl.QueryEscape(imdbID))
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", UserAgent())
			return c.HTTP.Do(req)
		},
		limited:    omdbLimited,
		limitBlock: omdbLimitBlock,
	})
	logSlow("OMDb", time.Since(start).Milliseconds())
	if err != nil {
		return nil, withKey(err, key)
	}
	if status != http.StatusOK {
		return nil, withKey(errors.NewOther(fmt.Sprintf("OMDb returned %d: %s", status, errSnippet(body))), key)
	}

	var result OmdbResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, withKey(errors.NewAPIError(err), key)
	}
	return &result, nil
}

func (c *OmdbClient) GetRatings(imdbID string) (*OmdbResponse, error) {
	return c.GetRatingsCtx(context.Background(), imdbID)
}
