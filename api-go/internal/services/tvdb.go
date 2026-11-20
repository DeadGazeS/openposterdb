package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	apperr "openposterdb/internal/errors"
)

// TVDB v4 client (enabled by TVDB_API_KEY). Used where TMDB has no
// cross-link: resolving tvdb: ids TMDB /find misses, backfilling the TVDB id
// Fanart indexes series by, and as the last artwork fallback.

const tvdbDefaultBaseURL = "https://api4.thetvdb.com/v4"

// tvdbTokenTTL: TVDB tokens are valid for a month; renew a little early.
const tvdbTokenTTL = 28 * 24 * time.Hour

// tvdbRecordTTL bounds how long extended records and remote-id lookups are
// reused (artwork and cross-links change rarely).
const tvdbRecordTTL = 24 * time.Hour

// tvdbMaxRetries / tvdbMaxBackoff: a 429 waits 1, 2, 4 s (Retry-After when
// given, capped at 30 s) before giving up.
const (
	tvdbMaxRetries = 3
	tvdbMaxBackoff = 30 * time.Second
)

// TVDB artwork type ids.
const (
	tvdbArtSeriesPoster     = 2
	tvdbArtSeriesBackground = 3
	tvdbArtSeriesLogo       = 23
	tvdbArtMoviePoster      = 14
	tvdbArtMovieBackground  = 15
	tvdbArtMovieLogo        = 25
)

type TVDBClient struct {
	apiKey  string
	HTTP    *http.Client
	BaseURL string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
	login    flightGroup[string]

	records *MemCache // "series/<id>" / "movie/<id>" → *TVDBRecord; "remote/<kind>/<id>" → uint64 (0 = none)
	// backoff is how long a 429 waits per attempt; tests shorten it.
	backoff func(attempt int) time.Duration
}

func NewTVDBClient(apiKey string, httpClient *http.Client) *TVDBClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &TVDBClient{
		apiKey:  apiKey,
		HTTP:    httpClient,
		BaseURL: tvdbDefaultBaseURL,
		records: NewMemCache(0, 20000, tvdbRecordTTL, 0),
		backoff: func(attempt int) time.Duration { return min(time.Second<<attempt, tvdbMaxBackoff) },
	}
}

// HasKey reports whether the client uses this API key.
func (c *TVDBClient) HasKey(key string) bool { return c != nil && c.apiKey == key }

// TVDBArtwork is one entry of an extended record's artworks.
type TVDBArtwork struct {
	ID       uint64  `json:"id"`
	Image    string  `json:"image"`
	Type     int     `json:"type"`
	Language *string `json:"language"`
	Score    float64 `json:"score"`
}

type tvdbRemoteID struct {
	ID         string `json:"id"`
	Type       int    `json:"type"`
	SourceName string `json:"sourceName"`
}

// TVDBRecord is the part of /series/{id}/extended and /movies/{id}/extended
// the resolver and artwork fallback use.
type TVDBRecord struct {
	ID               uint64
	IsMovie          bool
	Name             string
	OriginalLanguage string
	ReleaseDate      string // "YYYY-MM-DD" when known
	IMDbID           string
	TMDbID           uint64
	Artworks         []TVDBArtwork
}

type tvdbExtended struct {
	ID               uint64         `json:"id"`
	Name             string         `json:"name"`
	OriginalLanguage string         `json:"originalLanguage"`
	FirstAired       string         `json:"firstAired"`
	Year             string         `json:"year"`
	RemoteIDs        []tvdbRemoteID `json:"remoteIds"`
	Artworks         []TVDBArtwork  `json:"artworks"`
	FirstRelease     *struct {
		Date string `json:"date"`
	} `json:"first_release"`
}

func (e *tvdbExtended) record(isMovie bool) *TVDBRecord {
	r := &TVDBRecord{
		ID:               e.ID,
		IsMovie:          isMovie,
		Name:             strings.TrimSpace(e.Name),
		OriginalLanguage: e.OriginalLanguage,
		Artworks:         e.Artworks,
	}
	switch {
	case e.FirstAired != "":
		r.ReleaseDate = e.FirstAired
	case e.FirstRelease != nil && e.FirstRelease.Date != "":
		r.ReleaseDate = e.FirstRelease.Date
	}
	for _, rid := range e.RemoteIDs {
		src := strings.ToLower(rid.SourceName)
		id := strings.TrimSpace(rid.ID)
		switch {
		case strings.HasPrefix(src, "imdb") && strings.HasPrefix(id, "tt") && r.IMDbID == "":
			r.IMDbID = id
		case strings.HasPrefix(src, "themoviedb") && r.TMDbID == 0:
			if n, err := strconv.ParseUint(id, 10, 64); err == nil {
				r.TMDbID = n
			}
		}
	}
	return r
}

// token returns a valid login token. Concurrent callers share one login; a
// failed login is an error (never a not-found), so callers degrade instead
// of caching a miss.
func (c *TVDBClient) tokenCtx(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		t := c.token
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()
	return c.login.do(ctx, "login", func(fctx context.Context) (string, error) {
		c.mu.Lock()
		if c.token != "" && time.Now().Before(c.tokenExp) {
			t := c.token
			c.mu.Unlock()
			return t, nil
		}
		c.mu.Unlock()
		body, _ := json.Marshal(map[string]string{"apikey": c.apiKey})
		status, respBody, err := c.send(fctx, http.MethodPost, "/login", body, "")
		if err != nil {
			return "", apperr.NewOther("TVDB login failed: " + err.Error())
		}
		if status != http.StatusOK {
			return "", apperr.NewOther(fmt.Sprintf("TVDB login returned %d: %s", status, errSnippet(respBody)))
		}
		var out struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil || out.Data.Token == "" {
			return "", apperr.NewOther("TVDB login returned no token")
		}
		c.mu.Lock()
		c.token, c.tokenExp = out.Data.Token, time.Now().Add(tvdbTokenTTL)
		c.mu.Unlock()
		return out.Data.Token, nil
	})
}

func (c *TVDBClient) dropToken(token string) {
	c.mu.Lock()
	if c.token == token {
		c.token = ""
	}
	c.mu.Unlock()
}

// send performs one request, retrying a 429 with backoff.
func (c *TVDBClient) send(ctx context.Context, method, path string, body []byte, token string) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		start := time.Now()
		resp, err := c.HTTP.Do(req)
		logSlow("TVDB "+strings.SplitN(path, "?", 2)[0], time.Since(start).Milliseconds())
		if err != nil {
			return 0, nil, err
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
		resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= tvdbMaxRetries {
			return resp.StatusCode, respBody, nil
		}
		delay := c.backoff(attempt)
		if d, ok := RetryAfter(resp); ok {
			delay = min(d, tvdbMaxBackoff)
		}
		slog.Warn("TVDB 429, retrying", "attempt", attempt+1, "delay_ms", delay.Milliseconds())
		if err := sleepCtx(ctx, delay); err != nil {
			return 0, nil, err
		}
	}
}

// getJSON GETs an authenticated endpoint into out. 404 is an IDNotFound; an
// expired token (401) is renewed once.
func (c *TVDBClient) getJSON(ctx context.Context, path string, out any) error {
	for renewed := false; ; renewed = true {
		token, err := c.tokenCtx(ctx)
		if err != nil {
			return err
		}
		status, body, err := c.send(ctx, http.MethodGet, path, nil, token)
		if err != nil {
			return apperr.NewAPIError(err)
		}
		switch {
		case status == http.StatusUnauthorized && !renewed:
			c.dropToken(token)
			continue
		case status == http.StatusNotFound:
			return apperr.NewIDNotFound("TVDB " + path)
		case status != http.StatusOK:
			return apperr.NewOther(fmt.Sprintf("TVDB returned %d: %s", status, errSnippet(body)))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return apperr.NewAPIError(err)
		}
		return nil
	}
}

// SeriesCtx returns /series/{id}/extended (cached).
func (c *TVDBClient) SeriesCtx(ctx context.Context, id uint64) (*TVDBRecord, error) {
	return c.extended(ctx, false, id)
}

// MovieCtx returns /movies/{id}/extended (cached).
func (c *TVDBClient) MovieCtx(ctx context.Context, id uint64) (*TVDBRecord, error) {
	return c.extended(ctx, true, id)
}

var tvdbFlight flightGroup[*TVDBRecord]

func (c *TVDBClient) extended(ctx context.Context, isMovie bool, id uint64) (*TVDBRecord, error) {
	if c == nil {
		return nil, apperr.NewOther("TVDB client not configured")
	}
	kind, path := "series", fmt.Sprintf("/series/%d/extended", id)
	if isMovie {
		kind, path = "movie", fmt.Sprintf("/movies/%d/extended", id)
	}
	key := fmt.Sprintf("%s/%d", kind, id)
	if v, ok := c.records.Get(key); ok {
		if r, ok := v.(*TVDBRecord); ok {
			return r, nil
		}
		return nil, apperr.NewIDNotFound("TVDB " + key)
	}
	return tvdbFlight.do(ctx, fmt.Sprintf("%p|%s", c, key), func(fctx context.Context) (*TVDBRecord, error) {
		var out struct {
			Data *tvdbExtended `json:"data"`
		}
		if err := c.getJSON(fctx, path, &out); err != nil {
			if isIDNotFound(err) {
				c.records.Set(key, false, 1) // remembered miss
			}
			return nil, err
		}
		if out.Data == nil || out.Data.ID == 0 {
			c.records.Set(key, false, 1)
			return nil, apperr.NewIDNotFound("TVDB " + key)
		}
		r := out.Data.record(isMovie)
		c.records.Set(key, r, 1)
		return r, nil
	})
}

// RemoteIDCtx returns the TVDB id of the series (or movie, for
// MediaTypeMovie) carrying the given IMDb ("tt…") or TMDB id, via
// /search/remoteid. 0 with a nil error means TVDB has no such entry. The
// response holds one object per matched entity in no set order (a season can
// come before its series), so the entry of the requested kind is picked.
func (c *TVDBClient) RemoteIDCtx(ctx context.Context, remoteID string, kind MediaType) (uint64, error) {
	if c == nil || remoteID == "" {
		return 0, nil
	}
	field := "series"
	if kind == MediaTypeMovie {
		field = "movie"
	}
	key := "remote/" + field + "/" + remoteID
	if v, ok := c.records.Get(key); ok {
		return v.(uint64), nil
	}
	var out struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	err := c.getJSON(ctx, "/search/remoteid/"+neturl.PathEscape(remoteID), &out)
	if err != nil && !isIDNotFound(err) {
		return 0, err
	}
	id := tvdbIDFromRemoteResults(out.Data, field)
	c.records.Set(key, id, 1)
	return id, nil
}

func tvdbIDFromRemoteResults(results []map[string]json.RawMessage, field string) uint64 {
	for _, entry := range results {
		raw, ok := entry[field]
		if !ok {
			continue
		}
		var v struct {
			ID uint64 `json:"id"`
		}
		if json.Unmarshal(raw, &v) == nil && v.ID != 0 {
			return v.ID
		}
	}
	return 0
}

// TVDBLanguage converts a request language ("de", "pt-BR", "en-US") to
// TVDB's 3-letter code. pt-BR is TVDB's "pt", Portuguese otherwise "por".
// Unknown codes fall back to "eng".
func TVDBLanguage(lang string) string {
	if strings.EqualFold(lang, "pt-BR") {
		return "pt"
	}
	base := strings.ToLower(LangBase(lang))
	if base == "" {
		return "eng"
	}
	if code, ok := tvdbLang3[base]; ok {
		return code
	}
	return "eng"
}

var tvdbLang3 = map[string]string{
	"en": "eng", "de": "deu", "fr": "fra", "es": "spa", "it": "ita", "pt": "por", "nl": "nld",
	"sv": "swe", "no": "nor", "nb": "nor", "da": "dan", "fi": "fin", "pl": "pol", "cs": "ces",
	"sk": "slk", "sl": "slv", "hu": "hun", "ro": "ron", "bg": "bul", "hr": "hrv", "sr": "srp",
	"ru": "rus", "uk": "ukr", "el": "ell", "tr": "tur", "he": "heb", "ar": "ara", "fa": "fas",
	"hi": "hin", "th": "tha", "vi": "vie", "id": "ind", "ms": "msa", "ja": "jpn", "ko": "kor",
	"zh": "zho", "et": "est", "lv": "lav", "lt": "lit", "ca": "cat", "is": "isl", "ta": "tam",
	"te": "tel", "bn": "ben",
}

// tvdbLanguageChain is the language order for text-bearing artwork: the
// requested language, its Portuguese sibling, then English.
func tvdbLanguageChain(code string) []string {
	chain := []string{code}
	switch code {
	case "por":
		chain = append(chain, "pt")
	case "pt":
		chain = append(chain, "por")
	}
	if code != "eng" {
		chain = append(chain, "eng")
	}
	return chain
}

// SelectTVDBArtwork picks the image URL for kind ("poster", "backdrop",
// "logo") from a record's artworks: highest score within the first matching
// language tier. Posters and logos go lang → Portuguese sibling → eng →
// untagged → any; backgrounds (and textless posters) prefer untagged art
// first. "" when the record has none of that type.
func SelectTVDBArtwork(r *TVDBRecord, kind, lang string, textless bool) string {
	if r == nil {
		return ""
	}
	var artType int
	switch kind {
	case "poster":
		artType = tvdbArtSeriesPoster
		if r.IsMovie {
			artType = tvdbArtMoviePoster
		}
	case "backdrop":
		artType = tvdbArtSeriesBackground
		if r.IsMovie {
			artType = tvdbArtMovieBackground
		}
	case "logo":
		artType = tvdbArtSeriesLogo
		if r.IsMovie {
			artType = tvdbArtMovieLogo
		}
	default:
		return ""
	}
	var candidates []TVDBArtwork
	for _, a := range r.Artworks {
		// Movies file backgrounds under the series type (3) too.
		if a.Type == artType || (kind == "backdrop" && r.IsMovie && a.Type == tvdbArtSeriesBackground) {
			if strings.HasPrefix(a.Image, "https://") {
				candidates = append(candidates, a)
			}
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	best := func(match func(*string) bool) string {
		var pick *TVDBArtwork
		for i := range candidates {
			a := &candidates[i]
			if match(a.Language) && (pick == nil || a.Score > pick.Score) {
				pick = a
			}
		}
		if pick == nil {
			return ""
		}
		return pick.Image
	}
	untagged := func(l *string) bool { return l == nil || *l == "" }
	var tiers []func(*string) bool
	if kind == "backdrop" || textless {
		tiers = append(tiers, untagged)
	}
	for _, code := range tvdbLanguageChain(TVDBLanguage(lang)) {
		tiers = append(tiers, func(l *string) bool { return l != nil && *l == code })
	}
	tiers = append(tiers, untagged, func(*string) bool { return true })
	for _, tier := range tiers {
		if img := best(tier); img != "" {
			return img
		}
	}
	return ""
}

// TVDBArtworkHost is the CDN TVDB artwork is served from.
const TVDBArtworkHost = "artworks.thetvdb.com"
