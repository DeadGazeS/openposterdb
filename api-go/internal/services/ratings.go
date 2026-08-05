package services

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
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
	SourceMal        = &RatingSource{Key: "mal", Label: "MAL", CacheChar: 'm', ColorR: 34, ColorG: 60, ColorB: 120}
	SourceImdb       = &RatingSource{Key: "imdb", Label: "IMDb", CacheChar: 'i', ColorR: 180, ColorG: 145, ColorB: 15}
	SourceLetterboxd = &RatingSource{Key: "lb", Label: "LB", CacheChar: 'l', ColorR: 0, ColorG: 155, ColorB: 88}
	SourceRt         = &RatingSource{Key: "rt", Label: "RTC", CacheChar: 'r', ColorR: 185, ColorG: 35, ColorB: 8}
	SourceRtAudience = &RatingSource{Key: "rta", Label: "RTA", CacheChar: 'a', ColorR: 185, ColorG: 35, ColorB: 8}
	SourceMetacritic = &RatingSource{Key: "mc", Label: "MC", CacheChar: 'c', ColorR: 75, ColorG: 150, ColorB: 38}
	SourceTmdb       = &RatingSource{Key: "tmdb", Label: "TMDB", CacheChar: 't', ColorR: 1, ColorG: 155, ColorB: 88}
	SourceTrakt      = &RatingSource{Key: "trakt", Label: "Trakt", CacheChar: 'k', ColorR: 175, ColorG: 15, ColorB: 45}
	SourceMdblist    = &RatingSource{Key: "mdblist", Label: "MDB", CacheChar: 'd', ColorR: 66, ColorG: 132, ColorB: 202}
	SourceEbert      = &RatingSource{Key: "ebert", Label: "Ebert", CacheChar: 'e', ColorR: 232, ColorG: 89, ColorB: 12}
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

	if len(result) > int(limit) {
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

	for i := range resp.Ratings {
		r := &resp.Ratings[i]
		var badge *RatingBadge
		switch r.Source {
		case "imdb":
			if r.Value != nil {
				badge = &RatingBadge{Source: SourceImdb, Value: fmt.Sprintf("%.1f", *r.Value)}
			}
		case "trakt":
			if r.Score != nil && *r.Score > 0 {
				badge = &RatingBadge{Source: SourceTrakt, Value: fmt.Sprintf("%.0f%%", *r.Score)}
			}
		case "letterboxd":
			if r.Value != nil {
				badge = &RatingBadge{Source: SourceLetterboxd, Value: fmt.Sprintf("%.1f", *r.Value)}
			}
		case "popcorn":
			if r.Score != nil && *r.Score > 0 {
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
			if r.Score != nil && *r.Score > 0 {
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
	if rating <= 0 || votes == 0 {
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
}

// FetchRatingsResult carries the ratings fetch outcome: the auxiliary provider
// responses and the merged badge list.
type FetchRatingsResult struct {
	Mdblist *MdblistResponse
	OMDB    *OmdbResponse
	Badges  []RatingBadge
}

func FetchRatings(clients RatingsClients, q RatingsQuery) FetchRatingsResult {

	type tmdbResult struct {
		badge *RatingBadge
	}
	type omdbResult struct {
		resp *OmdbResponse
	}
	type mdblistResult struct {
		resp *MdblistResponse
	}
	type traktResult struct {
		b *RatingBadge
	}

	tmdbCh := make(chan tmdbResult, 1)
	omdbCh := make(chan omdbResult, 1)
	mdblistCh := make(chan mdblistResult, 1)
	traktCh := make(chan traktResult, 1)

	go func() {
		badge := fetchTmdbRating(clients.TMDB, q.ResolvedTMDbID, q.MediaType, q.EpisodeShowTMDbID, q.EpisodeSeason, q.EpisodeEpisode)
		tmdbCh <- tmdbResult{badge}
	}()

	go func() {
		var resp *OmdbResponse
		if clients.OMDB != nil && q.IMDbID != nil {
			if r, err := clients.OMDB.GetRatings(*q.IMDbID); err == nil {
				resp = r
			}
		}
		omdbCh <- omdbResult{resp}
	}()

	go func() {
		var resp *MdblistResponse
		if clients.MDBList != nil && q.MediaType != "episode" {
			if q.IMDbID != nil && *q.IMDbID != "" {
				if r, err := clients.MDBList.GetRatings(*q.IMDbID, q.MediaType); err == nil {
					resp = r
				}
			} else {
				if r, err := clients.MDBList.GetRatingsByTMDB(q.ResolvedTMDbID, q.MediaType); err == nil {
					resp = r
				}
			}
		}
		mdblistCh <- mdblistResult{resp}
	}()

	go func() {
		var b *RatingBadge
		if clients.Trakt != nil {
			switch q.MediaType {
			case "movie":
				if q.IMDbID != nil && *q.IMDbID != "" {
					if r, err := clients.Trakt.GetMovieRating(*q.IMDbID); err == nil && r != nil {
						b = TraktBadge(r.Rating, r.Votes)
					}
				}
			case "tv":
				if q.IMDbID != nil && *q.IMDbID != "" {
					if r, err := clients.Trakt.GetShowRating(*q.IMDbID); err == nil && r != nil {
						b = TraktBadge(r.Rating, r.Votes)
					}
				}
			case "episode":
				// Episode Trakt ratings require showing IMDb ID lookup, skipped for simplicity
			}
		}
		traktCh <- traktResult{b}
	}()

	tmdbR := <-tmdbCh
	omdbR := <-omdbCh
	mdblistR := <-mdblistCh
	traktR := <-traktCh

	var mdblistResp *MdblistResponse
	var omdbResp *OmdbResponse
	var traktBadge *RatingBadge

	if traktR.b != nil {
		traktBadge = traktR.b
	}

	if omdbR.resp != nil {
		omdbResp = omdbR.resp
	}

	var badges []RatingBadge

	if mdblistR.resp != nil {
		mdblistResp = mdblistR.resp
		mdblistBadges := MdblistBadges(mdblistResp)

		ordered := []*RatingSource{SourceImdb, SourceTmdb, SourceRt, SourceRtAudience, SourceMetacritic, SourceTrakt, SourceLetterboxd, SourceMal, SourceMdblist, SourceEbert}
		for _, src := range ordered {
			if src == SourceTmdb && tmdbR.badge != nil {
				badges = append(badges, *tmdbR.badge)
				continue
			}
			if src == SourceTrakt && traktBadge != nil {
				badges = append(badges, *traktBadge)
				traktBadge = nil
				continue
			}
			for _, mb := range mdblistBadges {
				if mb.Source == src {
					badges = append(badges, mb)
					break
				}
			}
		}

		if traktBadge != nil {
			badges = append(badges, *traktBadge)
		}
	} else {
		if tmdbR.badge != nil {
			badges = append(badges, *tmdbR.badge)
		}

		if omdbResp != nil {
			for _, r := range omdbResp.Ratings {
				switch r.Source {
				case "Rotten Tomatoes":
					badges = append(badges, RatingBadge{Source: SourceRt, Value: r.Value})
				}
			}
			if omdbResp.IMDBRating != nil && *omdbResp.IMDBRating != "N/A" {
				badges = append(badges, RatingBadge{Source: SourceImdb, Value: *omdbResp.IMDBRating})
			}
			if omdbResp.Metascore != nil && *omdbResp.Metascore != "N/A" {
				badges = append(badges, RatingBadge{Source: SourceMetacritic, Value: *omdbResp.Metascore})
			}
		}

		if traktBadge != nil {
			badges = append(badges, *traktBadge)
		}
	}

	return FetchRatingsResult{Mdblist: mdblistResp, OMDB: omdbResp, Badges: badges}
}

// FetchRatingsCached fetches ratings and caches the resulting badges in the
// given in-memory cache (key "tmdbID/mediaType" or "showID/episode/SsEe",
// matching the Rust ratings_cache). A nil cache bypasses caching. Only the
// badge list is cached; the auxiliary responses are refetched on a hit but are
// unused by all current callers.
func FetchRatingsCached(cache *MemCache, clients RatingsClients, q RatingsQuery) []RatingBadge {
	if cache != nil {
		key := fmt.Sprintf("%d/%s", q.ResolvedTMDbID, q.MediaType)
		if q.MediaType == "episode" {
			key = fmt.Sprintf("%d/episode/S%dE%d", q.EpisodeShowTMDbID, q.EpisodeSeason, q.EpisodeEpisode)
		}
		if v, ok := cache.Get(key); ok {
			return v.([]RatingBadge)
		}
		result := FetchRatings(clients, q)
		cache.Set(key, result.Badges, int64(len(result.Badges))*100+64)
		return result.Badges
	}
	result := FetchRatings(clients, q)
	return result.Badges
}

func fetchTmdbRating(tmdb *TmdbClient, tmdbID uint64, mediaType string, showID uint64, season, episode uint32) *RatingBadge {
	path := fmt.Sprintf("/%s/%d", mediaType, tmdbID)
	if mediaType == "episode" {
		path = fmt.Sprintf("/tv/%d/season/%d/episode/%d", showID, season, episode)
	}

	var result struct {
		VoteAverage *float64 `json:"vote_average"`
	}
	if err := tmdb.Get(path, nil, &result); err != nil {
		return nil
	}
	if result.VoteAverage == nil || *result.VoteAverage <= 0 {
		return nil
	}
	return &RatingBadge{Source: SourceTmdb, Value: fmt.Sprintf("%.0f%%", *result.VoteAverage*10)}
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
