package tinkerdown_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

const (
	skillPath          = "skills/tinkerdown/SKILL.md"
	skillReferencePath = "skills/tinkerdown/reference.md"
)

// TestStyleTokensDocumented is the forward drift-guard for the house-style
// vocabulary: every design token the framework accepts (config.KnownStyleTokens)
// must be named in the skill reference the LLM reads as generation context. A token
// added to the map but never documented is one the generator is never told it can
// theme — silent off-brand drift, the gap Phase 2 exists to close.
//
// Forward-only by design (mirrors the philosophy of attribute_docs_test.go, adapted
// to a vocabulary with no unique textual signature). The reverse direction — a
// documented token that no longer exists — would require scraping token-like words
// out of prose; "accent"/"card_bg" are ordinary words, unlike "lvt-", so such a
// scan would either false-match prose or silently miss tokens. That is a
// self-certifying guard, and the reverse case is low-harm, so it is not built.
func TestStyleTokensDocumented(t *testing.T) {
	doc := readDocFile(t, skillReferencePath)
	for key := range config.KnownStyleTokens {
		if !strings.Contains(doc, key) {
			t.Errorf("design token %q is in config.KnownStyleTokens but not documented in %s — "+
				"the generator is never told it can theme it", key, skillReferencePath)
		}
	}
}

// TestSkillWiresHouseStyle asserts the generation skill actually consumes the house
// style, closing the declared-but-unconsumed gap: SKILL.md instructs reading
// generation.style_guide + styling.tokens and carries the on-brand guidance
// (semantic HTML, don't hardcode colours), and reference.md documents the
// design-token system. Structural, in the spirit of skill_examples_test.go.
func TestSkillWiresHouseStyle(t *testing.T) {
	skill := readDocFile(t, skillPath)
	ref := readDocFile(t, skillReferencePath)

	// SKILL.md must wire the read path + the guidance lever.
	for _, want := range []string{"style_guide", "styling.tokens", "semantic HTML"} {
		if !strings.Contains(skill, want) {
			t.Errorf("%s does not wire the house style: missing %q", skillPath, want)
		}
	}
	// The never-hardcode-colours rule is the on-brand lever (accept either spelling).
	if !strings.Contains(skill, "hardcode colour") && !strings.Contains(skill, "hardcode color") {
		t.Errorf("%s is missing the 'never hardcode colours' guidance", skillPath)
	}
	// reference.md must carry the House style section + the token vocabulary.
	for _, want := range []string{"House style", "styling.tokens", "--accent"} {
		if !strings.Contains(ref, want) {
			t.Errorf("%s is missing house-style documentation: %q", skillReferencePath, want)
		}
	}
}

// TestPIIManifestHouseStyleConsumable is the integration check. There is no Go
// "generation context" to assemble — the skill is instructions, not code — so what
// is verifiable deterministically is that the demo manifest exposes the house style
// the skill reads: generation.style_guide is set and resolves to a real file, and
// styling.tokens is populated. This proves StyleGuide (formerly declared-but-
// unconsumed) is now consumable, not that a Go path consumes it.
func TestPIIManifestHouseStyleConsumable(t *testing.T) {
	const dir = "examples/pii-access-approval"
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if cfg.Generation == nil || cfg.Generation.StyleGuide == "" {
		t.Fatal("demo manifest has no generation.style_guide")
	}
	guidePath := filepath.Join(dir, cfg.Generation.StyleGuide)
	if _, err := os.Stat(guidePath); err != nil {
		t.Errorf("generation.style_guide %q does not resolve to a file (%s): %v",
			cfg.Generation.StyleGuide, guidePath, err)
	}
	if len(cfg.Styling.Tokens) == 0 {
		t.Fatal("demo manifest has no styling.tokens")
	}
	// A concrete anchor: the accent we set is present and correct. (An unknown key
	// would already have failed ValidateStyleTokens at load, above.)
	if got := cfg.Styling.Tokens["accent"]; got != "#3949ab" {
		t.Errorf("demo tokens.accent = %q, want #3949ab", got)
	}
}

// readDocFile reads a repo-relative doc file for the assertions above. Local to this
// (untagged) test file so it does not depend on helpers defined in //go:build !ci
// files, which are absent from CI builds.
func readDocFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
