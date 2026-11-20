package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPMDBConsensus(t *testing.T) {
	rows := []pmdbRow{
		{Label: "RT", Score: json.RawMessage(`91`), Updated: "2026-01-01"},
		{Label: "RT", Score: json.RawMessage(`91`), Updated: "2026-01-02"},
		{Label: "RT", Score: json.RawMessage(`88`), Updated: "2026-03-01"}, // outvoted
		{Label: "pc", Score: json.RawMessage(`"85"`)},
		{Label: "MC", Score: json.RawMessage(`140`)}, // out of range: dropped
		{Label: "LB", Score: json.RawMessage(`0`)},   // vote average 0 = no votes
		{Label: "RE", Score: json.RawMessage(`3.5`)},
		{Label: "TM", Score: json.RawMessage(`77`)}, // not a source it fills
		{Label: "RT overall", Score: json.RawMessage(`50`)},
	}
	got := pmdbConsensus(rows)
	want := map[*RatingSource]string{SourceRt: "91%", SourceRtAudience: "85%", SourceEbert: "3.5"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for src, v := range want {
		if got[src] != v {
			t.Errorf("%s = %q, want %q", src.Key, got[src], v)
		}
	}
}

// Lookups within the gather window share one request; badges only fill
// sources the other providers didn't.
func TestPublicMetaDB_BatchedAndFillsGaps(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "tmdb_id=550") || !strings.Contains(filter, "tmdb_id=1396") {
			t.Errorf("filter %q lacks a title", filter)
		}
		w.Write([]byte(`{"items":[
			{"tmdb_id":550,"media_type":"movie","label":"RT","score":79},
			{"tmdb_id":550,"media_type":"movie","label":"MC","score":67},
			{"tmdb_id":1396,"media_type":"tv","label":"LB","score":4.6}
		]}`))
	}))
	defer srv.Close()
	c := NewPublicMetaDBClient(srv.Client())
	c.URL = srv.URL

	var wg sync.WaitGroup
	var movie, show pmdbScores
	wg.Add(2)
	go func() { defer wg.Done(); movie, _ = c.ScoresCtx(t.Context(), 550, "movie") }()
	go func() { defer wg.Done(); show, _ = c.ScoresCtx(t.Context(), 1396, "tv") }()
	wg.Wait()
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1 batched request", requests.Load())
	}
	if movie[SourceRt] != "79%" || movie[SourceMetacritic] != "67" || show[SourceLetterboxd] != "4.6" {
		t.Errorf("movie %v show %v", movie, show)
	}

	badges := fillFromPublicMetaDB([]RatingBadge{{Source: SourceImdb, Value: "8.8"}, {Source: SourceRt, Value: "80%"}, {Source: SourceTrakt, Value: "88%"}}, movie)
	var keys []string
	for _, b := range badges {
		keys = append(keys, b.Source.Key+"="+b.Value)
	}
	if got := strings.Join(keys, " "); got != "imdb=8.8 rt=80% mc=67 trakt=88%" {
		t.Errorf("filled badges = %s (existing values kept, merge order)", got)
	}
}

// A 429 pauses every lookup for its Retry-After, without further requests.
func TestPublicMetaDB_PausesOn429(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewPublicMetaDBClient(srv.Client())
	c.URL = srv.URL
	if _, err := c.ScoresCtx(t.Context(), 550, "movie"); err == nil {
		t.Fatal("429: want an error")
	}
	if _, err := c.ScoresCtx(t.Context(), 551, "movie"); err != errPublicMetaDBPaused {
		t.Errorf("second lookup: err = %v, want paused", err)
	}
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}
	if until := time.Unix(0, c.pausedUntil.Load()); time.Until(until) < 50*time.Second {
		t.Errorf("paused until %v, want ~60 s", until)
	}
}

func TestBatcher_FlushesAtMax(t *testing.T) {
	var calls atomic.Int32
	b := newBatcher(time.Hour, 2, time.Second, func(_ context.Context, keys []int) (map[int]int, error) {
		calls.Add(1)
		out := map[int]int{}
		for _, k := range keys {
			out[k] = k * 10
		}
		return out, nil
	})
	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, ok, err := b.get(t.Context(), i+1)
			if !ok || err != nil {
				t.Errorf("key %d: ok=%v err=%v", i+1, ok, err)
			}
			results[i] = v
		}()
	}
	wg.Wait() // a full batch goes out without waiting for the (1 h) window
	if calls.Load() != 1 || results[0] != 10 || results[1] != 20 {
		t.Errorf("calls=%d results=%v", calls.Load(), results)
	}
}
