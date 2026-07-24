package tinkerdown_test

import (
	"encoding/csv"
	"os"
	"slices"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// frameworkSkills are the skills/ dirs that are framework-authoring tooling, not
// captured user workflows. The saved-skills gallery indexes the latter. This set is
// explicit, not auto-detected: adding a new capture (or a new framework skill) forces
// an update here or in the gallery index — a deliberate choice, never silent drift.
// Same explicit-list philosophy as TestCapturedSkillsWellFormed (which names the
// skills it checks rather than sniffing for them).
var frameworkSkills = []string{"tinkerdown", "tinkerdown-save"}

// TestSavedSkillsGalleryInSync asserts the committed gallery index
// (examples/gallery/skills.csv) lists exactly the captured-workflow skills present
// under skills/ — every skill dir that is not framework tooling.
//
// The gallery must render a static index because no source type reads a directory of
// SKILL.md frontmatter (the M5 Phase 2 STOP gate). This test is what keeps that static
// index honest as captures accumulate: a new skills/<name>/ that is neither framework
// tooling nor listed fails here, forcing the author to list it (or classify it).
func TestSavedSkillsGalleryInSync(t *testing.T) {
	// Captured workflows on disk = skills/ subdirs minus the framework set.
	entries, err := os.ReadDir("skills")
	if err != nil {
		t.Fatalf("read skills/: %v", err)
	}
	var onDisk []string
	for _, e := range entries {
		if e.IsDir() && !slices.Contains(frameworkSkills, e.Name()) {
			onDisk = append(onDisk, e.Name())
		}
	}

	// Workflows the gallery lists = first column of the committed index (minus header).
	f, err := os.Open("examples/gallery/skills.csv")
	if err != nil {
		t.Fatalf("open gallery index: %v", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("parse gallery index: %v", err)
	}
	if len(rows) < 1 {
		t.Fatal("gallery index has no header row")
	}
	if got := rows[0][0]; got != "name" {
		t.Fatalf("gallery index first column header is %q, want \"name\" (the app template reads {{.Name}})", got)
	}
	var listed []string
	for _, r := range rows[1:] {
		if len(r) > 0 {
			listed = append(listed, r[0])
		}
	}

	slices.Sort(onDisk)
	slices.Sort(listed)
	if !slices.Equal(onDisk, listed) {
		t.Errorf("gallery index out of sync with skills/:\n"+
			"  captured on disk: %v\n  listed in gallery: %v\n"+
			"add the new capture to examples/gallery/skills.csv, or the new framework skill to frameworkSkills",
			onDisk, listed)
	}
}

// TestSavedSkillsGalleryManifestLoads is the fast (non-Chrome) structural guard that
// the gallery is a real, loadable Tinkerdown app: its manifest loads and exposes the
// read-only csv source the page binds. The live render is verified by the E2E.
func TestSavedSkillsGalleryManifestLoads(t *testing.T) {
	cfg, err := config.LoadFromDir("examples/gallery")
	if err != nil {
		t.Fatalf("load gallery manifest: %v", err)
	}
	src, ok := cfg.Sources["skills"]
	if !ok {
		t.Fatal("gallery manifest has no \"skills\" source")
	}
	if src.Type != "csv" {
		t.Errorf("skills source type = %q, want csv", src.Type)
	}
	if !src.IsReadonly() {
		t.Error("skills source must be readonly — a gallery lists, it never mutates")
	}
}
