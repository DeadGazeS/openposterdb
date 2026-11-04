package services

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TMDB answers an unknown/deleted id with 404: that is a not-found (404 for
// the request, cached briefly), not a 500 retried on every request.
func TestTmdbGet_404IsNotFound(t *testing.T) {
	stub := &statusStub{status: 404, body: `{"status_code":34,"status_message":"The resource you requested could not be found."}`}
	c := NewTmdbClient("k", &http.Client{Transport: stub})
	var v map[string]any
	if err := c.GetCtx(t.Context(), "/movie/999999999", nil, &v); !isIDNotFound(err) {
		t.Fatalf("want IDNotFound, got %v", err)
	}
}

// Error bodies are bounded and truncated before they reach the error text
// (which is logged).
func TestTmdbGet_ErrorBodyTruncated(t *testing.T) {
	stub := &statusStub{status: 500, body: strings.Repeat("x", 1<<20)}
	c := NewTmdbClient("k", &http.Client{Transport: stub})
	var v map[string]any
	err := c.GetCtx(t.Context(), "/movie/1", nil, &v)
	if err == nil || len(err.Error()) > 1024 {
		t.Fatalf("error text not bounded: %d bytes", len(err.Error()))
	}
}

type captureStub struct{ url string }

func (s *captureStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.url = req.URL.String()
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}

// Query parameter values are escaped: a value can't add parameters.
func TestTmdbGet_ParamsEscaped(t *testing.T) {
	stub := &captureStub{}
	c := NewTmdbClient("k", &http.Client{Transport: stub})
	var v map[string]any
	_ = c.GetCtx(t.Context(), "/x", map[string]string{"language": "en&api_key=evil"}, &v)
	if strings.Contains(stub.url, "api_key=evil") {
		t.Fatalf("param value injected a parameter: %s", stub.url)
	}
}
