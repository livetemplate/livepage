package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
)

func TestServeRobotsAllowsAllByDefault(t *testing.T) {
	srv := New(t.TempDir())

	req := httptest.NewRequest("GET", "/robots.txt", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "User-agent: *") {
		t.Errorf("robots.txt missing User-agent line: %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Allow: /") {
		t.Errorf("robots.txt missing Allow: /: %q", w.Body.String())
	}
}

func TestServeRobotsIncludesSitemapWhenSiteURLConfigured(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Site = &config.SiteConfig{URL: "https://example.com"}
	srv := NewWithConfig(t.TempDir(), cfg)

	req := httptest.NewRequest("GET", "/robots.txt", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("missing Sitemap reference: %q", body)
	}
}

func TestServeRobotsTrimsTrailingSlash(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Site = &config.SiteConfig{URL: "https://example.com/"}
	srv := NewWithConfig(t.TempDir(), cfg)

	req := httptest.NewRequest("GET", "/robots.txt", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "https://example.com/sitemap.xml") {
		t.Errorf("trailing slash not trimmed: %q", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "https://example.com//sitemap.xml") {
		t.Errorf("double-slash leaked: %q", w.Body.String())
	}
}

func TestServeSitemapEnumeratesPagesInSiteMode(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "index.md"), []byte("---\ntitle: Home\n---\n# Home\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "about.md"), []byte("---\ntitle: About\n---\n# About\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "index.md", URL: "https://docs.example.com"}
	cfg.Navigation = []config.NavSection{
		{
			Title: "Top",
			Path:  "",
			Pages: []config.NavPage{
				{Title: "Home", Path: "index.md"},
				{Title: "About", Path: "about.md"},
			},
		},
	}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<urlset",
		"https://docs.example.com/about",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sitemap missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestServeSitemap404InHeadlessMode(t *testing.T) {
	srv := New(t.TempDir()) // No site manager
	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestBuildSEOTagsFallsBackToSiteDescription(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Title = "Docs"
	cfg.Description = "Site default description"
	cfg.Site = &config.SiteConfig{URL: "https://example.com"}
	srv := NewWithConfig(t.TempDir(), cfg)

	page := &tinkerdown.Page{Title: "Home"}
	tags := srv.buildSEOTags(page, "/")

	if !strings.Contains(tags, `name="description" content="Site default description"`) {
		t.Errorf("missing site description fallback: %s", tags)
	}
	if !strings.Contains(tags, `property="og:title" content="Home"`) {
		t.Errorf("missing og:title: %s", tags)
	}
	if !strings.Contains(tags, `property="og:url" content="https://example.com/"`) {
		t.Errorf("missing canonical og:url: %s", tags)
	}
	if !strings.Contains(tags, `property="og:site_name" content="Docs"`) {
		t.Errorf("missing og:site_name: %s", tags)
	}
}

func TestBuildSEOTagsPageOverridesSite(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Description = "Site default"
	cfg.Site = &config.SiteConfig{URL: "https://example.com"}
	srv := NewWithConfig(t.TempDir(), cfg)

	page := &tinkerdown.Page{
		Title:       "Recipe",
		Description: "Page-specific description",
		Image:       "/assets/recipe.png",
	}
	tags := srv.buildSEOTags(page, "/recipes/x")

	if !strings.Contains(tags, `content="Page-specific description"`) {
		t.Errorf("page description should win over site: %s", tags)
	}
	if !strings.Contains(tags, `og:image" content="https://example.com/assets/recipe.png"`) {
		t.Errorf("relative image should be made absolute via baseURL: %s", tags)
	}
	if !strings.Contains(tags, `twitter:card" content="summary_large_image"`) {
		t.Errorf("twitter card should upgrade to summary_large_image when image present: %s", tags)
	}
}

func TestBuildSEOTagsEscapesHTML(t *testing.T) {
	srv := New(t.TempDir())
	page := &tinkerdown.Page{
		Title:       `"<script>"`,
		Description: `evil & "html"`,
	}
	tags := srv.buildSEOTags(page, "/x")

	if strings.Contains(tags, "<script>") {
		t.Errorf("title injection not escaped: %s", tags)
	}
	if !strings.Contains(tags, "&amp;") || !strings.Contains(tags, "&#34;") {
		t.Errorf("description not properly HTML-escaped: %s", tags)
	}
}

func TestRenderedPageIncludesSEOTags(t *testing.T) {
	tmpDir := t.TempDir()

	pageContent := `---
title: "Hello"
description: "A short page"
---
# Hello
`
	if err := os.WriteFile(filepath.Join(tmpDir, "hello.md"), []byte(pageContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Type = "site"
	cfg.Site = &config.SiteConfig{Home: "hello.md", URL: "https://example.com"}
	cfg.Navigation = []config.NavSection{{
		Title: "Top",
		Pages: []config.NavPage{{Title: "Hello", Path: "hello.md"}},
	}}

	srv := NewWithConfig(tmpDir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	body := w.Body.String()
	for _, want := range []string{
		`<meta name="description" content="A short page">`,
		`<meta property="og:title" content="Hello">`,
		`<meta property="og:url" content="https://example.com/hello">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}
