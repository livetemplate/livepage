package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter wraps http.ResponseWriter to compress responses.
// gz is initialized lazily on the first Write so that handlers which
// bypass this wrapper (via Unwrap) never trigger a gz.Close that would
// flush an empty gzip stream into the real response body.
type gzipResponseWriter struct {
	gz          *gzip.Writer
	target      http.ResponseWriter
	gzReady     bool
	wroteHeader bool
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !w.gzReady {
		w.gz.Reset(w.target)
		w.gzReady = true
	}
	return w.gz.Write(b)
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

		// Get gzip writer from pool. The writer is left pointing at
		// io.Discard until the first body Write, so a handler that
		// unwraps and never writes through gz produces a clean response
		// (no trailing empty-stream bytes leaking into the wire).
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gzw := &gzipResponseWriter{
			gz:             gz,
			target:         w,
			Writer:         gz,
			ResponseWriter: w,
		}
		defer func() {
			if gzw.gzReady {
				gz.Close()
			}
			gz.Reset(io.Discard)
			gzipWriterPool.Put(gz)
		}()

		next.ServeHTTP(gzw, r)
	})
}

// WithCompression wraps an http.Handler with compression middleware
func WithCompression(h http.Handler) http.Handler {
	return compressionMiddleware(h)
}
