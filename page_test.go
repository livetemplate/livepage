package tinkerdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		filename  string
		content   string
		wantTitle string
		wantType  string
		checkPage func(*testing.T, *Page)
		wantError bool
	}{
		{
			name:     "complete tutorial",
			filename: "counter.md",
			content: `---
title: "Counter Tutorial"
type: tutorial
persist: localstorage
---

# Counter App

## Server State

` + "```go server readonly id=\"counter-state\"" + `
type CounterState struct {
    Counter int
}
` + "```" + `

## Interactive Demo

` + "```lvt interactive state=\"counter-state\"" + `
<button name="increment">{{.Counter}}</button>
` + "```" + `

## Try It

` + "```go wasm editable" + `
package main
func main() {}
` + "```",
			wantTitle: "Counter Tutorial",
			wantType:  "tutorial",
			checkPage: func(t *testing.T, p *Page) {
				// Check blocks
				if len(p.ServerBlocks) != 1 {
					t.Errorf("ServerBlocks count = %d, want 1", len(p.ServerBlocks))
				}
				if len(p.InteractiveBlocks) != 1 {
					t.Errorf("InteractiveBlocks count = %d, want 1", len(p.InteractiveBlocks))
				}
				if len(p.WasmBlocks) != 1 {
					t.Errorf("WasmBlocks count = %d, want 1", len(p.WasmBlocks))
				}

				// Check server block
				if sb, ok := p.ServerBlocks["counter-state"]; ok {
					if sb.Language != "go" {
						t.Errorf("ServerBlock language = %s, want go", sb.Language)
					}
					if sb.Content == "" {
						t.Error("ServerBlock content is empty")
					}
				} else {
					t.Error("ServerBlock 'counter-state' not found")
				}

				// Check interactive block references state
				for _, ib := range p.InteractiveBlocks {
					if ib.StateRef != "counter-state" {
						t.Errorf("InteractiveBlock StateRef = %s, want counter-state", ib.StateRef)
					}
				}

				// Check static HTML
				if p.StaticHTML == "" {
					t.Error("StaticHTML is empty")
				}
			},
		},
		{
			name:     "minimal page",
			filename: "simple.md",
			content: `---
title: "Simple Page"
---

# Simple

Just text, no code blocks.`,
			wantTitle: "Simple Page",
			wantType:  "tutorial",
			checkPage: func(t *testing.T, p *Page) {
				if len(p.ServerBlocks) != 0 {
					t.Error("Expected no server blocks")
				}
				if len(p.InteractiveBlocks) != 0 {
					t.Error("Expected no interactive blocks")
				}
				if len(p.WasmBlocks) != 0 {
					t.Error("Expected no wasm blocks")
				}
			},
		},
		{
			name:     "interactive without state reference",
			filename: "broken.md",
			content: `---
title: "Broken"
---

# Broken

` + "```lvt interactive" + `
<button>Click</button>
` + "```",
			wantError: true,
		},
		{
			name:     "interactive with invalid state reference",
			filename: "invalid-ref.md",
			content: `---
title: "Invalid"
---

# Invalid

` + "```lvt interactive state=\"nonexistent\"" + `
<button>Click</button>
` + "```",
			wantError: true,
		},
		{
			name:     "auto-generated block IDs",
			filename: "autoid.md",
			content: `---
title: "Auto IDs"
---

# Auto IDs

` + "```go server readonly" + `
type State1 struct {}
` + "```" + `

` + "```go server readonly" + `
type State2 struct {}
` + "```" + `

` + "```go wasm editable" + `
package main
` + "```",
			wantTitle: "Auto IDs",
			wantType:  "tutorial", // Default type
			checkPage: func(t *testing.T, p *Page) {
				if len(p.ServerBlocks) != 2 {
					t.Errorf("ServerBlocks count = %d, want 2", len(p.ServerBlocks))
				}

				// Check auto-generated IDs
				if _, ok := p.ServerBlocks["server-0"]; !ok {
					t.Error("Expected server-0 block")
				}
				if _, ok := p.ServerBlocks["server-1"]; !ok {
					t.Error("Expected server-1 block")
				}
				if _, ok := p.WasmBlocks["wasm-2"]; !ok {
					t.Error("Expected wasm-2 block")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test file
			path := filepath.Join(tmpDir, tt.filename)
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Parse file
			page, err := ParseFile(path)

			if tt.wantError {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			// Check basic fields
			if page.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", page.Title, tt.wantTitle)
			}
			if page.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", page.Type, tt.wantType)
			}

			// Run custom checks
			if tt.checkPage != nil {
				tt.checkPage(t, page)
			}
		})
	}
}

func TestPageConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		frontmatter string
		wantPersist PersistMode
		wantMultiStep bool
		wantSteps int
	}{
		{
			name: "default persist",
			frontmatter: `---
title: "Test"
---`,
			wantPersist: PersistLocalStorage,
			wantMultiStep: false,
			wantSteps: 0,
		},
		{
			name: "server persist",
			frontmatter: `---
title: "Test"
persist: server
---`,
			wantPersist: PersistServer,
		},
		{
			name: "multi-step tutorial",
			frontmatter: `---
title: "Test"
steps: 5
---`,
			wantPersist:   PersistLocalStorage, // Default
			wantMultiStep: true,
			wantSteps:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.frontmatter + "\n\n# Test\n\nContent"
			path := filepath.Join(tmpDir, "test.md")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			page, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			if page.Config.Persist != tt.wantPersist {
				t.Errorf("Persist = %q, want %q", page.Config.Persist, tt.wantPersist)
			}
			if page.Config.MultiStep != tt.wantMultiStep {
				t.Errorf("MultiStep = %v, want %v", page.Config.MultiStep, tt.wantMultiStep)
			}
			if page.Config.StepCount != tt.wantSteps {
				t.Errorf("StepCount = %d, want %d", page.Config.StepCount, tt.wantSteps)
			}
		})
	}
}

func TestParseFileInSite_PageRootConfinementByDefault(t *testing.T) {
	// ParseFile (no siteRoot) preserves the v1 behavior: includes are
	// confined to the markdown's own directory subtree. A `../` include
	// pointing at a sibling page's _app/ is rejected.
	siteDir := t.TempDir()
	pageA := filepath.Join(siteDir, "pageA")
	pageB := filepath.Join(siteDir, "pageB", "_app")
	if err := os.MkdirAll(pageA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pageB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pageB, "snippet.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageContent := "# Page A\n\n```go include=\"../pageB/_app/snippet.go\"\n```\n"
	pageMD := filepath.Join(pageA, "index.md")
	if err := os.WriteFile(pageMD, []byte(pageContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// ParseFile (legacy entry point) — page-root confined, the include
	// fails (warning logged), block renders empty/passthrough. We assert
	// the page DID render (heading present) AND the snippet content is
	// NOT present — without the heading check, an empty StaticHTML
	// would falsely pass.
	page, err := ParseFile(pageMD)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if !strings.Contains(page.StaticHTML, "Page A") {
		t.Fatalf("expected page to render its heading, got empty/missing StaticHTML: %q", page.StaticHTML)
	}
	if strings.Contains(page.StaticHTML, "package x") {
		t.Errorf("ParseFile should reject ../ include via page-root confinement; rendered HTML contained 'package x'")
	}
}

func TestParseFileInSite_RejectsPathOutsideSiteRoot(t *testing.T) {
	// ParseFileInSite enforces an explicit precondition: the markdown
	// path must be located within siteRoot. Otherwise every include —
	// even a local `include="snippet.go"` in the page's own directory —
	// would silently fail confinement against a root that isn't the
	// page's ancestor. Better to fail fast with a clear error than
	// produce confusing per-include warnings.
	pageOutside := t.TempDir() // outside any site
	siteRoot := t.TempDir()    // separate tree
	pageMD := filepath.Join(pageOutside, "stranded.md")
	if err := os.WriteFile(pageMD, []byte("# Stranded\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFileInSite(pageMD, siteRoot)
	if err == nil {
		t.Fatalf("expected error when path is not under siteRoot; got nil")
	}
	if !strings.Contains(err.Error(), "not under siteRoot") {
		t.Errorf("expected 'not under siteRoot' in error; got: %v", err)
	}
}

func TestParseFileInSite_AllowsSiblingPageIncludes(t *testing.T) {
	// ParseFileInSite (with siteRoot) widens confinement to the whole
	// site, so a page can include from a sibling page's _app/ — exactly
	// what livetemplate/docs needs for /getting-started/your-first-app
	// to cite /recipes/counter/_app/counter.go.
	siteDir := t.TempDir()
	pageA := filepath.Join(siteDir, "pageA")
	pageB := filepath.Join(siteDir, "pageB", "_app")
	if err := os.MkdirAll(pageA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pageB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pageB, "snippet.go"), []byte("package x\n// sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageMD := filepath.Join(pageA, "index.md")
	if err := os.WriteFile(pageMD, []byte("# Page A\n\n```go include=\"../pageB/_app/snippet.go\"\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParseFileInSite(pageMD, siteDir)
	if err != nil {
		t.Fatalf("ParseFileInSite error: %v", err)
	}
	if !strings.Contains(page.StaticHTML, "// sentinel") {
		t.Errorf("ParseFileInSite should resolve ../ include into siteRoot; rendered HTML missing snippet content. Got:\n%s", page.StaticHTML)
	}
}

func TestParseFileInSite_RejectsPathsEscapingSiteRoot(t *testing.T) {
	// Confinement still applies — a page cannot reach outside siteRoot
	// even when ParseFileInSite is used. siteRoot is the boundary, not
	// a removed check.
	siteDir := t.TempDir()
	outside := t.TempDir() // separate tree; will not be under siteDir
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), []byte("package secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageDir := filepath.Join(siteDir, "page")
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageMD := filepath.Join(pageDir, "index.md")
	rel, err := filepath.Rel(pageDir, filepath.Join(outside, "secret.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pageMD, []byte("# X\n\n```go include=\""+rel+"\"\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParseFileInSite(pageMD, siteDir)
	if err != nil {
		t.Fatalf("ParseFileInSite error: %v", err)
	}
	if strings.Contains(page.StaticHTML, "package secret") {
		t.Errorf("ParseFileInSite should reject path escaping siteRoot; rendered HTML contained leaked content")
	}
}

func TestParseFileInSite_EmptySiteRootBehavesLikeParseFile(t *testing.T) {
	// Documents the public-API guarantee: passing siteRoot="" is
	// equivalent to calling ParseFile. Lets callers pass through a
	// configured-or-empty value without branching.
	siteDir := t.TempDir()
	pageA := filepath.Join(siteDir, "pageA")
	pageB := filepath.Join(siteDir, "pageB", "_app")
	if err := os.MkdirAll(pageA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pageB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pageB, "snippet.go"), []byte("package x\n// sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageMD := filepath.Join(pageA, "index.md")
	if err := os.WriteFile(pageMD, []byte("# A\n\n```go include=\"../pageB/_app/snippet.go\"\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParseFileInSite(pageMD, "")
	if err != nil {
		t.Fatalf("ParseFileInSite(empty siteRoot) error: %v", err)
	}
	if strings.Contains(page.StaticHTML, "// sentinel") {
		t.Errorf("ParseFileInSite with empty siteRoot should fall back to page-root confinement; cross-page include leaked through")
	}
}
