package errors

import (
	stdErrors "errors"
	"net/url"
)

// secretQueryParams are the query parameters whose values must never reach
// logs when an upstream request fails. Mirrors the Rust redact_url_secrets.
var secretQueryParams = []string{
	"api_key",
	"apikey",
	"key",
	"client_id",
	"client_secret",
	"token",
	"access_token",
	"auth",
}

// RedactURLSecrets replaces the values of known secret-bearing query
// parameters inside any *url.Error it is given, so upstream API keys never
// appear in log output. The url.Error is mutated in place, which also
// sanitizes the error for every caller downstream.
func RedactURLSecrets(err error) error {
	if err == nil {
		return nil
	}
	var uerr *url.Error
	if stdErrors.As(err, &uerr) && uerr.URL != "" {
		uerr.URL = redactURL(uerr.URL)
	}
	return err
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := parsed.Query()
	changed := false
	for _, k := range secretQueryParams {
		if q.Has(k) {
			q.Set(k, "REDACTED")
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = q.Encode()
	}
	return parsed.String()
}
