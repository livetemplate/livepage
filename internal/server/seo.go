// SEO endpoints: sitemap.xml and robots.txt.
//
// These are generic features available to any tinkerdown site. They derive
// content from the site's discovered pages (via siteManager) and the
// canonical URL declared in `site.url` of tinkerdown.yaml. When `site.url`
// is unset, sitemap entries fall back to relative paths and robots.txt
// omits its Sitemap reference.

package server

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/livetemplate/tinkerdown"
)

// buildSEOTags returns the HTML <meta> tags injected into the page <head>.
// It emits a description, OpenGraph properties, and Twitter card tags using
// frontmatter overrides where present and falling back to site-level config.
// Returns an empty string when there's nothing useful to emit.
func (s *Server) buildSEOTags(page *tinkerdown.Page, currentPath string) string {
	if page == nil {
		return ""
	}

	// Resolve description: page > site.
	description := page.Description
	if description == "" && s.config.Description != "" {
		description = s.config.Description
	}

	// Resolve canonical absolute URL when site.url is configured.
	baseURL := ""
	siteName := s.config.Title
	siteLogo := ""
	if s.config.Site != nil {
		baseURL = strings.TrimRight(s.config.Site.URL, "/")
		siteLogo = s.config.Site.Logo
	}
	canonicalURL := ""
	if baseURL != "" {
		canonicalURL = baseURL + currentPath
	}

	// Resolve image: page > site logo. If image is a relative path and we
	// have a baseURL, prefix it so social platforms can resolve it.
	image := page.Image
	if image == "" {
		image = siteLogo
	}
	if image != "" && !strings.HasPrefix(image, "http://") && !strings.HasPrefix(image, "https://") {
		if baseURL != "" {
			if !strings.HasPrefix(image, "/") {
				image = "/" + image
			}
			image = baseURL + image
		}
	}

	var b strings.Builder
	b.WriteString("\n")

	if description != "" {
		fmt.Fprintf(&b, `    <meta name="description" content="%s">`+"\n", html.EscapeString(description))
	}
	if canonicalURL != "" {
		fmt.Fprintf(&b, `    <link rel="canonical" href="%s">`+"\n", html.EscapeString(canonicalURL))
	}

	// OpenGraph tags
	fmt.Fprintf(&b, `    <meta property="og:type" content="website">`+"\n")
	fmt.Fprintf(&b, `    <meta property="og:title" content="%s">`+"\n", html.EscapeString(page.Title))
	if description != "" {
		fmt.Fprintf(&b, `    <meta property="og:description" content="%s">`+"\n", html.EscapeString(description))
	}
	if canonicalURL != "" {
		fmt.Fprintf(&b, `    <meta property="og:url" content="%s">`+"\n", html.EscapeString(canonicalURL))
	}
	if siteName != "" {
		fmt.Fprintf(&b, `    <meta property="og:site_name" content="%s">`+"\n", html.EscapeString(siteName))
	}
	if image != "" {
		fmt.Fprintf(&b, `    <meta property="og:image" content="%s">`+"\n", html.EscapeString(image))
	}

	// Twitter card — summary_large_image when an image is present, else summary.
	twitterCard := "summary"
	if image != "" {
		twitterCard = "summary_large_image"
	}
	fmt.Fprintf(&b, `    <meta name="twitter:card" content="%s">`+"\n", twitterCard)
	fmt.Fprintf(&b, `    <meta name="twitter:title" content="%s">`+"\n", html.EscapeString(page.Title))
	if description != "" {
		fmt.Fprintf(&b, `    <meta name="twitter:description" content="%s">`+"\n", html.EscapeString(description))
	}
	if image != "" {
		fmt.Fprintf(&b, `    <meta name="twitter:image" content="%s">`+"\n", html.EscapeString(image))
	}

	return b.String()
}

// urlEntry represents a single <url> in a sitemap.
type urlEntry struct {
	XMLName xml.Name `xml:"url"`
	Loc     string   `xml:"loc"`
}

// urlSet is the root <urlset> element.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	Xmlns   string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// serveSitemap returns a sitemap.xml derived from discovered pages.
// In headless mode (no site manager) this returns 404 since there are no
// pages to enumerate.
func (s *Server) serveSitemap(w http.ResponseWriter, _ *http.Request) {
	if s.siteManager == nil {
		http.Error(w, "sitemap unavailable in headless mode", http.StatusNotFound)
		return
	}

	baseURL := ""
	if s.config.Site != nil {
		baseURL = strings.TrimRight(s.config.Site.URL, "/")
	}

	pages := s.siteManager.AllPages()
	entries := make([]urlEntry, 0, len(pages))
	for _, p := range pages {
		if p == nil || p.Path == "" {
			continue
		}
		loc := p.Path
		if baseURL != "" {
			loc = baseURL + p.Path
		}
		entries = append(entries, urlEntry{Loc: loc})
	}

	doc := urlSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		http.Error(w, "failed to encode sitemap", http.StatusInternalServerError)
		return
	}
}

// serveRobots returns a robots.txt that allows all by default and references
// the sitemap when site.url is configured.
func (s *Server) serveRobots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	body := "User-agent: *\nAllow: /\n"
	if s.config.Site != nil {
		base := strings.TrimRight(s.config.Site.URL, "/")
		if base != "" {
			body += fmt.Sprintf("\nSitemap: %s/sitemap.xml\n", base)
		}
	}

	if _, err := w.Write([]byte(body)); err != nil {
		return
	}
}
