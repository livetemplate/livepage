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

// Nested navigation: a section's pages may themselves be groups, so a long
// section (e.g. UI Patterns with 33 entries) renders as collapsible category
// sub-groups instead of one flat list. Groups + sections are native
// <details>/<summary>, so collapse needs no JS. These tests lock the three
// things that can silently break:
//
//  1. Registration — every nested leaf, at any depth, must still be served
//     (a missed leaf in the recursion is a silent 404 in site mode).
//  2. Structure — groups render as <details class="nav-group"> with a
//     <summary class="nav-group-title">, while top-level sections keep
//     class "nav-section-title" (the docs IA test counts those).
//  3. Collapse state — a section/group is `open` when not collapsed OR when
//     it holds the active page; otherwise closed.

// renderNestedSidebar builds a site with a nested "UI Patterns" section and
// returns the rendered HTML for the requested URL path (plus the server so a
// caller can issue more requests).
func renderNestedSidebar(t *testing.T, requestPath string) (body string, srv *Server, dir string) {
	t.Helper()
	tmpDir := t.TempDir()

	pages := map[string]string{
		"home.md":         "# Home",
		"forms/click.md":  "# Click to Edit",
		"forms/edit.md":   "# Edit Row",
		"lists/delete.md": "# Delete Row",
	}
	for rel, h1 := range pages {
		abs := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("---\ntitle: T\n---\n"+h1+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "Test Docs"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Features.Sidebar = true
	cfg.Navigation = []config.NavSection{
		{
			Title: "Top",
			Pages: []config.NavPage{{Title: "Home", Path: "home.md"}},
		},
		{
			Title:     "UI Patterns",
			Collapsed: true,
			Pages: []config.NavPage{
				{
					Title:     "Forms & Editing",
					Collapsed: true,
					Pages: []config.NavPage{
						{Title: "Click to Edit", Path: "forms/click.md"},
						{Title: "Edit Row", Path: "forms/edit.md"},
					},
				},
				{
					Title:     "Lists & Data",
					Collapsed: true,
					Pages:     []config.NavPage{{Title: "Delete Row", Path: "lists/delete.md"}},
				},
			},
		},
	}

	srv = NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", requestPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200; body=%s", requestPath, w.Code, w.Body.String())
	}
	return w.Body.String(), srv, tmpDir
}

// TestNestedLeavesAreServed is the registration invariant: every nested leaf,
// at any depth, resolves to 200. A regression here means deep links 404.
func TestNestedLeavesAreServed(t *testing.T) {
	_, srv, _ := renderNestedSidebar(t, "/forms/click")
	for _, path := range []string{"/forms/click", "/forms/edit", "/lists/delete"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("nested leaf %s = %d, want 200 (registration missed it)", path, w.Code)
		}
	}
}

// TestNestedSidebarStructure asserts groups render as <details> sub-groups
// with the group-title class, and top-level sections keep the section-title
// class (so the docs IA section count stays correct).
func TestNestedSidebarStructure(t *testing.T) {
	body, _, _ := renderNestedSidebar(t, "/forms/click")

	// Two top-level sections, two category groups.
	if got := strings.Count(body, `class="nav-section-title"`); got != 2 {
		t.Errorf("nav-section-title count = %d, want 2 (Top, UI Patterns)", got)
	}
	if got := strings.Count(body, `class="nav-group-title"`); got != 2 {
		t.Errorf("nav-group-title count = %d, want 2 (Forms, Lists)", got)
	}
	for _, want := range []string{
		`<summary class="nav-section-title">UI Patterns</summary>`,
		`<summary class="nav-group-title">Forms & Editing</summary>`,
		`<summary class="nav-group-title">Lists & Data</summary>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sidebar missing %q", want)
		}
	}
	// The Pico-override CSS must ship, or groups inherit Pico's accordion
	// margins/colors and the hierarchy reads wrong.
	if !strings.Contains(body, "#tinkerdown-sidebar .nav-group-title") {
		t.Error("nested-nav CSS override (#tinkerdown-sidebar .nav-group-title) missing")
	}
}

// TestNestedCollapseState asserts the open/closed logic: the group + section
// holding the active page open; sibling groups stay closed.
func TestNestedCollapseState(t *testing.T) {
	// Active page is /forms/click — UI Patterns + Forms open, Lists closed.
	body, _, _ := renderNestedSidebar(t, "/forms/click")

	if !strings.Contains(body, `<details class="nav-section" open><summary class="nav-section-title">UI Patterns</summary>`) {
		t.Error("UI Patterns section should be open (holds active page)")
	}
	if !strings.Contains(body, `<details class="nav-group" open><summary class="nav-group-title">Forms & Editing</summary>`) {
		t.Error("Forms group should be open (holds active page)")
	}
	if !strings.Contains(body, `<details class="nav-group"><summary class="nav-group-title">Lists & Data</summary>`) {
		t.Error("Lists group should be closed (collapsed, does not hold active page)")
	}

	// Active page is /home (in Top). The collapsed UI Patterns section, and
	// both its groups, should now be closed.
	body2, _, _ := renderNestedSidebar(t, "/home")
	if !strings.Contains(body2, `<details class="nav-section"><summary class="nav-section-title">UI Patterns</summary>`) {
		t.Error("UI Patterns section should be closed when active page is elsewhere")
	}
	if strings.Contains(body2, `<details class="nav-group" open>`) {
		t.Error("no group should be open when active page is outside UI Patterns")
	}
}
