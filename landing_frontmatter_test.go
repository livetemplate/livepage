package tinkerdown

import "testing"

// Frontmatter round-trip for the landing-layout addition: `layout` must
// unmarshal from YAML frontmatter so a page can opt into the minimal shell.
func TestFrontmatter_Layout(t *testing.T) {
	fm, _, _, err := ParseMarkdown([]byte("---\ntitle: Home\nlayout: landing\n---\n# Hi\n"))
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if fm.Layout != "landing" {
		t.Errorf("fm.Layout = %q, want %q", fm.Layout, "landing")
	}
}

// A page with no layout key leaves Layout empty (→ docs shell), proving the
// addition is backward-compatible.
func TestFrontmatter_LayoutDefaultsEmpty(t *testing.T) {
	fm, _, _, err := ParseMarkdown([]byte("---\ntitle: Plain\n---\n# Hi\n"))
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if fm.Layout != "" {
		t.Errorf("fm.Layout = %q, want empty", fm.Layout)
	}
}
