package config

import "testing"

func manifest() *Config {
	return &Config{
		Sources:    map[string]SourceConfig{"requests": {Type: "sqlite"}},
		Actions:    map[string]*Action{"approve": {Kind: "sql"}},
		Generation: &GenerationConfig{Sources: []string{"requests"}, Actions: []string{"approve"}},
	}
}

func TestCheckPolicy(t *testing.T) {
	t.Run("a document using only approved names is clean", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{
			Sources: []string{"requests"}, Actions: []string{"approve"},
		})
		if len(got) != 0 {
			t.Errorf("expected no violations, got %v", got)
		}
	})

	t.Run("referencing an unapproved source is reported", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{Sources: []string{"secrets"}})
		if len(got) != 1 || got[0].Name != "secrets" {
			t.Fatalf("expected one violation for secrets, got %v", got)
		}
		if got[0].Hint == "" {
			t.Error("a diagnostic without a hint cannot be self-corrected against")
		}
	})

	// The check the plan originally missed. A reference-only lint passes this
	// document: it declares `evil` and references `evil`, so every name it uses
	// resolves to something it defined.
	t.Run("declaring an unapproved source is reported even though its reference resolves", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{
			Sources:         []string{"evil"},
			DeclaredSources: []string{"evil"},
		})
		if len(got) != 1 {
			t.Fatalf("expected exactly one violation, got %v", got)
		}
		if got[0].Name != "evil" || got[0].Shadows {
			t.Errorf("expected an unapproved-declaration violation, got %+v", got[0])
		}
	})

	t.Run("declaring an unapproved action is reported", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{
			Actions: []string{"exfiltrate"}, DeclaredActions: []string{"exfiltrate"},
		})
		if len(got) != 1 || got[0].Kind != "action" {
			t.Fatalf("expected one action violation, got %v", got)
		}
	})

	t.Run("shadowing an approved name is flagged as ignored, not as a breach", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{
			Sources: []string{"requests"}, DeclaredSources: []string{"requests"},
		})
		if len(got) != 1 {
			t.Fatalf("expected one violation, got %v", got)
		}
		if !got[0].Shadows {
			t.Error("redefining an approved name should be marked as shadowing")
		}
	})

	t.Run("a project without a manifest has no approved set to violate", func(t *testing.T) {
		c := manifest()
		c.Generation = nil
		got := c.CheckPolicy(DocumentRefs{Sources: []string{"anything"}, DeclaredActions: []string{"whatever"}})
		if got != nil {
			t.Errorf("approval is opt-in; expected no violations, got %v", got)
		}
	})

	t.Run("an unapproved name is reported once, not twice", func(t *testing.T) {
		got := manifest().CheckPolicy(DocumentRefs{
			Sources:         []string{"evil"},
			DeclaredSources: []string{"evil"},
			Actions:         []string{"bad"},
			DeclaredActions: []string{"bad"},
		})
		if len(got) != 2 {
			t.Errorf("expected 2 violations (one per name), got %d: %v", len(got), got)
		}
	})
}
