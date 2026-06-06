package config

import (
	"os"
	"path/filepath"
	"testing"
)

// styling.custom_css must round-trip from tinkerdown.yaml into the loaded
// config, and default to empty when absent (backward-compatible).
func TestLoad_StylingCustomCSS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := "title: Site\n" +
		"type: site\n" +
		"styling:\n" +
		"  theme: clean\n" +
		"  custom_css: assets/landing.css\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Styling.CustomCSS != "assets/landing.css" {
		t.Errorf("cfg.Styling.CustomCSS = %q, want %q", cfg.Styling.CustomCSS, "assets/landing.css")
	}
}

func TestLoad_StylingCustomCSSDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	if err := os.WriteFile(path, []byte("title: Site\ntype: site\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Styling.CustomCSS != "" {
		t.Errorf("cfg.Styling.CustomCSS = %q, want empty", cfg.Styling.CustomCSS)
	}
}
