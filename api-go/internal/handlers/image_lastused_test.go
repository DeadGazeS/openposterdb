package handlers

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type recorderStub struct {
	mu  sync.Mutex
	ids []int64
}

func (r *recorderStub) Record(id int64) {
	r.mu.Lock()
	r.ids = append(r.ids, id)
	r.mu.Unlock()
}

// Image requests are what API keys are used for; "last used" in the admin
// list used to reflect only self-service logins.
func TestHandleImage_RecordsKeyUse(t *testing.T) {
	deps, apiKey := newCDNFlowDeps(t)
	deps.Config.EnableCDNRedirects = false
	rec := &recorderStub{}
	deps.LastUsed = rec
	h := HandleImage(func() ImageDeps { return deps }, func() bool { return false })
	mux := http.NewServeMux()
	mux.HandleFunc("/{apiKey}/{rest...}", h)

	for _, key := range []string{apiKey, "0000000000000000000000000000000000000000000000000000000000000000"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/"+key+"/imdb/poster-default/tt0111161.jpg", nil))
	}
	if len(rec.ids) != 1 || rec.ids[0] == 0 {
		t.Fatalf("recorded %v, want exactly the valid key's id", rec.ids)
	}
}
