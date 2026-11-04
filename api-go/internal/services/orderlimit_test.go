package services

import "testing"

// A negative limit (e.g. ?ratings_limit=-1 on the unauthenticated badge
// preview) used to panic with "slice bounds out of range".
func TestOrderAndLimit_NegativeLimitDoesNotPanic(t *testing.T) {
	src := parseOrder("imdb,tmdb")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	if got := OrderAndLimit(src, "imdb,tmdb", "", -1); len(got) > len(src) {
		t.Fatalf("got %d sources", len(got))
	}
}
