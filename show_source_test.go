package tinkerdown

import (
	"strings"
	"testing"
)

func TestLvtShowSource_PerBlockFlag(t *testing.T) {
	content := "```lvt interactive show-source\n" +
		"<div>{{.Counter}}</div>\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if !strings.Contains(html, `class="tinkerdown-lvt-demo`) {
		t.Errorf("expected demo wrapper in HTML; got:\n%s", html)
	}
	if !strings.Contains(html, `class="language-html"`) {
		t.Errorf("expected source view re-tagged as language-html for highlighting; got:\n%s", html)
	}
	if !strings.Contains(html, `class="tinkerdown-interactive-block"`) {
		t.Errorf("expected live container alongside source; got:\n%s", html)
	}
}

func TestLvtShowSource_DefaultOff(t *testing.T) {
	content := "```lvt interactive\n" +
		"<div>{{.Counter}}</div>\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if strings.Contains(html, `tinkerdown-lvt-demo`) {
		t.Errorf("expected no demo wrapper without show-source; got:\n%s", html)
	}
	if !strings.Contains(html, `class="tinkerdown-interactive-block"`) {
		t.Errorf("expected live container; got:\n%s", html)
	}
}

func TestLvtShowSource_FrontmatterDefault(t *testing.T) {
	content := `---
lvt_show_source: true
---

` + "```lvt interactive\n" +
		"<div>{{.Counter}}</div>\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if !strings.Contains(html, `tinkerdown-lvt-demo`) {
		t.Errorf("expected demo wrapper from frontmatter default; got:\n%s", html)
	}
}

func TestLvtShowSource_PerBlockHideOverridesFrontmatter(t *testing.T) {
	content := `---
lvt_show_source: true
---

` + "```lvt interactive hide-source\n" +
		"<div>{{.Counter}}</div>\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if strings.Contains(html, `tinkerdown-lvt-demo`) {
		t.Errorf("hide-source should suppress source even when frontmatter enables it; got:\n%s", html)
	}
}

func TestLvtShowSource_FlagParsedIntoBlock(t *testing.T) {
	content := "```lvt interactive show-source state=\"x\"\n<div></div>\n```\n"

	_, blocks, _, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if !containsString(blocks[0].Flags, "show-source") {
		t.Errorf("expected show-source in Flags %v", blocks[0].Flags)
	}
	if !containsString(blocks[0].Flags, "interactive") {
		t.Errorf("expected interactive in Flags %v", blocks[0].Flags)
	}
	if blocks[0].Metadata["state"] != "x" {
		t.Errorf("expected metadata state=x, got %v", blocks[0].Metadata)
	}
}

func TestLvtShowSource_MixedWithEmbedLvt(t *testing.T) {
	// A mixed page exercising both Track A (literate `lvt` block with
	// show-source) and Track B (`embed-lvt` block) ensures the two block
	// types coexist without their HTML-rewriting passes stepping on each
	// other.
	content := "```lvt interactive show-source\n" +
		"<div lvt-source=\"items\">{{range .Data}}<li>{{.Text}}</li>{{end}}</div>\n" +
		"```\n\n" +
		"```embed-lvt server=\"https://example.com\" path=\"/foo\"\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if !strings.Contains(html, `class="tinkerdown-lvt-demo`) {
		t.Errorf("expected literate demo wrapper; got:\n%s", html)
	}
	if !strings.Contains(html, `class="tinkerdown-interactive-block"`) {
		t.Errorf("expected live lvt container; got:\n%s", html)
	}
	if !strings.Contains(html, `class="tinkerdown-embed-lvt"`) {
		t.Errorf("expected embed-lvt placeholder; got:\n%s", html)
	}
	if !strings.Contains(html, `data-embed-server="https://example.com"`) {
		t.Errorf("expected embed-lvt data-embed-server; got:\n%s", html)
	}
}

func TestLvtShowSource_PreservesTemplateContent(t *testing.T) {
	content := "```lvt interactive show-source\n" +
		"<div>{{.Counter}}</div>\n" +
		"```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	// Template syntax should appear in the source view (HTML-escaped).
	if !strings.Contains(html, "{{.Counter}}") {
		t.Errorf("source view should contain raw template syntax; got:\n%s", html)
	}
}
