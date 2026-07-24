package tinkerdown_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// TestCapturedSkillsWellFormed checks the captured skills — the pii-access-approval
// workflow (M1 Phase 6) and the /tinkerdown:save capability — are well-formed: valid
// frontmatter, every repo path they point at exists, and (for a skill that points at a
// committed manifest) that manifest carries the house style the app ran on, so the
// saved workflow re-runs on-brand (M5 Phase 1 — capture completeness).
//
// The pii-access-approval skill deliberately POINTS at examples/pii-access-approval/
// rather than copying it (the artifacts are a committed source of truth; a copy would
// be drift waiting to happen). This test is what makes that pointer safe: if the
// example is moved, renamed, or loses its style file, the skill's dead reference fails
// here.
//
// The manifestDir round-trip below is real-state (config.LoadFromDir, not a SKILL.md
// substring). It overlaps TestPIIManifestHouseStyleConsumable by design — it asserts
// the same fact (the demo manifest exposes styling.tokens + generation.style_guide) but
// from the *capture* angle: a saved skill re-runs on-brand only if the manifest it
// points at still carries the style. The fixture path is a hardcoded constant, matching
// the refs below; it is NOT scraped out of the SKILL.md prose (that would be a
// self-certifying guard — a string re-asserting a string read from the same prose).
func TestCapturedSkillsWellFormed(t *testing.T) {
	skills := []struct {
		path string
		refs []string // repo-relative paths the skill references, which must exist
		// manifestDir, if set, is the committed manifest the skill points at; its
		// house style (styling.tokens + generation.style_guide) must round-trip.
		manifestDir string
	}{
		{
			path: "skills/pii-access-approval/SKILL.md",
			refs: []string{
				"examples/pii-access-approval/tinkerdown.yaml",
				"examples/pii-access-approval/app.md",
				"examples/pii-access-approval/seed.sql",
				"examples/pii-access-approval/style-guide.md",
				"examples/pii-access-approval/README.md",
			},
			manifestDir: "examples/pii-access-approval",
		},
		{
			path: "skills/tinkerdown-save/SKILL.md",
			refs: []string{
				"skills/pii-access-approval",
			},
		},
	}

	for _, s := range skills {
		content, err := os.ReadFile(s.path)
		if err != nil {
			t.Errorf("%s: %v", s.path, err)
			continue
		}
		cs := string(content)

		if !strings.HasPrefix(cs, "---") {
			t.Errorf("%s: missing frontmatter (should start with ---)", s.path)
		}
		for _, key := range []string{"name:", "description:", "triggers:"} {
			if !strings.Contains(cs, key) {
				t.Errorf("%s: missing frontmatter field %q", s.path, key)
			}
		}
		for _, ref := range s.refs {
			if _, err := os.Stat(ref); err != nil {
				t.Errorf("%s references %q, which does not exist: %v", s.path, ref, err)
			}
		}

		if s.manifestDir == "" {
			continue
		}
		// Capture completeness: the manifest the saved skill runs against must still
		// carry the house style, or the workflow re-runs off-brand on defaults.
		cfg, err := config.LoadFromDir(s.manifestDir)
		if err != nil {
			t.Errorf("%s: load manifest %q: %v", s.path, s.manifestDir, err)
			continue
		}
		if len(cfg.Styling.Tokens) == 0 {
			t.Errorf("%s: manifest %q carries no styling.tokens — saved workflow would re-run off-brand",
				s.path, s.manifestDir)
		}
		if cfg.Generation == nil || cfg.Generation.StyleGuide == "" {
			t.Errorf("%s: manifest %q has no generation.style_guide — house-style prose does not travel",
				s.path, s.manifestDir)
			continue
		}
		guidePath := filepath.Join(s.manifestDir, cfg.Generation.StyleGuide)
		if _, err := os.Stat(guidePath); err != nil {
			t.Errorf("%s: generation.style_guide %q does not resolve to a file (%s): %v",
				s.path, cfg.Generation.StyleGuide, guidePath, err)
		}
	}
}

// TestSaveSkillCapturesHouseStyle pins the M5 Phase 1 deliverable itself: the
// /tinkerdown:save instructions must tell a capture to carry the house style, not just
// sources + actions. This is a structural mention-check (in the spirit of
// TestSkillWiresHouseStyle) — it is the only guard on the prose edit, since the
// round-trip above guards the *example artifacts*, not the *instructions*. It does not,
// and cannot, prove a future LLM capture obeys; it only fails loudly if the instruction
// is reverted so the omission returns silently.
func TestSaveSkillCapturesHouseStyle(t *testing.T) {
	const savePath = "skills/tinkerdown-save/SKILL.md"
	doc := readDocFile(t, savePath)
	for _, want := range []string{"styling", "style_guide", "style-guide.md", "on-brand"} {
		if !strings.Contains(doc, want) {
			t.Errorf("%s does not instruct capturing the house style: missing %q", savePath, want)
		}
	}
}
