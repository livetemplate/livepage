package server

import (
	"fmt"
	"strings"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// validThemes maps the user-configurable theme name to the runtime value
// the client-side toggle accepts. "clean" is a legacy alias for "light".
var validThemes = map[string]string{
	"light": "light",
	"dark":  "dark",
	"auto":  "auto",
	"clean": "light",
}

// themeDefault returns the initial theme selection for a fresh visitor with
// no localStorage value. Falls back to "auto" (system preference) for empty
// or unrecognized config values.
func themeDefault(s config.StylingConfig) string {
	if v, ok := validThemes[strings.ToLower(strings.TrimSpace(s.Theme))]; ok {
		return v
	}
	return "auto"
}

// buildStylingOverrideCSS returns a <style> block overriding theme tokens
// based on user config. Returns "" if no overrides are set.
func buildStylingOverrideCSS(s config.StylingConfig) string {
	primary := sanitizeCSSValue(s.PrimaryColor)
	font := sanitizeCSSValue(s.Font)
	if primary == "" && font == "" {
		return ""
	}

	var out strings.Builder
	out.WriteString("<style>\n")
	if primary != "" {
		fmt.Fprintf(&out, "        :root { --accent: %s; }\n", primary)
	}
	if font != "" {
		fmt.Fprintf(&out, "        body { font-family: %s, -apple-system, BlinkMacSystemFont, \"Segoe UI\", Roboto, Helvetica, Arial, sans-serif; }\n", font)
	}
	out.WriteString("    </style>")
	return out.String()
}

// sanitizeCSSValue strips characters that could escape a CSS value/string
// context. Returns "" for any value containing a meta-character. Caller is
// expected to already have applied trimming to whitespace-only inputs.
func sanitizeCSSValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, "<>{};\"\n\r") {
		return ""
	}
	return v
}
