package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// PublicMetaDB: a keyless fallback for Rotten Tomatoes (critics and
// audience), Metacritic, Letterboxd and Roger Ebert, looked up by TMDB id.
// Lookups are batched (up to 150 titles a request, after a short gather
// window); when several rows disagree on a score the most common value wins
// (ties: the most recently updated). A 429 pauses all lookups for its
// Retry-After.

const (
	publicMetaDBURL      = "https://api.publicmetadb.com/api/collections/ratings/records"
	publicMetaDBBatch    = 150
	publicMetaDBPageSize = 500
	publicMetaDBWindow   = 100 * time.Millisecond
	publicMetaDBTimeout  = 10 * time.Second
	publicMetaDBPause    = 30 * time.Second // a 429 without Retry-After
)

// pmdbKey is one title: TMDB id and "movie" / "tv".
type pmdbKey struct {
	TMDbID uint64
	Kind   string
}

// pmdbScores maps a rating source to its badge value.
type pmdbScores map[*RatingSource]string

type PublicMetaDBClient struct {
	HTTP        *http.Client
	URL         string
	pausedUntil atomic.Int64 // unix nanos
	batch       *batcher[pmdbKey, pmdbScores]
}

// publicMetaDB is the process-wide client FetchRatingsCtx falls back to
// (nil: disabled).
var publicMetaDB atomic.Pointer[PublicMetaDBClient]

// SetPublicMetaDB installs the PublicMetaDB fallback; nil disables it.
func SetPublicMetaDB(c *PublicMetaDBClient) { publicMetaDB.Store(c) }

func NewPublicMetaDBClient(httpClient *http.Client) *PublicMetaDBClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c := &PublicMetaDBClient{HTTP: httpClient, URL: publicMetaDBURL}
	c.batch = newBatcher(publicMetaDBWindow, publicMetaDBBatch, publicMetaDBTimeout, c.fetchBatch)
	return c
}

var errPublicMetaDBPaused = errors.New("PublicMetaDB rate limited, paused")

// ScoresCtx returns the scores PublicMetaDB has for a title (nil when none).
func (c *PublicMetaDBClient) ScoresCtx(ctx context.Context, tmdbID uint64, kind string) (pmdbScores, error) {
	if c == nil || tmdbID == 0 || (kind != "movie" && kind != "tv") {
		return nil, nil
	}
	if time.Now().UnixNano() < c.pausedUntil.Load() {
		return nil, errPublicMetaDBPaused
	}
	s, _, err := c.batch.get(ctx, pmdbKey{tmdbID, kind})
	return s, err
}

type pmdbRow struct {
	TMDbID    uint64          `json:"tmdb_id"`
	MediaType string          `json:"media_type"`
	Label     string          `json:"label"`
	Score     json.RawMessage `json:"score"`
	Updated   string          `json:"updated"`
}

func pmdbFilter(keys []pmdbKey) string {
	var groups []string
	for _, kind := range []string{"movie", "tv"} {
		var ids []string
		for _, k := range keys {
			if k.Kind == kind {
				ids = append(ids, "tmdb_id="+strconv.FormatUint(k.TMDbID, 10))
			}
		}
		if len(ids) > 0 {
			groups = append(groups, fmt.Sprintf(`(media_type="%s" && (%s))`, kind, strings.Join(ids, " || ")))
		}
	}
	return "(" + strings.Join(groups, " || ") + `) && label!~"overall"`
}

func (c *PublicMetaDBClient) fetchBatch(ctx context.Context, keys []pmdbKey) (map[pmdbKey]pmdbScores, error) {
	filter := neturl.QueryEscape(pmdbFilter(keys))
	var rows []pmdbRow
	for page := 1; page <= 20; page++ {
		u := fmt.Sprintf("%s?page=%d&perPage=%d&skipTotal=1&fields=tmdb_id,media_type,label,score,updated&filter=%s", c.URL, page, publicMetaDBPageSize, filter)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent())
		start := time.Now()
		resp, err := c.HTTP.Do(req)
		logSlow("PublicMetaDB ratings", time.Since(start).Milliseconds())
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamJSON))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			pause := publicMetaDBPause
			if d, ok := RetryAfter(resp); ok && d > 0 {
				pause = d
			}
			c.pausedUntil.Store(time.Now().Add(pause).UnixNano())
			return nil, errPublicMetaDBPaused
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("PublicMetaDB returned %d: %s", resp.StatusCode, errSnippet(body))
		}
		var out struct {
			Items []pmdbRow `json:"items"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		rows = append(rows, out.Items...)
		if len(out.Items) < publicMetaDBPageSize {
			break
		}
	}
	grouped := map[pmdbKey][]pmdbRow{}
	for _, r := range rows {
		kind := "movie"
		if r.MediaType == "tv" {
			kind = "tv"
		}
		k := pmdbKey{r.TMDbID, kind}
		grouped[k] = append(grouped[k], r)
	}
	res := make(map[pmdbKey]pmdbScores, len(grouped))
	for k, rs := range grouped {
		if s := pmdbConsensus(rs); len(s) > 0 {
			res[k] = s
		}
	}
	return res, nil
}

// pmdbSources maps PublicMetaDB labels to the sources it fills, with the
// badge format. Letterboxd is a 0–5 average of votes and Roger Ebert 0–4
// stars; the others are 0–100.
var pmdbSources = map[string]struct {
	src         *RatingSource
	format      string
	voteAverage bool // 0 means "no votes", not a score
}{
	"RT": {SourceRt, "%.0f%%", false},
	"PC": {SourceRtAudience, "%.0f%%", false},
	"MC": {SourceMetacritic, "%.0f", false},
	"LB": {SourceLetterboxd, "%.1f", true},
	"RE": {SourceEbert, "%.1f", false},
}

// pmdbConsensus picks, per source, the score most rows agree on (ties: the
// most recently updated); scores outside 0–100, and 0 for vote averages,
// are dropped.
func pmdbConsensus(rows []pmdbRow) pmdbScores {
	type tally struct {
		count  int
		latest string
	}
	by := map[string]map[float64]*tally{}
	for _, r := range rows {
		label := strings.ToUpper(strings.TrimSpace(r.Label))
		def, ok := pmdbSources[label]
		if !ok {
			continue
		}
		score, ok := pmdbScore(r.Score)
		if !ok || score < 0 || score > 100 || (score == 0 && def.voteAverage) {
			continue
		}
		v := math.Round(score*10) / 10
		if by[label] == nil {
			by[label] = map[float64]*tally{}
		}
		t := by[label][v]
		if t == nil {
			t = &tally{}
			by[label][v] = t
		}
		t.count++
		if r.Updated > t.latest {
			t.latest = r.Updated
		}
	}
	out := pmdbScores{}
	for label, values := range by {
		var best float64
		var bt *tally
		for v, t := range values {
			if bt == nil || t.count > bt.count || (t.count == bt.count && (t.latest > bt.latest || (t.latest == bt.latest && v > best))) {
				best, bt = v, t
			}
		}
		def := pmdbSources[label]
		out[def.src] = fmt.Sprintf(def.format, best)
	}
	return out
}

func pmdbScore(raw json.RawMessage) (float64, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// fillFromPublicMetaDB adds the PublicMetaDB scores for the sources the
// badges lack, keeping the merge order.
func fillFromPublicMetaDB(badges []RatingBadge, scores pmdbScores) []RatingBadge {
	if len(scores) == 0 {
		return badges
	}
	have := map[*RatingSource]bool{}
	for _, b := range badges {
		have[b.Source] = true
	}
	added := false
	for src, v := range scores {
		if !have[src] {
			badges = append(badges, RatingBadge{Source: src, Value: v})
			added = true
		}
	}
	if added {
		sortBadgesMergeOrder(badges)
	}
	return badges
}

// mergeOrder is the badge order mergeRatings produces.
var mergeOrder = []*RatingSource{SourceImdb, SourceTmdb, SourceRt, SourceRtAudience, SourceMetacritic, SourceTrakt, SourceLetterboxd, SourceMal, SourceMdblist, SourceEbert}

func sortBadgesMergeOrder(badges []RatingBadge) {
	rank := func(s *RatingSource) int {
		for i, x := range mergeOrder {
			if x == s {
				return i
			}
		}
		return len(mergeOrder)
	}
	for i := 1; i < len(badges); i++ {
		for j := i; j > 0 && rank(badges[j].Source) < rank(badges[j-1].Source); j-- {
			badges[j], badges[j-1] = badges[j-1], badges[j]
		}
	}
}

// pmdbFills reports whether PublicMetaDB could add a source the badges lack.
func pmdbFills(badges []RatingBadge) bool {
	have := map[*RatingSource]bool{}
	for _, b := range badges {
		have[b.Source] = true
	}
	for _, def := range pmdbSources {
		if !have[def.src] {
			return true
		}
	}
	return false
}
