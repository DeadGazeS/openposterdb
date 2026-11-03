package services

import (
	"fmt"
	"io"
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
