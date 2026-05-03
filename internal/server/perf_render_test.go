package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// G4 (Phase 1 docs-site learning): every external script in the rendered
// page must be `defer`-loaded so the parser is not blocked. This locks the
// regression-target — if a future contributor re-introduces a synchronous
// <script src=...> at the bottom of a page, this test fires.
func TestRenderedPageScriptTagsAreDeferred(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "index.md"), []byte("---\ntitle: Home\n---\n# Home\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "index.md"}
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Home", Path: "index.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()

	// Every <script src=...> MUST carry the `defer` attribute.
	for _, line := range strings.Split(body, "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "<script") {
			continue
		}
		if !strings.Contains(l, "src=") {
			continue // inline scripts aren't required to defer
		}
		if !strings.Contains(l, "defer") {
			t.Errorf("script tag is render-blocking (no defer): %s", l)
		}
	}
}

// G4: Mermaid is conditional on HasMermaid — pages without mermaid blocks
// must not pay the ~3.3MB Mermaid runtime cost.
func TestMermaidScriptOmittedFromPagesWithoutMermaidBlocks(t *testing.T) {
	tmpDir := t.TempDir()

	plainContent := `---
title: "Plain"
---
# No diagrams here

Just prose, with a regular ` + "`code`" + ` span.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "plain.md"), []byte(plainContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "plain.md"}
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Plain", Path: "plain.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/plain", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "/assets/mermaid.js") {
		t.Errorf("mermaid script was included on a page with no ```mermaid blocks")
	}
}

func TestMermaidScriptIncludedOnPagesWithMermaidBlocks(t *testing.T) {
	tmpDir := t.TempDir()

	mermaidContent := `---
title: "Diagram"
---
# Flow

` + "```mermaid\nflowchart LR\n  A --> B\n```" + `
`
	if err := os.WriteFile(filepath.Join(tmpDir, "diagram.md"), []byte(mermaidContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "diagram.md"}
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Diagram", Path: "diagram.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/diagram", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "/assets/mermaid.js") {
		t.Errorf("mermaid script missing on a page that contains ```mermaid blocks")
	}
	if !strings.Contains(body, `<script defer src="/assets/mermaid.js">`) {
		t.Errorf("mermaid script must be deferred: %s", body)
	}
}
