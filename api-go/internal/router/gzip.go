package router

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// gzipWriters recycles gzip.Writers (each holds ~800 KB of compressor state,
// so allocating one per response is the dominant cost of small JSON bodies).
var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// gzippableTypes is the set of Content-Type prefixes we compress. Anything
// else (images, octet-stream, already-encoded responses) passes through
// untouched. Matching is prefix-based so "application/json; charset=utf-8"
// is also compressed.
var gzippableTypes = []string{
	"application/json",
	"application/javascript",
	"application/manifest+json",
	"image/svg+xml",
	"text/",
}

// shouldCompress reports whether a response with the given status + headers
// is worth gzipping. We skip 1xx/2xx/3xx/4xx/5xx codes with no body, images,
// octet-streams, and responses that already declare a content encoding.
func shouldCompress(status int, contentType, contentEncoding string) bool {
	// 206: a byte range of the identity encoding can't be gzipped on its own.
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified || status == http.StatusPartialContent {
		return false
	}
	if contentEncoding != "" {
		return false
	}
	for _, p := range gzippableTypes {
		if strings.HasPrefix(contentType, p) {
			return true
		}
	}
	return false
}

// gzipResponseWriter wraps http.ResponseWriter and decides at WriteHeader
// time (when Content-Type is known) whether to compress. Headers must be sent
// before any body bytes, so Content-Encoding is set there too.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz            *gzip.Writer
	headerWritten bool
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.headerWritten {
		return
	}
	g.headerWritten = true
	hdr := g.ResponseWriter.Header()
	if shouldCompress(status, hdr.Get("Content-Type"), hdr.Get("Content-Encoding")) {
		g.gz = gzipWriters.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
		hdr.Set("Content-Encoding", "gzip")
		hdr.Add("Vary", "Accept-Encoding")
		// Drop Content-Length — the gzipped body has a different size.
		hdr.Del("Content-Length")
	}
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.headerWritten {
		// Implicit 200, same as net/http.
		g.WriteHeader(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// close finishes the gzip stream and returns the writer to the pool.
func (g *gzipResponseWriter) close() error {
	if g.gz == nil {
		return nil
	}
	err := g.gz.Close()
	g.gz.Reset(io.Discard) // drop the reference to the ResponseWriter
	gzipWriters.Put(g.gz)
	g.gz = nil
	return err
}

// Flush pushes buffered compressed bytes to the client without ending the
// gzip stream (Close would write the footer and break later writes).
func (g *gzipResponseWriter) Flush() {
	if g.gz != nil {
		g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets net/http use the underlying conn if the handler upgrades.
func (g *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := g.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Middleware returns an http.Handler wrapper that gzips eligible responses when
// the request carries `Accept-Encoding: gzip`.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer func() {
			// Ensure the gzip writer is closed on any return path. Use the
			// ResponseWriter's own status if WriteHeader never fired.
			_ = gw.close()
		}()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	// Comma-separated list; accept anything containing "gzip".
	for _, p := range strings.Split(header, ",") {
		token := strings.TrimSpace(strings.SplitN(p, ";", 2)[0])
		if strings.EqualFold(token, "gzip") {
			return true
		}
	}
	return false
}
