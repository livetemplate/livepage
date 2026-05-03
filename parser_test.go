package tinkerdown

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   Frontmatter
		wantBody string
	}{
		{
			name: "complete frontmatter",
			content: `---
title: "Test Tutorial"
type: guide
persist: server
steps: 5
---

# Hello World`,
			wantFM: Frontmatter{
				Title:   "Test Tutorial",
				Type:    "guide",
				Persist: PersistServer,
				Steps:   5,
			},
			wantBody: "# Hello World",
		},
		{
			name: "no frontmatter",
			content: `# Hello World

Some content`,
			wantFM: Frontmatter{
				Type:    "tutorial",
				Persist: PersistLocalStorage,
			},
			wantBody: "# Hello World\n\nSome content",
		},
		{
			name: "minimal frontmatter",
			content: `---
title: "Simple"
---

Content`,
			wantFM: Frontmatter{
				Title:   "Simple",
				Type:    "tutorial",
				Persist: PersistLocalStorage,
			},
			wantBody: "Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, remaining, err := extractFrontmatter([]byte(tt.content))
			if err != nil {
				t.Fatalf("extractFrontmatter() error = %v", err)
			}

			if fm.Title != tt.wantFM.Title {
				t.Errorf("Title = %q, want %q", fm.Title, tt.wantFM.Title)
			}
			if fm.Type != tt.wantFM.Type {
				t.Errorf("Type = %q, want %q", fm.Type, tt.wantFM.Type)
			}
			if fm.Persist != tt.wantFM.Persist {
				t.Errorf("Persist = %q, want %q", fm.Persist, tt.wantFM.Persist)
			}
			if fm.Steps != tt.wantFM.Steps {
				t.Errorf("Steps = %d, want %d", fm.Steps, tt.wantFM.Steps)
			}

			body := strings.TrimSpace(string(remaining))
			want := strings.TrimSpace(tt.wantBody)
			if body != want {
				t.Errorf("remaining body = %q, want %q", body, want)
			}
		})
	}
}

func TestParseCodeBlocks(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantBlocks int
		checkBlock func(*testing.T, *CodeBlock)
	}{
		{
			name: "server readonly block",
			content: `# Tutorial

` + "```go server readonly id=\"counter-state\"" + `
type CounterState struct {
    Counter int
}
` + "```" + `

Text after`,
			wantBlocks: 1,
			checkBlock: func(t *testing.T, b *CodeBlock) {
				if b.Type != "server" {
					t.Errorf("Type = %q, want \"server\"", b.Type)
				}
				if b.Language != "go" {
					t.Errorf("Language = %q, want \"go\"", b.Language)
				}
				if !containsString(b.Flags, "readonly") {
					t.Errorf("Flags = %v, want to contain \"readonly\"", b.Flags)
				}
				if b.Metadata["id"] != "counter-state" {
					t.Errorf("Metadata[id] = %q, want \"counter-state\"", b.Metadata["id"])
				}
				if !strings.Contains(b.Content, "CounterState") {
					t.Errorf("Content missing \"CounterState\"")
				}
			},
		},
		{
			name: "wasm editable block",
			content: `# Tutorial

` + "```go wasm editable" + `
package main
func main() {}
` + "```",
			wantBlocks: 1,
			checkBlock: func(t *testing.T, b *CodeBlock) {
				if b.Type != "wasm" {
					t.Errorf("Type = %q, want \"wasm\"", b.Type)
				}
				if !containsString(b.Flags, "editable") {
					t.Errorf("Flags = %v, want to contain \"editable\"", b.Flags)
				}
				if !strings.Contains(b.Content, "package main") {
					t.Errorf("Content missing \"package main\"")
				}
			},
		},
		{
			name: "lvt interactive block",
			content: `# Tutorial

` + "```lvt interactive state=\"counter\"" + `
<button name="increment">{{.Counter}}</button>
` + "```",
			wantBlocks: 1,
			checkBlock: func(t *testing.T, b *CodeBlock) {
				if b.Type != "lvt" {
					t.Errorf("Type = %q, want \"lvt\"", b.Type)
				}
				if b.Language != "lvt" {
					t.Errorf("Language = %q, want \"lvt\"", b.Language)
				}
				if b.Metadata["state"] != "counter" {
					t.Errorf("Metadata[state] = %q, want \"counter\"", b.Metadata["state"])
				}
				if !strings.Contains(b.Content, `name="increment"`) {
					t.Errorf("Content missing button name=\"increment\"")
				}
			},
		},
		{
			name: "regular code block ignored",
			content: `# Tutorial

` + "```go" + `
func Hello() {}
` + "```",
			wantBlocks: 0,
		},
		{
			name: "multiple blocks",
			content: `# Tutorial

` + "```go server readonly id=\"state\"" + `
type State struct {}
` + "```" + `

Some text

` + "```lvt interactive state=\"state\"" + `
<div>Interactive</div>
` + "```" + `

More text

` + "```go wasm editable" + `
package main
` + "```",
			wantBlocks: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, blocks, _, err := ParseMarkdown([]byte(tt.content))
			if err != nil {
				t.Fatalf("ParseMarkdown() error = %v", err)
			}

			if len(blocks) != tt.wantBlocks {
				t.Fatalf("got %d blocks, want %d", len(blocks), tt.wantBlocks)
			}

			if tt.checkBlock != nil && len(blocks) > 0 {
				tt.checkBlock(t, blocks[0])
			}
		})
	}
}

func TestParseMarkdownComplete(t *testing.T) {
	content := `---
title: "Counter Tutorial"
type: tutorial
persist: localstorage
---

# Build a Counter

Learn how to build a counter.

## Step 1: State

` + "```go server readonly id=\"counter-state\"" + `
type CounterState struct {
    Counter int ` + "`json:\"counter\"`" + `
}

// Increment handles the "increment" action
func (s *CounterState) Increment(_ *livetemplate.Context) error {
    s.Counter++
    return nil
}
` + "```" + `

This defines the state.

## Step 2: UI

` + "```lvt interactive state=\"counter-state\"" + `
<div>
    <h2>Count: {{.Counter}}</h2>
    <button name="increment">+1</button>
</div>
` + "```" + `

Click to increment!

## Try It

` + "```go wasm editable" + `
package main

import "fmt"

func main() {
    fmt.Println("Hello")
}
` + "```" + `

Modify and run!
`

	fm, blocks, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	// Check frontmatter
	if fm.Title != "Counter Tutorial" {
		t.Errorf("Title = %q, want \"Counter Tutorial\"", fm.Title)
	}
	if fm.Type != "tutorial" {
		t.Errorf("Type = %q, want \"tutorial\"", fm.Type)
	}
	if fm.Persist != PersistLocalStorage {
		t.Errorf("Persist = %q, want \"localstorage\"", fm.Persist)
	}

	// Check blocks
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}

	// Block 1: server
	if blocks[0].Type != "server" {
		t.Errorf("Block 0 Type = %q, want \"server\"", blocks[0].Type)
	}
	if blocks[0].Metadata["id"] != "counter-state" {
		t.Errorf("Block 0 id = %q, want \"counter-state\"", blocks[0].Metadata["id"])
	}

	// Block 2: lvt
	if blocks[1].Type != "lvt" {
		t.Errorf("Block 1 Type = %q, want \"lvt\"", blocks[1].Type)
	}
	if blocks[1].Metadata["state"] != "counter-state" {
		t.Errorf("Block 1 state = %q, want \"counter-state\"", blocks[1].Metadata["state"])
	}

	// Block 3: wasm
	if blocks[2].Type != "wasm" {
		t.Errorf("Block 2 Type = %q, want \"wasm\"", blocks[2].Type)
	}

	// Check HTML (should contain prose)
	if !strings.Contains(html, "Build a Counter") {
		t.Error("HTML missing title")
	}
	if !strings.Contains(html, "Learn how to build") {
		t.Error("HTML missing intro text")
	}
	// Note: readonly code blocks are now intentionally included in HTML
	// for educational/documentation purposes, so we don't check that they're excluded
}

func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func TestParseFrontmatterWithSources(t *testing.T) {
	content := `---
title: "Source Test"
sources:
  users:
    type: json
    file: users.json
  api_data:
    type: rest
    from: https://api.example.com/data
  db_users:
    type: pg
    query: "SELECT * FROM users"
  shell_data:
    type: exec
    cmd: ./get-data.sh
  products:
    type: csv
    file: products.csv
---

# Test Content`

	fm, _, err := extractFrontmatter([]byte(content))
	if err != nil {
		t.Fatalf("extractFrontmatter() error = %v", err)
	}

	if fm.Title != "Source Test" {
		t.Errorf("Title = %q, want %q", fm.Title, "Source Test")
	}

	if len(fm.Sources) != 5 {
		t.Fatalf("got %d sources, want 5", len(fm.Sources))
	}

	// Check JSON source
	jsonSrc, ok := fm.Sources["users"]
	if !ok {
		t.Fatal("missing 'users' source")
	}
	if jsonSrc.Type != "json" {
		t.Errorf("users.Type = %q, want %q", jsonSrc.Type, "json")
	}
	if jsonSrc.File != "users.json" {
		t.Errorf("users.File = %q, want %q", jsonSrc.File, "users.json")
	}

	// Check REST source
	restSrc, ok := fm.Sources["api_data"]
	if !ok {
		t.Fatal("missing 'api_data' source")
	}
	if restSrc.Type != "rest" {
		t.Errorf("api_data.Type = %q, want %q", restSrc.Type, "rest")
	}
	if restSrc.From != "https://api.example.com/data" {
		t.Errorf("api_data.From = %q, want %q", restSrc.From, "https://api.example.com/data")
	}

	// Check PostgreSQL source
	pgSrc, ok := fm.Sources["db_users"]
	if !ok {
		t.Fatal("missing 'db_users' source")
	}
	if pgSrc.Type != "pg" {
		t.Errorf("db_users.Type = %q, want %q", pgSrc.Type, "pg")
	}
	if pgSrc.Query != "SELECT * FROM users" {
		t.Errorf("db_users.Query = %q, want %q", pgSrc.Query, "SELECT * FROM users")
	}

	// Check exec source
	execSrc, ok := fm.Sources["shell_data"]
	if !ok {
		t.Fatal("missing 'shell_data' source")
	}
	if execSrc.Type != "exec" {
		t.Errorf("shell_data.Type = %q, want %q", execSrc.Type, "exec")
	}
	if execSrc.Cmd != "./get-data.sh" {
		t.Errorf("shell_data.Cmd = %q, want %q", execSrc.Cmd, "./get-data.sh")
	}

	// Check CSV source
	csvSrc, ok := fm.Sources["products"]
	if !ok {
		t.Fatal("missing 'products' source")
	}
	if csvSrc.Type != "csv" {
		t.Errorf("products.Type = %q, want %q", csvSrc.Type, "csv")
	}
	if csvSrc.File != "products.csv" {
		t.Errorf("products.File = %q, want %q", csvSrc.File, "products.csv")
	}
}

func TestParseFrontmatterWithStyling(t *testing.T) {
	content := `---
title: "Styled App"
styling:
  theme: dark
  primary_color: "#6366f1"
  font: Inter
---

# Content`

	fm, _, err := extractFrontmatter([]byte(content))
	if err != nil {
		t.Fatalf("extractFrontmatter() error = %v", err)
	}

	if fm.Styling == nil {
		t.Fatal("Styling is nil")
	}
	if fm.Styling.Theme != "dark" {
		t.Errorf("Styling.Theme = %q, want %q", fm.Styling.Theme, "dark")
	}
	if fm.Styling.PrimaryColor != "#6366f1" {
		t.Errorf("Styling.PrimaryColor = %q, want %q", fm.Styling.PrimaryColor, "#6366f1")
	}
	if fm.Styling.Font != "Inter" {
		t.Errorf("Styling.Font = %q, want %q", fm.Styling.Font, "Inter")
	}
}

func TestParseFrontmatterWithBlocks(t *testing.T) {
	content := `---
title: "Block Config"
blocks:
  auto_id: true
  id_format: "block-%d"
  show_line_numbers: true
---

# Content`

	fm, _, err := extractFrontmatter([]byte(content))
	if err != nil {
		t.Fatalf("extractFrontmatter() error = %v", err)
	}

	if fm.Blocks == nil {
		t.Fatal("Blocks is nil")
	}
	if !fm.Blocks.AutoID {
		t.Error("Blocks.AutoID = false, want true")
	}
	if fm.Blocks.IDFormat != "block-%d" {
		t.Errorf("Blocks.IDFormat = %q, want %q", fm.Blocks.IDFormat, "block-%d")
	}
	if !fm.Blocks.ShowLineNumbers {
		t.Error("Blocks.ShowLineNumbers = false, want true")
	}
}

func TestParseFrontmatterWithFeatures(t *testing.T) {
	content := `---
title: "Feature Config"
features:
  hot_reload: true
---

# Content`

	fm, _, err := extractFrontmatter([]byte(content))
	if err != nil {
		t.Fatalf("extractFrontmatter() error = %v", err)
	}

	if fm.Features == nil {
		t.Fatal("Features is nil")
	}
	if !fm.Features.HotReload {
		t.Error("Features.HotReload = false, want true")
	}
}

func TestParseFrontmatterWithAllConfig(t *testing.T) {
	content := `---
title: "Full Config App"
type: page
persist: server
sources:
  users:
    type: json
    file: data.json
styling:
  theme: dark
  primary_color: "#ff5733"
blocks:
  auto_id: true
features:
  hot_reload: true
---

# Full Config App

Content here.`

	fm, remaining, err := extractFrontmatter([]byte(content))
	if err != nil {
		t.Fatalf("extractFrontmatter() error = %v", err)
	}

	// Check basic frontmatter
	if fm.Title != "Full Config App" {
		t.Errorf("Title = %q, want %q", fm.Title, "Full Config App")
	}
	if fm.Type != "page" {
		t.Errorf("Type = %q, want %q", fm.Type, "page")
	}
	if fm.Persist != PersistServer {
		t.Errorf("Persist = %q, want %q", fm.Persist, PersistServer)
	}

	// Check sources
	if len(fm.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(fm.Sources))
	}
	if src, ok := fm.Sources["users"]; !ok || src.Type != "json" {
		t.Error("users source not parsed correctly")
	}

	// Check styling
	if fm.Styling == nil || fm.Styling.Theme != "dark" {
		t.Error("Styling not parsed correctly")
	}

	// Check blocks
	if fm.Blocks == nil || !fm.Blocks.AutoID {
		t.Error("Blocks not parsed correctly")
	}

	// Check features
	if fm.Features == nil || !fm.Features.HotReload {
		t.Error("Features not parsed correctly")
	}

	// Check remaining content
	if !strings.Contains(string(remaining), "Full Config App") {
		t.Error("remaining content missing heading")
	}
}

func TestPageConfigMergeFromFrontmatter(t *testing.T) {
	t.Run("merge sources", func(t *testing.T) {
		pc := &PageConfig{}
		fm := &Frontmatter{
			Sources: map[string]SourceConfig{
				"users": {Type: "json", File: "users.json"},
				"api":   {Type: "rest", From: "https://api.example.com"},
			},
		}

		pc.MergeFromFrontmatter(fm)

		if len(pc.Sources) != 2 {
			t.Fatalf("got %d sources, want 2", len(pc.Sources))
		}
		if pc.Sources["users"].Type != "json" {
			t.Errorf("users.Type = %q, want %q", pc.Sources["users"].Type, "json")
		}
		if pc.Sources["api"].From != "https://api.example.com" {
			t.Errorf("api.From = %q, want %q", pc.Sources["api"].From, "https://api.example.com")
		}
	})

	t.Run("merge styling partial", func(t *testing.T) {
		pc := &PageConfig{
			Styling: StylingConfig{
				Theme:        "light",
				PrimaryColor: "#000000",
				Font:         "Arial",
			},
		}
		fm := &Frontmatter{
			Styling: &StylingConfig{
				Theme: "dark", // Override theme only
			},
		}

		pc.MergeFromFrontmatter(fm)

		// Theme should be overridden
		if pc.Styling.Theme != "dark" {
			t.Errorf("Styling.Theme = %q, want %q", pc.Styling.Theme, "dark")
		}
		// PrimaryColor should be preserved
		if pc.Styling.PrimaryColor != "#000000" {
			t.Errorf("Styling.PrimaryColor = %q, want %q (should be preserved)", pc.Styling.PrimaryColor, "#000000")
		}
		// Font should be preserved
		if pc.Styling.Font != "Arial" {
			t.Errorf("Styling.Font = %q, want %q (should be preserved)", pc.Styling.Font, "Arial")
		}
	})

	t.Run("merge blocks", func(t *testing.T) {
		pc := &PageConfig{
			Blocks: BlocksConfig{
				AutoID:   false,
				IDFormat: "old-%d",
			},
		}
		fm := &Frontmatter{
			Blocks: &BlocksConfig{
				AutoID:          true,
				IDFormat:        "new-%d",
				ShowLineNumbers: true,
			},
		}

		pc.MergeFromFrontmatter(fm)

		if !pc.Blocks.AutoID {
			t.Error("Blocks.AutoID = false, want true")
		}
		if pc.Blocks.IDFormat != "new-%d" {
			t.Errorf("Blocks.IDFormat = %q, want %q", pc.Blocks.IDFormat, "new-%d")
		}
		if !pc.Blocks.ShowLineNumbers {
			t.Error("Blocks.ShowLineNumbers = false, want true")
		}
	})

	t.Run("merge features", func(t *testing.T) {
		pc := &PageConfig{}
		fm := &Frontmatter{
			Features: &FeaturesConfig{
				HotReload: true,
			},
		}

		pc.MergeFromFrontmatter(fm)

		if !pc.Features.HotReload {
			t.Error("Features.HotReload = false, want true")
		}
	})

	t.Run("nil frontmatter values preserved", func(t *testing.T) {
		pc := &PageConfig{
			Sources: map[string]SourceConfig{
				"existing": {Type: "csv", File: "data.csv"},
			},
			Styling: StylingConfig{
				Theme: "light",
			},
		}
		fm := &Frontmatter{} // All nil

		pc.MergeFromFrontmatter(fm)

		// Existing values should be preserved
		if len(pc.Sources) != 1 {
			t.Errorf("got %d sources, want 1 (should preserve existing)", len(pc.Sources))
		}
		if pc.Styling.Theme != "light" {
			t.Errorf("Styling.Theme = %q, want %q (should preserve existing)", pc.Styling.Theme, "light")
		}
	})

	t.Run("frontmatter adds to existing sources", func(t *testing.T) {
		pc := &PageConfig{
			Sources: map[string]SourceConfig{
				"existing": {Type: "csv", File: "data.csv"},
			},
		}
		fm := &Frontmatter{
			Sources: map[string]SourceConfig{
				"new": {Type: "json", File: "new.json"},
			},
		}

		pc.MergeFromFrontmatter(fm)

		if len(pc.Sources) != 2 {
			t.Fatalf("got %d sources, want 2", len(pc.Sources))
		}
		if _, ok := pc.Sources["existing"]; !ok {
			t.Error("existing source should be preserved")
		}
		if _, ok := pc.Sources["new"]; !ok {
			t.Error("new source should be added")
		}
	})

	t.Run("frontmatter overrides same-name source", func(t *testing.T) {
		pc := &PageConfig{
			Sources: map[string]SourceConfig{
				"users": {Type: "csv", File: "old.csv"},
			},
		}
		fm := &Frontmatter{
			Sources: map[string]SourceConfig{
				"users": {Type: "json", File: "new.json"},
			},
		}

		pc.MergeFromFrontmatter(fm)

		if pc.Sources["users"].Type != "json" {
			t.Errorf("users.Type = %q, want %q (frontmatter should override)", pc.Sources["users"].Type, "json")
		}
		if pc.Sources["users"].File != "new.json" {
			t.Errorf("users.File = %q, want %q (frontmatter should override)", pc.Sources["users"].File, "new.json")
		}
	})
}

func TestProcessExpressions(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		wantExprCount int
		wantContains  string
	}{
		{
			name:          "single expression",
			html:          `<p>Count: <code>=count(tasks)</code></p>`,
			wantExprCount: 1,
			wantContains:  `data-expr="count(tasks)"`,
		},
		{
			name:          "multiple expressions",
			html:          `<p>Done: <code>=count(tasks where done)</code> / <code>=count(tasks)</code></p>`,
			wantExprCount: 2,
			wantContains:  `data-expr-id="expr-0"`,
		},
		{
			name:          "non-expression code",
			html:          `<p>Use <code>fmt.Println</code> to print.</p>`,
			wantExprCount: 0,
			wantContains:  `<code>fmt.Println</code>`,
		},
		{
			name:          "escaped expression",
			html:          `<p>Literal: <code>\=not an expression</code></p>`,
			wantExprCount: 0,
			wantContains:  `<code>=not an expression</code>`,
		},
		{
			name:          "mixed content",
			html:          `<p>Total: <code>=sum(items.price)</code> and code: <code>print(x)</code></p>`,
			wantExprCount: 1,
			wantContains:  `data-expr="sum(items.price)"`,
		},
		{
			name:          "empty equals",
			html:          `<p>Empty: <code>=</code></p>`,
			wantExprCount: 0,
			wantContains:  `<code>=</code>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, expressions := processExpressions(tt.html)

			if len(expressions) != tt.wantExprCount {
				t.Errorf("got %d expressions, want %d", len(expressions), tt.wantExprCount)
			}

			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("result does not contain %q\ngot: %s", tt.wantContains, result)
			}
		})
	}
}

func TestProcessExpressionsPreservesIDs(t *testing.T) {
	html := `<p><code>=count(a)</code> and <code>=count(b)</code> and <code>=count(c)</code></p>`

	_, expressions := processExpressions(html)

	if len(expressions) != 3 {
		t.Fatalf("got %d expressions, want 3", len(expressions))
	}

	// Check IDs are sequential
	if _, ok := expressions["expr-0"]; !ok {
		t.Error("missing expr-0")
	}
	if _, ok := expressions["expr-1"]; !ok {
		t.Error("missing expr-1")
	}
	if _, ok := expressions["expr-2"]; !ok {
		t.Error("missing expr-2")
	}

	// Check expression strings are correct
	if expressions["expr-0"] != "count(a)" {
		t.Errorf("expr-0 = %q, want %q", expressions["expr-0"], "count(a)")
	}
	if expressions["expr-1"] != "count(b)" {
		t.Errorf("expr-1 = %q, want %q", expressions["expr-1"], "count(b)")
	}
	if expressions["expr-2"] != "count(c)" {
		t.Errorf("expr-2 = %q, want %q", expressions["expr-2"], "count(c)")
	}
}

func TestParseMarkdownWithExpressions(t *testing.T) {
	content := `---
title: "Expression Test"
---

# Dashboard

**Tasks Done:** ` + "`=count(tasks where done)`" + ` / ` + "`=count(tasks)`" + `

**Total Spent:** ` + "`=sum(expenses.amount)`" + `
`

	fm, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	// Check expressions were extracted
	if len(fm.Expressions) != 3 {
		t.Errorf("got %d expressions, want 3", len(fm.Expressions))
	}

	// Check HTML contains expression placeholders
	if !strings.Contains(html, `class="tinkerdown-expr"`) {
		t.Error("HTML does not contain expression class")
	}
	if !strings.Contains(html, `data-expr-id`) {
		t.Error("HTML does not contain data-expr-id")
	}
}

func TestParseMarkdownWithEscapedExpression(t *testing.T) {
	// Test that escaped expressions (prefixed with backslash) render as literal text
	// Markdown: `\=count(tasks)` should become <code>=count(tasks)</code> (literal, not evaluated)
	content := `---
title: "Escaped Expression Test"
---

# Documentation

To show an expression literally, use: ` + "`\\=count(tasks)`" + `

This is a real expression: ` + "`=count(items)`" + `
`

	fm, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	// Only one expression should be extracted (the real one, not the escaped one)
	if len(fm.Expressions) != 1 {
		t.Errorf("got %d expressions, want 1 (escaped should not be counted)", len(fm.Expressions))
	}

	// The escaped expression should appear as literal text with the = sign
	if !strings.Contains(html, `<code>=count(tasks)</code>`) {
		t.Errorf("escaped expression should render as literal <code>=count(tasks)</code>\ngot: %s", html)
	}

	// The real expression should be converted to a span placeholder
	if !strings.Contains(html, `data-expr="count(items)"`) {
		t.Error("real expression should be converted to placeholder span")
	}
}

func TestProcessStatusBanners(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "success emoji transforms",
			input:  "<blockquote><p>✅ Done</p></blockquote>",
			expect: "tinkerdown-status-success",
		},
		{
			name:   "warning emoji transforms",
			input:  "<blockquote><p>⚠️ Caution</p></blockquote>",
			expect: "tinkerdown-status-warning",
		},
		{
			name:   "error emoji transforms",
			input:  "<blockquote><p>❌ Failed</p></blockquote>",
			expect: "tinkerdown-status-error",
		},
		{
			name:   "info emoji transforms",
			input:  "<blockquote><p>📊 Stats</p></blockquote>",
			expect: "tinkerdown-status-info",
		},
		{
			name:   "regular blockquote unchanged",
			input:  "<blockquote><p>Quote</p></blockquote>",
			expect: "<blockquote>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processStatusBanners(tt.input)
			if !strings.Contains(result, tt.expect) {
				t.Errorf("expected %q in result, got: %s", tt.expect, result)
			}
		})
	}
}

func TestProcessCharts(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		wantCharts   bool
		wantContains []string
		wantAbsent   []string
	}{
		{
			name: "bar chart from heading + table",
			html: "<h2 id=\"sales-by-region-chartbar\">Sales by Region {chart:bar}</h2>\n<table>\n<thead>\n<tr>\n<th>Region</th>\n<th>Sales</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>North</td>\n<td>100</td>\n</tr>\n<tr>\n<td>South</td>\n<td>150</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts: true,
			wantContains: []string{
				`data-chart-type="bar"`,
				`<canvas></canvas>`,
				`tinkerdown-chart`,
				`North`, // appears in collapsed data table
				`South`,
				`tinkerdown-chart-table`,
				`View data`,
			},
			wantAbsent: []string{`{chart:bar}`},
		},
		{
			name: "line chart",
			html: "<h2 id=\"trend-chartline\">Trend {chart:line}</h2>\n<table>\n<thead>\n<tr>\n<th>Month</th>\n<th>Value</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>Jan</td>\n<td>100</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   true,
			wantContains: []string{`data-chart-type="line"`},
		},
		{
			name: "pie chart",
			html: "<h2 id=\"share-chartpie\">Share {chart:pie}</h2>\n<table>\n<thead>\n<tr>\n<th>Product</th>\n<th>Share</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>A</td>\n<td>60</td>\n</tr>\n<tr>\n<td>B</td>\n<td>40</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   true,
			wantContains: []string{`data-chart-type="pie"`},
		},
		{
			name: "auto-detect pie (few labels, single series)",
			html: "<h2 id=\"data-chart\">Data {chart}</h2>\n<table>\n<thead>\n<tr>\n<th>Cat</th>\n<th>Val</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>A</td>\n<td>10</td>\n</tr>\n<tr>\n<td>B</td>\n<td>20</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   true,
			wantContains: []string{`data-chart-type="pie"`},
		},
		{
			name: "multi-dataset",
			html: "<h2 id=\"quarterly-chartbar\">Quarterly {chart:bar}</h2>\n<table>\n<thead>\n<tr>\n<th>Region</th>\n<th>Q1</th>\n<th>Q2</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>North</td>\n<td>100</td>\n<td>200</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   true,
			wantContains: []string{`Q1`, `Q2`, `datasets`},
		},
		{
			name: "heading without chart annotation unchanged",
			html: "<h2 id=\"normal\">Normal Heading</h2>\n<table>\n<thead>\n<tr>\n<th>A</th>\n<th>B</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>1</td>\n<td>2</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   false,
			wantContains: []string{`Normal Heading`},
		},
		{
			name: "chart heading without following table",
			html: "<h2 id=\"broken-chartbar\">Broken {chart:bar}</h2>\n<p>No table here</p>",
			wantCharts: false,
			wantAbsent: []string{`{chart:bar}`},
		},
		{
			name:       "no numeric columns skips chart",
			html:       "<h2 id=\"text-chartbar\">Text {chart:bar}</h2>\n<table>\n<thead>\n<tr>\n<th>Name</th>\n<th>City</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>Alice</td>\n<td>NYC</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts: false,
		},
		{
			name: "heading ID is cleaned",
			html: "<h2 id=\"sales-chartbar\">Sales {chart:bar}</h2>\n<table>\n<thead>\n<tr>\n<th>X</th>\n<th>Y</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>A</td>\n<td>1</td>\n</tr>\n</tbody>\n</table>\n",
			wantCharts:   true,
			wantContains: []string{`id="sales"`},
			wantAbsent:   []string{`chartbar`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, hasCharts := processCharts(tt.html, nil)
			if hasCharts != tt.wantCharts {
				t.Errorf("hasCharts = %v, want %v\nresult:\n%s", hasCharts, tt.wantCharts, result)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("expected %q in result, got:\n%s", want, result)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(result, absent) {
					t.Errorf("did not expect %q in result, got:\n%s", absent, result)
				}
			}
		})
	}
}

// stubRenderer is a controllable DiagramRenderer for tests.
type stubRenderer struct {
	output []byte
	err    error
}

func (s stubRenderer) Render(_ []byte) ([]byte, error) {
	return s.output, s.err
}

// captureRenderer records the source bytes passed to Render so tests
// can verify the unescape contract.
type captureRenderer struct {
	ref    *[]byte
	output []byte
}

func (c captureRenderer) Render(source []byte) ([]byte, error) {
	*c.ref = append([]byte{}, source...)
	return c.output, nil
}

type errStubT string

func (e errStubT) Error() string { return string(e) }

var errStub = errStubT("renderer down for test")

func TestProcessMermaidNoOpWithoutRenderer(t *testing.T) {
	in := `<p>before</p><pre><code class="language-mermaid">flowchart LR
  A --&gt; B
</code></pre><p>after</p>`
	out, remaining := processMermaid(in, nil)
	if out != in {
		t.Errorf("html should be unchanged when renderer is nil")
	}
	if remaining != 1 {
		t.Errorf("remaining = %d, want 1", remaining)
	}
}

func TestProcessMermaidReplacesBlockOnSuccess(t *testing.T) {
	in := `<pre><code class="language-mermaid">flowchart LR
  A --&gt; B
</code></pre>`
	r := stubRenderer{output: []byte(`<svg>diagram</svg>`)}
	out, remaining := processMermaid(in, r)

	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
	if !strings.Contains(out, `<div class="mermaid mermaid-prerendered"><svg>diagram</svg></div>`) {
		t.Errorf("expected pre-rendered div in output: %s", out)
	}
	if strings.Contains(out, "language-mermaid") {
		t.Errorf("original block should be removed: %s", out)
	}
}

func TestProcessMermaidLeavesBlockOnRendererError(t *testing.T) {
	in := `<pre><code class="language-mermaid">bad source</code></pre>`
	r := stubRenderer{err: errStub}
	out, remaining := processMermaid(in, r)

	if remaining != 1 {
		t.Errorf("remaining = %d, want 1 (failure should keep the block)", remaining)
	}
	if !strings.Contains(out, "language-mermaid") {
		t.Errorf("failed-render block should remain in output: %s", out)
	}
	if strings.Contains(out, "mermaid-prerendered") {
		t.Errorf("no pre-rendered div should be emitted on failure: %s", out)
	}
}

func TestProcessMermaidUnescapesSourceBeforeRendering(t *testing.T) {
	// Goldmark HTML-escapes < and > inside fenced blocks. The renderer
	// must receive the original mermaid syntax, not the escaped form.
	in := `<pre><code class="language-mermaid">flowchart LR
  A --&gt; B
  B --&lt; C
</code></pre>`
	var captured []byte
	r := captureRenderer{ref: &captured, output: []byte("<svg/>")}
	processMermaid(in, r)

	if !strings.Contains(string(captured), "-->") {
		t.Errorf("renderer received escaped HTML instead of raw mermaid: %q", captured)
	}
	if strings.Contains(string(captured), "&gt;") {
		t.Errorf("renderer received escaped &gt;: %q", captured)
	}
}

func TestProcessMermaidHandlesMultipleBlocks(t *testing.T) {
	in := `<pre><code class="language-mermaid">A</code></pre>
<p>between</p>
<pre><code class="language-mermaid">B</code></pre>`
	r := stubRenderer{output: []byte("<svg/>")}
	out, remaining := processMermaid(in, r)

	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
	if strings.Count(out, "mermaid-prerendered") != 2 {
		t.Errorf("expected 2 pre-rendered divs, got: %s", out)
	}
}

func TestParseMarkdownEndToEndPreRendersWhenRendererSet(t *testing.T) {
	r := stubRenderer{output: []byte(`<svg id="diag">x</svg>`)}
	SetMermaidRenderer(r)
	t.Cleanup(func() { SetMermaidRenderer(nil) })

	content := []byte("---\ntitle: Diag\n---\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	fm, _, html, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if fm.HasMermaid {
		t.Error("HasMermaid should be false when ALL blocks pre-rendered (skips runtime injection)")
	}
	if !strings.Contains(html, `mermaid-prerendered`) {
		t.Errorf("html missing pre-rendered marker: %s", html)
	}
	if strings.Contains(html, "language-mermaid") {
		t.Errorf("original mermaid block should be replaced: %s", html)
	}
}

func TestParseMarkdownEndToEndKeepsBlockOnRendererFailure(t *testing.T) {
	r := stubRenderer{err: errStub}
	SetMermaidRenderer(r)
	t.Cleanup(func() { SetMermaidRenderer(nil) })

	content := []byte("---\ntitle: Diag\n---\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	fm, _, html, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if !fm.HasMermaid {
		t.Error("HasMermaid should remain true when renderer fails (runtime needed for fallback)")
	}
	if !strings.Contains(html, "language-mermaid") {
		t.Errorf("original block must remain for client-side fallback: %s", html)
	}
}

func TestParseMarkdownDetectsMermaid(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "page with mermaid block",
			content: "---\ntitle: \"Diagram\"\n---\n\n# Flow\n\n```mermaid\nflowchart LR\n  A --> B\n```\n",
			want: true,
		},
		{
			name:    "page without mermaid block",
			content: "---\ntitle: \"No Diagram\"\n---\n\n# Just text\n\n```go\nfmt.Println(\"hi\")\n```\n",
			want:    false,
		},
		{
			name:    "no fenced blocks at all",
			content: "---\ntitle: \"Plain\"\n---\n\nJust prose, no code.\n",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, _, _, err := ParseMarkdown([]byte(tc.content))
			if err != nil {
				t.Fatalf("ParseMarkdown failed: %v", err)
			}
			if fm.HasMermaid != tc.want {
				t.Errorf("HasMermaid = %v, want %v", fm.HasMermaid, tc.want)
			}
		})
	}
}

func TestParseMarkdownWithCharts(t *testing.T) {
	content := []byte(`---
title: "Chart Test"
---

# Dashboard

## Sales by Region {chart:bar}

| Region | Sales |
|--------|-------|
| North  | 100   |
| South  | 150   |
| East   | 120   |
`)
	fm, _, html, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}
	if !fm.HasCharts {
		t.Error("Expected HasCharts to be true")
	}
	if !strings.Contains(html, "tinkerdown-chart") {
		t.Error("Expected tinkerdown-chart class in HTML")
	}
	if !strings.Contains(html, `data-chart-type="bar"`) {
		t.Error("Expected data-chart-type=bar in HTML")
	}
	if strings.Contains(html, "{chart:bar}") {
		t.Error("Chart annotation should be removed from HTML")
	}
	if !strings.Contains(html, `North`) {
		t.Error("Expected North in chart data or table")
	}
}

func TestProcessChartsWithOptions(t *testing.T) {
	tableHTML := "<h2 id=\"sales-chartbar\">Sales {chart:bar}</h2>\n<table>\n<thead>\n<tr>\n<th>X</th>\n<th>Y</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>A</td>\n<td>1</td>\n</tr>\n</tbody>\n</table>\n"

	opts := map[string]ChartOptions{
		"sales": {
			Colors:     []string{"#ff0000", "#00ff00"},
			Stacked:    true,
			Horizontal: true,
			Legend:      boolPtr(false),
		},
	}

	result, hasCharts := processCharts(tableHTML, opts)
	if !hasCharts {
		t.Fatal("Expected chart to be found")
	}
	if !strings.Contains(result, `data-chart-options`) {
		t.Error("Expected data-chart-options attribute")
	}
	if !strings.Contains(result, `#ff0000`) {
		t.Error("Expected custom color in options")
	}
	if !strings.Contains(result, `stacked`) {
		t.Error("Expected stacked option")
	}
	if !strings.Contains(result, `horizontal`) {
		t.Error("Expected horizontal option")
	}
}

func TestParseMarkdownWithChartOptions(t *testing.T) {
	content := []byte(`---
title: "Chart Options Test"
charts:
  sales-by-region:
    colors: ["#ff6384", "#36a2eb"]
    stacked: true
    horizontal: true
    legend: false
---

## Sales by Region {chart:bar}

| Region | Sales |
|--------|-------|
| North  | 100   |
| South  | 150   |
`)
	fm, _, html, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}
	if !fm.HasCharts {
		t.Error("Expected HasCharts to be true")
	}
	if len(fm.Charts) == 0 {
		t.Fatal("Expected Charts map to be populated from frontmatter")
	}
	if _, ok := fm.Charts["sales-by-region"]; !ok {
		t.Error("Expected sales-by-region in Charts map")
	}
	if !strings.Contains(html, "data-chart-options") {
		t.Error("Expected data-chart-options in HTML")
	}
	if !strings.Contains(html, "#ff6384") {
		t.Error("Expected custom color in chart options")
	}
}

func boolPtr(b bool) *bool { return &b }
