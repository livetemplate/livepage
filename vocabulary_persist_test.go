package tinkerdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLvtPersistMigration: lvt-persist is no longer a known attribute, and using
// it yields a migration hint pointing at lvt-source (M2 Phase 3). The client
// bundle still reads the attribute, but Tinkerdown's server no longer supports
// the persist model, so validate steers authors to the supported pattern.
func TestLvtPersistMigration(t *testing.T) {
	if knownAttributes["lvt-persist"] {
		t.Fatal("lvt-persist should no longer be a known attribute")
	}
	if hint := suggestAttribute("lvt-persist"); !strings.Contains(hint, "lvt-source") {
		t.Errorf("lvt-persist hint should point at lvt-source, got %q", hint)
	}
}

// TestStateRefDiagnosticTeachesLvtSource: a block with no state must be told the
// real fix (add lvt-source), not the undefined state="block-id" concept a
// self-correcting agent cannot act on (M2 Phase 3 / M1 Phase 5 feed-forward #3).
func TestStateRefDiagnosticTeachesLvtSource(t *testing.T) {
	dir := t.TempDir()
	doc := "```lvt\n<div>{{range .Data}}{{.x}}{{end}}</div>\n```\n"
	path := filepath.Join(dir, "app.md")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFileInSite(path, dir)
	if err == nil {
		t.Fatal("expected a 'no state reference' error for a block with no state")
	}
	if !strings.Contains(err.Error(), "lvt-source") {
		t.Errorf("state-ref diagnostic should teach lvt-source, got: %v", err)
	}
}
