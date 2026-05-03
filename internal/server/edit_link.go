package server

import (
	"strings"
)

// buildEditURL returns the GitHub "edit this file" URL for a page, or ""
// when no link can be constructed. Resolution order:
//
//  1. Page frontmatter `source_repo` + `source_path` (canonical when the
//     page was synced from another repo and that repo owns the content).
//  2. Site-level `repository` + the page's own relative file path within
//     the docs site (canonical when the docs site owns the content).
//
// The branch defaults to "main"; configurable per-site work is deferred.
func buildEditURL(siteRepo, sourceRepo, sourcePath, sitePagePath string) string {
	repo := strings.TrimSuffix(strings.TrimSpace(sourceRepo), "/")
	path := strings.TrimPrefix(strings.TrimSpace(sourcePath), "/")
	if repo == "" || path == "" {
		// Fall back to site-level repo + the page's relative path.
		repo = strings.TrimSuffix(strings.TrimSpace(siteRepo), "/")
		path = strings.TrimPrefix(strings.TrimSpace(sitePagePath), "/")
	}
	if repo == "" || path == "" {
		return ""
	}
	if !strings.HasPrefix(repo, "https://github.com/") {
		// Only GitHub URLs are supported in Phase 1. Other forges will need
		// per-host URL templates; keep them out for now to avoid a broken link.
		return ""
	}
	return repo + "/edit/main/" + path
}
