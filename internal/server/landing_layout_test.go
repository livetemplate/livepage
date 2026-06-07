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

// buildCustomCSSLink is the pure helper that turns a configured path into a
// <link> tag (or "" when unset/unsafe).
func TestBuildCustomCSSLink(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"plain", "assets/landing.css", `<link rel="stylesheet" href="/assets/landing.css">`},
		{"leading slash normalized", "/assets/landing.css", `<link rel="stylesheet" href="/assets/landing.css">`},
		{"reject double slash / remote-ish", "//evil.com/x.css", ""},
		{"reject quote", `assets/x".css`, ""},
		{"reject space", "assets/a b.css", ""},
		{"reject angle", "assets/<x>.css", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildCustomCSSLink(c.in); got != c.want {
				t.Errorf("buildCustomCSSLink(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// newLandingSite writes a single-page site whose home uses `layout: landing`,
// wires styling.custom_css, and returns a Discover()'d server.
func newLandingSite(t *testing.T, frontmatter string) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"),
		[]byte(frontmatter+"\n# Welcome\n\nSome prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "index.md"}
	cfg.Styling.CustomCSS = "assets/landing.css"
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Home", Path: "index.md"}},
	}}
	srv := NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return srv
}

func get(t *testing.T, srv *Server, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// Regression guard: a `layout: landing` page renders the minimal shell — no
// docs chrome — but still wires the client and custom CSS.
func TestLandingLayout_MinimalShell(t *testing.T) {
	srv := newLandingSite(t, "---\ntitle: Landing\nlayout: landing\n---")
	code, body := get(t, srv, "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	// MUST NOT carry docs chrome.
	for _, banned := range []string{`class="content-wrapper"`, "renderSidebar", `class="sidebar"`, "page-toolbar", "!important"} {
		if strings.Contains(body, banned) {
			t.Errorf("landing shell unexpectedly contains docs chrome %q", banned)
		}
	}
	// Landing KEEPS PicoCSS — the shared foundation embed-lvt demos rely on
	// (recipe apps keep CSS in-body referencing Pico vars). custom_css loads
	// after it, so bespoke landing styling still wins.
	if !strings.Contains(body, "/assets/pico.css") {
		t.Errorf("landing shell should load pico.css (embed foundation)")
	}
	// MUST wire the client, title, body marker, theme script, and custom CSS.
	for _, want := range []string{
		`<script defer src="/assets/tinkerdown-client.js">`,
		"<title>Landing</title>",
		`<body class="lvt-landing">`,
		"tinkerdown-theme", // silent theme-detection script
		`<link rel="stylesheet" href="/assets/landing.css">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("landing shell missing %q", want)
		}
	}
}

// The default (docs) layout is unchanged: it keeps the content-wrapper and does
// NOT pick up the landing-only custom CSS injection.
func TestDefaultLayout_KeepsDocsChrome(t *testing.T) {
	srv := newLandingSite(t, "---\ntitle: Docs Home\n---") // no layout: landing
	code, body := get(t, srv, "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, `class="content-wrapper"`) {
		t.Errorf("default layout should keep .content-wrapper")
	}
	if strings.Contains(body, `href="/assets/landing.css"`) {
		t.Errorf("custom_css must NOT leak into the docs shell (landing-only)")
	}
	if !strings.Contains(body, "<title>Docs Home</title>") {
		t.Errorf("default layout missing title")
	}
}

// site_css is the shared-brand stylesheet: unlike custom_css (landing-only), it
// must be injected into BOTH the docs shell and the landing shell — and on the
// landing it must load BEFORE custom_css so landing styling can still override.
func TestSiteCSS_InjectedInBothShells(t *testing.T) {
	const siteLink = `<link rel="stylesheet" href="/assets/brand.css">`
	const customLink = `<link rel="stylesheet" href="/assets/landing.css">`

	newSite := func(t *testing.T, frontmatter string) *Server {
		t.Helper()
		srv := newLandingSite(t, frontmatter)
		srv.config.Styling.SiteCSS = "assets/brand.css"
		return srv
	}

	t.Run("docs shell includes site_css but not custom_css", func(t *testing.T) {
		srv := newSite(t, "---\ntitle: Docs Home\n---") // no layout: landing
		_, body := get(t, srv, "/")
		if !strings.Contains(body, siteLink) {
			t.Errorf("docs shell missing site_css link %q", siteLink)
		}
		if strings.Contains(body, customLink) {
			t.Errorf("custom_css must stay landing-only, leaked into docs shell")
		}
	})

	t.Run("landing shell includes site_css before custom_css", func(t *testing.T) {
		srv := newSite(t, "---\ntitle: Landing\nlayout: landing\n---")
		_, body := get(t, srv, "/")
		si := strings.Index(body, siteLink)
		ci := strings.Index(body, customLink)
		if si < 0 {
			t.Fatalf("landing shell missing site_css link %q", siteLink)
		}
		if ci < 0 {
			t.Fatalf("landing shell missing custom_css link %q", customLink)
		}
		if si > ci {
			t.Errorf("site_css (at %d) must load before custom_css (at %d) so landing can override", si, ci)
		}
	})
}

// Filesystem asset fallback: a real file under <rootDir>/assets is served with
// the right content type + nosniff; vendor assets still win; traversal/missing
// are refused.
func TestServeUserAsset(t *testing.T) {
	srv := newLandingSite(t, "---\ntitle: Landing\nlayout: landing\n---")
	// Write the custom stylesheet the landing references.
	assetsDir := filepath.Join(srv.rootDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sentinel = ".lvt-landing{color:rebeccapurple}"
	if err := os.WriteFile(filepath.Join(assetsDir, "landing.css"), []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("serves real css", func(t *testing.T) {
		code, body := get(t, srv, "/assets/landing.css")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if body != sentinel {
			t.Errorf("body = %q, want sentinel", body)
		}
	})

	t.Run("sets content-type and nosniff", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/assets/landing.css", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
			t.Errorf("Content-Type = %q, want text/css", ct)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("missing X-Content-Type-Options: nosniff")
		}
	})

	t.Run("vendor pico.css still wins over a user file", func(t *testing.T) {
		// A user file named pico.css must NOT shadow the embedded vendor asset.
		if err := os.WriteFile(filepath.Join(assetsDir, "pico.css"), []byte("/*hijack*/"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, body := get(t, srv, "/assets/pico.css")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if strings.Contains(body, "hijack") {
			t.Errorf("user file shadowed vendor pico.css")
		}
	})

	t.Run("missing file 404s", func(t *testing.T) {
		code, _ := get(t, srv, "/assets/does-not-exist.css")
		if code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
	})

	t.Run("path traversal refused (unit)", func(t *testing.T) {
		// Direct unit check of the guard — http.ServeMux would clean most
		// "../" before routing, so exercise the method itself.
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/assets/x", nil)
		if srv.serveUserAsset(w, req, "../config.go") {
			t.Errorf("serveUserAsset should refuse traversal outside assets dir")
		}
	})

	t.Run("directory not served", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(assetsDir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/assets/sub", nil)
		if srv.serveUserAsset(w, req, "sub") {
			t.Errorf("serveUserAsset should not serve a directory")
		}
	})
}

// TestAssetContentType guards the host-independent MIME resolution. The
// fallback table is asserted directly so the test cannot false-pass on a host
// whose mime database already knows web fonts (e.g. a dev machine with
// /etc/mime.types) — that very host-dependence is what 404s fonts on a minimal
// alpine deploy.
func TestAssetContentType(t *testing.T) {
	t.Run("fallback table covers web fonts", func(t *testing.T) {
		want := map[string]string{
			".woff2": "font/woff2",
			".woff":  "font/woff",
			".ttf":   "font/ttf",
			".otf":   "font/otf",
			".eot":   "application/vnd.ms-fontobject",
		}
		for ext, ct := range want {
			if got := fallbackAssetTypes[ext]; got != ct {
				t.Errorf("fallbackAssetTypes[%q] = %q, want %q", ext, got, ct)
			}
		}
	})

	t.Run("resolves fonts and rejects unknown", func(t *testing.T) {
		// Host DB wins over the fallback: .css is in Go's built-in table on
		// every platform, so it resolves even where /etc/mime.types is absent.
		if got := assetContentType(".css"); got == "" {
			t.Errorf("assetContentType(.css) = empty, want a content type")
		}
		if got := assetContentType(".woff2"); got != "font/woff2" {
			t.Errorf("assetContentType(.woff2) = %q, want font/woff2", got)
		}
		// Case-insensitive, mirroring mime.TypeByExtension.
		if got := assetContentType(".WOFF2"); got != "font/woff2" {
			t.Errorf("assetContentType(.WOFF2) = %q, want font/woff2", got)
		}
		// Genuinely unknown extensions stay empty so serveUserAsset still refuses
		// them (nosniff defense against MIME confusion).
		if got := assetContentType(".totallybogus"); got != "" {
			t.Errorf("assetContentType(.totallybogus) = %q, want \"\"", got)
		}
	})
}
