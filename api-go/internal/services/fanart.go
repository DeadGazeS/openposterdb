package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"openposterdb/internal/errors"
)

type FanartClient struct {
	KeyPool *APIKeyPool
	HTTP    *http.Client
}

func NewFanartClient(keys []string, httpClient *http.Client) *FanartClient {
	return &FanartClient{
		KeyPool: NewAPIKeyPool(keys),
		HTTP:    httpClient,
	}
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
	apiKey := c.KeyPool.ActiveKeyRaw()
	url := fmt.Sprintf("https://webservice.fanart.tv/v3/movies/%d?api_key=%s", tmdbID, apiKey)

	start := time.Now()
	resp, err := SendWithRetry(&FanartRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		return c.HTTP.Do(req)
	})
	logSlow("Fanart movie images", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return &FanartImages{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart returned %d: %s", resp.StatusCode, string(body)))
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
	apiKey := c.KeyPool.ActiveKeyRaw()
	url := fmt.Sprintf("https://webservice.fanart.tv/v3/tv/%d?api_key=%s", id, apiKey)

	start := time.Now()
	resp, err := SendWithRetry(&FanartRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		return c.HTTP.Do(req)
	})
	logSlow("Fanart TV images", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return &FanartImages{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart returned %d: %s", resp.StatusCode, string(body)))
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
	resp, err := SendWithRetry(&FanartRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		return c.HTTP.Do(req)
	})
	logSlow("Fanart CDN", time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.NewAPIError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewOther(fmt.Sprintf("Fanart CDN returned %d", resp.StatusCode))
	}
	return io.ReadAll(resp.Body)
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

	if textless {
		if p := findBest("00"); p != nil {
			return p, PosterMatchTextless, true
		}
	}

	if p := findBest(lang); p != nil {
		return p, PosterMatchLanguage, true
	}

	base := LangBase(lang)
	if base != lang {
		if p := findBest(base); p != nil {
			return p, PosterMatchLanguage, true
		}
	}

	if lang != "en" && base != "en" {
		if p := findBest("en"); p != nil {
			return p, PosterMatchLanguage, true
		}
	}

	if p := findBest(""); p != nil {
		return p, PosterMatchLanguage, true
	}

	return nil, 0, false
}
