package router

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGzip_AcceptsEncoding(t *testing.T) {
	body := bytes.Repeat([]byte(`{"hello":"world"}`), 200) // > a few hundred bytes so gzip wins
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected Content-Encoding gzip, got %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("expected non-empty gzipped body")
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body not gzip-encoded: %v", err)
	}
	defer zr.Close()
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatalf("decoded body mismatch (len %d vs %d)", len(decoded), len(body))
	}
}

func TestGzip_NotAccepted(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	// no Accept-Encoding header
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("expected no Content-Encoding, got %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("body should pass through unmodified")
	}
}

func TestGzip_SkipsBinary(t *testing.T) {
	body := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0}, 100) // JPEG-ish bytes
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("image responses should not be gzipped, got %q", got)
	}
}

func TestGzip_SkipsAlreadyEncoded(t *testing.T) {
	body := []byte(`<html></html>`)
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write(body)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("existing Content-Encoding must be preserved, got %q", got)
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":             false,
		"gzip":         true,
		"GZIP":         true,
		"deflate,gzip": true,
		"br":           false,
		"gzip;q=0":     true, // q=0 means deprioritized but still accepted
	}
	for h, want := range cases {
		if got := acceptsGzip(h); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", h, got, want)
		}
	}
}

// Uncompressed responses with several writes must send the header once (the
// writer used to re-run WriteHeader on every Write when not gzipping).
func TestGzip_UncompressedMultiWrite(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("ab"))
		w.Write([]byte("cd"))
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rec, req)
	if rec.headerCalls != 1 {
		t.Errorf("WriteHeader called %d times, want 1", rec.headerCalls)
	}
	if rec.Body.String() != "abcd" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// Flush mid-stream must not terminate the gzip stream.
func TestGzip_FlushThenWrite(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello "))
		w.(http.Flusher).Flush()
		w.Write([]byte("world"))
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil || string(b) != "hello world" {
		t.Fatalf("got %q err %v", b, err)
	}
}

type countingRecorder struct {
	*httptest.ResponseRecorder
	headerCalls int
}

func (c *countingRecorder) WriteHeader(s int) { c.headerCalls++; c.ResponseRecorder.WriteHeader(s) }
