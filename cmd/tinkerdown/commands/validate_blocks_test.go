package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// TestValidateBlockTemplates covers the M2 Phase 2 consumption of
// livetemplate.Validate: `tinkerdown validate` now runs each lvt block through
// the real template parser, so a template-syntax or composition error in a
// block — previously invisible until it silently failed at serve — is caught.
func TestValidateBlockTemplates(t *testing.T) {
	sets := server.ComponentTemplates()

	cases := []struct {
		name     string
		fence    string // the lvt block body
		wantDiag string // substring expected in some diagnostic; "" means clean
	}{
		{
			name:  "clean",
			fence: `<ul lvt-source="items">{{range .Data}}<li>{{.name}}</li>{{end}}</ul>`,
		},
		{
			// The gap M2 closes: an unclosed {{range}} used to pass validate and
			// only fail at serve (the block silently rendered nothing).
			name:     "unclosed range",
			fence:    `<ul lvt-source="items">{{range .Data}}<li>{{.name}}</li></ul>`,
			wantDiag: "unexpected EOF",
		},
		{
			name:     "unknown function",
			fence:    `<div lvt-source="items">{{bogusFn .name}}</div>`,
			wantDiag: `function "bogusFn" not defined`,
		},
		{
			// The split helper (blockHelperFuncs) must be available to a block's
			// parse — this is the fix for the markdown-data-bookmarks bug the new
			// check surfaced.
			name:  "split helper resolves",
			fence: `<ul lvt-source="items">{{range (split "a, b" ", ")}}<li>{{.}}</li>{{end}}</ul>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			doc := "---\nsources:\n  items: {type: static, data: [{name: a}]}\n---\n\n```lvt\n" + c.fence + "\n```\n"
			path := filepath.Join(dir, "app.md")
			if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			page, err := tinkerdown.ParseFileInSite(path, dir)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			diags := validateBlockTemplates(page, sets)

			if c.wantDiag == "" {
				if len(diags) != 0 {
					t.Fatalf("expected clean, got diagnostics: %v", diags)
				}
				return
			}
			if len(diags) == 0 {
				t.Fatalf("expected a diagnostic containing %q, got none", c.wantDiag)
			}
			if joined := strings.Join(diags, " | "); !strings.Contains(joined, c.wantDiag) {
				t.Errorf("diagnostics %q do not contain %q", joined, c.wantDiag)
			}
		})
	}
}
