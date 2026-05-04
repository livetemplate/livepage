package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// Regression: G16-era + responsive-sidebar fix.
//
// Two long-standing layout bugs were caught after launch by visual
// inspection of livetemplate.fly.dev and locked in here:
//
//  1. .nav-pages didn't override PicoCSS's `nav ul { display: flex }`,
//     so sidebar items rendered horizontally (cramped, truncated).
//  2. Mobile breakpoint hid the sidebar with translateX(-100%) but
//     emitted no hamburger button — the sidebar was unreachable.
//
// These tests fail if a future change reintroduces either bug.

func TestSidebarOverridesPicoFlex(t *testing.T) {
	body := renderHomeWithSidebar(t)
	idx := strings.Index(body, ".nav-pages {")
	if idx < 0 {
		t.Fatal(".nav-pages CSS rule missing")
	}
	// The rule comment contains `}` so a naive close-brace scan cuts off
	// too early. Instead just check the next ~500 chars after the rule
	// opens for the explicit display override. Without it, PicoCSS's
	// `nav ul { display: flex }` cascades through and re-introduces
	// the cramped-horizontal-sidebar bug.
	window := body[idx:]
	if len(window) > 500 {
		window = window[:500]
	}
	if !strings.Contains(window, "display: block") {
		t.Errorf(".nav-pages must declare `display: block` to override PicoCSS nav-ul flex; first 500 chars after .nav-pages:\n%s", window)
	}
}

// PicoCSS sets `nav li { display: inline-block }` to lay out top-nav
// items horizontally. The first sidebar fix overrode the UL but not
// the LI, so list items continued packing inline-block in the sidebar
// (~135px wide each, two-or-three short titles still fit on one line).
// This locks the second-level override.
func TestSidebarLiIsListItem(t *testing.T) {
	body := renderHomeWithSidebar(t)
	idx := strings.Index(body, ".nav-pages li {")
	if idx < 0 {
		t.Fatal(".nav-pages li CSS rule missing — without it, PicoCSS lays out items as inline-block")
	}
	window := body[idx:]
	if len(window) > 500 {
		window = window[:500]
	}
	if !strings.Contains(window, "display: list-item") && !strings.Contains(window, "display: block") {
		t.Errorf(".nav-pages li must override PicoCSS's display: inline-block (use list-item or block); rule window:\n%s", window)
	}
}

func TestSidebarRendersMobileToggleAndBackdrop(t *testing.T) {
	body := renderHomeWithSidebar(t)

	if !strings.Contains(body, `class="tinkerdown-nav-toggle"`) {
		t.Error("hamburger toggle button missing — mobile users have no way to open the sidebar")
	}
	if !strings.Contains(body, `aria-label="Toggle navigation"`) {
		t.Error("hamburger button missing aria-label (a11y regression)")
	}
	if !strings.Contains(body, `aria-controls="tinkerdown-sidebar"`) {
		t.Error("hamburger missing aria-controls — assistive tech can't link button → menu")
	}
	if !strings.Contains(body, `class="tinkerdown-nav-backdrop"`) {
		t.Error("backdrop element missing — mobile users can't tap-outside to close")
	}

	// CSS must hide the toggle by default (desktop) and reveal it under
	// the mobile media query.
	if !strings.Contains(body, ".tinkerdown-nav-toggle {\n            display: none;") {
		t.Error(".tinkerdown-nav-toggle should default to display: none (hidden on desktop)")
	}
	// The page emits multiple `@media (max-width: 768px)` rules (one per
	// component). Rather than parse them all, just assert the
	// toggle-flips-to-flex rule exists anywhere in the document. The
	// rule body the responsive fix introduces:
	//
	//     .tinkerdown-nav-toggle {
	//         display: flex;
	//     }
	if !strings.Contains(body, ".tinkerdown-nav-toggle {\n                display: flex;") {
		t.Error("mobile breakpoint must flip toggle to display: flex (regression: sidebar unreachable on mobile)")
	}
}

// Regression: a long sidebar (10+ sections) was unscrollable because the
// base sidebar rule declared `overflow: hidden`. Bottom items were
// clipped and unreachable on viewports shorter than the full menu.
//
// The rendered CSS has variable indentation (depending on Sprintf
// nesting) so anchor-and-substring is tricky. Easiest: just assert
// `overflow-y: auto` exists somewhere in the document AND no
// `.tinkerdown-nav-sidebar` rule body still uses `overflow: hidden`.
func TestSidebarIsScrollable(t *testing.T) {
	body := renderHomeWithSidebar(t)

	if !strings.Contains(body, "overflow-y: auto") {
		t.Error(".tinkerdown-nav-sidebar must declare overflow-y: auto so long navs can scroll")
	}
	// Look at every .tinkerdown-nav-sidebar { rule body and ensure
	// none declare overflow: hidden (the original bug).
	rest := body
	for {
		idx := strings.Index(rest, ".tinkerdown-nav-sidebar {")
		if idx < 0 {
			break
		}
		end := strings.Index(rest[idx:], "}")
		if end < 0 {
			break
		}
		ruleBody := rest[idx : idx+end]
		if strings.Contains(ruleBody, "overflow: hidden") {
			t.Errorf("found .tinkerdown-nav-sidebar rule with overflow: hidden (clips bottom nav items):\n%s", ruleBody)
		}
		rest = rest[idx+end:]
	}
}

// G16 sub-issue: pre-rendered mermaid SVGs come back from Kroki at native
// pixel dimensions (e.g. 1681×585 for sequence diagrams). Without
// `max-width: 100%; height: auto` on the SVG, mobile Safari forces a
// horizontal scrollbar and auto-scales the whole page down to fit —
// making text appear squeezed at ~55% width and the hamburger button
// shrink to invisibility. Lock the responsive constraint.
func TestPrerenderedMermaidIsResponsive(t *testing.T) {
	body := renderHomeWithSidebar(t)

	idx := strings.Index(body, ".mermaid-prerendered svg {")
	if idx < 0 {
		t.Fatal(".mermaid-prerendered svg CSS rule missing — diagrams will overflow mobile viewports")
	}
	window := body[idx:]
	if len(window) > 300 {
		window = window[:300]
	}
	if !strings.Contains(window, "max-width: 100%") {
		t.Errorf(".mermaid-prerendered svg must declare max-width: 100%% to fit narrow viewports; rule:\n%s", window)
	}
	if !strings.Contains(window, "height: auto") {
		t.Errorf(".mermaid-prerendered svg must declare height: auto so scaling stays proportional; rule:\n%s", window)
	}
}

// Regression: <pre> code blocks and interactive blocks used a negative
// margin (margin: 1.5rem calc((800px - 100%) / 2 * -1)) to bleed past
// the article column on desktop. On mobile (393px viewport) the
// expression evaluates to ~-203px, pulling each block ~407px wider than
// the viewport and forcing the entire page into horizontal overflow.
// The fix moves the bleed-out trick behind a min-width media query so
// it only applies when the viewport is wide enough to absorb it.
func TestPreBleedOutGuardedByMediaQuery(t *testing.T) {
	body := renderHomeWithSidebar(t)

	// The base `pre {` rule must NOT declare the negative-margin
	// expression — that's the bug.
	rest := body
	for {
		idx := strings.Index(rest, "pre {")
		if idx < 0 {
			break
		}
		end := strings.Index(rest[idx:], "}")
		if end < 0 {
			break
		}
		ruleBody := rest[idx : idx+end]
		if strings.Contains(ruleBody, "(800px - 100%) / 2 * -1") {
			t.Errorf("base `pre` rule declares negative-margin bleed-out — overflows mobile viewports:\n%s", ruleBody)
		}
		rest = rest[idx+end:]
	}

	// The bleed-out IS allowed inside an @media (min-width: 900px) block.
	if !strings.Contains(body, "@media (min-width: 900px)") {
		t.Error("pre/wasm-block bleed-out must live inside @media (min-width: 900px) — guarantees no mobile overflow")
	}
	if !strings.Contains(body, "calc((800px - 100%) / 2 * -1)") {
		t.Error("desktop bleed-out math (calc((800px - 100%) / 2 * -1)) should still exist inside the media query")
	}
}

// Regression: wide reference tables (e.g. /reference/template-support-matrix)
// have many columns whose combined intrinsic width exceeds the content
// column on mobile. v0.1.10 added a rule but used `article table` as the
// selector — tinkerdown does not emit a semantic <article> element, so
// the rule matched nothing and pages still overflowed. Lock the
// .content-wrapper-scoped selector so the rule actually matches.
func TestContentTablesAreScrollable(t *testing.T) {
	body := renderHomeWithSidebar(t)

	idx := strings.Index(body, ".content-wrapper table {")
	if idx < 0 {
		t.Fatal(".content-wrapper table rule missing — wide tables will force horizontal page overflow on mobile")
	}
	window := body[idx:]
	if len(window) > 300 {
		window = window[:300]
	}
	if !strings.Contains(window, "display: block") {
		t.Errorf(".content-wrapper table must declare display: block so it can scroll independently; rule:\n%s", window)
	}
	if !strings.Contains(window, "overflow-x: auto") {
		t.Errorf(".content-wrapper table must declare overflow-x: auto; rule:\n%s", window)
	}
	// And explicitly assert the broken selector is GONE — defense in
	// depth against re-introducing it.
	if strings.Contains(body, "article table {") {
		t.Error("`article table` selector found — tinkerdown does not emit <article>, this matches nothing")
	}
}

// Regression: PicoCSS sets `code, kbd, samp { display: inline-block }`
// which prevents long inline <code> (CLI flags, package paths) from
// wrapping at word boundaries — the unbreakable inline-block forces
// horizontal page overflow on mobile. Override pico's display and add
// overflow-wrap so long inline tokens soft-wrap.
func TestInlineCodeIsWrappable(t *testing.T) {
	body := renderHomeWithSidebar(t)

	// Find the base `code {` rule and check it explicitly declares
	// display:inline + overflow-wrap. Use the leading-whitespace
	// anchor to avoid matching `pre code {` or `.foo code {` rules.
	idx := strings.Index(body, "        code {")
	if idx < 0 {
		t.Fatal("base `code` rule missing")
	}
	end := strings.Index(body[idx:], "}")
	if end < 0 {
		t.Fatal("malformed `code` rule")
	}
	rule := body[idx : idx+end]
	if !strings.Contains(rule, "display: inline") {
		t.Errorf("base `code` rule must declare display:inline to override PicoCSS inline-block; rule:\n%s", rule)
	}
	if !strings.Contains(rule, "overflow-wrap: anywhere") {
		t.Errorf("base `code` rule must declare overflow-wrap:anywhere so long inline code can wrap; rule:\n%s", rule)
	}
}

// Regression: body has margin-left: <sidebar-width> to clear the
// fixed-position sidebar, but margin-left does NOT shrink the body's
// own width (margins live outside the box even with box-sizing:
// border-box). Without an explicit width: calc(100% - <sidebar-width>),
// body's natural 100vw width plus the sidebar margin produces a total
// horizontal footprint of 100vw + sidebar-width, forcing the document
// into horizontal scroll on desktop and creating a wide visible gap
// between the sidebar's right edge and the centered content-wrapper.
func TestBodyWidthSubtractsSidebar(t *testing.T) {
	body := renderHomeWithSidebar(t)

	// Find the base body:has(.tinkerdown-nav-sidebar) rule.
	idx := strings.Index(body, "body:has(.tinkerdown-nav-sidebar) {")
	if idx < 0 {
		t.Fatal("body:has(.tinkerdown-nav-sidebar) rule missing")
	}
	end := strings.Index(body[idx:], "}")
	rule := body[idx : idx+end]
	if !strings.Contains(rule, "margin-left: 360px") {
		t.Errorf("base sidebar rule should declare margin-left: 360px; rule:\n%s", rule)
	}
	if !strings.Contains(rule, "width: calc(100% - 360px)") {
		t.Errorf("base sidebar rule MUST declare width: calc(100%% - 360px) so body footprint = viewport; rule:\n%s", rule)
	}

	// And the @media (max-width: 1024px) rule that narrows the sidebar
	// to 320px must apply the matching width override.
	if !strings.Contains(body, "width: calc(100% - 320px)") {
		t.Error("the 1024px breakpoint must also override width: calc(100% - 320px) to match its 320px sidebar")
	}
}

// Regression for the v0.1.13 → v0.1.14 mobile breakage: the desktop
// fix added `width: calc(100% - 360px)` to the base body rule, but the
// @media (max-width: 768px) block — which sets margin-left: 0 to hide
// the sidebar on mobile — did NOT also override width. Body inherited
// the desktop calc, evaluating to 393 - 360 = 33px on iPhone 14, and
// page text stacked one character per line ("Install" became I/n/s/t/a/l/l).
// Lock in: the body:has(.tinkerdown-nav-sidebar) rule that resets
// margin-left to 0 must ALSO reset width to 100%.
func TestMobileBodyWidthResetsToFullViewport(t *testing.T) {
	body := renderHomeWithSidebar(t)

	// Walk through every body:has(.tinkerdown-nav-sidebar) rule body
	// and find the one that contains "margin-left: 0" — that's the
	// mobile reset rule, which must also reset width.
	const marker = "body:has(.tinkerdown-nav-sidebar) {"
	rest := body
	var mobileRule string
	for {
		idx := strings.Index(rest, marker)
		if idx < 0 {
			break
		}
		end := strings.Index(rest[idx:], "}")
		if end < 0 {
			break
		}
		rule := rest[idx : idx+end]
		if strings.Contains(rule, "margin-left: 0;") || strings.Contains(rule, "margin-left: 0\n") {
			mobileRule = rule
			break
		}
		rest = rest[idx+end:]
	}
	if mobileRule == "" {
		t.Fatal("no body:has(.tinkerdown-nav-sidebar) rule with margin-left: 0 found — mobile body sizing is unhandled")
	}
	if !strings.Contains(mobileRule, "width: 100%") {
		t.Errorf("the mobile body rule (margin-left: 0) MUST declare width: 100%% to override the desktop calc(100%% - 360px), otherwise body collapses to ~33px on a 393px viewport; rule:\n%s", mobileRule)
	}
}

func TestSidebarToggleScriptIsBound(t *testing.T) {
	body := renderHomeWithSidebar(t)
	// The inline toggle script needs to be present and use the
	// double-bind guard so hot-reload doesn't double-wire handlers.
	if !strings.Contains(body, "toggle.dataset.bound") {
		t.Error("toggle JS should set/check dataset.bound to avoid double-binding on re-render")
	}
	if !strings.Contains(body, "Escape") {
		t.Error("toggle JS should close the sidebar on Escape (a11y / UX expectation)")
	}
}

// renderHomeWithSidebar boots a tiny in-memory site, hits /, and returns
// the rendered HTML body.
func renderHomeWithSidebar(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "home.md"), []byte("---\ntitle: Home\n---\n# Home\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Title = "Test Docs"
	cfg.Site = &config.SiteConfig{Home: "home.md"}
	cfg.Features.Sidebar = true
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Home", Path: "home.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/home", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

