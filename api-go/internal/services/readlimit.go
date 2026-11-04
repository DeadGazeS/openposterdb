package services

import (
	"fmt"
	"io"
	"strings"
)

// MaxArtworkBytes caps a single artwork download. Real posters/backdrops are
// a few MB even at TMDB "original"; this only stops a misbehaving or hostile
// upstream from streaming unbounded data into memory.
const MaxArtworkBytes = 50 << 20

// ReadAllLimited reads r fully, failing (instead of truncating) when it is
// larger than max bytes.
func ReadAllLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return b, nil
}

// maxUpstreamJSON bounds a JSON API response from an upstream provider.
const maxUpstreamJSON = 8 << 20

// upstreamErrorSnippet reads a little of an error response for the error
// text (which is logged): bounded, so a huge error page is neither buffered
// whole nor copied into logs.
func upstreamErrorSnippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 4<<10))
	return errSnippet(b)
}

// errSnippet shortens an upstream error body for an error message.
func errSnippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
