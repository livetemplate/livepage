package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter wraps http.ResponseWriter to compress responses
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Writer.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so handlers that must
// bypass gzip wrapping (notably reverse-proxied responses, where the
// upstream owns Content-Length / Content-Encoding) can write the raw
// bytes without double-encoding. Callers should also clear any eager
// `Content-Encoding: gzip` / `Vary: Accept-Encoding` headers the
// middleware already set on the underlying writer.
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// gzipWriterPool reuses gzip writers to reduce GC pressure
var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		return gzip.NewWriter(io.Discard)
	},
}

// compressionMiddleware adds gzip compression to responses
func compressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Don't compress WebSocket connections
		if r.Header.Get("Upgrade") == "websocket" {
			next.ServeHTTP(w, r)
			return
		}

		// Only compress certain content types
		// We'll set this based on the response
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")

		// Get gzip writer from pool
		gz := gzipWriterPool.Get().(*gzip.Writer)
		defer func() {
			gz.Close()
			gz.Reset(io.Discard)
			gzipWriterPool.Put(gz)
		}()

		gz.Reset(w)

		// Wrap response writer
		gzw := &gzipResponseWriter{
			Writer:         gz,
			ResponseWriter: w,
		}

		next.ServeHTTP(gzw, r)
	})
}

// WithCompression wraps an http.Handler with compression middleware
func WithCompression(h http.Handler) http.Handler {
	return compressionMiddleware(h)
}
