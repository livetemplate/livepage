package server

import (
	"fmt"
	"html"
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

// buildCustomCSSLink returns a <link rel="stylesheet"> tag for a configured
// custom stylesheet path, or "" if unset/unsafe. The path is site-relative
// (e.g. "assets/landing.css") and served from <rootDir>/assets by serveAsset.
// Paths containing characters that could break out of the href attribute are
// rejected (return "") rather than emitted unsafely.
func buildCustomCSSLink(cssPath string) string {
	p := strings.TrimSpace(cssPath)
	if p == "" {
		return ""
	}
	// Reject anything that could escape the attribute or smell like a remote /
	// protocol-relative URL. Check BEFORE trimming the leading slash so that a
	// "//host/x" input is caught (trimming one slash first would hide the "//"
	// and yield a protocol-relative href). This is a same-origin, server-
	// relative asset path only — no scheme, no "//", no escapes.
	if strings.ContainsAny(p, "\"'<>\n\r :") || strings.Contains(p, "//") {
		return ""
	}
	p = strings.TrimPrefix(p, "/") // normalize "/assets/x" and "assets/x" alike
	if p == "" {
		return ""
	}
	return fmt.Sprintf(`<link rel="stylesheet" href="/%s">`, html.EscapeString(p))
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
