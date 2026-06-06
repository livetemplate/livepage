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

// styling.site_css round-trips the same way; it's the site-wide companion to
// custom_css (loaded on every page, not just landing).
func TestLoad_StylingSiteCSS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := "title: Site\n" +
		"type: site\n" +
		"styling:\n" +
		"  theme: clean\n" +
		"  site_css: assets/brand.css\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Styling.SiteCSS != "assets/brand.css" {
		t.Errorf("cfg.Styling.SiteCSS = %q, want %q", cfg.Styling.SiteCSS, "assets/brand.css")
	}
}

func TestLoad_StylingSiteCSSDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	if err := os.WriteFile(path, []byte("title: Site\ntype: site\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Styling.SiteCSS != "" {
		t.Errorf("cfg.Styling.SiteCSS = %q, want empty", cfg.Styling.SiteCSS)
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
