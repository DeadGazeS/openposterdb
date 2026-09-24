package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type uaCapture struct{ got string }

func (c *uaCapture) RoundTrip(req *http.Request) (*http.Response, error) {
	c.got = req.Header.Get("User-Agent")
	rec := httptest.NewRecorder()
	rec.WriteString(`{}`)
	return rec.Result(), nil
}

// TestUserAgentCarriesBuildVersion: outgoing API requests identify as
// openposterdb/<build version> (set via -ldflags -X …services.Version), not
// a hard-coded number that goes stale on every release.
func TestUserAgentCarriesBuildVersion(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "1.3.0-dev-abc1234"

	capture := &uaCapture{}
	tmdb := NewTmdbClient("k", &http.Client{Transport: capture})
	var out map[string]any
	_ = tmdb.GetCtx(context.Background(), "/movie/1", nil, &out)
	if capture.got != "openposterdb/1.3.0-dev-abc1234" {
		t.Errorf("TMDB User-Agent = %q, want openposterdb/1.3.0-dev-abc1234", capture.got)
	}
}
