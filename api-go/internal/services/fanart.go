package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"slices"
	"strconv"
	"time"

	"openposterdb/internal/errors"
)

type FanartClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
	// ClientKey is an optional personal fanart.tv key (FANART_CLIENT_KEY):
	// sent alongside the project key, it unlocks images newer than the
	// project key alone sees.
	ClientKey string
	// images caches movie/TV image lists across renders. nil disables it.
	images *MemCache
}


// do GETs url with the client's User-Agent.
func (c *FanartClient) do(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent())
	return c.HTTP.Do(req)
}

// apiURL is a fanart.tv v3 API URL for path ("movies/550"), with the
// project key and the optional personal client key.
func (c *FanartClient) apiURL(path string) string {
	u := "https://webservice.fanart.tv/v3/" + path + "?api_key=" + neturl.QueryEscape(c.KeyPool.ActiveKeyRaw())
	if c.ClientKey != "" {
		u += "&client_key=" + neturl.QueryEscape(c.ClientKey)
	}
	return u
}

func NewFanartClient(keys []string, httpClient *http.Client) *FanartClient {
	return &FanartClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
		images:  NewMemCache(0, 5000, upstreamListTTL, 0),
	}
}

// cachedImages returns the cached list for key, or fetches and caches it.
// Errors are not cached. The returned value is shared: callers only read it.
func (c *FanartClient) cachedImages(key string, fetch func() (*FanartImages, error)) (*FanartImages, error) {
	if c.images != nil {
		if v, ok := upstreamGet(c.images, key); ok {
			return v.(*FanartImages), nil
		}
	}
	gen := upstreamGenNow()
	imgs, err := fetch()
	if err == nil && imgs != nil && c.images != nil {
		upstreamSet(c.images, key, gen, imgs)
	}
	return imgs, err
}

type FanartPoster struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Likes string `json:"likes"`
}

type FanartImages struct {
	Posters   []FanartPoster
	Logos     []FanartPoster
	Backdrops []FanartPoster
}

type movieImages struct {
	MoviePoster     []FanartPoster `json:"movieposter"`
	HDMovieLogo     []FanartPoster `json:"hdmovielogo"`
	MovieBackground []FanartPoster `json:"moviebackground"`
}

type tvImages struct {
	TVPoster       []FanartPoster `json:"tvposter"`
	HDTVLogo       []FanartPoster `json:"hdtvlogo"`
	ShowBackground []FanartPoster `json:"showbackground"`
}

type PosterMatch int

const (
	PosterMatchTextless PosterMatch = iota
	PosterMatchLanguage
)

func (c *FanartClient) GetMovieImagesCtx(ctx context.Context, tmdbID uint64) (*FanartImages, error) {
	return c.cachedImages(fmt.Sprintf("movie/%d", tmdbID), func() (*FanartImages, error) { return c.fetchMovieImages(ctx, tmdbID) })
}

func (c *FanartClient) fetchMovieImages(ctx context.Context, tmdbID uint64) (*FanartImages, error) {
	url := c.apiURL(fmt.Sprintf("movies/%d", tmdbID))

	start := time.Now()
	resp, err := SendWithRetryCtx(ctx, &FanartRetry, func() (*http.Response, error) {
		return c.do(ctx, url)
	})
	logSlow("Fanart movie images", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
	if resp.StatusCode == http.StatusNotFound {
		return &FanartImages{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart returned %d: %s", resp.StatusCode, errSnippet(body)))
	}

	var result movieImages
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.NewAPIError(err)
	}
	return &FanartImages{
		Posters:   result.MoviePoster,
		Logos:     result.HDMovieLogo,
		Backdrops: result.MovieBackground,
	}, nil
}

func (c *FanartClient) GetMovieImages(tmdbID uint64) (*FanartImages, error) {
	return c.GetMovieImagesCtx(context.Background(), tmdbID)
}

func (c *FanartClient) GetTVImagesCtx(ctx context.Context, id uint64) (*FanartImages, error) {
	return c.cachedImages(fmt.Sprintf("tv/%d", id), func() (*FanartImages, error) { return c.fetchTVImages(ctx, id) })
}

func (c *FanartClient) fetchTVImages(ctx context.Context, id uint64) (*FanartImages, error) {
	url := c.apiURL(fmt.Sprintf("tv/%d", id))

	start := time.Now()
	resp, err := SendWithRetryCtx(ctx, &FanartRetry, func() (*http.Response, error) {
		return c.do(ctx, url)
	})
	logSlow("Fanart TV images", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
	if resp.StatusCode == http.StatusNotFound {
		return &FanartImages{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart returned %d: %s", resp.StatusCode, errSnippet(body)))
	}

	var result tvImages
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.NewAPIError(err)
	}
	return &FanartImages{
		Posters:   result.TVPoster,
		Logos:     result.HDTVLogo,
		Backdrops: result.ShowBackground,
	}, nil
}

func (c *FanartClient) GetTVImages(id uint64) (*FanartImages, error) {
	return c.GetTVImagesCtx(context.Background(), id)
}

func (c *FanartClient) FetchPosterBytesCtx(ctx context.Context, url string) ([]byte, error) {
	start := time.Now()
	resp, err := SendWithRetryCtx(ctx, &FanartRetry, func() (*http.Response, error) {
		return c.do(ctx, url)
	})
	logSlow("Fanart CDN", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart CDN returned %d", resp.StatusCode))
	}
	return ReadAllLimited(resp.Body, MaxArtworkBytes)
}

func (c *FanartClient) FetchPosterBytes(url string) ([]byte, error) {
	return c.FetchPosterBytesCtx(context.Background(), url)
}

func SelectFanartImage(posters []FanartPoster, lang string, textless bool) (*FanartPoster, PosterMatch, bool) {
	if len(posters) == 0 {
		return nil, 0, false
	}

	findBest := func(targetLang string) *FanartPoster {
		var best *FanartPoster
		var bestLikes int64
		for i := range posters {
			p := &posters[i]
			if p.Lang == targetLang {
				likes, _ := strconv.ParseInt(p.Likes, 10, 64)
				if best == nil || likes > bestLikes {
					best = p
					bestLikes = likes
				}
			}
		}
		return best
	}

	// Tiers, most likes first within each: textless ("00") when asked for,
	// then lang → base language → en → textless → untagged ("") → any.
	if textless {
		if p := findBest("00"); p != nil {
			return p, PosterMatchTextless, true
		}
	}
	var tiers []string
	for _, l := range []string{lang, LangBase(lang), "en", "00", ""} {
		if !slices.Contains(tiers, l) {
			tiers = append(tiers, l)
		}
	}
	for _, l := range tiers {
		if p := findBest(l); p != nil {
			if l == "00" {
				return p, PosterMatchTextless, true
			}
			return p, PosterMatchLanguage, true
		}
	}

	var best *FanartPoster
	var bestLikes int64
	for i := range posters {
		likes, _ := strconv.ParseInt(posters[i].Likes, 10, 64)
		if best == nil || likes > bestLikes {
			best, bestLikes = &posters[i], likes
		}
	}
	return best, PosterMatchLanguage, true
}
