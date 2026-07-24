package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
)

// parseFixture writes app.md (+ optional tinkerdown.yaml) to a temp dir, parses
// the page, and loads the config — the inputs the M2 Phase 3 semantic checks take.
func parseFixture(t *testing.T, appMD, tinkerdownYAML string) (*tinkerdown.Page, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.md"), []byte(appMD), 0o644); err != nil {
		t.Fatal(err)
	}
	var cfg *config.Config
	if tinkerdownYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, "tinkerdown.yaml"), []byte(tinkerdownYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		loaded, err := config.LoadFromDir(dir)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		cfg = loaded
	}
	page, err := tinkerdown.ParseFileInSite(filepath.Join(dir, "app.md"), dir)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return page, cfg
}

// TestUnresolvedSourceDiags: a source bound via lvt-source must resolve to a
// declared one (frontmatter or config); a typo is caught.
func TestUnresolvedSourceDiags(t *testing.T) {
	// Undeclared source → flagged.
	page, cfg := parseFixture(t,
		"```lvt\n<ul lvt-source=\"typo\">{{range .Data}}{{.x}}{{end}}</ul>\n```\n", "")
	if diags := unresolvedSourceDiags(page, cfg); len(diags) != 1 || !strings.Contains(diags[0], "typo") {
		t.Fatalf("expected 'typo' unresolved, got %v", diags)
	}

	// Frontmatter-declared source → clean.
	page, cfg = parseFixture(t,
		"---\nsources:\n  items: {type: static, data: [{x: 1}]}\n---\n\n```lvt\n<ul lvt-source=\"items\">{{range .Data}}{{.x}}{{end}}</ul>\n```\n", "")
	if diags := unresolvedSourceDiags(page, cfg); len(diags) != 0 {
		t.Fatalf("frontmatter source should resolve, got %v", diags)
	}

	// Config-declared (shared) source → clean.
	page, cfg = parseFixture(t,
		"```lvt\n<ul lvt-source=\"shared\">{{range .Data}}{{.x}}{{end}}</ul>\n```\n",
		"sources:\n  shared: {type: static, data: [{x: 1}]}\n")
	if diags := unresolvedSourceDiags(page, cfg); len(diags) != 0 {
		t.Fatalf("config source should resolve, got %v", diags)
	}
}

// TestUnsuppliedActionParams: every :param a SQL action references must be
// supplied by a form field or data-*; :operator is server-set and excluded.
func TestUnsuppliedActionParams(t *testing.T) {
	yaml := "sources:\n  store: {type: sqlite, database: ./d.db, query: \"SELECT 1\"}\n" +
		"actions:\n  do:\n    kind: sql\n    source: store\n    statement: \"INSERT INTO t (a,b) VALUES (:aaa, :bbb)\"\n"

	// Missing :bbb (only data-aaa supplied) → flagged; :aaa is supplied, not flagged.
	page, cfg := parseFixture(t,
		"```lvt\n<div lvt-source=\"store\"><button name=\"do\" data-aaa=\"1\">Go</button></div>\n```\n", yaml)
	diags := unsuppliedActionParams(page, cfg)
	if len(diags) != 1 || !strings.Contains(diags[0], "bbb") {
		t.Fatalf("expected :bbb unsupplied, got %v", diags)
	}
	if strings.Contains(strings.Join(diags, " "), ":aaa") {
		t.Errorf(":aaa is supplied (data-aaa) and must not be flagged: %v", diags)
	}

	// Both supplied → clean.
	page, cfg = parseFixture(t,
		"```lvt\n<div lvt-source=\"store\"><button name=\"do\" data-aaa=\"1\" data-bbb=\"2\">Go</button></div>\n```\n", yaml)
	if diags := unsuppliedActionParams(page, cfg); len(diags) != 0 {
		t.Fatalf("complete form should pass, got %v", diags)
	}
}

// TestUnsuppliedActionParams_OperatorExcluded: :operator is injected server-side,
// so an action referencing it needs no form field for it.
func TestUnsuppliedActionParams_OperatorExcluded(t *testing.T) {
	yaml := "sources:\n  store: {type: sqlite, database: ./d.db, query: \"SELECT 1\"}\n" +
		"actions:\n  do:\n    kind: sql\n    source: store\n    statement: \"INSERT INTO audit (who, id) VALUES (:operator, :id)\"\n"
	page, cfg := parseFixture(t,
		"```lvt\n<div lvt-source=\"store\"><button name=\"do\" data-id=\"1\">Go</button></div>\n```\n", yaml)
	if diags := unsuppliedActionParams(page, cfg); len(diags) != 0 {
		t.Fatalf(":operator should be excluded (server-set); got %v", diags)
	}
}
