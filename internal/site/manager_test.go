package site

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// writeNavSite creates a tmp site with the given relative .md files (each gets
// a trivial body) and returns a discovered Manager for cfg.
func writeNavSite(t *testing.T, cfg *config.Config, files ...string) *Manager {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("---\ntitle: T\n---\n# "+rel+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(dir, cfg)
	if err := m.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return m
}

// TestNestedRegistrationAndSearchSection covers two things the recursive nav
// builder must get right at depth: (1) a leaf three levels deep (section →
// group → subgroup → leaf) is still registered/served, and (2) the search
// index resolves that nested leaf back to its top-level section title.
func TestNestedRegistrationAndSearchSection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "T"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Navigation = []config.NavSection{
		{
			Title: "Patterns",
			Pages: []config.NavPage{
				{Title: "Home", Path: "home.md"},
				{
					Title: "Forms",
					Pages: []config.NavPage{
						{
							Title: "Advanced",
							Pages: []config.NavPage{
								{Title: "Deep Leaf", Path: "forms/advanced/deep.md"},
							},
						},
					},
				},
			},
		},
	}

	m := writeNavSite(t, cfg, "home.md", "forms/advanced/deep.md")

	// Depth-3 leaf registered.
	if _, ok := m.GetPage("/forms/advanced/deep"); !ok {
		t.Fatal("depth-3 nested leaf /forms/advanced/deep not registered")
	}

	// Search index resolves it to the top-level section.
	var got string
	for _, e := range m.GenerateSearchIndex() {
		if e.Path == "/forms/advanced/deep" {
			got = e.Section
		}
	}
	if got != "Patterns" {
		t.Errorf("search section for nested leaf = %q, want %q", got, "Patterns")
	}
}

// TestEmptyNavEntryErrors verifies a malformed nav entry — neither path nor
// pages — fails Discover loudly rather than vanishing from the sidebar.
func TestEmptyNavEntryErrors(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "T"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Navigation = []config.NavSection{
		{Title: "Sec", Pages: []config.NavPage{{Title: "Broken"}}},
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "home.md"), []byte("# Home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(dir, cfg).Discover(); err == nil {
		t.Fatal("Discover should error on a nav entry with neither path nor pages")
	}
}

// TestGroupWithLandingPage covers a group entry that carries both a Path (its
// own landing page) and Pages (children): both the landing page and the
// children must be served.
func TestGroupWithLandingPage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "T"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Navigation = []config.NavSection{
		{
			Title: "Sec",
			Pages: []config.NavPage{
				{Title: "Home", Path: "home.md"},
				{
					Title: "Group",
					Path:  "group/landing.md", // landing page for the group
					Pages: []config.NavPage{
						{Title: "Child", Path: "group/child.md"},
					},
				},
			},
		},
	}

	m := writeNavSite(t, cfg, "home.md", "group/landing.md", "group/child.md")

	for _, want := range []string{"/group/landing", "/group/child"} {
		if _, ok := m.GetPage(want); !ok {
			t.Errorf("page %s not registered (group landing-page + children must both serve)", want)
		}
	}
}
