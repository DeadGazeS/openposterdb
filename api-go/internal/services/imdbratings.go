package services

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
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

// IMDb ratings from the official dataset (title.ratings.tsv.gz), opt-in with
// IMDB_RATINGS_DATASET=true: IMDb licenses its datasets for personal and
// non-commercial use only. When loaded it is the first IMDb source — before
// MDBList and OMDb, no API call — and it covers episodes too.
//
// The table is ~9 bytes a title (sorted ids, votes, rating×10), refreshed
// every 24 h with an ETag check, and snapshotted to disk so a restart has it
// at once.

const (
	imdbRatingsURL      = "https://datasets.imdbws.com/title.ratings.tsv.gz"
	imdbRatingsMinVotes = 20
	imdbRatingsRefresh  = 24 * time.Hour
	imdbRatingsRetry    = 15 * time.Minute // doubled per consecutive failure, capped at the refresh
	imdbRatingsMaxBytes = 512 << 20        // decompressed
	imdbSnapshotMagic   = "IMDR"
	imdbSnapshotVersion = 1
)

type imdbTable struct {
	ids     []uint32
	votes   []uint32
	ratings []uint8 // rating × 10
	etag    string
}

func (t *imdbTable) get(imdbID string) (float64, uint32, bool) {
	if t == nil {
		return 0, 0, false
	}
	id, ok := encodeMappingID(imdbID, true)
	if !ok {
		return 0, 0, false
	}
	i, found := slices.BinarySearch(t.ids, id)
	if !found {
		return 0, 0, false
	}
	return float64(t.ratings[i]) / 10, t.votes[i], true
}

// IMDbRatings is the loaded dataset. Lookups before the first load miss.
type IMDbRatings struct {
	HTTP *http.Client
	// SnapshotPath is the on-disk copy ("" disables it).
	SnapshotPath string
	URL          string

	table atomic.Pointer[imdbTable]
}

// imdbDataset is the process-wide dataset used by FetchRatingsCtx (nil when
// not enabled).
var imdbDataset atomic.Pointer[IMDbRatings]

// SetIMDbRatingsDataset installs the dataset FetchRatingsCtx consults; nil
// removes it.
func SetIMDbRatingsDataset(r *IMDbRatings) { imdbDataset.Store(r) }

func NewIMDbRatings(httpClient *http.Client, snapshotPath string) *IMDbRatings {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &IMDbRatings{HTTP: httpClient, SnapshotPath: snapshotPath, URL: imdbRatingsURL}
}

// Size is the number of titles loaded.
func (r *IMDbRatings) Size() int {
	if r == nil {
		return 0
	}
	if t := r.table.Load(); t != nil {
		return len(t.ids)
	}
	return 0
}

// Badge returns the IMDb badge of a title ("tt…"), nil when the dataset
// doesn't have it (or isn't loaded).
func (r *IMDbRatings) Badge(imdbID string) *RatingBadge {
	if r == nil {
		return nil
	}
	rating, _, ok := r.table.Load().get(imdbID)
	if !ok || rating <= 0 {
		return nil
	}
	return &RatingBadge{Source: SourceImdb, Value: fmt.Sprintf("%.1f", rating)}
}

// Start loads the snapshot, then refreshes until ctx is done: every 24 h,
// after a failure in 15 min doubling up to 24 h.
func (r *IMDbRatings) Start(ctx context.Context) {
	go func() {
		if err := r.loadSnapshot(); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("IMDb ratings snapshot unusable; downloading", "error", err)
		}
		failures := 0
		for {
			wait := imdbRatingsRefresh
			if err := r.Refresh(ctx); err != nil {
				failures++
				wait = min(imdbRatingsRetry<<(failures-1), imdbRatingsRefresh)
				slog.Warn("IMDb ratings refresh failed", "error", err, "retry_in", wait)
			} else {
				failures = 0
			}
			if sleepCtx(ctx, wait) != nil {
				return
			}
		}
	}()
}

// Refresh downloads the dataset unless the loaded copy's ETag still matches,
// and swaps it in. An empty or broken download keeps the loaded table.
func (r *IMDbRatings) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent())
	if cur := r.table.Load(); cur != nil && cur.etag != "" {
		req.Header.Set("If-None-Match", cur.etag)
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	t, filtered, err := parseIMDbRatings(io.LimitReader(gz, imdbRatingsMaxBytes))
	if err != nil {
		return err
	}
	t.etag = resp.Header.Get("ETag")
	r.table.Store(t)
	slog.Info("IMDb ratings dataset loaded", "titles", len(t.ids), "below_min_votes", filtered)
	if err := r.saveSnapshot(t); err != nil {
		slog.Warn("IMDb ratings snapshot write failed", "error", err)
	}
	return nil
}

// parseIMDbRatings reads title.ratings.tsv (header line first). Titles with
// fewer than imdbRatingsMinVotes votes are dropped; a file without a single
// usable row is an error.
func parseIMDbRatings(rd io.Reader) (*imdbTable, int, error) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	t := &imdbTable{
		ids:     make([]uint32, 0, 1<<20),
		votes:   make([]uint32, 0, 1<<20),
		ratings: make([]uint8, 0, 1<<20),
	}
	filtered := 0
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "tconst") {
				continue
			}
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		id, ok := encodeMappingID(f[0], true)
		rating, err1 := strconv.ParseFloat(f[1], 64)
		votes, err2 := strconv.ParseUint(f[2], 10, 32)
		if !ok || err1 != nil || err2 != nil || rating < 0 || rating > 10 {
			continue
		}
		if votes < imdbRatingsMinVotes {
			filtered++
			continue
		}
		t.ids = append(t.ids, id)
		t.votes = append(t.votes, uint32(votes))
		t.ratings = append(t.ratings, uint8(rating*10+0.5))
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if len(t.ids) == 0 {
		return nil, 0, errors.New("IMDb ratings dataset has no usable rows")
	}
	t.sortByID()
	t.ids, t.votes, t.ratings = slices.Clip(t.ids), slices.Clip(t.votes), slices.Clip(t.ratings)
	return t, filtered, nil
}

func (t *imdbTable) sortByID() {
	if slices.IsSorted(t.ids) {
		return
	}
	idx := make([]int, len(t.ids))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int { return int(t.ids[a]) - int(t.ids[b]) })
	ids, votes, ratings := make([]uint32, len(idx)), make([]uint32, len(idx)), make([]uint8, len(idx))
	for i, j := range idx {
		ids[i], votes[i], ratings[i] = t.ids[j], t.votes[j], t.ratings[j]
	}
	t.ids, t.votes, t.ratings = ids, votes, ratings
}

// Snapshot format (little endian): "IMDR", version u32, count u32, etag
// length u32, etag, ids [count]u32, votes [count]u32, ratings [count]u8.
func (r *IMDbRatings) saveSnapshot(t *imdbTable) error {
	if r.SnapshotPath == "" {
		return nil
	}
	var buf bytes.Buffer
	buf.Grow(16 + len(t.etag) + 9*len(t.ids))
	buf.WriteString(imdbSnapshotMagic)
	for _, v := range []uint32{imdbSnapshotVersion, uint32(len(t.ids)), uint32(len(t.etag))} {
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	buf.WriteString(t.etag)
	_ = binary.Write(&buf, binary.LittleEndian, t.ids)
	_ = binary.Write(&buf, binary.LittleEndian, t.votes)
	buf.Write(t.ratings)
	return writeFileAtomic(r.SnapshotPath, buf.Bytes())
}

func (r *IMDbRatings) loadSnapshot() error {
	if r.SnapshotPath == "" {
		return os.ErrNotExist
	}
	data, err := os.ReadFile(r.SnapshotPath)
	if err != nil {
		return err
	}
	t, err := decodeIMDbSnapshot(data)
	if err != nil {
		return err
	}
	r.table.Store(t)
	slog.Info("IMDb ratings dataset loaded from snapshot", "titles", len(t.ids), "file", filepath.Base(r.SnapshotPath))
	return nil
}

func decodeIMDbSnapshot(data []byte) (*imdbTable, error) {
	bad := errors.New("not an IMDb ratings snapshot of this version")
	if len(data) < 16 || string(data[:4]) != imdbSnapshotMagic {
		return nil, bad
	}
	le := binary.LittleEndian
	if le.Uint32(data[4:]) != imdbSnapshotVersion {
		return nil, bad
	}
	count, etagLen := int(le.Uint32(data[8:])), int(le.Uint32(data[12:]))
	if len(data) != 16+etagLen+9*count {
		return nil, bad
	}
	off := 16
	t := &imdbTable{etag: string(data[off : off+etagLen]), ids: make([]uint32, count), votes: make([]uint32, count)}
	off += etagLen
	for i := range count {
		t.ids[i] = le.Uint32(data[off+4*i:])
	}
	off += 4 * count
	for i := range count {
		t.votes[i] = le.Uint32(data[off+4*i:])
	}
	off += 4 * count
	t.ratings = slices.Clone(data[off : off+count])
	if count == 0 || !slices.IsSorted(t.ids) {
		return nil, bad
	}
	return t, nil
}
