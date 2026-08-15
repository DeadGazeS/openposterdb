package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	apperr "openposterdb/internal/errors"
)

const kitsuAPIBase = "https://kitsu.io/api/edge"

// kitsuUserAgent is set on every Kitsu request so Cloudflare's bot challenge
// (which returns an HTML challenge page instead of JSON for requests missing
// a User-Agent) lets us through. The string is intentionally a recent
// desktop Chrome so we look like a browser.
const kitsuUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// KitsuClient talks to the Kitsu JSON:API. The endpoint is unauthenticated
// (no API key) and rate-limited by Cloudflare, so we wrap requests in the
// shared SendWithRetry helper and pace callers behind the existing per-IP
// rate limiter at the HTTP layer.
type KitsuClient struct {
	HTTP *http.Client
}

func NewKitsuClient(httpClient *http.Client) *KitsuClient {
	return &KitsuClient{HTTP: httpClient}
}

// KitsuAnime is the slimmed projection of the JSON:API anime resource that
// the openposterdb ID resolver needs. Mirrors the fields aiometadata reads
// off kitsuCacheNormalizers.ts (slug + canonicalTitle + subtype +
// posterImage.original + coverImage.original) so the resolved result can
// drive poster/backdrop rendering without going back through TMDB.
type KitsuAnime struct {
	ID                   uint64
	Slug                 string
	CanonicalTitle       string
	Subtype              string
	PosterImageOriginal  *string
	CoverImageOriginal   *string
}

// KitsuMapping is one entry in the `included` array when include=mappings
// is set on the anime request. ExternalSite values seen in the wild:
// "myanimelist/anime", "hulu", "aozora", "myanimelist/character", and
// a handful of others. Only the myanimelist/anime variant is consumed.
type KitsuMapping struct {
	ExternalSite string
	ExternalID   string
}

type kitsuAnimeResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Slug           string `json:"slug"`
			CanonicalTitle string `json:"canonicalTitle"`
			Subtype        string `json:"subtype"`
			PosterImage    struct {
				Original *string `json:"original"`
			} `json:"posterImage"`
			CoverImage struct {
				Original *string `json:"original"`
			} `json:"coverImage"`
		} `json:"attributes"`
	} `json:"data"`
	Included []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			ExternalSite string `json:"externalSite"`
			ExternalID   string `json:"externalId"`
		} `json:"attributes"`
	} `json:"included"`
}

// GetAnimeCtx fetches the anime resource plus its external-id mappings in
// one round trip. Kitsu returns 404 for unknown ids; we surface that as
// NewIDNotFound so the resolveWithFallback chain can move on to the next
// source instead of treating it as a transient API error.
//
// The idOrSlug parameter accepts both numeric ids ("12") and Kitsu slugs
// ("fairy-tail-2018"). Kitsu's /anime/{id} path-segment endpoint is
// strictly numeric and rejects slugs with a 400; the slug path uses
// ?filter[slug]=... which returns a list-shaped response. The response's
// data.id is always numeric regardless of input form.
func (c *KitsuClient) GetAnimeCtx(ctx context.Context, idOrSlug string) (*KitsuAnime, []KitsuMapping, error) {
	if c == nil || c.HTTP == nil {
		return nil, nil, apperr.NewOther("Kitsu client not configured")
	}

	// Pick the endpoint based on shape. Numeric → /anime/{id} (fast, single
	// object response). Non-numeric → /anime?filter[slug]=... (list response).
	_, parseErr := strconv.ParseUint(idOrSlug, 10, 64)
	var url string
	if parseErr == nil {
		url = fmt.Sprintf("%s/anime/%s?include=mappings", kitsuAPIBase, idOrSlug)
	} else {
		url = fmt.Sprintf("%s/anime?filter[slug]=%s&include=mappings", kitsuAPIBase, idOrSlug)
	}

	start := time.Now()
	resp, err := SendWithRetry(&KitsuRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.api+json")
		req.Header.Set("User-Agent", kitsuUserAgent)
		return c.HTTP.Do(req)
	})
	logSlow("Kitsu", time.Since(start).Milliseconds())
	if err != nil {
		return nil, nil, apperr.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, apperr.NewIDNotFound(fmt.Sprintf("kitsu anime %q not found", idOrSlug))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, apperr.NewOther(fmt.Sprintf("Kitsu returned %d: %s", resp.StatusCode, string(body)))
	}

	var idNum uint64
	var attrs struct {
		ID                  uint64 `json:"id"`
		Type                string `json:"type"`
		Attributes          struct {
			Slug           string `json:"slug"`
			CanonicalTitle string `json:"canonicalTitle"`
			Subtype        string `json:"subtype"`
			PosterImage    struct {
				Original *string `json:"original"`
			} `json:"posterImage"`
			CoverImage struct {
				Original *string `json:"original"`
			} `json:"coverImage"`
		} `json:"attributes"`
	}
	var included []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			ExternalSite string `json:"externalSite"`
			ExternalID   string `json:"externalId"`
		} `json:"attributes"`
	}

	if parseErr == nil {
		// Numeric path: response.data is a single object.
		var single struct {
			Data struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					Slug           string `json:"slug"`
					CanonicalTitle string `json:"canonicalTitle"`
					Subtype        string `json:"subtype"`
					PosterImage    struct {
						Original *string `json:"original"`
					} `json:"posterImage"`
					CoverImage struct {
						Original *string `json:"original"`
					} `json:"coverImage"`
				} `json:"attributes"`
			} `json:"data"`
			Included []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					ExternalSite string `json:"externalSite"`
					ExternalID   string `json:"externalId"`
				} `json:"attributes"`
			} `json:"included"`
		}
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, nil, apperr.NewAPIError(err)
		}
		parsed, perr := strconv.ParseUint(single.Data.ID, 10, 64)
		if perr != nil {
			return nil, nil, apperr.NewOther(fmt.Sprintf("kitsu returned non-numeric id %q", single.Data.ID))
		}
		idNum = parsed
		attrs.Type = single.Data.Type
		attrs.Attributes = single.Data.Attributes
		included = single.Included
	} else {
		// Slug path: response.data is a list. Pick the first match; slug is
		// documented as unique on Kitsu's side.
		var list struct {
			Data []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					Slug           string `json:"slug"`
					CanonicalTitle string `json:"canonicalTitle"`
					Subtype        string `json:"subtype"`
					PosterImage    struct {
						Original *string `json:"original"`
					} `json:"posterImage"`
					CoverImage struct {
						Original *string `json:"original"`
					} `json:"coverImage"`
				} `json:"attributes"`
			} `json:"data"`
			Included []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					ExternalSite string `json:"externalSite"`
					ExternalID   string `json:"externalId"`
				} `json:"attributes"`
			} `json:"included"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, nil, apperr.NewAPIError(err)
		}
		if len(list.Data) == 0 {
			return nil, nil, apperr.NewIDNotFound(fmt.Sprintf("kitsu anime %q not found", idOrSlug))
		}
		first := list.Data[0]
		parsed, perr := strconv.ParseUint(first.ID, 10, 64)
		if perr != nil {
			return nil, nil, apperr.NewOther(fmt.Sprintf("kitsu returned non-numeric id %q", first.ID))
		}
		idNum = parsed
		attrs.Type = first.Type
		attrs.Attributes = first.Attributes
		included = list.Included
	}

	anime := &KitsuAnime{
		ID:                  idNum,
		Slug:                attrs.Attributes.Slug,
		CanonicalTitle:      attrs.Attributes.CanonicalTitle,
		Subtype:             attrs.Attributes.Subtype,
		PosterImageOriginal: attrs.Attributes.PosterImage.Original,
		CoverImageOriginal:  attrs.Attributes.CoverImage.Original,
	}

	var mappings []KitsuMapping
	for _, inc := range included {
		if inc.Type != "mappings" {
			continue
		}
		mappings = append(mappings, KitsuMapping{
			ExternalSite: inc.Attributes.ExternalSite,
			ExternalID:   inc.Attributes.ExternalID,
		})
	}

	slog.Debug("kitsu anime resolved",
		"id", idNum,
		"slug", anime.Slug,
		"subtype", anime.Subtype,
		"mappings", len(mappings),
		"via", map[bool]string{true: "slug_filter", false: "direct_id"}[parseErr != nil],
	)

	return anime, mappings, nil
}

// MALIDForMapping pulls the MAL anime id from a mappings list, or returns
// nil if absent. Kept package-level so the resolver in id.go can call it
// without importing this file's private types.
func MALIDForMapping(mappings []KitsuMapping) *uint64 {
	for _, m := range mappings {
		if m.ExternalSite == "myanimelist/anime" {
			id, err := strconv.ParseUint(m.ExternalID, 10, 64)
			if err == nil {
				return &id
			}
		}
	}
	return nil
}
