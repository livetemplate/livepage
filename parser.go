package tinkerdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/livetemplate/tinkerdown/internal/schedule"
	"github.com/livetemplate/tinkerdown/internal/slug"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// SourceConfig represents a data source configuration for lvt-source blocks.
type SourceConfig struct {
	Type        string            `yaml:"type"`                   // exec, pg, rest, csv, json, markdown, sqlite, wasm
	Cmd         string            `yaml:"cmd,omitempty"`          // For exec type
	Query       string            `yaml:"query,omitempty"`        // For pg type
	From        string            `yaml:"from,omitempty"`         // For rest type: API endpoint URL
	File        string            `yaml:"file,omitempty"`         // For csv/json/markdown types
	Anchor      string            `yaml:"anchor,omitempty"`       // For markdown: section anchor (e.g., "#todos")
	DB          string            `yaml:"db,omitempty"`           // For sqlite: database file path
	Table       string            `yaml:"table,omitempty"`        // For sqlite: table name
	Path        string            `yaml:"path,omitempty"`         // For wasm: path to .wasm file
	QueryFile   string                 `yaml:"query_file,omitempty"`   // For graphql: path to .graphql file
	Variables   map[string]interface{} `yaml:"variables,omitempty"`    // For graphql: query variables
	Headers     map[string]string      `yaml:"headers,omitempty"`      // For rest: HTTP headers (env vars expanded)
	QueryParams map[string]string `yaml:"query_params,omitempty"` // For rest: URL query parameters
	ResultPath  string            `yaml:"result_path,omitempty"`  // For rest: dot-path to extract array (e.g., "data.items")
	Readonly    *bool             `yaml:"readonly,omitempty"`     // For markdown/sqlite: read-only mode (default: true)
	Options     map[string]string `yaml:"options,omitempty"`
	Manual      bool              `yaml:"manual,omitempty"`    // For exec: require Run button click
	Format      string            `yaml:"format,omitempty"`    // For exec: output format (json, lines, csv)
	Delimiter   string            `yaml:"delimiter,omitempty"` // For exec CSV: field delimiter (default ",")
	Env         map[string]string `yaml:"env,omitempty"`       // For exec: environment variables (env vars expanded)
	Timeout     string            `yaml:"timeout,omitempty"`   // For exec/rest: timeout (e.g., "30s", "1m")
	AutoBind    *bool             `yaml:"auto_bind,omitempty"` // Set to false to exclude from auto-table matching

	// For computed sources
	GroupBy   string            `yaml:"group_by,omitempty"`   // Field to group by
	Aggregate map[string]string `yaml:"aggregate,omitempty"`  // Field → aggregation expression
	Filter    string            `yaml:"filter,omitempty"`     // Optional filter expression
}

// StylingConfig represents styling/theme configuration.
type StylingConfig struct {
	Theme        string `yaml:"theme"`
	PrimaryColor string `yaml:"primary_color"`
	Font         string `yaml:"font"`
}

// BlocksConfig represents code block display configuration.
type BlocksConfig struct {
	AutoID          bool   `yaml:"auto_id"`
	IDFormat        string `yaml:"id_format"`
	ShowLineNumbers bool   `yaml:"show_line_numbers"`
}

// FeaturesConfig represents feature flags.
type FeaturesConfig struct {
	HotReload bool `yaml:"hot_reload"`
	Sidebar   bool `yaml:"sidebar"` // Show navigation sidebar
}

// Action defines a custom action that can be triggered via button name routing or lvt-on:click.
type Action struct {
	Kind      string              `yaml:"kind"`                // Action kind: "sql", "http", "exec"
	Source    string              `yaml:"source,omitempty"`    // For sql: source name to execute against
	Statement string              `yaml:"statement,omitempty"` // For sql: SQL statement with :param placeholders
	URL       string              `yaml:"url,omitempty"`       // For http: request URL (supports template expressions)
	Method    string              `yaml:"method,omitempty"`    // For http: HTTP method (default: POST)
	Body      string              `yaml:"body,omitempty"`      // For http: request body template
	Cmd       string              `yaml:"cmd,omitempty"`       // For exec: command to run
	Params    map[string]ParamDef `yaml:"params,omitempty"`    // Parameter definitions
	Confirm   string              `yaml:"confirm,omitempty"`   // Confirmation message (triggers dialog)
}

// ParamDef defines a parameter for an action.
type ParamDef struct {
	Type     string `yaml:"type,omitempty"`     // Parameter type: "string", "number", "date", "bool"
	Required bool   `yaml:"required,omitempty"` // Whether the parameter is required
	Default  string `yaml:"default,omitempty"`  // Default value
}

// Frontmatter represents the YAML frontmatter at the top of a markdown file.
type Frontmatter struct {
	// Page metadata
	Title       string      `yaml:"title"`
	Description string      `yaml:"description,omitempty"` // Used for <meta name="description"> + og:description
	Image       string      `yaml:"image,omitempty"`       // Path/URL used for og:image (falls back to site logo)
	Type        string      `yaml:"type"`                  // tutorial, guide, reference, playground
	Persist     PersistMode `yaml:"persist"`               // none, localstorage, server
	Steps       int         `yaml:"steps"`

	// Top-level convenience options
	Sidebar *bool `yaml:"sidebar,omitempty"` // Show navigation sidebar (overrides features.sidebar)

	// Source provenance — used to render an "Edit this page" link that
	// points at the canonical source file in its origin repo. Useful when
	// a page was synced from another repo and the docs site is not the
	// canonical home of the content. Both default to "" (use site-level
	// repository + the page's own relative path).
	SourceRepo string `yaml:"source_repo,omitempty"` // e.g. "https://github.com/livetemplate/livetemplate"
	SourcePath string `yaml:"source_path,omitempty"` // e.g. "docs/guides/progressive-complexity.md"

	// Chart customization (keyed by heading slug)
	Charts map[string]ChartOptions `yaml:"charts,omitempty"`

	// Config options (can override livemdtools.yaml)
	Sources  map[string]SourceConfig `yaml:"sources,omitempty"`
	Actions  map[string]Action       `yaml:"actions,omitempty"`
	Styling  *StylingConfig          `yaml:"styling,omitempty"`
	Blocks   *BlocksConfig           `yaml:"blocks,omitempty"`
	Features *FeaturesConfig         `yaml:"features,omitempty"`

	// Computed expressions found in the markdown content (populated during parsing)
	// Map of expression ID to expression string (e.g., "expr-1" -> "count(tasks where done)")
	Expressions map[string]string `yaml:"-"`

	// Schedule tokens found in the markdown content (populated during parsing)
	Schedules []*schedule.Token `yaml:"-"`

	// Imperative commands (Notify, Run action) found in the markdown (populated during parsing)
	Imperatives []*schedule.Imperative `yaml:"-"`

	// Schedule parsing warnings (populated during parsing)
	ScheduleWarnings []schedule.ParseWarning `yaml:"-"`

	// HasCharts indicates the page has {chart:...} annotations (populated during parsing)
	HasCharts bool `yaml:"-"`

	// HasMermaid indicates the page has ```mermaid fenced blocks (populated during parsing)
	HasMermaid bool `yaml:"-"`
}

// CodeBlock represents a code block extracted from markdown.
type CodeBlock struct {
	Type     string            // "server", "wasm", "lvt"
	Language string            // "go", etc.
	Flags    []string          // "readonly", "editable"
	Metadata map[string]string // id, state, etc.
	Content  string
	Line     int // Line number in source file
}

// ParseMarkdown parses a markdown file and extracts frontmatter and code blocks.
func ParseMarkdown(content []byte) (*Frontmatter, []*CodeBlock, string, error) {
	// Extract frontmatter
	frontmatter, remaining, err := extractFrontmatter(content)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Parse markdown with goldmark
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader(remaining)
	doc := md.Parser().Parse(reader)

	// Extract and collect livemdtools code blocks (but don't remove from AST)
	var codeBlocks []*CodeBlock
	blockMap := make(map[ast.Node]*CodeBlock) // Map AST nodes to CodeBlocks
	lineOffset := bytes.Count(content[:len(content)-len(remaining)], []byte("\n"))

	// Walk AST and identify livemdtools code blocks
	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if fenced, ok := n.(*ast.FencedCodeBlock); ok {
			block, parseErr := parseCodeBlock(fenced, remaining, lineOffset)
			if parseErr != nil {
				return ast.WalkStop, parseErr
			}
			if block != nil {
				// This is a livemdtools block - collect it and map it to AST node
				codeBlocks = append(codeBlocks, block)
				blockMap[n] = block
			} else if fenced.Info != nil {
				// Non-livemdtools fenced block — check for mermaid so the
				// server can conditionally load the heavy Mermaid runtime.
				info := strings.Fields(string(fenced.Info.Text(remaining)))
				if len(info) > 0 && info[0] == "mermaid" {
					frontmatter.HasMermaid = true
				}
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to walk AST: %w", err)
	}

	// Generate HTML (basic rendering first)
	var htmlBuf bytes.Buffer
	if err := md.Renderer().Render(&htmlBuf, remaining, doc); err != nil {
		return nil, nil, "", fmt.Errorf("failed to render HTML: %w", err)
	}

	// Post-process HTML to add data attributes to livemdtools blocks
	html := htmlBuf.String()
	html = injectBlockAttributes(html, codeBlocks, frontmatter.Sources)

	// Process status banners (> ✅ message)
	html = processStatusBanners(html)

	// Process computed expressions (`=expr` code spans)
	html, expressions := processExpressions(html)
	if len(expressions) > 0 {
		frontmatter.Expressions = expressions
	}

	// Process tabbed headings (## [Tab1] | [Tab2] filter)
	html = processTabbedHeadings(html)

	// Process chart headings (## Title {chart:bar} followed by table)
	html, hasCharts := processCharts(html, frontmatter.Charts)
	if hasCharts {
		frontmatter.HasCharts = true
	}

	// Parse schedule tokens and imperatives from markdown content
	schedules, scheduleWarnings := parseScheduleTokens(remaining)
	if len(schedules) > 0 {
		frontmatter.Schedules = schedules
	}
	if len(scheduleWarnings) > 0 {
		frontmatter.ScheduleWarnings = scheduleWarnings
	}

	// Extract imperative commands (Notify, Run action)
	imperatives := parseImperatives(remaining)
	if len(imperatives) > 0 {
		frontmatter.Imperatives = imperatives
	}

	return frontmatter, codeBlocks, html, nil
}

// extractFrontmatter extracts YAML frontmatter from the beginning of content.
// Returns the parsed frontmatter and the remaining content.
func extractFrontmatter(content []byte) (*Frontmatter, []byte, error) {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		// No frontmatter, use defaults
		return &Frontmatter{
			Type:    "tutorial",
			Persist: PersistLocalStorage,
		}, content, nil
	}

	// Find the closing ---
	endIdx := bytes.Index(content[4:], []byte("\n---\n"))
	if endIdx == -1 {
		return nil, nil, fmt.Errorf("unclosed frontmatter")
	}

	yamlContent := content[4 : 4+endIdx]
	remaining := content[4+endIdx+5:] // Skip "\n---\n"

	var fm Frontmatter
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set defaults
	if fm.Type == "" {
		fm.Type = "tutorial"
	}
	if fm.Persist == "" {
		fm.Persist = PersistLocalStorage
	}

	return &fm, remaining, nil
}

// injectBlockAttributes post-processes HTML to wrap livemdtools code blocks with data attributes.
func injectBlockAttributes(html string, blocks []*CodeBlock, sources map[string]SourceConfig) string {
	// For each livemdtools block, find its HTML representation and wrap it
	for i, block := range blocks {
		// Determine readonly/editable
		readonly := containsFlag(block.Flags, "readonly")
		editable := containsFlag(block.Flags, "editable")
		if !readonly && !editable {
			if block.Type == "server" {
				readonly = true
			} else if block.Type == "wasm" {
				editable = true
			}
		}

		// Get block ID (use same logic as page.go getBlockID)
		blockID := block.Metadata["id"]
		if blockID == "" {
			// Auto-generate: type-index (e.g., "server-0", "lvt-1")
			// Must match the index used in buildBlocks()
			blockID = fmt.Sprintf("%s-%d", block.Type, i)
		}

		// For interactive (lvt) blocks, replace with a container div instead of code block
		if block.Type == "lvt" {
			// Build container div with data attributes
			container := fmt.Sprintf(
				`<div class="tinkerdown-interactive-block" data-tinkerdown-block data-block-id="%s" data-block-type="lvt" data-language="lvt"`,
				escapeHTML(blockID),
			)

			if stateRef, ok := block.Metadata["state"]; ok {
				container += fmt.Sprintf(` data-state-ref="%s"`, escapeHTML(stateRef))
			}

			// Check if this block has an exec source and add toolbar attributes
			if sources != nil {
				sourceName := getLvtSourceFromContent(block.Content)
				if sourceName != "" {
					if srcCfg, ok := sources[sourceName]; ok && srcCfg.Type == "exec" {
						container += ` data-exec-source="true"`
						container += fmt.Sprintf(` data-exec-command="%s"`, escapeHTML(srcCfg.Cmd))
					}
				}
			}

			// Add a placeholder that will be replaced by WebSocket initial state
			container += ` data-interactive-content><div class="loading">Connecting...</div></div>`

			// Find and replace the <pre><code> block with our container
			oldPre := fmt.Sprintf(`<pre><code class="language-%s">`, block.Language)

			// Find the closing tags
			preStart := strings.Index(html, oldPre)
			if preStart != -1 {
				// Find the end of this code block
				codeEnd := strings.Index(html[preStart:], "</code></pre>")
				if codeEnd != -1 {
					// Replace the entire <pre><code>...</code></pre> with our container
					before := html[:preStart]
					after := html[preStart+codeEnd+len("</code></pre>"):]
					html = before + container + after
				}
			}
			continue
		}

		// For server/wasm blocks, wrap the existing <pre><code> with attributes
		wrapper := fmt.Sprintf(
			`<div data-tinkerdown-block data-block-id="%s" data-block-type="%s" data-language="%s"`,
			escapeHTML(blockID),
			escapeHTML(block.Type),
			escapeHTML(block.Language),
		)

		if readonly {
			wrapper += ` data-readonly="true"`
		}
		if editable {
			wrapper += ` data-editable="true"`
		}
		wrapper += ">"

		// Find <pre><code> blocks and wrap the first match
		oldPre := fmt.Sprintf(`<pre><code class="language-%s">`, block.Language)
		newPre := wrapper + oldPre

		// Only replace the first occurrence (to handle multiple blocks)
		html = strings.Replace(html, oldPre, newPre, 1)

		// Close the wrapper after </pre>
		html = strings.Replace(html, "</pre>", "</pre></div>", 1)
	}

	return html
}

// getLvtSourceFromContent extracts the lvt-source attribute value from block content.
// Returns empty string if not found.
func getLvtSourceFromContent(content string) string {
	// Look for lvt-source="name" on any element
	sourceRegex := regexp.MustCompile(`lvt-source="([^"]+)"`)
	match := sourceRegex.FindStringSubmatch(content)
	if match != nil && len(match) > 1 {
		return match[1]
	}
	return ""
}

// containsFlag checks if a flag is in the flags slice.
func containsFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	return html.EscapeString(s)
}

// exprCodePattern matches inline code spans: <code>...</code>
// We look for <code> tags containing content starting with =
var exprCodePattern = regexp.MustCompile(`<code>([^<]+)</code>`)

// escapedExprPattern matches literal escaped expressions: `\=...`
// These should NOT be treated as expressions
var escapedExprPattern = regexp.MustCompile(`<code>\\=([^<]+)</code>`)

// processExpressions scans HTML for expression code spans (`=expr`) and replaces
// them with span placeholders that will be evaluated at runtime.
// Returns the modified HTML and a map of expression ID to expression string.
func processExpressions(htmlStr string) (string, map[string]string) {
	expressions := make(map[string]string)
	exprCounter := 0

	// First, temporarily protect escaped expressions
	// Replace `\=something` with a placeholder
	escapedPlaceholders := make(map[string]string)
	htmlStr = escapedExprPattern.ReplaceAllStringFunc(htmlStr, func(match string) string {
		// Extract the content after \=
		submatch := escapedExprPattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		placeholder := fmt.Sprintf("__ESCAPED_EXPR_%d__", len(escapedPlaceholders))
		// Store the original content (without the backslash)
		escapedPlaceholders[placeholder] = fmt.Sprintf("<code>=%s</code>", submatch[1])
		return placeholder
	})

	// Now process actual expressions
	htmlStr = exprCodePattern.ReplaceAllStringFunc(htmlStr, func(match string) string {
		submatch := exprCodePattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}

		content := submatch[1]

		// Check if this is an expression (starts with =)
		if !strings.HasPrefix(content, "=") {
			return match // Not an expression, leave as-is
		}

		// Extract the expression (remove leading =)
		expr := strings.TrimPrefix(content, "=")
		if expr == "" {
			return match // Empty expression, leave as-is
		}

		// Generate unique ID
		id := fmt.Sprintf("expr-%d", exprCounter)
		exprCounter++

		// Store the expression
		expressions[id] = expr

		// Return a span placeholder that will be filled at runtime
		return fmt.Sprintf(
			`<span class="tinkerdown-expr" data-expr-id="%s" data-expr="%s"><span class="expr-loading">…</span></span>`,
			id,
			html.EscapeString(expr),
		)
	})

	// Restore escaped expressions (now shown as literal `=something`)
	for placeholder, original := range escapedPlaceholders {
		htmlStr = strings.ReplaceAll(htmlStr, placeholder, original)
	}

	return htmlStr, expressions
}

// StatusType represents the type of status banner
type StatusType string

const (
	StatusSuccess StatusType = "success"
	StatusWarning StatusType = "warning"
	StatusError   StatusType = "error"
	StatusInfo    StatusType = "info"
)

type statusEmojiEntry struct {
	emoji      string
	statusType StatusType
}

var statusEmojiOrder = []statusEmojiEntry{
	{"⚠️", StatusWarning},
	{"ℹ️", StatusInfo},
	{"✅", StatusSuccess},
	{"❌", StatusError},
	{"📊", StatusInfo},
	{"🟢", StatusSuccess},
	{"🟡", StatusWarning},
	{"🔴", StatusError},
	{"⚠", StatusWarning},
	{"ℹ", StatusInfo},
}

var statusRoleMap = map[StatusType]string{
	StatusSuccess: "status",
	StatusWarning: "alert",
	StatusError:   "alert",
	StatusInfo:    "status",
}

var blockquotePattern = regexp.MustCompile(`(?s)<blockquote>\s*<p>([^<]*(?:<[^>]+>[^<]*)*)</p>\s*</blockquote>`)

func processStatusBanners(htmlStr string) string {
	return blockquotePattern.ReplaceAllStringFunc(htmlStr, func(match string) string {
		submatch := blockquotePattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		content := submatch[1]
		for _, entry := range statusEmojiOrder {
			if strings.HasPrefix(content, entry.emoji) {
				msg := strings.TrimPrefix(content, entry.emoji)
				msg = strings.TrimLeft(msg, " ")
				role := statusRoleMap[entry.statusType]
				return fmt.Sprintf(
					`<div class="tinkerdown-status-banner tinkerdown-status-%s" role="%s"><span class="status-icon" aria-hidden="true">%s</span><span class="status-content">%s</span></div>`,
					entry.statusType, role, entry.emoji, msg,
				)
			}
		}
		return match
	})
}

// Tab represents a single tab definition parsed from a heading.
type Tab struct {
	Name   string // Display name of the tab
	Filter string // Filter expression (empty for "All" tab)
}

// tabbedHeadingPattern matches headings with tab syntax: ## [Tab1] | [Tab2] filter
// Captures: 1=tag name, 2=attributes, 3=heading content
// Note: Go's RE2 doesn't support backreferences, so we match any closing h1-h6 tag
var tabbedHeadingPattern = regexp.MustCompile(`<(h[1-6])([^>]*)>([^<]*\[[^\]]+\][^<]*)</h[1-6]>`)

// tabPattern matches individual tab definitions: [TabName] optional filter
var tabPattern = regexp.MustCompile(`\[([^\]]+)\](\s+[^|]*)?`)

// headingIDPattern extracts the id attribute from a heading element
var headingIDPattern = regexp.MustCompile(`id="([^"]+)"`)

// processTabbedHeadings transforms headings with tab syntax into interactive tab bars.
// Syntax: ## [All] | [Active] not done | [Done] done
// Each [Name] creates a tab button, and text after it becomes the filter expression.
func processTabbedHeadings(htmlStr string) string {
	tabsCounter := 0

	return tabbedHeadingPattern.ReplaceAllStringFunc(htmlStr, func(match string) string {
		submatch := tabbedHeadingPattern.FindStringSubmatch(match)
		if len(submatch) < 4 {
			return match
		}

		tagName := submatch[1]
		attrs := submatch[2]
		content := submatch[3]

		// Check if the content contains tab syntax (at least one [Name])
		if !strings.Contains(content, "[") || !strings.Contains(content, "]") {
			return match
		}

		// Parse tabs from the content
		tabs := parseTabsFromContent(content)
		if len(tabs) == 0 {
			return match // No valid tabs found
		}

		// Generate a unique ID for this tab group
		tabsID := fmt.Sprintf("tabs-%d", tabsCounter)
		tabsCounter++

		// Extract existing id from heading if present
		if idMatch := headingIDPattern.FindStringSubmatch(attrs); len(idMatch) > 1 {
			tabsID = idMatch[1] + "-tabs"
		}

		// Build the tab bar HTML
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf(`<div class="tinkerdown-tabs" data-tabs-id="%s">`, escapeHTML(tabsID)))
		builder.WriteString("\n")

		// Generate the tab bar (styled like the original heading)
		builder.WriteString(fmt.Sprintf(`<%s%s class="tinkerdown-tabs-heading">`, tagName, attrs))
		builder.WriteString(`<span class="tinkerdown-tabs-bar" role="tablist">`)

		for i, tab := range tabs {
			activeClass := ""
			ariaSelected := "false"
			tabIndex := "-1"
			if i == 0 {
				activeClass = " active"
				ariaSelected = "true"
				tabIndex = "0"
			}

			filter := escapeHTML(tab.Filter)
			tabID := fmt.Sprintf("%s-tab-%d", tabsID, i)
			panelID := fmt.Sprintf("%s-panel", tabsID)

			builder.WriteString(fmt.Sprintf(
				`<button type="button" role="tab" class="tinkerdown-tab%s" data-tab-index="%d" data-filter="%s" id="%s" aria-selected="%s" aria-controls="%s" tabindex="%s">%s</button>`,
				activeClass,
				i,
				filter,
				tabID,
				ariaSelected,
				panelID,
				tabIndex,
				escapeHTML(tab.Name),
			))
		}

		builder.WriteString(`</span>`)
		builder.WriteString(fmt.Sprintf(`</%s>`, tagName))
		builder.WriteString("\n")

		// Add a marker for the content wrapper (client-side will handle the wrapping)
		builder.WriteString(fmt.Sprintf(`<div class="tinkerdown-tabs-content" id="%s-panel" role="tabpanel" aria-labelledby="%s-tab-0" data-tabs-content>`,
			tabsID, tabsID))
		builder.WriteString("</div>")
		builder.WriteString("</div>")

		return builder.String()
	})
}

// parseTabsFromContent extracts tab definitions from heading content.
// Input: "[All] | [Active] not done | [Done] done"
// Returns: [{Name: "All", Filter: ""}, {Name: "Active", Filter: "not done"}, ...]
func parseTabsFromContent(content string) []Tab {
	var tabs []Tab

	// Split by pipe to get individual tab definitions
	parts := strings.Split(content, "|")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Match [TabName] optional filter
		matches := tabPattern.FindStringSubmatch(part)
		if len(matches) < 2 {
			continue
		}

		name := strings.TrimSpace(matches[1])
		filter := ""
		if len(matches) > 2 {
			filter = strings.TrimSpace(matches[2])
		}

		tabs = append(tabs, Tab{
			Name:   name,
			Filter: filter,
		})
	}

	return tabs
}

// htmlTagPattern strips HTML tags from text (used to clean heading text and table headers).
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// chartAnnotationPattern matches headings with {chart:type} or {chart} at the end.
// Goldmark renders: <h2 id="sales-by-region-chartbar">Sales by Region {chart:bar}</h2>
var chartAnnotationPattern = regexp.MustCompile(
	`(<h([1-6])\s+id="[^"]*"[^>]*>)(.*?)\s*\{chart(?::(\w+))?\}\s*(</h[1-6]>)`,
)

// chartTablePattern matches a GFM-rendered table immediately after a heading.
// Uses <table[^>]*> to tolerate attributes Goldmark may add via extensions.
var chartTablePattern = regexp.MustCompile(
	`(?s)<table[^>]*>\n<thead>\n<tr>\n((?:<th>.*?</th>\n)+)</tr>\n</thead>\n<tbody>\n((?:<tr>\n(?:<td>.*?</td>\n)+</tr>\n)+)</tbody>\n</table>\n`,
)

// Pre-compiled regexes for table parsing (avoid recompilation per call).
var (
	chartThPattern = regexp.MustCompile(`<th>(.*?)</th>`)
	chartTrPattern = regexp.MustCompile(`(?s)<tr>\n((?:<td>.*?</td>\n)+)</tr>`)
	chartTdPattern = regexp.MustCompile(`<td>(.*?)</td>`)
)

var validChartTypes = map[string]bool{
	"bar": true, "line": true, "pie": true, "doughnut": true, "auto": true, "": true,
}

// ChartOptions holds per-chart customization from frontmatter.
type ChartOptions struct {
	Colors     []string `yaml:"colors,omitempty" json:"colors,omitempty"`
	Stacked    bool     `yaml:"stacked,omitempty" json:"stacked,omitempty"`
	Horizontal bool     `yaml:"horizontal,omitempty" json:"horizontal,omitempty"`
	Legend     *bool    `yaml:"legend,omitempty" json:"legend,omitempty"`
}

// chartData is the JSON structure passed to Chart.js via data attributes.
type chartData struct {
	Labels   []string     `json:"labels"`
	Datasets []chartDataset `json:"datasets"`
}

type chartDataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
}

// processCharts detects {chart:type} headings followed by tables and transforms
// them into chart container elements with JSON data attributes for Chart.js.
// chartOpts provides per-chart customization from frontmatter (keyed by heading slug).
// Returns the modified HTML and whether any charts were found.
func processCharts(htmlStr string, chartOpts map[string]ChartOptions) (string, bool) {
	found := false

	// Find all chart-annotated headings
	matches := chartAnnotationPattern.FindAllStringSubmatchIndex(htmlStr, -1)
	if len(matches) == 0 {
		return htmlStr, false
	}

	// Process matches in reverse order to preserve indices
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		fullMatchStart := m[0]
		fullMatchEnd := m[1]

		// Extract heading parts
		headingText := htmlStr[m[6]:m[7]]   // "Sales by Region"
		chartType := ""
		if m[8] >= 0 {
			chartType = htmlStr[m[8]:m[9]]  // "bar"
		}
		headingClose := htmlStr[m[10]:m[11]] // </h2>

		// Validate chart type
		if !validChartTypes[chartType] {
			continue
		}

		// Look for a table immediately after this heading (with optional whitespace)
		remaining := htmlStr[fullMatchEnd:]
		trimmed := strings.TrimLeft(remaining, " \t\n\r")
		if !strings.HasPrefix(trimmed, "<table>") {
			// No table follows — just remove the annotation from the heading
			cleanTitle := strings.TrimSpace(html.UnescapeString(htmlTagPattern.ReplaceAllString(headingText, "")))
			cleanID := slug.Heading(cleanTitle)
			newHeading := fmt.Sprintf(`<h%s id="%s">%s%s`, htmlStr[m[4]:m[5]], cleanID, cleanTitle, headingClose)
			htmlStr = htmlStr[:fullMatchStart] + newHeading + htmlStr[fullMatchEnd:]
			continue
		}

		// Find the table
		tableMatch := chartTablePattern.FindStringIndex(trimmed)
		if tableMatch == nil {
			continue
		}

		tableHTML := trimmed[tableMatch[0]:tableMatch[1]]
		// Calculate the absolute position of the table end in the original string
		tableEndAbs := fullMatchEnd + (len(remaining) - len(trimmed)) + tableMatch[1]

		// Parse table headers
		headers := parseChartTableHeaders(tableHTML)
		if len(headers) < 2 {
			continue
		}

		// Parse table rows
		rows := parseChartTableRows(tableHTML)
		if len(rows) == 0 {
			continue
		}

		// Determine which columns are numeric
		numericCols := make([]bool, len(headers))
		for col := range headers {
			numericCols[col] = true
			for _, row := range rows {
				if col < len(row) {
					_, err := strconv.ParseFloat(strings.TrimSpace(row[col]), 64)
					if err != nil {
						numericCols[col] = false
						break
					}
				}
			}
		}

		// Find label column (first non-numeric) and data columns
		labelCol := -1
		var dataCols []int
		for col := range headers {
			if !numericCols[col] {
				if labelCol < 0 {
					labelCol = col
				}
			} else {
				dataCols = append(dataCols, col)
			}
		}

		if len(dataCols) == 0 {
			continue // No numeric columns, skip chart
		}
		if labelCol < 0 {
			labelCol = 0 // All numeric, use first column as labels
			dataCols = dataCols[1:]
			if len(dataCols) == 0 {
				continue
			}
		}

		// Build chart data
		labels := make([]string, len(rows))
		for r, row := range rows {
			if labelCol < len(row) {
				labels[r] = strings.TrimSpace(html.UnescapeString(row[labelCol]))
			}
		}

		datasets := make([]chartDataset, len(dataCols))
		for di, col := range dataCols {
			datasets[di].Label = headers[col]
			datasets[di].Data = make([]float64, len(rows))
			for r, row := range rows {
				if col < len(row) {
					val, _ := strconv.ParseFloat(strings.TrimSpace(row[col]), 64)
					datasets[di].Data[r] = val
				}
			}
		}

		// Auto-detect chart type
		if chartType == "" || chartType == "auto" {
			if len(dataCols) == 1 && len(labels) <= 8 {
				chartType = "pie"
			} else {
				chartType = "bar"
			}
		}

		// Build JSON
		cd := chartData{Labels: labels, Datasets: datasets}
		jsonBytes, err := json.Marshal(cd)
		if err != nil {
			continue
		}

		// Build replacement HTML — strip any inline HTML tags from heading text
		cleanTitle := strings.TrimSpace(html.UnescapeString(htmlTagPattern.ReplaceAllString(headingText, "")))
		cleanID := slug.Heading(cleanTitle)
		headingLevel := htmlStr[m[4]:m[5]]

		// Look up per-chart options from frontmatter
		optionsAttr := ""
		if opts, ok := chartOpts[cleanID]; ok {
			if optJSON, err := json.Marshal(opts); err == nil && string(optJSON) != "{}" {
				optionsAttr = fmt.Sprintf(` data-chart-options="%s"`, html.EscapeString(string(optJSON)))
			}
		}

		var buf strings.Builder
		fmt.Fprintf(&buf, "<h%s id=\"%s\">%s%s\n", headingLevel, cleanID, cleanTitle, headingClose)
		fmt.Fprintf(&buf, "<div class=\"tinkerdown-chart\" data-chart-type=\"%s\" data-chart-title=\"%s\" data-chart-data=\"%s\"%s>\n",
			chartType, html.EscapeString(cleanTitle), html.EscapeString(string(jsonBytes)), optionsAttr)
		buf.WriteString("  <canvas></canvas>\n")
		buf.WriteString("</div>\n")
		buf.WriteString("<details class=\"tinkerdown-chart-table\">\n")
		buf.WriteString("  <summary>View data</summary>\n")
		buf.WriteString("  ")
		buf.WriteString(tableHTML)
		buf.WriteString("</details>\n")

		htmlStr = htmlStr[:fullMatchStart] + buf.String() + htmlStr[tableEndAbs:]
		found = true
	}

	return htmlStr, found
}

// parseChartTableHeaders extracts column headers from a GFM table's <thead>.
// Strips HTML tags and unescapes entities to produce clean text labels.
func parseChartTableHeaders(tableHTML string) []string {
	matches := chartThPattern.FindAllStringSubmatch(tableHTML, -1)
	headers := make([]string, len(matches))
	for i, m := range matches {
		clean := htmlTagPattern.ReplaceAllString(m[1], "")
		headers[i] = strings.TrimSpace(html.UnescapeString(clean))
	}
	return headers
}

// parseChartTableRows extracts row data from a GFM table's <tbody>.
func parseChartTableRows(tableHTML string) [][]string {
	tbodyIdx := strings.Index(tableHTML, "<tbody>")
	if tbodyIdx < 0 {
		return nil
	}
	tbody := tableHTML[tbodyIdx:]

	trMatches := chartTrPattern.FindAllStringSubmatch(tbody, -1)
	rows := make([][]string, len(trMatches))
	for i, tr := range trMatches {
		tdMatches := chartTdPattern.FindAllStringSubmatch(tr[1], -1)
		row := make([]string, len(tdMatches))
		for j, td := range tdMatches {
			row[j] = td[1]
		}
		rows[i] = row
	}
	return rows
}

// parseCodeBlock parses a fenced code block and extracts livemdtools metadata.
// Code block info string format: "go server readonly id=counter"
func parseCodeBlock(fenced *ast.FencedCodeBlock, source []byte, lineOffset int) (*CodeBlock, error) {
	// Handle fenced code blocks without language info
	if fenced.Info == nil {
		return nil, nil
	}

	info := string(fenced.Info.Text(source))
	parts := strings.Fields(info)

	if len(parts) == 0 {
		// Not a livemdtools code block, skip
		return nil, nil
	}

	language := parts[0]
	remaining := parts[1:]

	// Check for livemdtools block types
	blockType := ""
	flags := []string{}
	metadata := make(map[string]string)

	// Special case: if language is "lvt", it's the block type
	if language == "lvt" {
		blockType = "lvt"
	}

	for _, part := range remaining {
		if strings.Contains(part, "=") {
			// Metadata: key=value
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				metadata[kv[0]] = strings.Trim(kv[1], `"'`)
			}
		} else {
			// Block type or flag
			switch part {
			case "server", "wasm", "lvt":
				blockType = part
			case "readonly", "editable":
				flags = append(flags, part)
			case "interactive":
				// Interactive is a special flag for lvt blocks
				flags = append(flags, part)
			default:
				// Unknown flag, ignore
			}
		}
	}

	if blockType == "" {
		// Not a livemdtools block (just regular code block)
		return nil, nil
	}

	// Extract content
	var buf bytes.Buffer
	lines := fenced.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}

	return &CodeBlock{
		Type:     blockType,
		Language: language,
		Flags:    flags,
		Metadata: metadata,
		Content:  buf.String(),
		Line:     lineOffset + fenced.Lines().At(0).Start,
	}, nil
}

// ParseFiles parses markdown files and updates the Page.
func (p *Page) ParseFiles(files ...string) error {
	// TODO: Implement multi-file parsing
	// For now, assume single file
	return fmt.Errorf("not implemented")
}

// partialRegex matches {{partial "filename"}} or {{partial "filename.md"}}
var partialRegex = regexp.MustCompile(`\{\{\s*partial\s+"([^"]+)"\s*\}\}`)

// ProcessPartials recursively processes {{partial "file.md"}} directives in content.
// baseDir is the directory to resolve relative paths from.
// seen tracks already-included files to prevent circular dependencies.
func ProcessPartials(content []byte, baseDir string, seen map[string]bool) ([]byte, error) {
	if seen == nil {
		seen = make(map[string]bool)
	}

	// Find all partial directives
	matches := partialRegex.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	// Process in reverse order to maintain correct positions
	result := make([]byte, len(content))
	copy(result, content)

	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		fullMatchStart := match[0]
		fullMatchEnd := match[1]
		filenameStart := match[2]
		filenameEnd := match[3]

		filename := string(content[filenameStart:filenameEnd])

		// Resolve path relative to baseDir
		partialPath := filepath.Join(baseDir, filename)
		absPath, err := filepath.Abs(partialPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve partial path '%s': %w", filename, err)
		}

		// Check for circular dependency
		if seen[absPath] {
			return nil, fmt.Errorf("circular dependency detected: %s", absPath)
		}
		seen[absPath] = true

		// Read the partial file
		partialContent, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read partial '%s': %w", filename, err)
		}

		// Strip frontmatter from partial (partials don't contribute frontmatter)
		_, partialBody, err := extractFrontmatter(partialContent)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partial '%s': %w", filename, err)
		}

		// Recursively process partials in the included content
		partialDir := filepath.Dir(absPath)
		processedPartial, err := ProcessPartials(partialBody, partialDir, seen)
		if err != nil {
			return nil, fmt.Errorf("error processing partial '%s': %w", filename, err)
		}

		// Replace the directive with the partial content
		result = append(result[:fullMatchStart], append(processedPartial, result[fullMatchEnd:]...)...)
	}

	return result, nil
}

// ParseMarkdownWithPartials parses markdown with partial file support.
// baseDir is used to resolve relative paths in {{partial "file.md"}} directives.
func ParseMarkdownWithPartials(content []byte, baseDir string) (*Frontmatter, []*CodeBlock, string, error) {
	// First, extract frontmatter before processing partials
	frontmatter, remaining, err := extractFrontmatter(content)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Process partials in the remaining content
	processed, err := ProcessPartials(remaining, baseDir, nil)
	if err != nil {
		return nil, nil, "", err
	}

	// Now parse the processed content (without frontmatter since we already extracted it)
	// We need to reconstruct the content for ParseMarkdown or parse directly here

	// Parse markdown with goldmark
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader(processed)
	doc := md.Parser().Parse(reader)

	// Extract and collect livemdtools code blocks
	var codeBlocks []*CodeBlock
	blockMap := make(map[ast.Node]*CodeBlock)
	lineOffset := bytes.Count(content[:len(content)-len(remaining)], []byte("\n"))

	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if fenced, ok := n.(*ast.FencedCodeBlock); ok {
			block, parseErr := parseCodeBlock(fenced, processed, lineOffset)
			if parseErr != nil {
				return ast.WalkStop, parseErr
			}
			if block != nil {
				codeBlocks = append(codeBlocks, block)
				blockMap[n] = block
			} else if fenced.Info != nil {
				info := strings.Fields(string(fenced.Info.Text(processed)))
				if len(info) > 0 && info[0] == "mermaid" {
					frontmatter.HasMermaid = true
				}
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to walk AST: %w", err)
	}

	// Generate HTML
	var htmlBuf bytes.Buffer
	if err := md.Renderer().Render(&htmlBuf, processed, doc); err != nil {
		return nil, nil, "", fmt.Errorf("failed to render HTML: %w", err)
	}

	// Post-process HTML to add data attributes
	htmlStr := htmlBuf.String()
	htmlStr = injectBlockAttributes(htmlStr, codeBlocks, frontmatter.Sources)

	// Process status banners (> ✅ message)
	htmlStr = processStatusBanners(htmlStr)

	// Process computed expressions (`=expr` code spans)
	htmlStr, expressions := processExpressions(htmlStr)
	if len(expressions) > 0 {
		frontmatter.Expressions = expressions
	}

	// Process tabbed headings (## [Tab1] | [Tab2] filter)
	htmlStr = processTabbedHeadings(htmlStr)

	// Process chart headings (## Title {chart:bar} followed by table)
	htmlStr, hasCharts := processCharts(htmlStr, frontmatter.Charts)
	if hasCharts {
		frontmatter.HasCharts = true
	}

	// Parse schedule tokens and imperatives from markdown content
	schedules, scheduleWarnings := parseScheduleTokens(processed)
	if len(schedules) > 0 {
		frontmatter.Schedules = schedules
	}
	if len(scheduleWarnings) > 0 {
		frontmatter.ScheduleWarnings = scheduleWarnings
	}

	// Extract imperative commands (Notify, Run action)
	imperatives := parseImperatives(processed)
	if len(imperatives) > 0 {
		frontmatter.Imperatives = imperatives
	}

	return frontmatter, codeBlocks, htmlStr, nil
}

// parseScheduleTokens extracts schedule tokens from markdown content.
// It uses the schedule package parser to find and parse tokens like @daily:9am, @weekly:mon, etc.
func parseScheduleTokens(content []byte) ([]*schedule.Token, []schedule.ParseWarning) {
	p := schedule.NewParser(time.Local)
	tokens, _ := p.ParseText(string(content))
	return tokens, p.Warnings
}

// parseImperatives extracts imperative commands (Notify, Run action) from markdown content.
func parseImperatives(content []byte) []*schedule.Imperative {
	return schedule.ExtractImperatives(string(content), time.Local)
}
