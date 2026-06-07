package site

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
)

// PageNode represents a page in the site structure
type PageNode struct {
	Title     string           // Page title from frontmatter or config
	Path      string           // URL path (e.g., "/getting-started/installation")
	FilePath  string           // Relative file path (e.g., "getting-started/installation.md")
	Page      *tinkerdown.Page // Parsed page content
	IsHome    bool             // Whether this is the home page
	Collapsed bool             // For sections/groups: whether collapsed by default
	Children  []*PageNode      // Child pages (for sections and groups)
}

// ContainsPath reports whether this node or any descendant has the given URL
// path. Used to keep a collapsed section/group open when it holds the active
// page, and to resolve a nested page's section in the search index.
func (n *PageNode) ContainsPath(urlPath string) bool {
	if n.Path == urlPath {
		return true
	}
	for _, child := range n.Children {
		if child.ContainsPath(urlPath) {
			return true
		}
	}
	return false
}

// Manager handles multi-page site discovery and navigation
type Manager struct {
	rootDir string
	config  *config.Config
	pages   map[string]*PageNode // Maps URL path to PageNode
	nav     []*PageNode          // Navigation tree (top-level nodes)
	home    *PageNode            // Home page
}

// New creates a new site manager
func New(rootDir string, cfg *config.Config) *Manager {
	return &Manager{
		rootDir: rootDir,
		config:  cfg,
		pages:   make(map[string]*PageNode),
		nav:     make([]*PageNode, 0),
	}
}

// Discover scans the directory and builds the site structure
func (m *Manager) Discover() error {
	// If config has explicit navigation structure, use it
	if len(m.config.Navigation) > 0 {
		return m.discoverFromConfig()
	}

	// Otherwise, auto-discover from directory structure
	return m.discoverFromFiles()
}

// discoverFromConfig builds the site structure from the config navigation
func (m *Manager) discoverFromConfig() error {
	for _, section := range m.config.Navigation {
		sectionNode := &PageNode{
			Title:     section.Title,
			Path:      "/" + section.Path,
			Collapsed: section.Collapsed,
			Children:  make([]*PageNode, 0),
		}

		// Process pages in this section (each may itself be a group of pages)
		for _, page := range section.Pages {
			child, err := m.buildNavPageNode(page)
			if err != nil {
				return err
			}
			sectionNode.Children = append(sectionNode.Children, child)
		}

		m.nav = append(m.nav, sectionNode)
	}

	// If home page wasn't found in navigation, look for it
	if m.home == nil && m.config.Site != nil && m.config.Site.Home != "" {
		homePath := m.config.Site.Home
		absPath := filepath.Join(m.rootDir, homePath)

		parsed, err := tinkerdown.ParseFileInSite(absPath, m.rootDir)
		if err != nil {
			return fmt.Errorf("failed to parse home page %s: %w", homePath, err)
		}

		urlPath := mdToURLPath(homePath)
		m.home = &PageNode{
			Title:    m.config.Title,
			Path:     urlPath,
			FilePath: homePath,
			Page:     parsed,
			IsHome:   true,
		}
		m.pages[urlPath] = m.home
	}

	return nil
}

// buildNavPageNode builds a PageNode for one nav entry, recursing into any
// sub-pages. An entry with a Path is parsed and registered in m.pages so the
// site server will serve it — this registration must reach every leaf at any
// depth, or a nested page silently 404s. An entry with Pages becomes a group
// whose Children are built recursively (a group may also carry its own Path,
// acting as a landing page for the group).
func (m *Manager) buildNavPageNode(page config.NavPage) (*PageNode, error) {
	// A nav entry with neither a path nor sub-pages is meaningless and would
	// otherwise vanish silently from the sidebar — fail fast on the typo.
	if page.Path == "" && len(page.Pages) == 0 {
		return nil, fmt.Errorf("nav entry %q has neither a path nor pages", page.Title)
	}

	node := &PageNode{
		Title:     page.Title,
		Collapsed: page.Collapsed,
	}

	if page.Path != "" {
		filePath := page.Path
		if !strings.HasSuffix(filePath, ".md") {
			filePath += ".md"
		}

		absPath := filepath.Join(m.rootDir, filePath)
		parsed, err := tinkerdown.ParseFileInSite(absPath, m.rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
		}

		node.Path = mdToURLPath(filePath)
		node.FilePath = filePath
		node.Page = parsed

		if m.config.Site != nil && filePath == m.config.Site.Home {
			node.IsHome = true
			m.home = node
		}

		m.pages[node.Path] = node
	}

	for _, sub := range page.Pages {
		child, err := m.buildNavPageNode(sub)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, child)
	}

	return node, nil
}

// discoverFromFiles auto-discovers pages from directory structure
func (m *Manager) discoverFromFiles() error {
	err := filepath.WalkDir(m.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			name := d.Name()
			// Skip hidden directories (starting with _ or .)
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			// Skip common non-documentation directories
			skipDirs := []string{"node_modules", "vendor", "dist", "build", "target"}
			for _, skip := range skipDirs {
				if name == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only process .md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(m.rootDir, path)
		if err != nil {
			return err
		}

		// Skip files in _ directories or starting with _
		if strings.Contains(relPath, "/_") || strings.HasPrefix(relPath, "_") {
			return nil
		}

		// Parse the page
		parsed, err := tinkerdown.ParseFileInSite(path, m.rootDir)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", relPath, err)
		}

		// Generate URL path
		urlPath := mdToURLPath(relPath)

		// Determine title (from frontmatter or filename)
		title := parsed.Title
		if title == "" {
			// Use filename as title
			title = strings.TrimSuffix(filepath.Base(relPath), ".md")
			title = strings.ReplaceAll(title, "-", " ")
			title = strings.ReplaceAll(title, "_", " ")
			// Capitalize first letter
			if len(title) > 0 {
				title = strings.ToUpper(title[:1]) + title[1:]
			}
		}

		pageNode := &PageNode{
			Title:    title,
			Path:     urlPath,
			FilePath: relPath,
			Page:     parsed,
		}

		// Check if this is the home page
		if relPath == "index.md" || (m.config.Site != nil && relPath == m.config.Site.Home) {
			pageNode.IsHome = true
			m.home = pageNode
		}

		m.pages[urlPath] = pageNode

		return nil
	})

	if err != nil {
		return err
	}

	// Build navigation tree from flat pages
	m.buildNavigationTree()

	return nil
}

// buildNavigationTree organizes flat pages into a hierarchical navigation tree
func (m *Manager) buildNavigationTree() {
	// Create a map of directory -> pages
	sections := make(map[string]*PageNode)
	topLevel := make([]*PageNode, 0)

	for _, page := range m.pages {
		// Skip home page
		if page.IsHome {
			continue
		}

		// Get directory path
		dir := filepath.Dir(page.FilePath)
		if dir == "." {
			// Top-level page
			topLevel = append(topLevel, page)
		} else {
			// Page in subdirectory
			section, exists := sections[dir]
			if !exists {
				// Create section node
				sectionTitle := filepath.Base(dir)
				sectionTitle = strings.ReplaceAll(sectionTitle, "-", " ")
				sectionTitle = strings.ReplaceAll(sectionTitle, "_", " ")
				if len(sectionTitle) > 0 {
					sectionTitle = strings.ToUpper(sectionTitle[:1]) + sectionTitle[1:]
				}

				section = &PageNode{
					Title:    sectionTitle,
					Path:     "/" + filepath.ToSlash(dir),
					Children: make([]*PageNode, 0),
				}
				sections[dir] = section
			}

			section.Children = append(section.Children, page)
		}
	}

	// Add sections to nav
	for _, section := range sections {
		m.nav = append(m.nav, section)
	}

	// Add top-level pages to nav
	m.nav = append(m.nav, topLevel...)
}

// GetPage returns a page by its URL path
func (m *Manager) GetPage(urlPath string) (*PageNode, bool) {
	page, exists := m.pages[urlPath]
	return page, exists
}

// GetHome returns the home page
func (m *Manager) GetHome() *PageNode {
	return m.home
}

// GetNavigation returns the navigation tree
func (m *Manager) GetNavigation() []*PageNode {
	return m.nav
}

// AllPages returns all pages (flat list)
func (m *Manager) AllPages() []*PageNode {
	pages := make([]*PageNode, 0, len(m.pages))
	for _, page := range m.pages {
		pages = append(pages, page)
	}
	return pages
}

// GetPrevNext returns the previous and next pages for navigation
func (m *Manager) GetPrevNext(currentPath string) (prev, next *PageNode) {
	// Build a flat ordered list from navigation tree
	ordered := m.flattenNav(m.nav)

	// Find current page index
	currentIdx := -1
	for i, page := range ordered {
		if page.Path == currentPath {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return nil, nil
	}

	if currentIdx > 0 {
		prev = ordered[currentIdx-1]
	}
	if currentIdx < len(ordered)-1 {
		next = ordered[currentIdx+1]
	}

	return prev, next
}

// flattenNav converts navigation tree to flat ordered list
func (m *Manager) flattenNav(nodes []*PageNode) []*PageNode {
	result := make([]*PageNode, 0)
	for _, node := range nodes {
		if node.Page != nil {
			// This is an actual page, not just a section
			result = append(result, node)
		}
		if len(node.Children) > 0 {
			result = append(result, m.flattenNav(node.Children)...)
		}
	}
	return result
}

// GetBreadcrumbs returns the breadcrumb trail for a given path
func (m *Manager) GetBreadcrumbs(urlPath string) []*PageNode {
	breadcrumbs := make([]*PageNode, 0)

	// Always start with home if it exists
	if m.home != nil {
		breadcrumbs = append(breadcrumbs, m.home)
	}

	// If this is the home page, we're done
	if urlPath == "/" || urlPath == m.home.Path {
		return breadcrumbs
	}

	// Build path segments
	segments := strings.Split(strings.Trim(urlPath, "/"), "/")
	currentPath := ""

	for i, segment := range segments {
		if i < len(segments)-1 {
			// This is a section
			currentPath += "/" + segment
			// Try to find a section node
			for _, navNode := range m.nav {
				if navNode.Path == currentPath {
					breadcrumbs = append(breadcrumbs, navNode)
					break
				}
			}
		} else {
			// This is the final page
			if page, exists := m.pages[urlPath]; exists {
				breadcrumbs = append(breadcrumbs, page)
			}
		}
	}

	return breadcrumbs
}

// Reload reloads a specific file (for hot reload)
func (m *Manager) Reload(filePath string) error {
	// Get relative path
	relPath, err := filepath.Rel(m.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// Generate URL path
	urlPath := mdToURLPath(relPath)

	// Check if this page exists
	pageNode, exists := m.pages[urlPath]
	if !exists {
		// New file - trigger full rediscovery
		return m.Discover()
	}

	// Re-parse the file
	parsed, err := tinkerdown.ParseFileInSite(filePath, m.rootDir)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", relPath, err)
	}

	// Update the page node
	pageNode.Page = parsed
	if parsed.Title != "" {
		pageNode.Title = parsed.Title
	}

	return nil
}

// SearchEntry represents a single entry in the search index
type SearchEntry struct {
	Title   string `json:"title"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Section string `json:"section,omitempty"`
}

// GenerateSearchIndex creates a search index from all pages
func (m *Manager) GenerateSearchIndex() []SearchEntry {
	entries := make([]SearchEntry, 0)

	for _, page := range m.pages {
		if page.Page == nil {
			continue
		}

		// Extract text content from the page
		content := extractTextContent(page.Page)

		// Find the top-level section this page belongs to, searching the
		// whole subtree so nested (grouped) pages still resolve their section.
		section := ""
		for _, navNode := range m.nav {
			if navNode.ContainsPath(page.Path) {
				section = navNode.Title
				break
			}
		}

		entries = append(entries, SearchEntry{
			Title:   page.Title,
			Path:    page.Path,
			Content: content,
			Section: section,
		})
	}

	return entries
}

// extractTextContent extracts plain text from a page for search indexing
func extractTextContent(page *tinkerdown.Page) string {
	var content strings.Builder

	// Add page title
	if page.Title != "" {
		content.WriteString(page.Title)
		content.WriteString(" ")
	}

	// Extract text from HTML content
	// Simple approach: strip HTML tags
	htmlContent := page.StaticHTML

	// Remove script and style tags completely
	htmlContent = removeTagContent(htmlContent, "script")
	htmlContent = removeTagContent(htmlContent, "style")

	// Remove code blocks (pre tags - usually not useful for search)
	htmlContent = removeTagContent(htmlContent, "pre")

	// Remove all HTML tags but keep the text
	htmlContent = stripHTMLTags(htmlContent)

	// Remove extra whitespace
	lines := strings.Split(htmlContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			content.WriteString(trimmed)
			content.WriteString(" ")
		}
	}

	// Limit content length for search index (first ~500 chars)
	result := content.String()
	if len(result) > 500 {
		result = result[:500]
	}

	return result
}

// removeTagContent removes a tag and its content
func removeTagContent(html, tag string) string {
	// Simple regex-like replacement for removing tags and their content
	// Not using regex to avoid dependency
	result := html
	for {
		start := strings.Index(result, "<"+tag)
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "</"+tag+">")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+len("</"+tag+">"):]
	}
	return result
}

// stripHTMLTags removes all HTML tags but keeps text content
func stripHTMLTags(html string) string {
	var result strings.Builder
	inTag := false

	for i := 0; i < len(html); i++ {
		if html[i] == '<' {
			inTag = true
		} else if html[i] == '>' {
			inTag = false
		} else if !inTag {
			result.WriteByte(html[i])
		}
	}

	return result.String()
}

// mdToURLPath converts a markdown file path to a URL path
// Examples:
//   - "index.md" → "/"
//   - "getting-started.md" → "/getting-started"
//   - "guides/intro.md" → "/guides/intro"
//   - "guides/index.md" → "/guides/"
func mdToURLPath(relPath string) string {
	// Remove .md extension
	path := strings.TrimSuffix(relPath, ".md")

	// Convert to URL path
	path = filepath.ToSlash(path)

	// Handle index files
	if path == "index" {
		return "/"
	}
	if strings.HasSuffix(path, "/index") {
		return "/" + strings.TrimSuffix(path, "index")
	}

	// Add leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return path
}
