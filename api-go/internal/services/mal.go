package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	apperr "openposterdb/internal/errors"
)

// AniListClient talks to https://graphql.anilist.co for MAL-id lookups.
// We deliberately route MAL resolution through AniList rather than Jikan
// (jikan.moe, sunset 2026-10-01) or MAL's own OAuth-gated v2 API: AniList
// has the richest MAL↔IMDb↔TMDB cross-reference for anime, is unauthenticated
// for read-only queries, and is rate-limited at ~30 req/min per IP. The
// resolver in id.go uses this client to populate ResolvedID with direct
// poster/cover URLs (AniList coverImage.extraLarge + bannerImage) so the
// image pipeline doesn't need a TMDB id for anime not covered there.
type AniListClient struct {
	HTTP       *http.Client
	cacheMu    sync.RWMutex
	imdbByMAL  map[uint64]string // MAL→IMDb lookup cache. "" = negative cache (known missing), missing key = not yet looked up.
}

func NewAniListClient(httpClient *http.Client) *AniListClient {
	return &AniListClient{HTTP: httpClient, imdbByMAL: map[uint64]string{}}
}

const anilistGraphQLEndpoint = "https://graphql.anilist.co"

// imdbURLRegexp extracts the "tt1234567" id from a typical AniList IMDB
// externalLink URL like "https://www.imdb.com/title/tt0213338/". The trailing
// slash and query string are common variations; the regex is intentionally
// tolerant of both.
var imdbURLRegexp = regexp.MustCompile(`imdb\.com/title/(tt\d+)`)

// extractIMDBFromURL returns the IMDb id ("tt..." prefix) from a URL, or
// empty string if the URL isn't an IMDB title URL.
func extractIMDBFromURL(rawURL string) string {
	m := imdbURLRegexp.FindStringSubmatch(rawURL)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// AniListMedia is the slimmed projection of Media(idMal: N, type: ANIME)
// that the openposterdb ID resolver needs. CoverImage and BannerImage are
// returned by AniList as full URLs and used directly by image/serve.go —
// no CDN re-fetch with a size prefix like TMDB requires. ExternalLinks
// is included so the MAL→IMDb translation source in resolveKitsuMalCtx
// can pull the IMDb equivalent from AniList when one is listed.
type AniListMedia struct {
	AniListID         uint64
	MALID             uint64
	TitleRomaji       string
	TitleEnglish      string
	CoverImageExtra   *string
	BannerImage       *string
	ExternalLinks     []AniListExternalLink
}

// AniListExternalLink is one row of Media.externalLinks. Site is the
// canonical platform name (e.g. "IMDB", "Hulu"); URL is the full link.
type AniListExternalLink struct {
	URL  string
	Site string
}

type anilistMediaResponse struct {
	Data struct {
		Media struct {
			ID     uint64 `json:"id"`
			IDMal  uint64 `json:"idMal"`
			Title  struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
			} `json:"title"`
			CoverImage struct {
				ExtraLarge *string `json:"extraLarge"`
			} `json:"coverImage"`
			BannerImage   *string                  `json:"bannerImage"`
			ExternalLinks []anilistExternalLinkRaw `json:"externalLinks"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"errors"`
}

type anilistExternalLinkRaw struct {
	URL  string `json:"url"`
	Site string `json:"site"`
}

// MediaByMALCtx looks up an AniList Media record by its MAL anime id. If
// the user provides a MAL id that doesn't exist (or AniList returns
// idMal=null for a licensed-only entry), this returns NewIDNotFound so
// resolveWithFallback moves on to the next source. idMal=null arrives as
// AniListResponse.Errors in practice, not a missing data field.
func (c *AniListClient) MediaByMALCtx(ctx context.Context, malID uint64) (*AniListMedia, error) {
	if c == nil || c.HTTP == nil {
		return nil, apperr.NewOther("AniList client not configured")
	}

	query := fmt.Sprintf(`{ Media(idMal: %d, type: ANIME) { id idMal title { romaji english } coverImage { extraLarge } bannerImage externalLinks { url site } } }`, malID)
	payload, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, apperr.NewAPIError(err)
	}

	start := time.Now()
	resp, err := SendWithRetry(&MALRetry, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", anilistGraphQLEndpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return c.HTTP.Do(req)
	})
	logSlow("AniList", time.Since(start).Milliseconds())
	if err != nil {
		return nil, apperr.NewAPIError(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, apperr.NewOther(fmt.Sprintf("AniList rate-limited (429) for MAL id %d", malID))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apperr.NewOther(fmt.Sprintf("AniList returned %d: %s", resp.StatusCode, string(body)))
	}

	var raw anilistMediaResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, apperr.NewAPIError(err)
	}

	// AniList returns 200 with `errors: [{message:"Not Found.", status:404}]`
	// for unknown MAL ids; check that before the data check so we don't surface
	// a half-empty Media struct as if it were a hit.
	if len(raw.Errors) > 0 {
		return nil, apperr.NewIDNotFound(fmt.Sprintf("AniList MAL id %d: %s", malID, raw.Errors[0].Message))
	}

	m := raw.Data.Media
	if m.IDMal == 0 || m.ID == 0 {
		return nil, apperr.NewIDNotFound(fmt.Sprintf("AniList returned empty Media for MAL id %d", malID))
	}

	media := &AniListMedia{
		AniListID:       m.ID,
		MALID:           m.IDMal,
		TitleRomaji:     m.Title.Romaji,
		TitleEnglish:    m.Title.English,
		CoverImageExtra: m.CoverImage.ExtraLarge,
		BannerImage:     m.BannerImage,
	}
	if len(m.ExternalLinks) > 0 {
		media.ExternalLinks = make([]AniListExternalLink, len(m.ExternalLinks))
		for i, raw := range m.ExternalLinks {
			media.ExternalLinks[i] = AniListExternalLink{URL: raw.URL, Site: raw.Site}
		}
	}

	slog.Debug("anilist media resolved",
		"mal_id", m.IDMal,
		"anilist_id", m.ID,
		"title_en", media.TitleEnglish,
		"has_banner", media.BannerImage != nil,
	)

	return media, nil
}

// ParseMALID parses a numeric MAL id string. The id_type=kitsu/mal path
// segments accept plain numeric values; we don't try to validate Kitsu slugs
// because the resolver never receives them on this side — those go through
// KitsuClient.GetAnimeCtx instead.
func ParseMALID(s string) (uint64, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, apperr.NewInvalidIDType(fmt.Sprintf("mal id must be numeric: %q", s))
	}
	if id == 0 {
		return 0, apperr.NewInvalidIDType("mal id must be non-zero")
	}
	return id, nil
}

// LookupIMDBByMAL returns the IMDb id ("tt..." prefix) for the given MAL
// anime id, or nil if AniList doesn't carry the cross-reference. Backed
// by a process-local cache (positive + negative) so repeat lookups for the
// same MAL id — including the "no IMDB listed" outcome — never hit
// AniList twice.
//
// Coverage is limited: verified 2026-08-15 that 0/6 popular anime
// (Cowboy Bebop, One Piece, Attack on Titan, FMA Brotherhood, Death Note,
// Jujutsu Kaisen) had IMDb in AniList's externalLinks. Use as a best-effort
// translation; the caller falls back to direct MAL resolution when this
// returns nil.
func (c *AniListClient) LookupIMDBByMAL(ctx context.Context, malID uint64) *string {
	if c == nil || c.HTTP == nil || malID == 0 {
		return nil
	}

	// Cache fast path. The cache key is the MAL id; presence of "" means
	// "we already asked AniList and got back no IMDB link" — the negative
	// cache avoids re-hitting AniList for every request on the same anime.
	c.cacheMu.RLock()
	if v, ok := c.imdbByMAL[malID]; ok {
		c.cacheMu.RUnlock()
		if v == "" {
			return nil
		}
		return &v
	}
	c.cacheMu.RUnlock()

	// Slow path: ask AniList. Use the existing MediaByMALCtx which now
	// fetches externalLinks as part of the same query.
	media, err := c.MediaByMALCtx(ctx, malID)
	if err != nil || media == nil {
		// Cache the negative result so we don't retry on every request.
		c.cacheMu.Lock()
		c.imdbByMAL[malID] = ""
		c.cacheMu.Unlock()
		return nil
	}

	var imdbID string
	for _, link := range media.ExternalLinks {
		if link.Site == "IMDB" {
			imdbID = extractIMDBFromURL(link.URL)
			if imdbID != "" {
				break
			}
		}
	}

	// Cache both positive and negative results. Empty string = negative.
	c.cacheMu.Lock()
	c.imdbByMAL[malID] = imdbID
	c.cacheMu.Unlock()

	if imdbID == "" {
		return nil
	}
	return &imdbID
}
