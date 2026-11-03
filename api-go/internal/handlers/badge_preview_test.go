package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The unauthenticated badge endpoint rejects oversized values instead of
// rendering arbitrary-length text.
func TestBadgePreview_RejectsLongValue(t *testing.T) {
	db := newHandlersTestDB(t)
	h := HandleBadgePreview(db)
	req := httptest.NewRequest(http.MethodGet, "/api/icons/badge?source=imdb&value="+strings.Repeat("9", 17), nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too long") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
