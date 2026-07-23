package tinkerdown_test

import (
	"os"
	"strings"
	"testing"
)

// TestCapturedSkillsWellFormed checks the M1 Phase 6 skills — the captured
// pii-access-approval workflow and the /tinkerdown:save capability — are
// well-formed: valid frontmatter, and every repo path they point at exists.
//
// The pii-access-approval skill deliberately POINTS at examples/pii-access-approval/
// rather than copying it (the artifacts are a committed source of truth; a copy
// would be drift waiting to happen). This test is what makes that pointer safe: if
// the example is moved or renamed, the skill's dead reference fails here.
func TestCapturedSkillsWellFormed(t *testing.T) {
	skills := []struct {
		path string
		refs []string // repo-relative paths the skill references, which must exist
	}{
		{
			path: "skills/pii-access-approval/SKILL.md",
			refs: []string{
				"examples/pii-access-approval/tinkerdown.yaml",
				"examples/pii-access-approval/app.md",
				"examples/pii-access-approval/seed.sql",
				"examples/pii-access-approval/README.md",
			},
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
	}
}
