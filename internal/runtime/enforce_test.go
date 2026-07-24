package runtime

import (
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// TestHandleAction_ApprovedSurfaceGate proves the M3 runtime gate fires on both
// WS paths — a custom action and a builtin write — before the action executes,
// so no real source is needed to exercise the reject path.
func TestHandleAction_ApprovedSurfaceGate(t *testing.T) {
	cfg := &config.Config{Generation: &config.GenerationConfig{
		Actions: []string{"ok-action"},
		Sources: []string{"ok-source"},
	}}

	// Unapproved custom action → rejected (gate is before executeCustomAction).
	s := &GenericState{
		actions: map[string]*config.Action{"evil-action": {Kind: "sql"}},
		cfg:     cfg,
	}
	if err := s.HandleAction("evil-action", nil); err == nil {
		t.Error("unapproved custom action should be rejected")
	} else if !strings.Contains(err.Error(), "approved set") {
		t.Errorf("expected approved-set error, got: %v", err)
	}

	// Builtin write on an unapproved bound source → rejected (before handleWriteAction).
	s2 := &GenericState{sourceName: "evil-source", cfg: cfg}
	if err := s2.HandleAction("add", nil); err == nil {
		t.Error("builtin write on an unapproved source should be rejected")
	}

	// No generation block (nil cfg) → opt-in, the gate does not fire: an
	// unapproved-looking custom action reaches execution (and fails there, not at
	// the gate). A nil *config.Config receiver is safe by EnforceApprovedAction's
	// contract.
	s3 := &GenericState{actions: map[string]*config.Action{"whatever": {Kind: "sql"}}}
	if err := s3.HandleAction("whatever", nil); err != nil && strings.Contains(err.Error(), "approved set") {
		t.Errorf("no generation block should not gate: %v", err)
	}
}
