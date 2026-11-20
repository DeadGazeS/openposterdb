package services

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const imdbTSV = "tconst\taverageRating\tnumVotes\n" +
	"tt0903747\t9.5\t2200000\n" +
	"tt0000001\t5.7\t19\n" + // below the vote minimum
	"tt0111161\t9.3\t3000000\n" + // out of order: sorted on load
	"tt2301451\t10.0\t230000\n" + // an episode (Ozymandias)
	"bad\tline\n"

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func TestIMDbRatings_RefreshSnapshotETag(t *testing.T) {
	var body atomic.Value
	body.Store(gz(t, imdbTSV))
	var notModified atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"e1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"e1"`)
		w.Write(body.Load().([]byte))
	}))
	defer srv.Close()

	snap := filepath.Join(t.TempDir(), "imdb-ratings.bin")
	r := NewIMDbRatings(srv.Client(), snap)
	r.URL = srv.URL
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.Size() != 3 {
		t.Errorf("titles = %d, want 3", r.Size())
	}
	for id, want := range map[string]string{"tt0903747": "9.5", "tt0111161": "9.3", "tt2301451": "10.0"} {
		if b := r.Badge(id); b == nil || b.Value != want || b.Source != SourceImdb {
			t.Errorf("%s: badge %+v, want %s", id, b, want)
		}
	}
	if r.Badge("tt0000001") != nil {
		t.Error("title below the vote minimum has a badge")
	}

	// Unchanged upstream: a conditional request, the table stays.
	if err := r.Refresh(t.Context()); err != nil || notModified.Load() != 1 {
		t.Errorf("second refresh: err=%v 304s=%d", err, notModified.Load())
	}

	// A restart loads the snapshot without a download.
	r2 := NewIMDbRatings(nil, snap)
	if err := r2.loadSnapshot(); err != nil {
		t.Fatal(err)
	}
	if b := r2.Badge("tt0111161"); b == nil || b.Value != "9.3" {
		t.Errorf("snapshot: %+v", b)
	}
	if r2.table.Load().etag != `"e1"` {
		t.Errorf("snapshot etag = %q", r2.table.Load().etag)
	}
}

// An empty (or header-only) download is rejected and keeps the loaded table.
func TestIMDbRatings_EmptyDownloadRejected(t *testing.T) {
	var body atomic.Value
	body.Store(gz(t, imdbTSV))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body.Load().([]byte))
	}))
	defer srv.Close()
	r := NewIMDbRatings(srv.Client(), "")
	r.URL = srv.URL
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	body.Store(gz(t, "tconst\taverageRating\tnumVotes\n"))
	if err := r.Refresh(t.Context()); err == nil {
		t.Error("empty dataset accepted")
	}
	if r.Size() != 3 {
		t.Errorf("titles after a rejected download = %d, want 3", r.Size())
	}
}

func TestDecodeIMDbSnapshot_RejectsGarbage(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("IMDR"), []byte(strings.Repeat("x", 40))} {
		if _, err := decodeIMDbSnapshot(data); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
}

// The dataset's rating is the IMDb badge, ahead of OMDb's, and covers titles
// no provider rates (episodes without OMDb).
func TestFetchRatings_DatasetFirst(t *testing.T) {
	r := &IMDbRatings{}
	tbl, _, err := parseIMDbRatings(strings.NewReader(imdbTSV))
	if err != nil {
		t.Fatal(err)
	}
	r.table.Store(tbl)
	SetIMDbRatingsDataset(r)
	t.Cleanup(func() { SetIMDbRatingsDataset(nil) })

	if got := withDatasetIMDb([]RatingBadge{{Source: SourceImdb, Value: "9.4"}, {Source: SourceRt, Value: "96%"}}, r.Badge("tt0903747")); got[0].Value != "9.5" || len(got) != 2 {
		t.Errorf("replace: %+v", got)
	}
	if got := withDatasetIMDb([]RatingBadge{{Source: SourceRt, Value: "96%"}}, r.Badge("tt0903747")); got[0].Source != SourceImdb || len(got) != 2 {
		t.Errorf("insert: %+v", got)
	}

	ep := "tt2301451"
	res := FetchRatingsCtx(t.Context(), RatingsClients{}, RatingsQuery{MediaType: "episode", IMDbID: &ep})
	if len(res.Badges) != 1 || res.Badges[0].Source != SourceImdb || res.Badges[0].Value != "10.0" {
		t.Errorf("episode with no providers: %+v", res.Badges)
	}
}
