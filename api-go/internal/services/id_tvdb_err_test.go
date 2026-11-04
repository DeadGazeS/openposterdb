package services

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type tvdbDetailFailStub struct{ found string }

func (s tvdbDetailFailStub) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/find/") {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s.found))}, nil
	}
	return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("busy"))}, nil
}

// A failed detail lookup after /find must fail the resolve (errors aren't
// cached); it used to return a ResolvedID without poster/IMDb id, cached for
// an hour and rendered without artwork or badges.
func TestResolveTVDB_DetailErrorPropagates(t *testing.T) {
	for name, found := range map[string]string{
		"tv":    `{"tv_results":[{"id":1399,"name":"x"}]}`,
		"movie": `{"movie_results":[{"id":278,"title":"x"}]}`,
	} {
		c := NewTmdbClient("k", &http.Client{Transport: tvdbDetailFailStub{found}})
		if r, err := resolveTVDBCtx(t.Context(), "81189", c); err == nil {
			t.Errorf("%s: detail failure swallowed, got %+v", name, r)
		}
	}
}
