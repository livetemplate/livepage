package server

import (
	"testing"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
)

// TestGetEffectiveSourcePrecedence covers the three-tier lookup order, and in
// particular the tier that exists for security rather than convenience.
//
// A page's frontmatter can declare its own sources. Without pinning, a generated page
// could reference an approved source name while defining that name as something the
// operator never approved — defeating any check that reasons about names alone, since
// the name really is in the approved set. Pinning makes an approved name mean one
// thing no matter what the page says.
//
// The two lower tiers (frontmatter beats site config) are long-standing behavior and
// must not change for projects that declare no generation block.
func TestGetEffectiveSourcePrecedence(t *testing.T) {
	const name = "requests"

	approved := config.SourceConfig{Type: "json", File: "./approved.json"}
	pageDefined := tinkerdown.SourceConfig{Type: "json", File: "./page.json"}

	pageWith := func(srcs map[string]tinkerdown.SourceConfig) *tinkerdown.Page {
		return &tinkerdown.Page{Config: tinkerdown.PageConfig{Sources: srcs}}
	}

	tests := []struct {
		name     string
		config   *config.Config
		page     *tinkerdown.Page
		wantFile string
		wantOK   bool
	}{
		{
			name: "approved source is pinned against a shadowing page",
			config: &config.Config{
				Sources:    map[string]config.SourceConfig{name: approved},
				Generation: &config.GenerationConfig{Sources: []string{name}},
			},
			page:     pageWith(map[string]tinkerdown.SourceConfig{name: pageDefined}),
			wantFile: "./approved.json",
			wantOK:   true,
		},
		{
			name: "without a generation block frontmatter still wins",
			config: &config.Config{
				Sources: map[string]config.SourceConfig{name: approved},
			},
			page:     pageWith(map[string]tinkerdown.SourceConfig{name: pageDefined}),
			wantFile: "./page.json",
			wantOK:   true,
		},
		{
			name: "a manifest does not pin names outside its approved set",
			config: &config.Config{
				Sources:    map[string]config.SourceConfig{name: approved},
				Generation: &config.GenerationConfig{Sources: []string{"something-else"}},
			},
			page:     pageWith(map[string]tinkerdown.SourceConfig{name: pageDefined}),
			wantFile: "./page.json",
			wantOK:   true,
		},
		{
			name: "approved name resolves from site config when the page declares nothing",
			config: &config.Config{
				Sources:    map[string]config.SourceConfig{name: approved},
				Generation: &config.GenerationConfig{Sources: []string{name}},
			},
			page:     pageWith(nil),
			wantFile: "./approved.json",
			wantOK:   true,
		},
		{
			name: "site config still serves as the fallback tier",
			config: &config.Config{
				Sources: map[string]config.SourceConfig{name: approved},
			},
			page:     pageWith(nil),
			wantFile: "./approved.json",
			wantOK:   true,
		},
		{
			name: "approving a name the project never defined does not invent a source",
			config: &config.Config{
				Generation: &config.GenerationConfig{Sources: []string{name}},
			},
			page:   pageWith(nil),
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &WebSocketHandler{config: tc.config, page: tc.page}
			got, ok := h.getEffectiveSource(name)
			if ok != tc.wantOK {
				t.Fatalf("getEffectiveSource(%q) ok = %v, want %v", name, ok, tc.wantOK)
			}
			if ok && got.File != tc.wantFile {
				t.Errorf("resolved to %q, want %q", got.File, tc.wantFile)
			}
		})
	}
}

// TestGetPageActionsPrecedence covers the action side, which needed two changes rather
// than one.
//
// Actions never had the site-config fallback sources have: a page could only invoke
// actions declared in its own frontmatter. That left `generation.actions` inert — it
// named a surface a generated page had no way to reach, forcing it to declare those
// actions itself, which is what approval exists to prevent. So approved actions must
// both become *reachable* and be *pinned* against redefinition.
func TestGetPageActionsPrecedence(t *testing.T) {
	const name = "approve-request"

	site := &config.Action{Kind: "sql", Statement: "SITE"}
	page := tinkerdown.Action{Kind: "sql", Statement: "PAGE"}

	handler := func(gen *config.GenerationConfig, pageActions map[string]tinkerdown.Action) *WebSocketHandler {
		return &WebSocketHandler{
			config: &config.Config{
				Actions:    map[string]*config.Action{name: site},
				Generation: gen,
			},
			page: &tinkerdown.Page{Config: tinkerdown.PageConfig{Actions: pageActions}},
		}
	}

	t.Run("approved site action is reachable without the page declaring it", func(t *testing.T) {
		h := handler(&config.GenerationConfig{Actions: []string{name}}, nil)
		got := h.getPageActions()
		if got[name] == nil {
			t.Fatalf("approved action %q unreachable; generation.actions would be inert", name)
		}
		if got[name].Statement != "SITE" {
			t.Errorf("resolved to %q, want SITE", got[name].Statement)
		}
	})

	t.Run("approved action is pinned against a shadowing page", func(t *testing.T) {
		h := handler(&config.GenerationConfig{Actions: []string{name}},
			map[string]tinkerdown.Action{name: page})
		if got := h.getPageActions()[name]; got == nil || got.Statement != "SITE" {
			t.Errorf("page redefinition won; approved actions must be pinned")
		}
	})

	t.Run("without a generation block site actions stay unreachable", func(t *testing.T) {
		// Long-standing behavior: actions written for schedules or webhooks are not
		// callable from arbitrary pages. Only approval opts an action in.
		h := handler(nil, nil)
		if got := h.getPageActions(); got[name] != nil {
			t.Errorf("site action leaked to a page with no generation block")
		}
	})

	t.Run("unapproved page actions still work", func(t *testing.T) {
		h := handler(&config.GenerationConfig{Actions: []string{name}},
			map[string]tinkerdown.Action{"page-only": page})
		got := h.getPageActions()
		if got["page-only"] == nil || got["page-only"].Statement != "PAGE" {
			t.Errorf("a page action outside the approved set should be unaffected")
		}
	})

	t.Run("approving an undefined action does not synthesise one", func(t *testing.T) {
		h := &WebSocketHandler{
			config: &config.Config{Generation: &config.GenerationConfig{Actions: []string{"typo"}}},
			page:   &tinkerdown.Page{},
		}
		if got := h.getPageActions(); got["typo"] != nil {
			t.Errorf("approving an undefined name invented an action")
		}
	})
}
