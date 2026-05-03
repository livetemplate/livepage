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

// Regression: G16-era + responsive-sidebar fix.
//
// Two long-standing layout bugs were caught after launch by visual
// inspection of livetemplate.fly.dev and locked in here:
//
//  1. .nav-pages didn't override PicoCSS's `nav ul { display: flex }`,
//     so sidebar items rendered horizontally (cramped, truncated).
//  2. Mobile breakpoint hid the sidebar with translateX(-100%) but
//     emitted no hamburger button — the sidebar was unreachable.
//
// These tests fail if a future change reintroduces either bug.

func TestSidebarOverridesPicoFlex(t *testing.T) {
	body := renderHomeWithSidebar(t)
	idx := strings.Index(body, ".nav-pages {")
	if idx < 0 {
		t.Fatal(".nav-pages CSS rule missing")
	}
	// The rule comment contains `}` so a naive close-brace scan cuts off
	// too early. Instead just check the next ~500 chars after the rule
	// opens for the explicit display override. Without it, PicoCSS's
	// `nav ul { display: flex }` cascades through and re-introduces
	// the cramped-horizontal-sidebar bug.
	window := body[idx:]
	if len(window) > 500 {
		window = window[:500]
	}
	if !strings.Contains(window, "display: block") {
		t.Errorf(".nav-pages must declare `display: block` to override PicoCSS nav-ul flex; first 500 chars after .nav-pages:\n%s", window)
	}
}

// PicoCSS sets `nav li { display: inline-block }` to lay out top-nav
// items horizontally. The first sidebar fix overrode the UL but not
// the LI, so list items continued packing inline-block in the sidebar
// (~135px wide each, two-or-three short titles still fit on one line).
// This locks the second-level override.
func TestSidebarLiIsListItem(t *testing.T) {
	body := renderHomeWithSidebar(t)
	idx := strings.Index(body, ".nav-pages li {")
	if idx < 0 {
		t.Fatal(".nav-pages li CSS rule missing — without it, PicoCSS lays out items as inline-block")
	}
	window := body[idx:]
	if len(window) > 500 {
		window = window[:500]
	}
	if !strings.Contains(window, "display: list-item") && !strings.Contains(window, "display: block") {
		t.Errorf(".nav-pages li must override PicoCSS's display: inline-block (use list-item or block); rule window:\n%s", window)
	}
}

func TestSidebarRendersMobileToggleAndBackdrop(t *testing.T) {
	body := renderHomeWithSidebar(t)

	if !strings.Contains(body, `class="tinkerdown-nav-toggle"`) {
		t.Error("hamburger toggle button missing — mobile users have no way to open the sidebar")
	}
	if !strings.Contains(body, `aria-label="Toggle navigation"`) {
		t.Error("hamburger button missing aria-label (a11y regression)")
	}
	if !strings.Contains(body, `aria-controls="tinkerdown-sidebar"`) {
		t.Error("hamburger missing aria-controls — assistive tech can't link button → menu")
	}
	if !strings.Contains(body, `class="tinkerdown-nav-backdrop"`) {
		t.Error("backdrop element missing — mobile users can't tap-outside to close")
	}

	// CSS must hide the toggle by default (desktop) and reveal it under
	// the mobile media query.
	if !strings.Contains(body, ".tinkerdown-nav-toggle {\n            display: none;") {
		t.Error(".tinkerdown-nav-toggle should default to display: none (hidden on desktop)")
	}
	// The page emits multiple `@media (max-width: 768px)` rules (one per
	// component). Rather than parse them all, just assert the
	// toggle-flips-to-flex rule exists anywhere in the document. The
	// rule body the responsive fix introduces:
	//
	//     .tinkerdown-nav-toggle {
	//         display: flex;
	//     }
	if !strings.Contains(body, ".tinkerdown-nav-toggle {\n                display: flex;") {
		t.Error("mobile breakpoint must flip toggle to display: flex (regression: sidebar unreachable on mobile)")
	}
}

func TestSidebarToggleScriptIsBound(t *testing.T) {
	body := renderHomeWithSidebar(t)
	// The inline toggle script needs to be present and use the
	// double-bind guard so hot-reload doesn't double-wire handlers.
	if !strings.Contains(body, "toggle.dataset.bound") {
		t.Error("toggle JS should set/check dataset.bound to avoid double-binding on re-render")
	}
	if !strings.Contains(body, "Escape") {
		t.Error("toggle JS should close the sidebar on Escape (a11y / UX expectation)")
	}
}

// renderHomeWithSidebar boots a tiny in-memory site, hits /, and returns
// the rendered HTML body.
func renderHomeWithSidebar(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "home.md"), []byte("---\ntitle: Home\n---\n# Home\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "Test Docs"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Features.Sidebar = true
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Home", Path: "home.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/home", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

