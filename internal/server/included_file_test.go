package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// TestIsIncludedFile_Tracking covers the watcher-callback predicate:
// after Discover(), a file referenced by a page's `include="..."` fence
// must be reported as included; an unrelated file must not; and `.md`
// files always short-circuit to false (they go down the page-file path
// instead, regardless of whether some page also includes them).
func TestIsIncludedFile_Tracking(t *testing.T) {
	tmpDir := t.TempDir()
	included := filepath.Join(tmpDir, "snippet.go")
	if err := os.WriteFile(included, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(tmpDir, "other.go")
	if err := os.WriteFile(unrelated, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mdLike := filepath.Join(tmpDir, "looks-like-include.md")
	if err := os.WriteFile(mdLike, []byte("---\ntitle: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	page := "---\ntitle: Page\n---\n\n```go include=\"./snippet.go\"\n```\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "index.md"), []byte(page), 0o644); err != nil {
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

	if !srv.isIncludedFile(included) {
		t.Errorf("expected snippet.go to be tracked as included after Discover")
	}
	if srv.isIncludedFile(unrelated) {
		t.Errorf("expected other.go NOT to be tracked as included")
	}
	// .md files always go down the page-file path, even if some page's
	// frontmatter or include-attr happened to point at one.
	if srv.isIncludedFile(mdLike) {
		t.Errorf("expected .md path to short-circuit to false in isIncludedFile")
	}
}
