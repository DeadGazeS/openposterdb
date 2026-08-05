package errors

import (
	stdErrors "errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURLSecrets_StripsAPIKey(t *testing.T) {
	err := &url.Error{
		Op:  "Get",
		URL: "https://api.themoviedb.org/3/find/tt123?api_key=super-secret-key&external_source=imdb_id",
		Err: stdErrors.New("dial tcp: connection refused"),
	}
	RedactURLSecrets(err)
	if strings.Contains(err.URL, "super-secret-key") {
		t.Fatalf("api_key leaked in URL: %s", err.URL)
	}
	if !strings.Contains(err.URL, "REDACTED") {
		t.Fatalf("api_key not redacted: %s", err.URL)
	}
	// Non-secret params must survive.
	if !strings.Contains(err.URL, "external_source=imdb_id") {
		t.Fatalf("non-secret param lost: %s", err.URL)
	}
}

func TestRedactURLSecrets_CoversAllParamNames(t *testing.T) {
	for _, name := range []string{"api_key", "apikey", "key", "client_id", "client_secret", "token", "access_token", "auth"} {
		raw := "https://x.test/path?" + name + "=secret123"
		if got := redactURL(raw); strings.Contains(got, "secret123") {
			t.Errorf("param %q not redacted: %s", name, got)
		}
	}
}

func TestRedactURLSecrets_LeavesNonSecretParams(t *testing.T) {
	raw := "https://x.test/path?i=tt123&season=1&episode=2"
	if got := redactURL(raw); got != raw {
		t.Fatalf("non-secret URL changed: %s", got)
	}
}

func TestRedactURLSecrets_WrappedErrorChain(t *testing.T) {
	inner := &url.Error{
		Op:  "Get",
		URL: "https://omdb.test/?apikey=abc&i=tt1",
		Err: stdErrors.New("boom"),
	}
	wrapped := fmt.Errorf("request failed after 2 retries: %w", inner)
	// The chain contains the url.Error; RedactURLSecrets must find it.
	RedactURLSecrets(wrapped)
	if strings.Contains(inner.URL, "abc") {
		t.Fatalf("nested url.Error not redacted: %s", inner.URL)
	}
}

func TestRedactURLSecrets_NilAndPlain(t *testing.T) {
	if RedactURLSecrets(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	plain := stdErrors.New("some error without a URL")
	if RedactURLSecrets(plain) != plain {
		t.Fatal("non-url errors must pass through unchanged")
	}
}

func TestRedactURLSecrets_UnparseableURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "http://[::1]:namedport", Err: stdErrors.New("x")}
	RedactURLSecrets(err)
	// Must not panic; URL left as-is when it can't be parsed.
	if err.URL != "http://[::1]:namedport" {
		t.Fatalf("unparseable URL changed: %s", err.URL)
	}
}
