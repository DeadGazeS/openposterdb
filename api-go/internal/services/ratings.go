package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	apperr "openposterdb/internal/errors"
)

type RatingSource struct {
	Key       string
	Label     string
	CacheChar byte
	ColorR    uint8
	ColorG    uint8
	ColorB    uint8
}

var (
	SourceMal        = &RatingSource{Key: "mal", Label: "MyAnimeList", CacheChar: 'e', ColorR: 34, ColorG: 60, ColorB: 120}
	SourceImdb       = &RatingSource{Key: "imdb", Label: "IMDb", CacheChar: 'i', ColorR: 180, ColorG: 145, ColorB: 15}
	SourceLetterboxd = &RatingSource{Key: "lb", Label: "Letterboxd", CacheChar: 'l', ColorR: 0, ColorG: 155, ColorB: 88}
	SourceRt         = &RatingSource{Key: "rt", Label: "Rotten Tomatoes Critics", CacheChar: 'c', ColorR: 185, ColorG: 35, ColorB: 8}
	SourceRtAudience = &RatingSource{Key: "rta", Label: "Rotten Tomatoes Audience", CacheChar: 'a', ColorR: 185, ColorG: 35, ColorB: 8}
	SourceMetacritic = &RatingSource{Key: "mc", Label: "Metacritic", CacheChar: 'm', ColorR: 75, ColorG: 150, ColorB: 38}
	SourceTmdb       = &RatingSource{Key: "tmdb", Label: "TMDB", CacheChar: 't', ColorR: 1, ColorG: 155, ColorB: 88}
	SourceTrakt      = &RatingSource{Key: "trakt", Label: "Trakt", CacheChar: 'k', ColorR: 175, ColorG: 15, ColorB: 45}
	SourceMdblist    = &RatingSource{Key: "mdblist", Label: "MDBList", CacheChar: 'd', ColorR: 66, ColorG: 132, ColorB: 202}
	SourceEbert      = &RatingSource{Key: "ebert", Label: "Roger Ebert", CacheChar: 'r', ColorR: 232, ColorG: 89, ColorB: 12}
)

var allSources = []*RatingSource{
	SourceMal, SourceImdb, SourceLetterboxd, SourceRt, SourceRtAudience,
	SourceMetacritic, SourceTmdb, SourceTrakt, SourceMdblist, SourceEbert,
}

var sourceByKey = buildSourceMap()
var sourceByChar = buildCharMap()

func buildSourceMap() map[string]*RatingSource {
	m := make(map[string]*RatingSource)
	for _, s := range allSources {
		m[s.Key] = s
	}
	return m
}

func buildCharMap() map[byte]*RatingSource {
	m := make(map[byte]*RatingSource)
	for _, s := range allSources {
		m[s.CacheChar] = s
	}
	return m
}

func SourceFromKey(key string) *RatingSource {
	return sourceByKey[key]
}

func SourceFromCacheChar(c byte) *RatingSource {
	return sourceByChar[c]
}

func AllSourceKeys() []string {
	keys := make([]string, len(allSources))
	for i, s := range allSources {
		keys[i] = s.Key
	}
	return keys
}

type RatingBadge struct {
	Source *RatingSource
	Value  string
}

type RatingsResult struct {
	Badges []RatingBadge
	TmdbID *uint64
	TvdbID *uint64
	ImdbID *string
}

var canonicalOrder = []string{"mal", "imdb", "lb", "rt", "rta", "mc", "tmdb", "trakt", "mdblist", "ebert"}

func badgeToKey(badge *RatingBadge) string {
	return badge.Source.Key
}

// --- Badges cache suffix ---

func BadgesCacheSuffix(badges []RatingBadge) string {
	chars := make([]byte, len(badges))
	for i, b := range badges {
		chars[i] = b.Source.CacheChar
	}
	return "@" + string(chars)
}

// --- Available sources string ---

func AvailableSourcesString(badges []RatingBadge) string {
	sources := make([]*RatingSource, len(badges))
	for i, b := range badges {
		sources[i] = b.Source
	}
	sort.Slice(sources, func(i, j int) bool {
		return indexOf(canonicalOrder, sources[i].Key) < indexOf(canonicalOrder, sources[j].Key)
	})

	var result strings.Builder
	seen := make(map[byte]bool)
	for _, s := range sources {
		if !seen[s.CacheChar] {
			result.WriteByte(s.CacheChar)
			seen[s.CacheChar] = true
		}
	}
	return result.String()
}

// ValidSourcesString reports whether s is an AvailableSourcesString (every
// byte a known source's cache char). The preview stores JSON badges in the
// same table for its demo title; those never match.
func ValidSourcesString(s string) bool {
	for i := 0; i < len(s); i++ {
		if SourceFromCacheChar(s[i]) == nil {
			return false
		}
	}
	return true
}

// --- Exclude cache token ---

func ExcludeCacheToken(exclude string) string {
	sources := parseOrder(exclude)
	sort.Slice(sources, func(i, j int) bool {
		return indexOf(canonicalOrder, sources[i].Key) < indexOf(canonicalOrder, sources[j].Key)
	})

	var result strings.Builder
	seen := make(map[byte]bool)
	for _, s := range sources {
		if !seen[s.CacheChar] {
			result.WriteByte(s.CacheChar)
			seen[s.CacheChar] = true
		}
	}
	return result.String()
}

// --- Parse order ---

func parseOrder(order string) []*RatingSource {
	if order == "" {
		return nil
	}
	var sources []*RatingSource
	for key := range strings.SplitSeq(order, ",") {
		key = strings.TrimSpace(key)
		if s := SourceFromKey(key); s != nil {
			sources = append(sources, s)
		}
	}
	return sources
}

// --- Order and limit ---

func OrderAndLimit(sources []*RatingSource, order, exclude string, limit int32) []*RatingSource {
	if limit == 0 {
		return nil
	}

	excluded := parseOrder(exclude)
	if len(excluded) > 0 {
		excludeSet := make(map[*RatingSource]bool)
		for _, s := range excluded {
			excludeSet[s] = true
		}
		filtered := make([]*RatingSource, 0, len(sources))
		for _, s := range sources {
			if !excludeSet[s] {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}

	var result []*RatingSource
	if order == "" {
		result = sources
	} else {
		preferred := parseOrder(order)
		seen := make(map[*RatingSource]bool)
		for _, src := range preferred {
			for _, s := range sources {
				if s == src && !seen[s] {
					result = append(result, s)
					seen[s] = true
				}
			}
		}
		for _, s := range sources {
			if !seen[s] {
				result = append(result, s)
				seen[s] = true
			}
		}
	}

	// A negative limit means "no limit" (it used to slice out of range).
	if limit > 0 && len(result) > int(limit) {
		result = result[:limit]
	}
	return result
}

// --- Badges suffix from available ---

func BadgesSuffixFromAvailable(availableSources, order, exclude string, limit int32) string {
	var sources []*RatingSource
	for i := 0; i < len(availableSources); i++ {
		if s := SourceFromCacheChar(availableSources[i]); s != nil {
			sources = append(sources, s)
		}
	}

	ordered := OrderAndLimit(sources, order, exclude, limit)

	chars := make([]byte, len(ordered))
	for i, s := range ordered {
		chars[i] = s.CacheChar
	}
	return "@" + string(chars)
}

// --- Ratings cache suffix ---

func RatingsCacheSuffix(order, exclude string, limit int32) string {
	if limit == 0 {
		return "@"
	}

	sources := parseOrder(order)

	for _, key := range canonicalOrder {
		s := sourceByKey[key]
		found := slices.Contains(sources, s)
		if !found {
			sources = append(sources, s)
		}
	}

	excluded := parseOrder(exclude)
	if len(excluded) > 0 {
		excludeSet := make(map[*RatingSource]bool)
		for _, s := range excluded {
			excludeSet[s] = true
		}
		filtered := make([]*RatingSource, 0, len(sources))
		for _, s := range sources {
			if !excludeSet[s] {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}

	if len(sources) > int(limit) {
		sources = sources[:limit]
	}

	chars := make([]byte, len(sources))
	for i, s := range sources {
		chars[i] = s.CacheChar
	}
	return "@" + string(chars)
}

// --- Apply rating preferences ---

func ApplyRatingPreferences(badges []RatingBadge, order, exclude string, limit int32) []RatingBadge {
	if len(badges) == 0 {
		return nil
	}

	sources := make([]*RatingSource, len(badges))
	for i, b := range badges {
		sources[i] = b.Source
	}

	orderedSources := OrderAndLimit(sources, order, exclude, limit)

	result := make([]RatingBadge, 0, len(orderedSources))
	for _, src := range orderedSources {
		for _, badge := range badges {
			if badge.Source == src {
				result = append(result, badge)
				break
			}
		}
	}
	return result
}

// --- MDBList badges ---

func MdblistBadges(resp *MdblistResponse) []RatingBadge {
	var badges []RatingBadge

	// fewVotes: MDBList reports a vote count for a source and it is too low
	// to mean anything (user-vote sources only, see minRatingVotes).
	fewVotes := func(r *MdblistRating) bool {
		return r.Votes != nil && *r.Votes < minRatingVotes
	}
	for i := range resp.Ratings {
		r := &resp.Ratings[i]
		var badge *RatingBadge
		switch r.Source {
		case "imdb":
			if r.Value != nil && *r.Value > 0 && !fewVotes(r) {
				badge = &RatingBadge{Source: SourceImdb, Value: fmt.Sprintf("%.1f", *r.Value)}
			}
		case "trakt":
			if r.Score != nil && *r.Score > 0 && !fewVotes(r) {
				badge = &RatingBadge{Source: SourceTrakt, Value: fmt.Sprintf("%.0f%%", *r.Score)}
			}
		case "letterboxd":
			// From the 0–100 score: MDBList's batch POST reports the value
			// on a 0–10 scale, the single GET on Letterboxd's own 0–5.
			if fewVotes(r) {
				break
			}
			if r.Score != nil && *r.Score > 0 {
				badge = &RatingBadge{Source: SourceLetterboxd, Value: fmt.Sprintf("%.1f", *r.Score/20)}
			} else if r.Value != nil && *r.Value > 0 {
				badge = &RatingBadge{Source: SourceLetterboxd, Value: fmt.Sprintf("%.1f", *r.Value)}
			}
		case "popcorn":
			if r.Score != nil && *r.Score > 0 && !fewVotes(r) {
				badge = &RatingBadge{Source: SourceRtAudience, Value: fmt.Sprintf("%.0f%%", *r.Score)}
			}
		case "tomatoes":
			if r.Score != nil && *r.Score > 0 {
				badge = &RatingBadge{Source: SourceRt, Value: fmt.Sprintf("%.0f%%", *r.Score)}
			}
		case "metacritic":
			if r.Score != nil && *r.Score > 0 {
				badge = &RatingBadge{Source: SourceMetacritic, Value: fmt.Sprintf("%.0f", *r.Score)}
			}
		case "myanimelist":
			if r.Score != nil && *r.Score > 0 && !fewVotes(r) {
				badge = &RatingBadge{Source: SourceMal, Value: fmt.Sprintf("%.2f", *r.Score/10.0)}
			}
		case "rogerebert":
			if r.Value != nil && *r.Value > 0 {
				badge = &RatingBadge{Source: SourceEbert, Value: fmt.Sprintf("%.1f", *r.Value)}
			}
		}
		if badge != nil {
			badges = append(badges, *badge)
		}
	}

	if resp.Score != nil && *resp.Score > 0 {
		badges = append(badges, RatingBadge{Source: SourceMdblist, Value: fmt.Sprintf("%.0f", *resp.Score)})
	}

	return badges
}

// --- Trakt badge ---

func TraktBadge(rating float64, votes uint64) *RatingBadge {
	if rating <= 0 || votes < minRatingVotes {
		return nil
	}
	return &RatingBadge{Source: SourceTrakt, Value: fmt.Sprintf("%.0f%%", rating*10)}
}

// --- Fetch ratings (parallel) ---

// RatingsClients bundles the rating-provider clients used by FetchRatings.
type RatingsClients struct {
	TMDB    *TmdbClient
	OMDB    *OmdbClient
	MDBList *MdblistClient
	Trakt   *TraktClient
}

// RatingsQuery identifies the title whose ratings are being fetched.
type RatingsQuery struct {
	ResolvedTMDbID    uint64
	MediaType         string
	IMDbID            *string
	EpisodeShowTMDbID uint64
	EpisodeSeason     uint32
	EpisodeEpisode    uint32
	// ReleaseDate ("YYYY-MM-DD") sets how long the badges are cached
	// (ratingsRefreshFor).
	ReleaseDate *string
}

// FetchRatingsResult carries the ratings fetch outcome: the auxiliary provider
// responses, the merged badge list and how many providers were asked / failed
// (a failure is an error, not "no rating for this title").
type FetchRatingsResult struct {
	Mdblist   *MdblistResponse
	OMDB      *OmdbResponse
	Badges    []RatingBadge
	Attempted int
	Failed    int
}

// AllFailed: every provider that was asked failed — the empty result says
// nothing about the title.
func (r FetchRatingsResult) AllFailed() bool { return r.Attempted > 0 && r.Failed == r.Attempted }

// slowRatingsMs: a ratings fetch slower than this is logged with the time of
// each provider (Rust SLOW_RATINGS_MS).
const slowRatingsMs = 2000

// providerOutcome is one provider's part of a ratings fetch.
type providerOutcome struct {
	asked  bool
	err    error
	keyRef string // hashed key, for the failure log
	took   time.Duration
}

func FetchRatingsCtx(ctx context.Context, clients RatingsClients, q RatingsQuery) FetchRatingsResult {
	start := time.Now()
	var (
		tmdbBadge  *RatingBadge
		omdbResp   *OmdbResponse
		mdbResp    *MdblistResponse
		traktBadge *RatingBadge
		outcomes   [4]providerOutcome // tmdb, omdb, mdblist, trakt
		wg         sync.WaitGroup
	)
	run := func(i int, f func() (string, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.Now()
			key, err := f()
			outcomes[i] = providerOutcome{asked: true, err: err, keyRef: key, took: time.Since(t)}
		}()
	}
	hasIMDb := q.IMDbID != nil && *q.IMDbID != ""

	if clients.TMDB != nil && (q.ResolvedTMDbID != 0 || q.MediaType == "episode") {
		run(0, func() (string, error) {
			var err error
			tmdbBadge, err = fetchTmdbRatingCtx(ctx, clients.TMDB, q.ResolvedTMDbID, q.MediaType, q.EpisodeShowTMDbID, q.EpisodeSeason, q.EpisodeEpisode)
			return "", err
		})
	}
	if clients.OMDB != nil && hasIMDb {
		run(1, func() (string, error) {
			var err error
			omdbResp, err = clients.OMDB.GetRatingsCtx(ctx, *q.IMDbID)
			return clients.OMDB.KeyPool.ActiveKeyHash(), err
		})
	}
	if clients.MDBList != nil && q.MediaType != "episode" {
		run(2, func() (string, error) {
			var err error
			if hasIMDb {
				mdbResp, err = clients.MDBList.GetRatingsCtx(ctx, *q.IMDbID, q.MediaType)
			} else {
				mdbResp, err = clients.MDBList.GetRatingsByTMDBCtx(ctx, q.ResolvedTMDbID, q.MediaType)
			}
			// A series without a TVDB id on TMDB can still get Fanart art
			// through the id MDBList knows (SeriesTVDBID).
			if mdbResp != nil && q.MediaType == "tv" && mdbResp.IDs.TVDB != nil {
				RememberTVDBID(q.ResolvedTMDbID, *mdbResp.IDs.TVDB)
			}
			return clients.MDBList.KeyPool.ActiveKeyHash(), err
		})
	}
	if clients.Trakt != nil && (hasIMDb || q.MediaType == "episode") {
		run(3, func() (string, error) {
			var err error
			traktBadge, err = fetchTraktRatingCtx(ctx, clients, q)
			return clients.Trakt.KeyPool.ActiveKeyHash(), err
		})
	}
	wg.Wait()

	result := FetchRatingsResult{Mdblist: mdbResp, OMDB: omdbResp}
	names := [4]string{"tmdb", "omdb", "mdblist", "trakt"}
	for i, o := range outcomes {
		if !o.asked {
			continue
		}
		result.Attempted++
		if o.err != nil {
			result.Failed++
			attrs := []any{"provider", names[i], "media_type", q.MediaType, "tmdb_id", q.ResolvedTMDbID, "error", apperr.RedactURLSecrets(o.err)}
			if hasIMDb {
				attrs = append(attrs, "imdb_id", *q.IMDbID)
			}
			if o.keyRef != "" {
				attrs = append(attrs, "key", o.keyRef)
			}
			slog.Warn("rating provider failed", attrs...)
		}
	}
	if total := time.Since(start).Milliseconds(); total > slowRatingsMs {
		slog.Warn("slow ratings fetch", "tmdb_id", q.ResolvedTMDbID, "media_type", q.MediaType, "total_ms", total,
			"tmdb_ms", outcomes[0].took.Milliseconds(), "omdb_ms", outcomes[1].took.Milliseconds(),
			"mdblist_ms", outcomes[2].took.Milliseconds(), "trakt_ms", outcomes[3].took.Milliseconds())
	}

	result.Badges = mergeRatings(tmdbBadge, omdbBadges(omdbResp), mdbResp, traktBadge)
	if hasIMDb {
		result.Badges = withDatasetIMDb(result.Badges, imdbDataset.Load().Badge(*q.IMDbID))
	}
	// Keyless fallback for the critic sources MDBList / OMDb didn't fill.
	if pmdb := publicMetaDB.Load(); pmdb != nil && q.ResolvedTMDbID != 0 && (q.MediaType == "movie" || q.MediaType == "tv") && pmdbFills(result.Badges) {
		result.Attempted++
		scores, err := pmdb.ScoresCtx(ctx, q.ResolvedTMDbID, q.MediaType)
		if err != nil {
			result.Failed++
			if !errors.Is(err, errPublicMetaDBPaused) {
				slog.Warn("rating provider failed", "provider", "publicmetadb", "media_type", q.MediaType, "tmdb_id", q.ResolvedTMDbID, "error", err)
			}
		}
		result.Badges = fillFromPublicMetaDB(result.Badges, scores)
	}
	return result
}

// withDatasetIMDb makes the IMDb dataset's rating (when it has the title)
// the IMDb badge: it is IMDb's own number, ahead of MDBList's and OMDb's
// copies. IMDb is first in the merge order.
func withDatasetIMDb(badges []RatingBadge, dataset *RatingBadge) []RatingBadge {
	if dataset == nil {
		return badges
	}
	for i := range badges {
		if badges[i].Source == SourceImdb {
			badges[i] = *dataset
			return badges
		}
	}
	return append([]RatingBadge{*dataset}, badges...)
}

// mergeRatings builds the badge list in the Rust order — IMDb, TMDB, RT,
// RT Audience, Metacritic, Trakt, Letterboxd, MAL, MDBList, Ebert — per
// source preferring MDBList, with OMDb filling IMDb / RT / Metacritic and the
// direct Trakt call filling Trakt when MDBList has no value.
func mergeRatings(tmdb *RatingBadge, omdb []RatingBadge, mdb *MdblistResponse, trakt *RatingBadge) []RatingBadge {
	var mdbBadges []RatingBadge
	if mdb != nil {
		mdbBadges = MdblistBadges(mdb)
	}
	find := func(list []RatingBadge, src *RatingSource) *RatingBadge {
		for i := range list {
			if list[i].Source == src {
				return &list[i]
			}
		}
		return nil
	}
	firstOf := func(bs ...*RatingBadge) *RatingBadge {
		for _, b := range bs {
			if b != nil {
				return b
			}
		}
		return nil
	}
	ordered := []*RatingBadge{
		firstOf(find(mdbBadges, SourceImdb), find(omdb, SourceImdb)),
		tmdb,
		firstOf(find(mdbBadges, SourceRt), find(omdb, SourceRt)),
		find(mdbBadges, SourceRtAudience),
		firstOf(find(mdbBadges, SourceMetacritic), find(omdb, SourceMetacritic)),
		firstOf(find(mdbBadges, SourceTrakt), trakt),
		find(mdbBadges, SourceLetterboxd),
		find(mdbBadges, SourceMal),
		find(mdbBadges, SourceMdblist),
		find(mdbBadges, SourceEbert),
	}
	var out []RatingBadge
	for _, b := range ordered {
		if b != nil {
			out = append(out, *b)
		}
	}
	return out
}

// omdbBadges extracts IMDb, Rotten Tomatoes and Metacritic from an OMDb
// response, skipping OMDb's "N/A" placeholders.
func omdbBadges(resp *OmdbResponse) []RatingBadge {
	if resp == nil {
		return nil
	}
	var out []RatingBadge
	if resp.IMDBRating != nil && *resp.IMDBRating != "N/A" && *resp.IMDBRating != "" {
		out = append(out, RatingBadge{Source: SourceImdb, Value: *resp.IMDBRating})
	}
	for _, r := range resp.Ratings {
		if r.Source == "Rotten Tomatoes" && r.Value != "N/A" && r.Value != "" {
			out = append(out, RatingBadge{Source: SourceRt, Value: r.Value})
			break
		}
	}
	if resp.Metascore != nil && *resp.Metascore != "N/A" && *resp.Metascore != "" {
		out = append(out, RatingBadge{Source: SourceMetacritic, Value: *resp.Metascore})
	}
	return out
}

// fetchTraktRatingCtx gets the Trakt rating: movies and shows by IMDb id,
// episodes by the show's IMDb id (looked up on TMDB) plus season/episode. A
// title Trakt doesn't know is (nil, nil).
func fetchTraktRatingCtx(ctx context.Context, clients RatingsClients, q RatingsQuery) (*RatingBadge, error) {
	var r *TraktRatingsResponse
	var err error
	switch q.MediaType {
	case "movie":
		r, err = clients.Trakt.GetMovieRatingCtx(ctx, *q.IMDbID)
	case "tv":
		r, err = clients.Trakt.GetShowRatingCtx(ctx, *q.IMDbID)
	case "episode":
		if clients.TMDB == nil || q.EpisodeShowTMDbID == 0 {
			return nil, nil
		}
		showIMDb, lookupErr := showIMDbIDCtx(ctx, clients.TMDB, q.EpisodeShowTMDbID)
		if lookupErr != nil || showIMDb == "" {
			return nil, lookupErr
		}
		r, err = clients.Trakt.GetEpisodeRatingCtx(ctx, showIMDb, q.EpisodeSeason, q.EpisodeEpisode)
	default:
		return nil, nil
	}
	if err != nil || r == nil {
		return nil, err
	}
	return TraktBadge(r.Rating, r.Votes), nil
}

// showIMDbIDs caches a show's IMDb id by TMDB id for the Trakt episode path
// (every episode of a show would otherwise look it up again).
var showIMDbIDs = NewMemCache(0, 20000, 24*time.Hour, 0)

func showIMDbIDCtx(ctx context.Context, tmdb *TmdbClient, showTMDbID uint64) (string, error) {
	key := strconv.FormatUint(showTMDbID, 10)
	if v, ok := showIMDbIDs.Get(key); ok {
		return v.(string), nil
	}
	var ids struct {
		IMDbID *string `json:"imdb_id"`
	}
	if err := tmdb.GetCtx(ctx, fmt.Sprintf("/tv/%d/external_ids", showTMDbID), nil, &ids); err != nil {
		if isIDNotFound(err) {
			return "", nil
		}
		return "", err
	}
	id := ""
	if ids.IMDbID != nil {
		id = *ids.IMDbID
	}
	showIMDbIDs.Set(key, id, 1)
	return id, nil
}

// Refresh ages of cached badges, by outcome. Past its age an entry is still
// served (stale-while-revalidate) while one background refresh runs, so a
// cached image never waits on the rating providers to build its cache key.
var (
	// ratingsRefreshAfter: titles released 30+ days ago, whose ratings move
	// slowly.
	ratingsRefreshAfter = 24 * time.Hour
	// newReleaseRefreshAfter: titles younger than newReleaseAge, or with no
	// known release date.
	newReleaseRefreshAfter = 6 * time.Hour
	// partialRatingsTTL: at least one provider failed, so badges may be
	// missing; retried sooner.
	partialRatingsTTL = 30 * time.Minute
	// emptyRatingsTTL: every provider failed. The previous badges (if any)
	// are kept and retried after this; an expired entry without badges is
	// refetched synchronously.
	emptyRatingsTTL = 2 * time.Minute
	// ratingsStaleServeMax caps how long badges are served after their last
	// successful fetch while refreshes keep failing. Longer than
	// ratingsRefreshAfter, so the daily refresh runs in the background.
	ratingsStaleServeMax = 72 * time.Hour
)

const newReleaseAge = 30 * 24 * time.Hour

// ratingsRefreshFor returns how long a successful fetch is trusted.
func ratingsRefreshFor(releaseDate *string, now time.Time) time.Duration {
	if releaseDate == nil {
		return newReleaseRefreshAfter
	}
	t, err := time.Parse("2006-01-02", *releaseDate)
	if err != nil || now.Sub(t) < newReleaseAge {
		return newReleaseRefreshAfter
	}
	return ratingsRefreshAfter
}

type ratingsEntry struct {
	badges       []RatingBadge // empty = no ratings (or every provider failed)
	refreshAfter time.Time
	fetched      time.Time // last non-empty fetch; staleServeMax counts from here
}

var (
	ratingsFlight     flightGroup[[]RatingBadge]
	ratingsRefreshing sync.Map // flight key -> struct{}: one background refresh at a time
)

// ratingsCacheKey is "tmdbID/mediaType" or "showID/episode/SsEe" (matching
// the Rust ratings_cache). Titles without a TMDB id (IMDb-only targets) are
// keyed by IMDb id — they all used to share "0/movie".
func ratingsCacheKey(q RatingsQuery) string {
	if q.MediaType == "episode" {
		return fmt.Sprintf("%d/episode/S%dE%d", q.EpisodeShowTMDbID, q.EpisodeSeason, q.EpisodeEpisode)
	}
	if q.ResolvedTMDbID == 0 && q.IMDbID != nil && *q.IMDbID != "" {
		return "imdb:" + *q.IMDbID + "/" + q.MediaType
	}
	return fmt.Sprintf("%d/%s", q.ResolvedTMDbID, q.MediaType)
}

// FetchRatingsCachedCtx fetches ratings and caches the resulting badges in the
// given in-memory cache (see ratingsCacheKey). A nil cache bypasses caching.
// Concurrent misses for the same title share one fetch; entries past
// ratingsRefreshAfter are served while a background refresh runs. Only the
// badge list is cached; the auxiliary responses are unused by all callers.
func FetchRatingsCachedCtx(ctx context.Context, cache *MemCache, clients RatingsClients, q RatingsQuery) []RatingBadge {
	return fetchRatingsCached(ctx, cache, clients, q, true)
}

// FetchRatingsFreshCtx is FetchRatingsCachedCtx without stale-while-
// revalidate: an entry past its refresh age is refetched synchronously. For
// background image refreshes, which would otherwise re-render with the very
// badges they are meant to replace.
func FetchRatingsFreshCtx(ctx context.Context, cache *MemCache, clients RatingsClients, q RatingsQuery) []RatingBadge {
	return fetchRatingsCached(ctx, cache, clients, q, false)
}

func fetchRatingsCached(ctx context.Context, cache *MemCache, clients RatingsClients, q RatingsQuery, allowStale bool) []RatingBadge {
	if cache == nil {
		return FetchRatingsCtx(ctx, clients, q).Badges
	}
	key := ratingsCacheKey(q)
	flightKey := fmt.Sprintf("%p|%s", cache, key)
	if e, ok := cachedRatings(cache, key); ok {
		if time.Now().Before(e.refreshAfter) {
			return e.badges
		}
		if allowStale && len(e.badges) > 0 && time.Since(e.fetched) <= ratingsStaleServeMax {
			if _, running := ratingsRefreshing.LoadOrStore(flightKey, struct{}{}); !running {
				go func() {
					defer ratingsRefreshing.Delete(flightKey)
					ratingsFlight.do(context.Background(), flightKey, func(fctx context.Context) ([]RatingBadge, error) {
						return refreshRatings(fctx, cache, clients, q, key), nil
					})
				}()
			}
			return e.badges
		}
	}
	badges, _ := ratingsFlight.do(ctx, flightKey, func(fctx context.Context) ([]RatingBadge, error) {
		if e, ok := cachedRatings(cache, key); ok && time.Now().Before(e.refreshAfter) {
			return e.badges, nil
		}
		return refreshRatings(fctx, cache, clients, q, key), nil
	})
	return badges
}

// refreshRatings fetches and stores the badges. How long they are trusted
// depends on the outcome (see the refresh-age variables): an empty result
// never replaces cached badges — when every provider failed it is an outage,
// otherwise a provider briefly answering empty — so the old ones are kept
// (up to ratingsStaleServeMax) and retried soon.
func refreshRatings(ctx context.Context, cache *MemCache, clients RatingsClients, q RatingsQuery, key string) []RatingBadge {
	res := FetchRatingsCtx(ctx, clients, q)
	badges := res.Badges
	now := time.Now()
	ttl := ratingsRefreshFor(q.ReleaseDate, now)
	switch {
	case res.AllFailed():
		ttl = emptyRatingsTTL
	case res.Failed > 0:
		ttl = partialRatingsTTL
	}
	if len(badges) == 0 {
		if prev, ok := cachedRatings(cache, key); ok && len(prev.badges) > 0 && now.Sub(prev.fetched) <= ratingsStaleServeMax {
			retry := emptyRatingsTTL
			if !res.AllFailed() {
				retry = partialRatingsTTL
			}
			cache.Set(key, ratingsEntry{badges: prev.badges, refreshAfter: now.Add(retry), fetched: prev.fetched}, int64(len(prev.badges))*100+64)
			return prev.badges
		}
		// No badges to keep: a title without ratings is cached like a
		// successful fetch, a failed fetch only briefly.
		cache.Set(key, ratingsEntry{refreshAfter: now.Add(ttl)}, 64)
		return nil
	}
	cache.Set(key, ratingsEntry{badges: badges, refreshAfter: now.Add(ttl), fetched: now}, int64(len(badges))*100+64)
	return badges
}

func cachedRatings(cache *MemCache, key string) (ratingsEntry, bool) {
	v, ok := cache.Get(key)
	if !ok {
		return ratingsEntry{}, false
	}
	e, ok := v.(ratingsEntry)
	return e, ok
}

func FetchRatingsCached(cache *MemCache, clients RatingsClients, q RatingsQuery) []RatingBadge {
	return FetchRatingsCachedCtx(context.Background(), cache, clients, q)
}

// minRatingVotes: an average of fewer user votes is not shown (one 10/10
// vote rendered "100%"). Applies to user-vote sources (TMDB, IMDb, Trakt,
// Letterboxd, RT Audience, MAL); critic scores (RT, Metacritic, Ebert) are
// based on a handful of reviews by nature.
const minRatingVotes = 10

func fetchTmdbRatingCtx(ctx context.Context, tmdb *TmdbClient, tmdbID uint64, mediaType string, showID uint64, season, episode uint32) (*RatingBadge, error) {
	// No TMDB id (an IMDb-only title TMDB doesn't know, e.g. some anime
	// movies): /movie/0 would just 404 — the other providers use the IMDb id.
	if tmdbID == 0 && mediaType != "episode" {
		return nil, nil
	}
	path := fmt.Sprintf("/%s/%d", mediaType, tmdbID)
	if mediaType == "episode" {
		path = fmt.Sprintf("/tv/%d/season/%d/episode/%d", showID, season, episode)
	}

	var result struct {
		VoteAverage *float64 `json:"vote_average"`
		VoteCount   *int64   `json:"vote_count"`
	}
	if err := tmdb.GetCtx(ctx, path, nil, &result); err != nil {
		if isIDNotFound(err) {
			return nil, nil // not on TMDB: no rating, not a failure
		}
		return nil, err
	}
	if result.VoteAverage == nil || *result.VoteAverage <= 0 {
		return nil, nil
	}
	if result.VoteCount != nil && *result.VoteCount < minRatingVotes {
		return nil, nil
	}
	return &RatingBadge{Source: SourceTmdb, Value: fmt.Sprintf("%.0f%%", *result.VoteAverage*10)}, nil
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return len(slice)
}

// MarshalRatingBadges serializes a badge list as a compact JSON object mapping
// source key → value (e.g. {"imdb":"8.7","tmdb":"92%"}). Used to persist the
// preview title's ratings in available_ratings so they are fetched only once.
func MarshalRatingBadges(badges []RatingBadge) string {
	m := make(map[string]string, len(badges))
	for _, b := range badges {
		if b.Source != nil {
			m[b.Source.Key] = b.Value
		}
	}
	if len(m) == 0 {
		return ""
	}
	data, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(data)
}

// UnmarshalRatingBadges parses the format produced by MarshalRatingBadges,
// rebuilding each badge from its source key. Returns nil for empty input.
func UnmarshalRatingBadges(sources string) []RatingBadge {
	if sources == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(sources), &m); err != nil {
		return nil
	}
	badges := make([]RatingBadge, 0, len(m))
	for key, value := range m {
		if src := SourceFromKey(key); src != nil {
			badges = append(badges, RatingBadge{Source: src, Value: value})
		}
	}
	return badges
}

// InvalidateTitleRatings drops the cached ratings of the title an id
// resolves to (title purge). The ratings cache is keyed by TMDB/IMDb id, not
// by the request id, so the resolution is read from the ID cache; when that
// has already expired the ratings simply refresh on their own schedule.
func InvalidateTitleRatings(ids, ratings *MemCache, idType, idValue string) {
	if ids == nil || ratings == nil {
		return
	}
	var targets []*ResolvedID
	if v, ok := ids.Get(idType + "/" + idValue); ok {
		if e, ok := v.(idEntry); ok && e.resolved != nil {
			targets = append(targets, e.resolved, e.resolved.RatingsTarget)
		}
	}
	if v, ok := ids.Get(ratingsTargetKey(idType, idValue)); ok {
		if r, ok := v.(*ResolvedID); ok {
			targets = append(targets, r)
		}
	}
	for _, r := range targets {
		if r == nil {
			continue
		}
		for _, mt := range []string{"movie", "tv"} {
			ratings.Delete(ratingsCacheKey(RatingsQuery{ResolvedTMDbID: r.TMDbID, MediaType: mt, IMDbID: r.IMDbID}))
		}
		if ep := r.Episode; ep != nil {
			ratings.Delete(ratingsCacheKey(RatingsQuery{MediaType: "episode", EpisodeShowTMDbID: ep.ShowTMDbID, EpisodeSeason: ep.SeasonNumber, EpisodeEpisode: ep.EpisodeNumber}))
		}
	}
}

func ratingsTargetKey(idType, idValue string) string { return "rt:" + idType + "/" + idValue }

// RememberRatingsTarget records the title a kitsu:/mal: id takes its ratings
// from. That cross-reference is computed per request (it isn't part of the
// cached resolution), so without this InvalidateTitleRatings never found the
// target's ratings. Nil-safe; a no-op when the same target is already stored.
func RememberRatingsTarget(ids *MemCache, idType, idValue string, target *ResolvedID) {
	if ids == nil || target == nil {
		return
	}
	key := ratingsTargetKey(idType, idValue)
	if v, ok := ids.Peek(key); ok {
		if r, ok := v.(*ResolvedID); ok && r.TMDbID == target.TMDbID && r.MediaType == target.MediaType &&
			(r.IMDbID == nil) == (target.IMDbID == nil) && (r.IMDbID == nil || *r.IMDbID == *target.IMDbID) {
			return
		}
	}
	cp := *target
	ids.Set(key, &cp, 1)
}
