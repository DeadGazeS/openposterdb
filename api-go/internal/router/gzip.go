package router

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
)

// gzippableTypes is the set of Content-Type prefixes we compress. Anything
// else (images, octet-stream, already-encoded responses) passes through
// untouched. Matching is prefix-based so "application/json; charset=utf-8"
// is also compressed.
var gzippableTypes = []string{
	"application/json",
	"text/",
}

// shouldCompress reports whether a response with the given status + headers
// is worth gzipping. We skip 1xx/2xx/3xx/4xx/5xx codes with no body, images,
// octet-streams, and responses that already declare a content encoding.
func shouldCompress(status int, contentType, contentEncoding string) bool {
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified {
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

// gzipResponseWriter wraps http.ResponseWriter so the first Write triggers a
// gzip writer. Headers must be sent before any body bytes (HTTP semantics), so
// we set the Content-Encoding header at write time and let the original
// ResponseWriter hand the encoded stream to the wire.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz            io.WriteCloser
	wrote         bool
	originalWrite func([]byte) (int, error)
	originalHdr   http.Header
	shouldGzip    bool
	status        int
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	g.status = status
	g.wrote = false // permit Write after
	g.originalWrite = g.ResponseWriter.Write
	g.originalHdr = g.ResponseWriter.Header()
	g.shouldGzip = shouldCompress(status, g.originalHdr.Get("Content-Type"), g.originalHdr.Get("Content-Encoding"))
	if g.shouldGzip {
		g.gz = gzip.NewWriter(g.ResponseWriter)
		g.originalHdr.Set("Content-Encoding", "gzip")
		g.originalHdr.Set("Vary", "Accept-Encoding")
		// Drop Content-Length — the gzipped body has a different size.
		g.originalHdr.Del("Content-Length")
	} else {
		g.gz = nil
	}
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if g.gz == nil {
		// WriteHeader wasn't called yet — flush implicit 200 and install the
		// gzip writer before any body bytes hit the wire.
		g.WriteHeader(http.StatusOK)
	}
	if g.shouldGzip {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// Close the gzip writer to flush any trailer bytes (gzip has no trailer, but
// the underlying Writer contract may require Close). Wrap once.
func (g *gzipResponseWriter) close() error {
	if g.gz == nil {
		return nil
	}
	return g.gz.Close()
}

// Flush bridges bufio.Flusher if the underlying ResponseWriter supports it.
func (g *gzipResponseWriter) Flush() {
	g.close()
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
