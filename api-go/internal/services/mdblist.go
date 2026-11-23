package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strconv"
	"time"

	"openposterdb/internal/errors"
)

type MdblistClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
	// batch gathers concurrent lookups into one POST of up to
	// mdblistBatchMax ids (MDBList meters requests, not rows). nil: one
	// GET per title.
	batch *batcher[mdblistKey, mdblistResult]
}

const (
	mdblistBatchMax     = 200
	mdblistBatchWindow  = 50 * time.Millisecond
	mdblistBatchTimeout = 30 * time.Second
)

// mdblistKey is one lookup: provider "imdb" / "tmdb", kind "movie" /
// "show", and the id.
type mdblistKey struct{ provider, kind, id string }

type mdblistResult struct {
	resp *MdblistResponse
	err  error
}

func NewMdblistClient(keys []string, httpClient *http.Client) *MdblistClient {
	c := &MdblistClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
	}
	c.batch = newBatcher(mdblistBatchWindow, mdblistBatchMax, mdblistBatchTimeout, c.fetchBatch)
	return c
}

func (k mdblistKey) url() string {
	return fmt.Sprintf("https://api.mdblist.com/%s/%s/%s", k.provider, k.kind, neturl.PathEscape(k.id))
}

// lookup answers one title through the batcher (or directly without one).
func (c *MdblistClient) lookup(ctx context.Context, k mdblistKey) (*MdblistResponse, error) {
	if c.batch == nil {
		return c.fetchCtx(ctx, k.url())
	}
	r, ok, err := c.batch.get(ctx, k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.NewOther("MDBList: no result for " + k.provider + "/" + k.id)
	}
	return r.resp, r.err
}

// fetchBatch answers a gathered batch: one POST per provider/kind group
// (a group of one keeps the plain GET). Items are matched back to the
// requested ids by their "ids" block; a title missing from the answer is
// MDBList's not-found (no ratings, no error). Should no item match at all (an unexpected response
// shape), the group falls back to one GET per title.
func (c *MdblistClient) fetchBatch(ctx context.Context, keys []mdblistKey) (map[mdblistKey]mdblistResult, error) {
	groups := map[[2]string][]mdblistKey{}
	for _, k := range keys {
		g := [2]string{k.provider, k.kind}
		groups[g] = append(groups[g], k)
	}
	out := make(map[mdblistKey]mdblistResult, len(keys))
	singles := func(ks []mdblistKey) {
		for _, k := range ks {
			r, err := c.fetchCtx(ctx, k.url())
			out[k] = mdblistResult{r, err}
		}
	}
	for g, ks := range groups {
		if len(ks) == 1 {
			singles(ks)
			continue
		}
		items, err := c.postBatch(ctx, g[0], g[1], ks)
		if err != nil {
			for _, k := range ks {
				out[k] = mdblistResult{err: err}
			}
			continue
		}
		byID := map[string]*MdblistResponse{}
		for i := range items {
			it := &items[i]
			switch g[0] {
			case "imdb":
				if it.IDs.IMDB != nil {
					byID[*it.IDs.IMDB] = it
				}
			case "tmdb":
				if it.IDs.TMDB != nil {
					byID[strconv.FormatUint(*it.IDs.TMDB, 10)] = it
				}
			}
		}
		if len(items) > 0 && len(byID) == 0 {
			slog.Warn("MDBList batch answer could not be matched to the requested ids; asking one by one", "titles", len(ks))
			singles(ks)
			continue
		}
		for _, k := range ks {
			// A title missing from the answer is MDBList's not-found: no
			// ratings, not a failure (as on the single GET).
			out[k] = mdblistResult{resp: byID[k.id]}
		}
	}
	return out, nil
}

func (c *MdblistClient) postBatch(ctx context.Context, provider, kind string, ks []mdblistKey) ([]MdblistResponse, error) {
	ids := make([]string, len(ks))
	for i, k := range ks {
		ids[i] = k.id
	}
	payload, _ := json.Marshal(map[string]any{"ids": ids})
	start := time.Now()
	status, body, key, err := pooledDo(ctx, c.KeyPool, pooledCall{
		service: "MDBList",
		retry:   &MDBListRetry,
		do: func(key string) (*http.Response, error) {
			u := fmt.Sprintf("https://api.mdblist.com/%s/%s?apikey=%s", provider, kind, neturl.QueryEscape(key))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", UserAgent())
			return c.HTTP.Do(req)
		},
		limited: is429,
	})
	logSlow("MDBList batch", time.Since(start).Milliseconds())
	if err != nil {
		return nil, withKey(err, key)
	}
	if status != http.StatusOK {
		return nil, withKey(errors.NewOther(fmt.Sprintf("MDBList returned %d: %s", status, errSnippet(body))), key)
	}
	var items []MdblistResponse
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, withKey(errors.NewAPIError(err), key)
	}
	for i := range items {
		items[i].fromBatch = true
	}
	return items, nil
}

func (c *MdblistClient) ActiveKeyHash() string {
	return c.KeyPool.ActiveKeyHash()
}

type MdblistResponse struct {
	Ratings []MdblistRating `json:"ratings"`
	IDs     MdblistIDs      `json:"ids"`
	Score   *float64        `json:"score"`
	// fromBatch: the answer came from the batch POST, which reports some
	// values on another scale than the single GET (Letterboxd: 0–10, not
	// 0–5).
	fromBatch bool
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

func (c *MdblistClient) GetRatingsCtx(ctx context.Context, imdbID, mediaType string) (*MdblistResponse, error) {
	kind, err := mdblistKind(mediaType)
	if err != nil {
		return nil, err
	}
	return c.lookup(ctx, mdblistKey{"imdb", kind, imdbID})
}

func (c *MdblistClient) GetRatings(imdbID, mediaType string) (*MdblistResponse, error) {
	return c.GetRatingsCtx(context.Background(), imdbID, mediaType)
}

func (c *MdblistClient) GetRatingsByTMDBCtx(ctx context.Context, tmdbID uint64, mediaType string) (*MdblistResponse, error) {
	kind, err := mdblistKind(mediaType)
	if err != nil {
		return nil, err
	}
	return c.lookup(ctx, mdblistKey{"tmdb", kind, strconv.FormatUint(tmdbID, 10)})
}

func (c *MdblistClient) GetRatingsByTMDB(tmdbID uint64, mediaType string) (*MdblistResponse, error) {
	return c.GetRatingsByTMDBCtx(context.Background(), tmdbID, mediaType)
}

func (c *MdblistClient) fetchCtx(ctx context.Context, url string) (*MdblistResponse, error) {
	start := time.Now()
	status, body, key, err := pooledDo(ctx, c.KeyPool, pooledCall{
		service: "MDBList",
		retry:   &MDBListRetry,
		do: func(key string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", url+"?apikey="+neturl.QueryEscape(key), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", UserAgent())
			return c.HTTP.Do(req)
		},
		limited: is429,
	})
	logSlow("MDBList", time.Since(start).Milliseconds())
	if err != nil {
		return nil, withKey(err, key)
	}
	// A title MDBList doesn't know has no MDBList ratings: not a failure.
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, withKey(errors.NewOther(fmt.Sprintf("MDBList returned %d: %s", status, errSnippet(body))), key)
	}

	var result MdblistResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, withKey(errors.NewAPIError(err), key)
	}
	return &result, nil
}

func (c *MdblistClient) fetch(url string) (*MdblistResponse, error) {
	return c.fetchCtx(context.Background(), url)
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
