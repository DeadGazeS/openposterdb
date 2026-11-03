package services

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// animeStub serves fixed bodies per host (Kitsu / AniList), counting calls
// and optionally failing every request.
type animeStub struct {
	calls  atomic.Int32
	fail   atomic.Bool
	kitsu  string
	anilst string
}

func (s *animeStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	if s.fail.Load() {
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("down"))}, nil
	}
	body := s.kitsu
	if req.URL.Host == "graphql.anilist.co" {
		body = s.anilst
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

const kitsuMappingBody = `{"data":[{"id":"1","type":"mappings","relationships":{"item":{"data":{"type":"anime","id":"4676"}}}}],"included":[{"id":"4676","type":"anime"}]}`

// mal:N requests looked the MAL→Kitsu mapping up on every request.
func TestKitsuAnimeIDByMAL_Cached(t *testing.T) {
	stub := &animeStub{kitsu: kitsuMappingBody}
	c := NewKitsuClient(&http.Client{Transport: stub})
	for i := 0; i < 3; i++ {
		if id := c.AnimeIDByMALCtx(t.Context(), 6702); id == nil || *id != 4676 {
			t.Fatalf("mapping = %v", id)
		}
	}
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("Kitsu calls = %d, want 1", n)
	}
}

// A failed lookup is not cached; "Kitsu has no mapping" is.
func TestKitsuAnimeIDByMAL_FailureNotCached(t *testing.T) {
	stub := &animeStub{kitsu: `{"data":[]}`}
	stub.fail.Store(true)
	c := NewKitsuClient(&http.Client{Transport: stub})
	c.AnimeIDByMALCtx(t.Context(), 1)
	stub.fail.Store(false)
	before := stub.calls.Load()
	c.AnimeIDByMALCtx(t.Context(), 1) // retried: the failure wasn't cached
	c.AnimeIDByMALCtx(t.Context(), 1) // cached: "no mapping" is definitive
	if n := stub.calls.Load() - before; n != 1 {
		t.Fatalf("calls after recovery = %d, want 1", n)
	}
}

const anilistMediaBody = `{"data":{"Media":{"id":1,"idMal":6702,"title":{"romaji":"x"},"coverImage":{"extraLarge":"u"},"externalLinks":[{"url":"https://www.imdb.com/title/tt1528406/","site":"IMDB"}]}}}`

func TestAniListMediaByMAL_CachedAndIMDbDerived(t *testing.T) {
	stub := &animeStub{anilst: anilistMediaBody}
	c := NewAniListClient(&http.Client{Transport: stub})
	for i := 0; i < 3; i++ {
		if m, err := c.MediaByMALCtx(t.Context(), 6702); err != nil || m.MALID != 6702 {
			t.Fatalf("media: %v %v", m, err)
		}
	}
	if id := c.LookupIMDBByMAL(t.Context(), 6702); id == nil || *id != "tt1528406" {
		t.Fatalf("imdb = %v", id)
	}
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("AniList calls = %d, want 1", n)
	}
}

// A transient AniList failure used to be stored forever as "no IMDb link".
func TestAniListLookupIMDB_TransientErrorNotCached(t *testing.T) {
	stub := &animeStub{anilst: anilistMediaBody}
	stub.fail.Store(true)
	c := NewAniListClient(&http.Client{Transport: stub})
	if id := c.LookupIMDBByMAL(t.Context(), 6702); id != nil {
		t.Fatalf("want nil while AniList is down, got %v", *id)
	}
	stub.fail.Store(false)
	if id := c.LookupIMDBByMAL(t.Context(), 6702); id == nil || *id != "tt1528406" {
		t.Fatalf("after recovery imdb = %v (the failure was cached)", id)
	}
}

type statusStub struct {
	calls  atomic.Int32
	status int
	body   string
}

func (s *statusStub) RoundTrip(*http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return &http.Response{StatusCode: s.status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}

// AniList answers an unknown idMal with HTTP 404 (checked live). That must be
// a not-found (404 for the request, cached briefly), not a server error that
// is retried against AniList on every request.
func TestAniListMediaByMAL_HTTP404IsNotFound(t *testing.T) {
	stub := &statusStub{status: 404, body: `{"errors":[{"message":"Not Found.","status":404}],"data":{"Media":null}}`}
	c := NewAniListClient(&http.Client{Transport: stub})
	for i := 0; i < 3; i++ {
		_, err := c.MediaByMALCtx(t.Context(), 999999999)
		if !isIDNotFound(err) {
			t.Fatalf("want IDNotFound, got %v", err)
		}
	}
	if n := stub.calls.Load(); n != 1 {
		t.Fatalf("AniList calls = %d, want 1 (not-found cached)", n)
	}
}

// A non-404 GraphQL error (e.g. rate limit) is transient: not cached as
// not-found.
func TestAniListMediaByMAL_OtherGraphQLErrorNotCached(t *testing.T) {
	stub := &statusStub{status: 200, body: `{"errors":[{"message":"Too Many Requests.","status":429}],"data":{"Media":null}}`}
	c := NewAniListClient(&http.Client{Transport: stub})
	for i := 0; i < 2; i++ {
		if _, err := c.MediaByMALCtx(t.Context(), 1); err == nil || isIDNotFound(err) {
			t.Fatalf("want a transient error, got %v", err)
		}
	}
	if n := stub.calls.Load(); n != 2 {
		t.Fatalf("AniList calls = %d, want 2 (transient errors not cached)", n)
	}
}
