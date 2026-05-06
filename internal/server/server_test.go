package server

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMdToPattern(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"index.md", "/"},
		{"counter.md", "/counter"},
		{"tutorials/intro.md", "/tutorials/intro"},
		{"tutorials/index.md", "/tutorials/"},
		{"advanced/state.md", "/advanced/state"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mdToPattern(tt.input)
			if got != tt.want {
				t.Errorf("mdToPattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestServerDiscover(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create test markdown files
	files := map[string]string{
		"index.md": `---
title: "Home"
---
# Home Page`,
		"counter.md": `---
title: "Counter"
---
# Counter Tutorial`,
		"tutorials/intro.md": `---
title: "Introduction"
---
# Introduction`,
		"_drafts/draft.md": `---
title: "Draft"
---
# Draft (should be ignored)`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	// Create server and discover
	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Check discovered routes
	routes := srv.Routes()
	if len(routes) != 3 { // index, counter, tutorials/intro (not _drafts)
		t.Errorf("Expected 3 routes, got %d", len(routes))
		for _, r := range routes {
			t.Logf("  Route: %s -> %s", r.Pattern, r.FilePath)
		}
	}

	// Check specific routes exist
	patterns := make(map[string]bool)
	for _, r := range routes {
		patterns[r.Pattern] = true
	}

	if !patterns["/"] {
		t.Error("Expected route / not found")
	}
	if !patterns["/counter"] {
		t.Error("Expected route /counter not found")
	}
	if !patterns["/tutorials/intro"] {
		t.Error("Expected route /tutorials/intro not found")
	}
	if patterns["/_drafts/draft"] {
		t.Error("Route /_drafts/draft should not exist (underscore prefix)")
	}
}

func TestServerServeHTTP(t *testing.T) {
	// Create temp directory with test file
	tmpDir := t.TempDir()

	content := `---
title: "Test Page"
---
# Test Content

This is a test page.`

	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Create and discover
	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Test HTTP request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Response body is empty")
	}

	// Check that HTML contains title
	if !strings.Contains(body, "Test Page") {
		t.Error("Response does not contain page title")
	}

	// Check that HTML contains content
	if !strings.Contains(body, "Test Content") {
		t.Error("Response does not contain page content")
	}

	// HTML responses must opt out of caching so that intermediate caches
	// (browser, CDN, fly.io edge) can't pin a stale render alongside
	// fresh long-cached static assets — a divergence that produces the
	// "first paint missing styles, refresh fixes it" symptom class.
	if got := resp.Header.Get("Cache-Control"); got != "no-cache, must-revalidate" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache, must-revalidate")
	}

	// Every inline SVG in the page chrome must declare its rendered
	// dimensions as HTML attributes (width="..." height="...") rather
	// than relying solely on a CSS rule like `.theme-toggle button svg
	// { width: 1.1rem }` to size them. Without explicit attributes,
	// browsers render an SVG at its intrinsic ~300×150 default for the
	// brief window between HTML parsing and the inline <style> block
	// being applied — which manifests as a giant logo-blob covering
	// the toolbar on first paint, only resolving on reload because by
	// then the CSS is in cache and applies before paint. Pinning
	// dimensions in markup makes the size correct from the first byte.
	svgIdx := 0
	for i := 0; i < len(body); {
		open := strings.Index(body[i:], "<svg")
		if open == -1 {
			break
		}
		open += i
		close := strings.Index(body[open:], ">")
		if close == -1 {
			break
		}
		tag := body[open : open+close+1]
		i = open + close + 1
		// Skip the body-content mermaid SVG, which carries its own
		// explicit width/height (verified separately by mermaid tests).
		if strings.Contains(tag, `id="container"`) {
			continue
		}
		if !strings.Contains(tag, "width=") || !strings.Contains(tag, "height=") {
			t.Errorf("SVG #%d in page chrome lacks width/height attributes — will render at intrinsic 300x150 before CSS applies. Tag: %s", svgIdx, tag)
		}
		svgIdx++
	}
}

func TestServerNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	resp := w.Result()
	// Server now redirects unknown pages to home instead of returning 404
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusNotFound {
		t.Errorf("Status = %d, want %d or %d", resp.StatusCode, http.StatusSeeOther, http.StatusNotFound)
	}
}

func TestSortRoutes(t *testing.T) {
	routes := []*Route{
		{Pattern: "/counter"},
		{Pattern: "/"},
		{Pattern: "/tutorials/intro"},
		{Pattern: "/tutorials/"},
		{Pattern: "/advanced"},
	}

	sortRoutes(routes)

	// Root should be first
	if routes[0].Pattern != "/" {
		t.Errorf("First route = %s, want /", routes[0].Pattern)
	}

	// Find position of directory index
	tutorialsIdx := -1
	for i, r := range routes {
		if r.Pattern == "/tutorials/" {
			tutorialsIdx = i
			break
		}
	}

	// Check that /tutorials/ comes before /tutorials/intro
	introIdx := -1
	for i, r := range routes {
		if r.Pattern == "/tutorials/intro" {
			introIdx = i
			break
		}
	}

	if tutorialsIdx != -1 && introIdx != -1 && tutorialsIdx > introIdx {
		t.Error("/tutorials/ should come before /tutorials/intro")
	}
}

func TestWebSocketURLContainsPage(t *testing.T) {
	// Create temp directory with multiple test pages
	tmpDir := t.TempDir()

	files := map[string]string{
		"index.md": `---
title: "Home"
---
# Home Page`,
		"counter.md": `---
title: "Counter"
---
# Counter Tutorial`,
		"getting-started.md": `---
title: "Getting Started"
---
# Getting Started Guide`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	// Create and discover
	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Test each page has correct WebSocket URL with page parameter
	tests := []struct {
		path         string
		expectedPage string
	}{
		{"/", "%2F"},                            // / URL-encoded
		{"/counter", "%2Fcounter"},              // /counter URL-encoded
		{"/getting-started", "%2Fgetting-started"}, // /getting-started URL-encoded
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = "localhost:8080"
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			body := w.Body.String()

			// Check that the WebSocket URL contains the correct page parameter
			expectedWSURL := "ws://localhost:8080/ws?page=" + tt.expectedPage
			if !strings.Contains(body, expectedWSURL) {
				t.Errorf("Expected WebSocket URL with page=%s not found in response for path %s", tt.expectedPage, tt.path)
			}
		})
	}
}

func TestServeWebSocketPageRouting(t *testing.T) {
	// Create temp directory with multiple test pages
	tmpDir := t.TempDir()

	files := map[string]string{
		"index.md": `---
title: "Home"
---
# Home Page`,
		"counter.md": `---
title: "Counter"
---
# Counter Tutorial`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	// Create and discover
	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Helper to capture log output during request
	captureLogOutput := func(fn func()) string {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(os.Stderr)
		fn()
		return buf.String()
	}

	// Test that /ws endpoint without page parameter defaults to home
	t.Run("default to home page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ws", nil)
		w := httptest.NewRecorder()

		logOutput := captureLogOutput(func() {
			srv.ServeHTTP(w, req)
		})

		resp := w.Result()
		// WebSocket upgrade fails with 400 Bad Request (no proper handshake)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Status = %d, want %d (failed WebSocket upgrade)", resp.StatusCode, http.StatusBadRequest)
		}

		// Verify log shows routing to home page (pattern: /)
		if !strings.Contains(logOutput, "WebSocket connection for page: / (pattern: /)") {
			t.Errorf("Expected log to show routing to home page, got: %s", logOutput)
		}
	})

	// Test that /ws?page=/counter routes to counter page
	t.Run("route to specific page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ws?page=/counter", nil)
		w := httptest.NewRecorder()

		logOutput := captureLogOutput(func() {
			srv.ServeHTTP(w, req)
		})

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Status = %d, want %d (failed WebSocket upgrade)", resp.StatusCode, http.StatusBadRequest)
		}

		// Verify log shows routing to counter page
		if !strings.Contains(logOutput, "WebSocket connection for page: /counter (pattern: /counter)") {
			t.Errorf("Expected log to show routing to /counter, got: %s", logOutput)
		}
	})

	// Test that unknown page falls back gracefully
	t.Run("unknown page fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ws?page=/nonexistent", nil)
		w := httptest.NewRecorder()

		logOutput := captureLogOutput(func() {
			srv.ServeHTTP(w, req)
		})

		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Status = %d, want %d (failed WebSocket upgrade)", resp.StatusCode, http.StatusBadRequest)
		}

		// Verify log shows fallback behavior
		if !strings.Contains(logOutput, `Page "/nonexistent" not found, falling back to first route`) {
			t.Errorf("Expected fallback log message, got: %s", logOutput)
		}
		// Verify it fell back to home page (first route)
		if !strings.Contains(logOutput, "WebSocket connection for page: /nonexistent (pattern: /)") {
			t.Errorf("Expected log to show fallback to home page, got: %s", logOutput)
		}
	})
}

func TestWebSocketURLEncodingDecoding(t *testing.T) {
	// Test that URL-encoded page paths are correctly decoded by the server
	tmpDir := t.TempDir()

	// Create page with special characters in the path
	files := map[string]string{
		"index.md": `---
title: "Home"
---
# Home`,
		"getting-started.md": `---
title: "Getting Started"
---
# Getting Started`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Helper to capture log output
	captureLogOutput := func(fn func()) string {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(os.Stderr)
		fn()
		return buf.String()
	}

	// Test URL-encoded path is correctly decoded
	// %2Fgetting-started should decode to /getting-started
	t.Run("URL-encoded path decoded correctly", func(t *testing.T) {
		// The client sends URL-encoded paths, which Go's Query().Get() decodes automatically
		req := httptest.NewRequest("GET", "/ws?page=%2Fgetting-started", nil)
		w := httptest.NewRecorder()

		logOutput := captureLogOutput(func() {
			srv.ServeHTTP(w, req)
		})

		// Verify the path was decoded and matched correctly
		if !strings.Contains(logOutput, "WebSocket connection for page: /getting-started (pattern: /getting-started)") {
			t.Errorf("Expected URL-encoded path to be decoded and matched, got: %s", logOutput)
		}
	})
}

func TestServeWebSocketEmptyRoutes(t *testing.T) {
	// Test WebSocket handling when no routes are configured
	tmpDir := t.TempDir()

	// Create server without any markdown files
	srv := New(tmpDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Verify no routes were discovered
	if len(srv.Routes()) != 0 {
		t.Fatalf("Expected 0 routes, got %d", len(srv.Routes()))
	}

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	resp := w.Result()
	// Should return 404 "No pages available" when no routes exist
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Status = %d, want %d for empty routes", resp.StatusCode, http.StatusNotFound)
	}

	// Verify error message
	body := w.Body.String()
	if !strings.Contains(body, "No pages available") {
		t.Errorf("Expected 'No pages available' error, got: %s", body)
	}
}
