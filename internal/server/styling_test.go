package server

import (
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

func TestThemeDefault(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"light", "light"},
		{"dark", "dark"},
		{"clean", "light"},
		{"CLEAN", "light"},
		{"  dark  ", "dark"},
		{"unknown", "auto"},
	}
	for _, c := range cases {
		got := themeDefault(config.StylingConfig{Theme: c.in})
		if got != c.want {
			t.Errorf("themeDefault(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildStylingOverrideCSS_Empty(t *testing.T) {
	if got := buildStylingOverrideCSS(config.StylingConfig{}); got != "" {
		t.Errorf("empty config should return empty string, got %q", got)
	}
}

func TestBuildStylingOverrideCSS_PrimaryColorOnly(t *testing.T) {
	got := buildStylingOverrideCSS(config.StylingConfig{PrimaryColor: "#5a67d8"})
	if !strings.Contains(got, "--accent: #5a67d8") {
		t.Errorf("missing accent override: %q", got)
	}
	if strings.Contains(got, "font-family") {
		t.Errorf("font-family should not appear when font is empty: %q", got)
	}
}

func TestBuildStylingOverrideCSS_FontOnly(t *testing.T) {
	got := buildStylingOverrideCSS(config.StylingConfig{Font: "system-ui"})
	if !strings.Contains(got, "font-family: system-ui") {
		t.Errorf("missing font-family: %q", got)
	}
	if strings.Contains(got, "--accent") {
		t.Errorf("--accent should not appear when primary color is empty: %q", got)
	}
}

func TestBuildStylingOverrideCSS_Both(t *testing.T) {
	got := buildStylingOverrideCSS(config.StylingConfig{
		PrimaryColor: "rebeccapurple",
		Font:         "Inter",
	})
	if !strings.Contains(got, "--accent: rebeccapurple") {
		t.Errorf("missing accent: %q", got)
	}
	if !strings.Contains(got, "font-family: Inter") {
		t.Errorf("missing font: %q", got)
	}
	if !strings.HasPrefix(got, "<style>") || !strings.HasSuffix(got, "</style>") {
		t.Errorf("malformed style block: %q", got)
	}
}

func TestBuildStylingOverrideCSS_Tokens(t *testing.T) {
	got := buildStylingOverrideCSS(config.StylingConfig{
		Tokens: map[string]string{"accent": "#ff00ff", "card_bg": "#00ff00"},
	})
	// snake_case keys map to the CSS custom property, emitted into :root.
	if !strings.Contains(got, "--accent: #ff00ff;") {
		t.Errorf("missing --accent override: %q", got)
	}
	if !strings.Contains(got, "--card-bg: #00ff00;") {
		t.Errorf("missing --card-bg override: %q", got)
	}
}

func TestBuildStylingOverrideCSS_TokensSanitized(t *testing.T) {
	// A token value that tries to break out of the CSS context is dropped, not emitted.
	got := buildStylingOverrideCSS(config.StylingConfig{
		Tokens: map[string]string{"accent": "red; } body { display:none"},
	})
	if strings.Contains(got, "display:none") {
		t.Errorf("malicious token value should be sanitized away, got: %q", got)
	}
}

func TestSanitizeCSSValue_BlocksInjection(t *testing.T) {
	blocked := []string{
		"</style><script>alert(1)</script>",
		"red; } body { display: none",
		`abc"def`,
		"abc\nbad",
		"abc\rbad",
		"<svg>",
	}
	for _, v := range blocked {
		if got := sanitizeCSSValue(v); got != "" {
			t.Errorf("sanitizeCSSValue(%q) should be blocked, got %q", v, got)
		}
	}
}

func TestSanitizeCSSValue_AllowsValidValues(t *testing.T) {
	allowed := []string{
		"#5a67d8",
		"rebeccapurple",
		"rgb(90, 103, 216)",
		"system-ui",
		"Inter",
	}
	for _, v := range allowed {
		if got := sanitizeCSSValue(v); got != v {
			t.Errorf("sanitizeCSSValue(%q) should pass, got %q", v, got)
		}
	}
}

// Integration check: when both override CSS and theme default are wired, they
// must not interfere with each other or produce conflicting output.
func TestStylingOverrideAndThemeDefault_Compose(t *testing.T) {
	cfg := config.StylingConfig{
		Theme:        "dark",
		PrimaryColor: "#5a67d8",
		Font:         "Inter",
	}
	if got := themeDefault(cfg); got != "dark" {
		t.Errorf("themeDefault dark: got %q", got)
	}
	got := buildStylingOverrideCSS(cfg)
	if !strings.Contains(got, "--accent: #5a67d8") || !strings.Contains(got, "font-family: Inter") {
		t.Errorf("override missing: %q", got)
	}
}
