package services

import (
	"net/http"
	"testing"
)

// A key with control characters or spaces made the validation request fail
// to build, which was treated as "valid" and saved.
func TestValidateServiceKey_RejectsBadCharacters(t *testing.T) {
	for _, k := range []string{"abc\x00defgh", "abcdef\nghij", "abc def ghi", "abcdef&i=x"} {
		if err := ValidateServiceKeyCtx(t.Context(), "tmdb", k, &http.Client{Transport: &captureStub{}}); err == nil {
			t.Errorf("%q accepted", k)
		}
	}
	if err := ValidateServiceKeyCtx(t.Context(), "tmdb", "abc123DEF456-_.", &http.Client{Transport: &captureStub{}}); err != nil {
		t.Errorf("plain key rejected: %v", err)
	}
}
