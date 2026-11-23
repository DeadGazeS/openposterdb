package services

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// Concurrent lookups go out as one POST per provider/kind, matched back by
// the items' ids; a title MDBList doesn't return has no ratings (its
// not-found), which is not an error.
func TestMdblist_BatchesConcurrentLookups(t *testing.T) {
	var posts, gets atomic.Int32
	c := NewMdblistClient([]string{"k"}, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			gets.Add(1)
			return jsonResponse(200, `{"ids":{"imdb":"tt9"},"ratings":[],"score":50}`), nil
		}
		posts.Add(1)
		if req.URL.Path != "/imdb/movie" || req.URL.Query().Get("apikey") != "k" {
			t.Errorf("POST %s", req.URL.Redacted())
		}
		b, _ := io.ReadAll(req.Body)
		var body struct{ IDs []string }
		json.Unmarshal(b, &body)
		if len(body.IDs) != 3 {
			t.Errorf("batch ids = %v, want 3", body.IDs)
		}
		return jsonResponse(200, `[{"ids":{"imdb":"tt1"},"score":71},{"ids":{"imdb":"tt2"},"score":72}]`), nil
	})})

	ids := []string{"tt1", "tt2", "tt3"}
	scores := make([]*float64, 3)
	errs := make([]error, 3)
	found := make([]bool, 3)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.GetRatingsCtx(t.Context(), id, "movie")
			errs[i] = err
			if r != nil {
				scores[i] = r.Score
				found[i] = true
			}
		}()
	}
	wg.Wait()
	if posts.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("posts=%d gets=%d, want one POST", posts.Load(), gets.Load())
	}
	if scores[0] == nil || *scores[0] != 71 || scores[1] == nil || *scores[1] != 72 {
		t.Errorf("scores = %v %v", scores[0], scores[1])
	}
	if errs[2] != nil || found[2] {
		t.Errorf("tt3: err = %v found = %v, want no ratings and no error", errs[2], found[2])
	}

	// A lone lookup keeps the plain GET.
	if _, err := c.GetRatingsCtx(t.Context(), "tt9", "movie"); err != nil || gets.Load() != 1 {
		t.Errorf("single: err=%v gets=%d", err, gets.Load())
	}
}

// An answer whose items can't be matched to the ids falls back to one GET
// per title rather than dropping every rating.
func TestMdblist_BatchShapeMismatchFallsBack(t *testing.T) {
	var gets atomic.Int32
	c := NewMdblistClient([]string{"k"}, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			gets.Add(1)
			return jsonResponse(200, `{"ids":{"tmdb":550},"score":80}`), nil
		}
		return jsonResponse(200, `[{"title":"no ids block"},{"title":"neither"}]`), nil
	})})
	var wg sync.WaitGroup
	var ok atomic.Int32
	for _, id := range []uint64{550, 551} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, err := c.GetRatingsByTMDBCtx(t.Context(), id, "movie"); err == nil && r.Score != nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if gets.Load() != 2 || ok.Load() != 2 {
		t.Errorf("gets=%d ok=%d, want 2/2", gets.Load(), ok.Load())
	}
}
