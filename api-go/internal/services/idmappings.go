package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Offline ID mapping lists, consulted before any API call:
//   - dins + Wikidata IMDb↔TMDB↔TVDB CSVs (dins first; Wikidata rows only
//     add titles dins lacks, or fill a series' missing TVDB id — its TMDB and
//     movie TVDB ids were too often wrong);
//   - Fribb's anime-list-full.json (MAL / Kitsu / AniList / AniDB ↔ TMDB /
//     TVDB / IMDb).
//
// Each list is downloaded with an ETag check, parsed before it replaces the
// last good copy, snapshotted to disk (so a restart works offline) and
// refreshed every 24 h.

const (
	dinsBase         = "https://raw.githubusercontent.com/0xConstant1/dins-mappings/main"
	wikidataBase     = "https://raw.githubusercontent.com/0xConstant1/Wikidata-Fetcher/refs/heads/main/data"
	fribbAnimeURL    = "https://raw.githubusercontent.com/Fribb/anime-lists/refs/heads/master/anime-list-full.json"
	mappingRefresh   = 24 * time.Hour
	mappingRetry     = time.Hour
	mappingMaxBytes  = 256 << 20
)

type mappingSource struct {
	name string
	url  string
	file string // snapshot file name inside IDMappings.Dir
}

var (
	srcDinsSeries = mappingSource{"dins series", dinsBase + "/tv_mappings.csv", "dins_tv_mappings.csv"}
	srcDinsMovies = mappingSource{"dins movies", dinsBase + "/movie_mappings.csv", "dins_movie_mappings.csv"}
	srcWikiSeries = mappingSource{"Wikidata series", wikidataBase + "/tv_mappings.csv", "wikidata_tv_mappings.csv"}
	srcWikiMovies = mappingSource{"Wikidata movies", wikidataBase + "/movie_mappings.csv", "wikidata_movie_mappings.csv"}
	srcFribb      = mappingSource{"Fribb anime list", fribbAnimeURL, "anime-list-full.json"}
)

// IDMap is one title's cross-ids from the offline lists (zero/"" = unknown).
type IDMap struct {
	IMDb string
	TMDB uint64
	TVDB uint64
}

// AnimeIDs is one Fribb anime-list entry.
type AnimeIDs struct {
	MAL, Kitsu, AniList, AniDB uint64
	IMDb                       string // first IMDb id; see MultiPart
	TMDB                       uint64
	TMDBType                   string // "movie", "tv" or "" (unknown)
	TVDB                       uint64
	Type                       string // lower-case: "tv", "movie", "ova", "ona", "special", …
	// MultiPart: Fribb collapses multi-part films into one entry, so its
	// IMDb/TMDB ids name several titles and none can stand for the entry.
	MultiPart bool
	// SharedIMDb: other entries list the same IMDb id (seasons of one show,
	// or a film carrying its franchise show's id).
	SharedIMDb bool
}

// IDMappings holds the offline indexes. Safe for concurrent use; lookups
// before the first load simply miss.
type IDMappings struct {
	HTTP *http.Client
	Dir  string // snapshot directory; "" disables snapshots

	series atomic.Pointer[idIndex]
	movies atomic.Pointer[idIndex]
	anime  atomic.Pointer[animeIndex]

	// sources overrides the download URLs (tests).
	sources map[string]string
}

func NewIDMappings(httpClient *http.Client, dir string) *IDMappings {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &IDMappings{HTTP: httpClient, Dir: dir}
}

// Ready reports whether the IMDb/TMDB/TVDB lists are loaded.
func (m *IDMappings) Ready() bool {
	return m != nil && m.series.Load() != nil && m.movies.Load() != nil
}

// AnimeReady reports whether the anime list is loaded.
func (m *IDMappings) AnimeReady() bool {
	return m != nil && m.anime.Load() != nil
}

// Start loads the disk snapshots, then downloads changes and keeps
// refreshing every 24 h (an hour after a failure) until ctx is done.
func (m *IDMappings) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		m.loadSnapshots()
		for {
			err := m.Refresh(ctx)
			wait := mappingRefresh
			if err != nil {
				slog.Warn("id mapping refresh failed", "error", err)
				wait = mappingRetry
			}
			if sleepCtx(ctx, wait) != nil {
				return
			}
		}
	}()
}

// loadSnapshots builds the indexes from the on-disk copies, if any.
func (m *IDMappings) loadSnapshots() {
	read := func(s mappingSource) []byte {
		if m.Dir == "" {
			return nil
		}
		b, err := os.ReadFile(filepath.Join(m.Dir, s.file))
		if err != nil {
			return nil
		}
		return b
	}
	if err := m.buildTables(read(srcDinsSeries), read(srcWikiSeries), read(srcDinsMovies), read(srcWikiMovies)); err != nil {
		slog.Debug("id mapping snapshots not usable", "error", err)
	}
	if b := read(srcFribb); b != nil {
		if idx, err := parseFribb(b); err == nil {
			m.anime.Store(idx)
			slog.Info("anime id mappings loaded from snapshot", "entries", len(idx.entries))
		}
	}
}

// LoadAnimeList installs an anime list (Fribb's JSON format) directly.
func (m *IDMappings) LoadAnimeList(data []byte) error {
	idx, err := parseFribb(data)
	if err != nil {
		return err
	}
	m.anime.Store(idx)
	return nil
}

// LoadTables installs IMDb/TMDB/TVDB lists (CSV) directly; nil backfills
// are allowed.
func (m *IDMappings) LoadTables(dinsSeries, wikiSeries, dinsMovies, wikiMovies []byte) error {
	return m.buildTables(dinsSeries, wikiSeries, dinsMovies, wikiMovies)
}

// Refresh downloads every list that changed and swaps in rebuilt indexes.
// A list that fails to download or parse keeps its last good copy.
func (m *IDMappings) Refresh(ctx context.Context) error {
	var errs []error
	type fetched struct {
		body    []byte
		changed bool
	}
	get := func(s mappingSource, validate func([]byte) error) fetched {
		body, changed, err := m.fetch(ctx, s, validate)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
		}
		return fetched{body, changed}
	}
	csvOK := func(b []byte) error {
		_, err := parseMappingCSV(b, true)
		return err
	}
	ds, ws, dm, wm := get(srcDinsSeries, csvOK), get(srcWikiSeries, csvOK), get(srcDinsMovies, csvOK), get(srcWikiMovies, csvOK)
	if ds.changed || ws.changed || dm.changed || wm.changed || !m.Ready() {
		if err := m.buildTables(ds.body, ws.body, dm.body, wm.body); err != nil {
			errs = append(errs, err)
		}
	}
	var animeIdx *animeIndex
	fr := get(srcFribb, func(b []byte) error {
		idx, err := parseFribb(b)
		animeIdx = idx
		return err
	})
	if animeIdx == nil && fr.body != nil && (fr.changed || !m.AnimeReady()) {
		animeIdx, _ = parseFribb(fr.body)
	}
	if animeIdx != nil {
		m.anime.Store(animeIdx)
		slog.Info("anime id mappings loaded", "entries", len(animeIdx.entries))
	}
	return errors.Join(errs...)
}

// buildTables rebuilds the series/movie indexes; a missing dins list keeps
// the current index for that kind.
func (m *IDMappings) buildTables(dinsSeries, wikiSeries, dinsMovies, wikiMovies []byte) error {
	var errs []error
	build := func(kind string, primary, backfill []byte, fillTVDB, backfillTVDB bool, dst *atomic.Pointer[idIndex]) {
		if primary == nil {
			return
		}
		p, err := parseMappingCSV(primary, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", kind, err))
			return
		}
		var b []idRow
		if backfill != nil {
			b, _ = parseMappingCSV(backfill, backfillTVDB)
		}
		idx := buildIDIndex(p, b, fillTVDB)
		dst.Store(idx)
		slog.Info("id mappings loaded", "kind", kind, "rows", len(idx.rows), "backfilled", idx.backfilled, "conflicts", idx.conflicts)
	}
	// Wikidata TVDB ids are kept for series only.
	build("series", dinsSeries, wikiSeries, true, true, &m.series)
	build("movies", dinsMovies, wikiMovies, false, false, &m.movies)
	return errors.Join(errs...)
}

// fetch returns the list's current bytes and whether they changed since the
// snapshot. A conditional GET (If-None-Match) answered 304 returns the
// snapshot. validate runs before the snapshot is replaced, so a broken
// upstream file never overwrites the last good copy; on any failure the
// snapshot (when present) is returned with the error.
func (m *IDMappings) fetch(ctx context.Context, s mappingSource, validate func([]byte) error) ([]byte, bool, error) {
	snapPath, etagPath := "", ""
	var snapshot []byte
	if m.Dir != "" {
		snapPath = filepath.Join(m.Dir, s.file)
		etagPath = snapPath + ".etag"
		snapshot, _ = os.ReadFile(snapPath)
	}
	url := s.url
	if u, ok := m.sources[s.file]; ok {
		url = u
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return snapshot, false, err
	}
	req.Header.Set("User-Agent", UserAgent())
	if snapshot != nil {
		if etag, err := os.ReadFile(etagPath); err == nil && len(etag) > 0 {
			req.Header.Set("If-None-Match", strings.TrimSpace(string(etag)))
		}
	}
	resp, err := SendWithRetryCtx(ctx, &mappingRetryCfg, func() (*http.Response, error) { return m.HTTP.Do(req.Clone(ctx)) })
	if err != nil {
		return snapshot, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && snapshot != nil {
		return snapshot, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return snapshot, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := ReadAllLimited(resp.Body, mappingMaxBytes)
	if err != nil {
		return snapshot, false, err
	}
	if err := validate(body); err != nil {
		return snapshot, false, fmt.Errorf("downloaded list unusable: %w", err)
	}
	if snapPath != "" {
		if err := writeFileAtomic(snapPath, body); err != nil {
			slog.Warn("id mapping snapshot write failed", "file", snapPath, "error", err)
		} else if etag := resp.Header.Get("ETag"); etag != "" {
			_ = os.WriteFile(etagPath, []byte(etag), 0o644)
		} else {
			_ = os.Remove(etagPath)
		}
	}
	return body, !bytes.Equal(body, snapshot), nil
}

var mappingRetryCfg = RetryConfig{MaxRetries: 3, BaseDelay: time.Second, MaxDelay: 30 * time.Second, ServiceName: "id-mappings"}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// --- IMDb / TMDB / TVDB tables ---

// idRow stores ids as uint32 (0 = missing); IMDb ids without the "tt".
type idRow struct{ imdb, tmdb, tvdb uint32 }

// sortedIDs maps an id to its row by binary search (compact: 8 bytes/id).
type sortedIDs struct{ ids, rows []uint32 }

func (s *sortedIDs) find(id uint32) (uint32, bool) {
	i, ok := slices.BinarySearch(s.ids, id)
	if !ok {
		return 0, false
	}
	return s.rows[i], true
}

type idIndex struct {
	rows                 []idRow
	byIMDb, byTMDB, byTV sortedIDs
	backfilled           int
	conflicts            int
}

// parseMappingCSV parses a mapping CSV (columns imdb_id / tmdb_id / tvdb_id
// in any order, others ignored). Rows with a malformed id are skipped; a
// file without an imdb column or without a single valid row is an error.
// keepTVDB=false drops the tvdb column.
func parseMappingCSV(data []byte, keepTVDB bool) ([]idRow, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	if !sc.Scan() {
		return nil, errors.New("empty mapping CSV")
	}
	header := strings.Split(strings.TrimSpace(sc.Text()), ",")
	col := func(names ...string) int {
		for i, h := range header {
			for _, n := range names {
				if strings.TrimSpace(h) == n {
					return i
				}
			}
		}
		return -1
	}
	ci, cm, cv := col("imdb_id", "imdbId"), col("tmdb_id", "tmdbId"), col("tvdb_id", "tvdbId")
	if ci < 0 {
		return nil, errors.New("mapping CSV has no imdb column")
	}
	if !keepTVDB {
		cv = -1
	}
	rows := make([]idRow, 0, bytes.Count(data, []byte{'\n'}))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) != len(header) {
			continue
		}
		var r idRow
		ok := true
		get := func(c int, imdb bool) uint32 {
			if c < 0 || !ok {
				return 0
			}
			v := strings.TrimSpace(f[c])
			if v == "" {
				return 0
			}
			n, valid := encodeMappingID(v, imdb)
			if !valid {
				ok = false
			}
			return n
		}
		r.imdb, r.tmdb, r.tvdb = get(ci, true), get(cm, false), get(cv, false)
		if ok && (r.imdb != 0 || r.tmdb != 0 || r.tvdb != 0) {
			rows = append(rows, r)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("mapping CSV has no valid rows")
	}
	return rows, nil
}

// encodeMappingID parses a numeric id, or an IMDb id "tt" + digits that
// round-trips (≥ 7 digits, no extra leading zeros).
func encodeMappingID(v string, imdb bool) (uint32, bool) {
	digits := v
	if imdb {
		rest, ok := strings.CutPrefix(v, "tt")
		if !ok {
			return 0, false
		}
		digits = rest
	}
	n, err := strconv.ParseUint(digits, 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	if imdb && formatIMDb(uint32(n)) != v {
		return 0, false
	}
	return uint32(n), true
}

func formatIMDb(n uint32) string { return fmt.Sprintf("tt%07d", n) }

// buildIDIndex merges the primary (dins) rows with the backfill (Wikidata)
// rows: a backfill row matching no primary id is added as a new title; one
// matching exactly one primary row without contradicting it may fill that
// row's empty TVDB id (fillTVDB); anything else is a conflict and skipped.
// The first row to claim an id owns it.
func buildIDIndex(primary, backfill []idRow, fillTVDB bool) *idIndex {
	idx := &idIndex{rows: make([]idRow, 0, len(primary)+len(backfill))}
	owners := [3]map[uint32]uint32{{}, {}, {}}
	fields := func(r *idRow) [3]*uint32 { return [3]*uint32{&r.imdb, &r.tmdb, &r.tvdb} }
	for _, r := range primary {
		row := uint32(len(idx.rows))
		indexed := false
		f := fields(&r)
		for i, p := range f {
			if *p == 0 {
				continue
			}
			if _, taken := owners[i][*p]; !taken {
				owners[i][*p] = row
				indexed = true
			}
		}
		if indexed {
			idx.rows = append(idx.rows, r)
		}
	}
	for _, r := range backfill {
		target, matches := -1, 0
		f := fields(&r)
		for i, p := range f {
			if *p == 0 {
				continue
			}
			if owner, ok := owners[i][*p]; ok && int(owner) != target {
				target = int(owner)
				matches++
			}
		}
		if matches == 0 {
			row := uint32(len(idx.rows))
			for i, p := range f {
				if *p != 0 {
					owners[i][*p] = row
				}
			}
			idx.rows = append(idx.rows, r)
			idx.backfilled++
			continue
		}
		cur := fields(&idx.rows[target])
		disagrees := false
		for i, p := range f {
			if *p != 0 && *cur[i] != 0 && *p != *cur[i] {
				disagrees = true
			}
		}
		if matches > 1 || disagrees {
			idx.conflicts++
			continue
		}
		if fillTVDB && r.tvdb != 0 && idx.rows[target].tvdb == 0 {
			idx.rows[target].tvdb = r.tvdb
			owners[2][r.tvdb] = uint32(target)
		}
	}
	sorted := func(m map[uint32]uint32) sortedIDs {
		s := sortedIDs{ids: make([]uint32, 0, len(m)), rows: make([]uint32, len(m))}
		for id := range m {
			s.ids = append(s.ids, id)
		}
		slices.Sort(s.ids)
		for i, id := range s.ids {
			s.rows[i] = m[id]
		}
		return s
	}
	idx.byIMDb, idx.byTMDB, idx.byTV = sorted(owners[0]), sorted(owners[1]), sorted(owners[2])
	return idx
}

func (idx *idIndex) lookup(field IDType, value string) (IDMap, bool) {
	if idx == nil {
		return IDMap{}, false
	}
	var s *sortedIDs
	imdb := false
	switch field {
	case IDTypeIMDB:
		s, imdb = &idx.byIMDb, true
	case IDTypeTMDB:
		s = &idx.byTMDB
	case IDTypeTVDB:
		s = &idx.byTV
	default:
		return IDMap{}, false
	}
	id, ok := encodeMappingID(value, imdb)
	if !ok {
		return IDMap{}, false
	}
	row, ok := s.find(id)
	if !ok {
		return IDMap{}, false
	}
	r := idx.rows[row]
	out := IDMap{TMDB: uint64(r.tmdb), TVDB: uint64(r.tvdb)}
	if r.imdb != 0 {
		out.IMDb = formatIMDb(r.imdb)
	}
	return out, true
}

// Lookup finds a series (MediaTypeTV) or movie (MediaTypeMovie) by its IMDb
// ("tt…"), TMDB or TVDB id.
func (m *IDMappings) Lookup(kind MediaType, field IDType, value string) (IDMap, bool) {
	if m == nil {
		return IDMap{}, false
	}
	switch kind {
	case MediaTypeMovie:
		return m.movies.Load().lookup(field, value)
	case MediaTypeTV:
		return m.series.Load().lookup(field, value)
	}
	return IDMap{}, false
}

// SeriesTVDB returns a series' TVDB id from its IMDb or TMDB id (0 = unknown).
func (m *IDMappings) SeriesTVDB(imdbID *string, tmdbID uint64) uint64 {
	if m == nil {
		return 0
	}
	if tmdbID != 0 {
		if e, ok := m.Lookup(MediaTypeTV, IDTypeTMDB, strconv.FormatUint(tmdbID, 10)); ok && e.TVDB != 0 {
			return e.TVDB
		}
	}
	if imdbID != nil && *imdbID != "" {
		if e, ok := m.Lookup(MediaTypeTV, IDTypeIMDB, *imdbID); ok && e.TVDB != 0 {
			return e.TVDB
		}
	}
	return 0
}

// --- Fribb anime list ---

type animeIndex struct {
	entries []AnimeIDs
	byMAL   map[uint64]int32
	byKitsu map[uint64]int32
	byIMDb  map[string][]int32
}

var animeSeriesLike = map[string]bool{"tv": true, "ova": true, "ona": true, "special": true}

// fribbIDs decodes a Fribb id field: a number, a string, an array of
// either, or (themoviedb_id) {"tv": …, "movie": …}.
type fribbIDs struct {
	ids   []string
	space string // "tv" / "movie" when the field was the object form
}

func (f *fribbIDs) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	switch b[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			return nil // malformed: treat as missing
		}
		for _, raw := range arr {
			var one fribbIDs
			_ = one.UnmarshalJSON(raw)
			f.ids = append(f.ids, one.ids...)
		}
	case '{':
		var obj struct {
			TV    fribbIDs `json:"tv"`
			Movie fribbIDs `json:"movie"`
		}
		if err := json.Unmarshal(b, &obj); err != nil {
			return nil
		}
		switch {
		case len(obj.TV.ids) > 0:
			f.space = "tv"
		case len(obj.Movie.ids) > 0:
			f.space = "movie"
		}
		f.ids = append(obj.TV.ids, obj.Movie.ids...)
	case '"':
		var s string
		if json.Unmarshal(b, &s) == nil && strings.TrimSpace(s) != "" {
			f.ids = []string{strings.TrimSpace(s)}
		}
	default:
		var n json.Number
		if json.Unmarshal(b, &n) == nil && n.String() != "" {
			f.ids = []string{n.String()}
		}
	}
	return nil
}

func (f fribbIDs) first() uint64 {
	for _, s := range f.ids {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil && n != 0 {
			return n
		}
	}
	return 0
}

type fribbEntry struct {
	MAL      fribbIDs `json:"mal_id"`
	Kitsu    fribbIDs `json:"kitsu_id"`
	AniList  fribbIDs `json:"anilist_id"`
	AniDB    fribbIDs `json:"anidb_id"`
	IMDb     fribbIDs `json:"imdb_id"`
	TMDB     fribbIDs `json:"themoviedb_id"`
	TMDBType string   `json:"themoviedb_type"`
	TVDB     fribbIDs `json:"tvdb_id"`
	Type     string   `json:"type"`
}

func parseFribb(data []byte) (*animeIndex, error) {
	var raw []fribbEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	idx := &animeIndex{
		entries: make([]AnimeIDs, 0, len(raw)),
		byMAL:   map[uint64]int32{},
		byKitsu: map[uint64]int32{},
		byIMDb:  map[string][]int32{},
	}
	for _, e := range raw {
		var imdbs []string
		for _, s := range e.IMDb.ids {
			if strings.HasPrefix(s, "tt") {
				imdbs = append(imdbs, s)
			}
		}
		a := AnimeIDs{
			MAL: e.MAL.first(), Kitsu: e.Kitsu.first(), AniList: e.AniList.first(), AniDB: e.AniDB.first(),
			TMDB: e.TMDB.first(), TVDB: e.TVDB.first(),
			Type:      strings.ToLower(strings.TrimSpace(e.Type)),
			MultiPart: len(e.TMDB.ids) > 1 || len(imdbs) > 1,
		}
		if len(imdbs) > 0 {
			a.IMDb = imdbs[0]
		}
		a.TMDBType = e.TMDB.space
		if a.TMDBType == "" {
			if t := strings.ToLower(e.TMDBType); t == "tv" || t == "movie" {
				a.TMDBType = t
			}
		}
		if a.MAL == 0 && a.Kitsu == 0 && a.IMDb == "" && a.TMDB == 0 {
			continue
		}
		i := int32(len(idx.entries))
		idx.entries = append(idx.entries, a)
		if a.MAL != 0 {
			if _, ok := idx.byMAL[a.MAL]; !ok {
				idx.byMAL[a.MAL] = i
			}
		}
		if a.Kitsu != 0 {
			if _, ok := idx.byKitsu[a.Kitsu]; !ok {
				idx.byKitsu[a.Kitsu] = i
			}
		}
		for _, id := range imdbs {
			idx.byIMDb[id] = append(idx.byIMDb[id], i)
		}
	}
	if len(idx.entries) == 0 {
		return nil, errors.New("anime list has no entries")
	}
	for i := range idx.entries {
		if a := &idx.entries[i]; a.IMDb != "" {
			a.SharedIMDb = len(idx.byIMDb[a.IMDb]) > 1
		}
	}
	return idx, nil
}

// AnimeByKitsu returns the anime-list entry of a Kitsu anime id.
func (m *IDMappings) AnimeByKitsu(id uint64) (AnimeIDs, bool) {
	idx := m.animeIdx()
	if idx == nil || id == 0 {
		return AnimeIDs{}, false
	}
	i, ok := idx.byKitsu[id]
	if !ok {
		return AnimeIDs{}, false
	}
	return idx.entries[i], true
}

// AnimeByMAL returns the anime-list entry of a MAL anime id.
func (m *IDMappings) AnimeByMAL(id uint64) (AnimeIDs, bool) {
	idx := m.animeIdx()
	if idx == nil || id == 0 {
		return AnimeIDs{}, false
	}
	i, ok := idx.byMAL[id]
	if !ok {
		return AnimeIDs{}, false
	}
	return idx.entries[i], true
}

// IMDbForKitsu returns the IMDb id of a Kitsu anime (nil when unknown or
// the entry stands for several titles).
func (m *IDMappings) IMDbForKitsu(id uint64) *string {
	a, ok := m.AnimeByKitsu(id)
	if !ok {
		return nil
	}
	imdb, _, _ := a.RatingsIDs()
	return imdb
}

// KitsuForMAL returns the Kitsu id of a MAL anime.
func (m *IDMappings) KitsuForMAL(id uint64) *uint64 {
	a, ok := m.AnimeByMAL(id)
	if !ok || a.Kitsu == 0 {
		return nil
	}
	return &a.Kitsu
}

// KitsuByIMDb returns the Kitsu id of the anime entry an IMDb id belongs to
// (the first one listed when several share it — seasons of one show).
func (m *IDMappings) KitsuByIMDb(imdbID string) *uint64 {
	idx := m.animeIdx()
	if idx == nil {
		return nil
	}
	for _, i := range idx.byIMDb[imdbID] {
		if k := idx.entries[i].Kitsu; k != 0 {
			return &k
		}
	}
	return nil
}

func (m *IDMappings) animeIdx() *animeIndex {
	if m == nil {
		return nil
	}
	return m.anime.Load()
}

// RatingsIDs returns the IMDb id and/or the TMDB title (id + media type) an
// anime entry stands for, for ratings. Multi-part entries yield nothing (no
// single title); a TMDB id is only used when its namespace (movie / tv) is
// known or follows from the entry's type.
//
// A film entry often carries its franchise show's ids, so for a film the
// entry's own type decides: a TMDB id not stated to be in the movie space and
// an IMDb id other entries share are refused (as aiometadata's filmIdsFor
// does).
func (a AnimeIDs) RatingsIDs() (imdbID *string, tmdbID uint64, mediaType MediaType) {
	if a.MultiPart {
		return nil, 0, MediaTypeTV
	}
	if a.Type != "" && !animeSeriesLike[a.Type] {
		if a.IMDb != "" && !a.SharedIMDb {
			v := a.IMDb
			imdbID = &v
		}
		if a.TMDB != 0 && a.TMDBType == "movie" {
			tmdbID = a.TMDB
		}
		return imdbID, tmdbID, MediaTypeMovie
	}
	if a.IMDb != "" {
		v := a.IMDb
		imdbID = &v
	}
	space := a.TMDBType
	if space == "" && a.Type != "" {
		space = "movie"
		if animeSeriesLike[a.Type] {
			space = "tv"
		}
	}
	mediaType = MediaTypeTV
	if space == "movie" {
		mediaType = MediaTypeMovie
	}
	if a.TMDB != 0 && space != "" {
		tmdbID = a.TMDB
	}
	return imdbID, tmdbID, mediaType
}
