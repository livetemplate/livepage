package diagrams

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKrokiRendersAndCachesByContentHash(t *testing.T) {
	var hits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/mermaid/svg", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(200)
		// Echo the source back wrapped in fake SVG so tests can verify
		// content flowed end-to-end.
		w.Write([]byte("<svg>" + string(body) + "</svg>"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	r := NewKrokiMermaid(server.URL)

	src := []byte("flowchart LR\n  A --> B\n")

	// First call hits the server.
	svg, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(svg), "flowchart LR") {
		t.Errorf("svg %q does not contain source", svg)
	}

	// Second call (same source) MUST be served from cache.
	svg2, err := r.Render(src)
	if err != nil {
		t.Fatalf("Render (cached): %v", err)
	}
	if string(svg) != string(svg2) {
		t.Errorf("cache returned different bytes")
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (cache miss + 1 hit)", got)
	}

	// Different source MUST hit the server again.
	if _, err := r.Render([]byte("different source")); err != nil {
		t.Fatalf("Render (different): %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Errorf("hits = %d, want 2", got)
	}
}

func TestKrokiReturnsErrorOnNon200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mermaid/svg", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("syntax error: unrecognized token"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	r := NewKrokiMermaid(server.URL)
	_, err := r.Render([]byte("not mermaid"))
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should include status code: %v", err)
	}
}

func TestKrokiDoesNotCacheFailures(t *testing.T) {
	var hits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/mermaid/svg", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		// Fail first time, succeed after.
		if atomic.LoadInt64(&hits) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("<svg>ok</svg>"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	r := NewKrokiMermaid(server.URL)
	src := []byte("flowchart LR\n  A --> B\n")

	if _, err := r.Render(src); err == nil {
		t.Fatal("first render should have errored")
	}
	// Retry MUST hit the server again, not return the cached error.
	svg, err := r.Render(src)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !strings.Contains(string(svg), "ok") {
		t.Errorf("expected successful response on retry, got %q", svg)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Errorf("hits = %d, want 2 (retry should not be cached)", got)
	}
}

func TestKrokiTimeoutHonored(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mermaid/svg", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("<svg>too late</svg>"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	r := NewKrokiMermaid(server.URL)
	r.Timeout = 50 * time.Millisecond

	if _, err := r.Render([]byte("source")); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestKrokiDefaultsToPublicInstance(t *testing.T) {
	r := NewKrokiMermaid("")
	if r.BaseURL != "https://kroki.io" {
		t.Errorf("default BaseURL = %q, want https://kroki.io", r.BaseURL)
	}
	if r.DiagramKey != "mermaid" {
		t.Errorf("DiagramKey = %q, want mermaid", r.DiagramKey)
	}
}
