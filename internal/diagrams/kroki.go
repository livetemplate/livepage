// Package diagrams provides server-side renderers for diagram-as-code
// blocks (currently mermaid via Kroki) so tinkerdown sites can ship
// pre-rendered SVGs and skip the heavyweight client-side runtime.
//
// The package exposes a small Renderer interface; the concrete Kroki
// implementation talks to https://kroki.io by default but accepts any
// Kroki-compatible URL (self-hosted instances are encouraged for
// production use). Results are content-hash-cached in memory so each
// unique source string costs at most one HTTP round-trip per process
// lifetime.
package diagrams

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Renderer renders a diagram source string (e.g. mermaid markup) to an
// SVG byte slice. Implementations MUST return a non-nil error on
// failure — callers use that as the signal to fall back to client-side
// rendering rather than serving a blank or partial diagram.
type Renderer interface {
	Render(source []byte) ([]byte, error)
}

// KrokiRenderer talks to a Kroki HTTP service. The default base URL
// targets the public instance at https://kroki.io which is rate-limited
// but adequate for low-traffic sites whose unique-diagram count is
// small (because cache hits skip the network entirely). For higher
// traffic, point BaseURL at a self-hosted Kroki container.
type KrokiRenderer struct {
	BaseURL    string        // e.g. "https://kroki.io"
	DiagramKey string        // Kroki diagram type, e.g. "mermaid"
	Timeout    time.Duration // per-request timeout (default 5s)
	HTTPClient *http.Client  // injectable for tests; nil → built from Timeout

	mu    sync.RWMutex
	cache map[string][]byte // key: sha256 of source; value: SVG bytes
}

// NewKrokiMermaid returns a KrokiRenderer pre-configured for mermaid.
// baseURL may be empty to use the public https://kroki.io instance.
func NewKrokiMermaid(baseURL string) *KrokiRenderer {
	if baseURL == "" {
		baseURL = "https://kroki.io"
	}
	return &KrokiRenderer{
		BaseURL:    baseURL,
		DiagramKey: "mermaid",
		Timeout:    5 * time.Second,
		cache:      make(map[string][]byte),
	}
}

// Render returns the SVG bytes for the given mermaid source. Cache
// hits (sha256-keyed) skip the network. On any HTTP error or non-2xx
// response the source is NOT cached — transient failures get retried
// on the next call.
func (k *KrokiRenderer) Render(source []byte) ([]byte, error) {
	key := hashKey(source)

	k.mu.RLock()
	if cached, ok := k.cache[key]; ok {
		k.mu.RUnlock()
		return cached, nil
	}
	k.mu.RUnlock()

	svg, err := k.fetch(source)
	if err != nil {
		return nil, err
	}

	k.mu.Lock()
	k.cache[key] = svg
	k.mu.Unlock()

	return svg, nil
}

// fetch performs the actual HTTP POST to Kroki. Kroki accepts the raw
// diagram source as the request body; the URL determines the diagram
// type and output format.
func (k *KrokiRenderer) fetch(source []byte) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/svg", k.BaseURL, k.DiagramKey)

	client := k.HTTPClient
	if client == nil {
		timeout := k.Timeout
		if timeout == 0 {
			timeout = 5 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("kroki: build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Accept", "image/svg+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kroki: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kroki: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Kroki returns a plain-text error body on syntax errors;
		// surface enough of it to be diagnostic without flooding logs.
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return nil, fmt.Errorf("kroki: status %d: %s", resp.StatusCode, snippet)
	}

	return body, nil
}

func hashKey(source []byte) string {
	h := sha256.Sum256(source)
	return hex.EncodeToString(h[:])
}
