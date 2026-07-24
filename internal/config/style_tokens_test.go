package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// styling.tokens round-trips from tinkerdown.yaml into the loaded config, and
// each known snake_case key is accepted.
func TestLoad_StylingTokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := "title: Site\n" +
		"type: site\n" +
		"styling:\n" +
		"  tokens:\n" +
		"    accent: \"#5a67d8\"\n" +
		"    card_bg: \"#ffffff\"\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Styling.Tokens["accent"]; got != "#5a67d8" {
		t.Errorf("tokens[accent] = %q, want %q", got, "#5a67d8")
	}
	if got := cfg.Styling.Tokens["card_bg"]; got != "#ffffff" {
		t.Errorf("tokens[card_bg] = %q, want %q", got, "#ffffff")
	}
}

// An unknown token key is an author mistake — Load fails loudly, naming the
// offending key and listing the known tokens, rather than silently ignoring it.
func TestLoad_StylingTokensUnknownKeyRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := "title: Site\n" +
		"type: site\n" +
		"styling:\n" +
		"  tokens:\n" +
		"    accnet: \"#5a67d8\"\n" // typo'd "accent"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should reject an unknown token key")
	}
	if !strings.Contains(err.Error(), "accnet") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "accent") {
		t.Errorf("error should list known tokens (incl. accent), got: %v", err)
	}
}

// A known token key with an unsafe value is NOT a config error — ValidateStyleTokens
// gates keys, not values. The value round-trips into config and is dropped later at
// render (sanitizeCSSValue), exactly as primary_color/font handle an unsafe value.
// This locks the key-vs-value split so it can't silently change: Load succeeds, the
// bad value survives in config, and the render layer (not load) is what omits it.
func TestLoad_StylingTokensUnsafeValueAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := "title: Site\n" +
		"type: site\n" +
		"styling:\n" +
		"  tokens:\n" +
		"    accent: \"red; }\"\n" // known key, unsafe value (CSS meta-characters)
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load should accept a known key regardless of value (values are gated at render): %v", err)
	}
	if got := cfg.Styling.Tokens["accent"]; got != "red; }" {
		t.Errorf("value should round-trip into config unchanged (render sanitizes, not load): got %q", got)
	}
}

// Absent styling.tokens is backward-compatible: an empty map, no error.
func TestLoad_StylingTokensDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	if err := os.WriteFile(path, []byte("title: Site\ntype: site\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Styling.Tokens) != 0 {
		t.Errorf("cfg.Styling.Tokens = %v, want empty", cfg.Styling.Tokens)
	}
}

// With more than one unknown key, ValidateStyleTokens names the sorted-first key so
// the error is deterministic run-to-run (not whichever the map iteration yields).
func TestValidateStyleTokens_MultipleUnknownDeterministic(t *testing.T) {
	c := &Config{Styling: StylingConfig{Tokens: map[string]string{
		"zzz_bogus": "x",
		"aaa_bogus": "x",
	}}}
	err := c.ValidateStyleTokens()
	if err == nil {
		t.Fatal("expected an error for unknown token keys")
	}
	if !strings.Contains(err.Error(), "aaa_bogus") {
		t.Errorf("error should name the sorted-first unknown key (aaa_bogus): %v", err)
	}
	if strings.Contains(err.Error(), "zzz_bogus") {
		t.Errorf("error should name only the deterministic first key, not zzz_bogus: %v", err)
	}
}

// ValidateStyleTokens accepts every key in KnownStyleTokens — guards against the
// map and the validator drifting apart.
func TestValidateStyleTokens_AllKnownKeysAccepted(t *testing.T) {
	tokens := make(map[string]string, len(KnownStyleTokens))
	for k := range KnownStyleTokens {
		tokens[k] = "#000000"
	}
	c := &Config{Styling: StylingConfig{Tokens: tokens}}
	if err := c.ValidateStyleTokens(); err != nil {
		t.Errorf("all known tokens should validate, got: %v", err)
	}
}
