package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"openposterdb/internal/httpx"
)

func writtenStatus(err error) int {
	rec := httptest.NewRecorder()
	httpx.WriteAppError(rec, err)
	return rec.Code
}

// A provider answering 5xx is an upstream failure (502), not ours (500); a
// 404 stays a not-found. The 503 carries a Retry-After beyond the retry
// policy, so it is handed back without waiting through retries.
func TestTMDB_Upstream5xxIs502(t *testing.T) {
	status := http.StatusServiceUnavailable
	c := NewTmdbClient("k", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := jsonResponse(status, `{}`)
		resp.Header.Set("Retry-After", "3600")
		return resp, nil
	})})
	var out map[string]any
	if got := writtenStatus(c.GetCtx(t.Context(), "/movie/1", nil, &out)); got != http.StatusBadGateway {
		t.Errorf("TMDB API 503: status = %d, want 502", got)
	}
	_, err := c.FetchImageBytesCtx(t.Context(), "/p.jpg", "w500")
	if got := writtenStatus(err); got != http.StatusBadGateway {
		t.Errorf("TMDB CDN 503: status = %d, want 502", got)
	}
	_, _, _, err = c.FetchPosterBytesConditionalCtx(t.Context(), "/p.jpg", "w500", nil, "")
	if got := writtenStatus(err); got != http.StatusBadGateway {
		t.Errorf("TMDB CDN conditional 503: status = %d, want 502", got)
	}
	status = http.StatusNotFound
	if got := writtenStatus(c.GetCtx(t.Context(), "/movie/1", nil, &out)); got != http.StatusNotFound {
		t.Errorf("TMDB API 404: status = %d, want 404", got)
	}
}

func TestTVDB_Upstream5xxIs502(t *testing.T) {
	// Login answered with 500.
	c, _ := newFakeTVDB(t, nil)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	c.BaseURL = failing.URL
	_, err := c.SeriesCtx(t.Context(), 1)
	if got := writtenStatus(err); got != http.StatusBadGateway {
		t.Errorf("TVDB login 500: status = %d, want 502 (err %v)", got, err)
	}

	// Data endpoint answered with 502.
	c, _ = newFakeTVDB(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err = c.SeriesCtx(t.Context(), 1)
	if got := writtenStatus(err); got != http.StatusBadGateway {
		t.Errorf("TVDB GET 502: status = %d, want 502 (err %v)", got, err)
	}
}

func TestKitsu_Upstream5xxIs502(t *testing.T) {
	c := NewKitsuClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `{}`), nil
	})})
	_, _, err := c.GetAnimeCtx(t.Context(), "1")
	if got := writtenStatus(err); got != http.StatusBadGateway {
		t.Errorf("Kitsu 500: status = %d, want 502", got)
	}
}
