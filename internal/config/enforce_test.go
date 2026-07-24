package config

import "testing"

// TestEnforceApprovedSurface covers the M3 runtime gate predicates: approval is
// opt-in (no generation block → no gate), and with a block only approved
// names pass.
func TestEnforceApprovedSurface(t *testing.T) {
	// No generation block → opt-in, nothing is gated.
	plain := &Config{}
	if err := plain.EnforceApprovedAction("anything"); err != nil {
		t.Errorf("no generation block should not gate actions: %v", err)
	}
	if err := plain.EnforceApprovedSource("anything"); err != nil {
		t.Errorf("no generation block should not gate sources: %v", err)
	}

	c := &Config{Generation: &GenerationConfig{
		Actions: []string{"approve-export"},
		Sources: []string{"access_requests"},
	}}
	if err := c.EnforceApprovedAction("approve-export"); err != nil {
		t.Errorf("approved action should pass: %v", err)
	}
	if err := c.EnforceApprovedAction("evil"); err == nil {
		t.Error("unapproved action should be rejected")
	}
	if err := c.EnforceApprovedSource("access_requests"); err != nil {
		t.Errorf("approved source should pass: %v", err)
	}
	if err := c.EnforceApprovedSource("access_store"); err == nil {
		t.Error("unapproved source should be rejected")
	}
}
