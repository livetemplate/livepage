package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestGenerationYAMLRoundTrip exercises the yaml tags on the generation block and the
// describes: fields. Every other test for this feature builds Config structs directly
// in Go, so a mistyped tag would be invisible to all of them — the config would parse
// into zero values and approval would silently do nothing.
func TestGenerationYAMLRoundTrip(t *testing.T) {
	const src = `
sources:
  requests:
    describes: "Pending PII access requests"
    type: sqlite
    db: ./requests.db
    table: requests
actions:
  approve-request:
    describes: "Grants scoped access and writes an audit record"
    kind: sql
    source: requests
    statement: "UPDATE requests SET status = 'approved' WHERE id = :id"
    confirm: "Approve this request?"
generation:
  sources: [requests]
  actions: [approve-request]
  style_guide: ./style-guide.md
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !cfg.IsManifest() {
		t.Fatal("generation block did not parse; IsManifest() is false")
	}
	if !cfg.ApprovedSource("requests") {
		t.Error(`generation.sources did not parse: "requests" not approved`)
	}
	if !cfg.ApprovedAction("approve-request") {
		t.Error(`generation.actions did not parse: "approve-request" not approved`)
	}
	if got := cfg.Generation.StyleGuide; got != "./style-guide.md" {
		t.Errorf("style_guide = %q, want ./style-guide.md", got)
	}
	if got := cfg.Sources["requests"].Describes; got != "Pending PII access requests" {
		t.Errorf("source describes = %q, want the declared text", got)
	}
	if got := cfg.Actions["approve-request"].Describes; got != "Grants scoped access and writes an audit record" {
		t.Errorf("action describes = %q, want the declared text", got)
	}
	if err := cfg.ValidateGeneration(); err != nil {
		t.Errorf("a well-formed manifest should validate: %v", err)
	}
}

// TestValidateGeneration covers the check that an approved name refers to something
// real.
//
// This is a security check, not a tidiness one: approval is what pins a name against
// redefinition by page frontmatter, so an approved name with nothing behind it never
// pins anything — leaving that name shadowable. A typo in generation.sources would
// remove a protection while appearing to add one.
func TestValidateGeneration(t *testing.T) {
	base := func() *Config {
		return &Config{
			Sources: map[string]SourceConfig{"requests": {Type: "sqlite"}},
			Actions: map[string]*Action{"approve": {Kind: "sql"}},
		}
	}

	t.Run("approving declared names is valid", func(t *testing.T) {
		c := base()
		c.Generation = &GenerationConfig{Sources: []string{"requests"}, Actions: []string{"approve"}}
		if err := c.ValidateGeneration(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("a mistyped source name is rejected", func(t *testing.T) {
		c := base()
		c.Generation = &GenerationConfig{Sources: []string{"requets"}}
		err := c.ValidateGeneration()
		if err == nil {
			t.Fatal("a typo in generation.sources silently disables pinning; it must fail")
		}
		if !strings.Contains(err.Error(), "requets") {
			t.Errorf("error should name the offending entry, got: %v", err)
		}
	})

	t.Run("a mistyped action name is rejected", func(t *testing.T) {
		c := base()
		c.Generation = &GenerationConfig{Actions: []string{"aprove"}}
		if err := c.ValidateGeneration(); err == nil {
			t.Error("a typo in generation.actions must fail")
		}
	})

	t.Run("an action declared as nil is rejected", func(t *testing.T) {
		c := base()
		c.Actions["broken"] = nil
		c.Generation = &GenerationConfig{Actions: []string{"broken"}}
		if err := c.ValidateGeneration(); err == nil {
			t.Error("approving a nil action must fail rather than pin nothing")
		}
	})

	t.Run("no generation block is always valid", func(t *testing.T) {
		if err := base().ValidateGeneration(); err != nil {
			t.Errorf("a project without a manifest must be unaffected: %v", err)
		}
	})
}
