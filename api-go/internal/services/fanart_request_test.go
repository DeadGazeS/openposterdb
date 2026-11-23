package services

import (
	"net/http"
	"testing"
)

// Fanart requests carry a User-Agent and, when configured, the personal
// client_key next to the project api_key.
func TestFanart_RequestCarriesUserAgentAndClientKey(t *testing.T) {
	var got *http.Request
	c := NewFanartClient([]string{"proj"}, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req
		return jsonResponse(200, `{"movieposter":[]}`), nil
	})})
	c.ClientKey = "mine"
	if _, err := c.GetMovieImagesCtx(t.Context(), 550); err != nil {
		t.Fatal(err)
	}
	q := got.URL.Query()
	if q.Get("api_key") != "proj" || q.Get("client_key") != "mine" || got.URL.Path != "/v3/movies/550" {
		t.Errorf("url = %s", got.URL.Redacted())
	}
	if got.Header.Get("User-Agent") != UserAgent() {
		t.Errorf("User-Agent = %q", got.Header.Get("User-Agent"))
	}

	c2 := NewFanartClient([]string{"proj"}, c.HTTP)
	if _, err := c2.GetTVImagesCtx(t.Context(), 81189); err != nil {
		t.Fatal(err)
	}
	if got.URL.Query().Has("client_key") {
		t.Error("client_key sent without one configured")
	}
}
