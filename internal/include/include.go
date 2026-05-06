// Package include resolves and slices files referenced by the
// `include="..."` fence-block attribute in tinkerdown markdown.
//
// The package is intentionally small: it does not concern itself with
// HTML rendering, syntax highlighting, or the markdown AST — those
// stay in parser.go. Three helpers do the work:
//
//   - Resolve confines a relative include path to a discovery root,
//     so authors can't escape with "../../etc/passwd".
//   - Slice reads the resolved file and returns either the whole text
//     or a 1-indexed inclusive line range, clamping the end to the
//     file's actual length when the author's range overruns.
//   - Dedent strips the common leading whitespace across non-blank
//     lines, so a snippet pulled from inside a function body renders
//     without stray indentation.
package include

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// fenceOpenerRe matches a code-block fence opener with at least three
// backticks, capturing fence chars, language, and the rest of the
// info string. Only standard backtick fences are recognized in v1
// (tilde fences exist in CommonMark but tinkerdown's own examples
// universally use backticks).
var fenceOpenerRe = regexp.MustCompile("^(`{3,})([A-Za-z0-9_+-]*)(.*)$")

// includeAttrRe pulls the `include="..."` value out of a fence info
// suffix; tolerant of single quotes too.
var includeAttrRe = regexp.MustCompile(`include=("[^"]*"|'[^']*')`)

// linesAttrRe pulls the `lines="..."` value out of a fence info suffix.
var linesAttrRe = regexp.MustCompile(`lines=("[^"]*"|'[^']*')`)

// regionAttrRe pulls the `region="..."` value (named-region marker
// extraction) out of a fence info suffix.
var regionAttrRe = regexp.MustCompile(`region=("[^"]*"|'[^']*')`)

// highlightAttrRe pulls the `highlight="..."` value (Prism-style
// `data-line` ranges, e.g. "3-5" or "3,5,7-9") out of a fence info
// suffix.
var highlightAttrRe = regexp.MustCompile(`highlight=("[^"]*"|'[^']*')`)

// LinkOptions configures the optional "View on GitHub →" footer that
// Preprocess appends to each substituted block. When RepoURL is empty
// or any of the path-derivation inputs are missing, the footer is
// omitted gracefully. Branch defaults to "main" when unset.
//
// Example for examples/literate-counter-include/index.md including
// ./_app/counter.go: with RepoURL="https://github.com/foo/bar",
// Branch="main", PagePathInRepo="examples/literate-counter-include",
// the footer href becomes
//   https://github.com/foo/bar/blob/main/examples/literate-counter-include/_app/counter.go#L13-L35
type LinkOptions struct {
	RepoURL        string
	Branch         string
	PagePathInRepo string // forward-slash-separated, no trailing slash; e.g. "docs/guides"
}

// Preprocess scans content for fenced code blocks whose fence info
// contains `include="..."`, replaces the (empty) fence body with the
// resolved file slice, and returns the transformed content plus the
// absolute paths of every included file (for the watcher to track)
// plus any warnings (bad path, missing file, range error). Bad
// includes pass through with the original empty body so the page
// still renders — same posture as the embed-lvt unavailable badge.
func Preprocess(content []byte, baseDir, root string) ([]byte, []string, []string) {
	return PreprocessWithLinks(content, baseDir, root, LinkOptions{})
}

// PreprocessWithLinks behaves like Preprocess but also appends a
// "View on GitHub" footer link below each substituted block when
// link options are provided.
func PreprocessWithLinks(content []byte, baseDir, root string, link LinkOptions) ([]byte, []string, []string) {
	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines))
	included := []string{}
	warnings := []string{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		match := fenceOpenerRe.FindStringSubmatch(line)
		if match == nil {
			out = append(out, line)
			continue
		}
		fence, lang, rest := match[1], match[2], match[3]
		incMatch := includeAttrRe.FindStringSubmatch(rest)
		if incMatch == nil {
			// Not an include block. Copy this opener AND its body
			// verbatim, then jump past the matching closer — so any
			// nested `LANG include="..."` fences inside (e.g. an
			// authoring example showing the syntax) are NOT
			// reprocessed. Without this, a docs page demonstrating
			// `include=` could accidentally trigger substitution.
			out = append(out, line)
			closer := -1
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == fence {
					closer = j
					break
				}
			}
			if closer == -1 {
				continue
			}
			for k := i + 1; k <= closer; k++ {
				out = append(out, lines[k])
			}
			i = closer
			continue
		}
		incPath := strings.Trim(incMatch[1], `"'`)
		linesAttr := ""
		if l := linesAttrRe.FindStringSubmatch(rest); l != nil {
			linesAttr = strings.Trim(l[1], `"'`)
		}
		regionAttr := ""
		if r := regionAttrRe.FindStringSubmatch(rest); r != nil {
			regionAttr = strings.Trim(r[1], `"'`)
		}
		highlightAttr := ""
		if h := highlightAttrRe.FindStringSubmatch(rest); h != nil {
			highlightAttr = strings.Trim(h[1], `"'`)
		}

		// Find the matching closing fence — same backtick count, on
		// its own line. Bodies cannot contain a closer of the same
		// length (CommonMark rule), so a forward scan is sufficient.
		closer := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == fence {
				closer = j
				break
			}
		}
		if closer == -1 {
			// Unclosed fence — let goldmark handle the diagnostic.
			out = append(out, line)
			continue
		}

		// Resolve, slice, dedent. On any failure, pass the original
		// (empty) block through with a warning so the page still
		// renders.
		absPath, err := Resolve(baseDir, root, incPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("include %q: %v", incPath, err))
			for k := i; k <= closer; k++ {
				out = append(out, lines[k])
			}
			i = closer
			continue
		}

		// region= and lines= are mutually exclusive — one names an
		// abstract span via markers in the source, the other gives
		// raw line numbers. If both set, region= wins and lines= is
		// dropped with a warning.
		if regionAttr != "" {
			if linesAttr != "" {
				warnings = append(warnings, fmt.Sprintf("include %q: ignoring lines=%q because region=%q is set", incPath, linesAttr, regionAttr))
			}
			rs, re, rerr := FindRegion(absPath, regionAttr)
			if rerr != nil {
				warnings = append(warnings, fmt.Sprintf("include %q: %v", incPath, rerr))
				for k := i; k <= closer; k++ {
					out = append(out, lines[k])
				}
				i = closer
				continue
			}
			linesAttr = fmt.Sprintf("%d-%d", rs, re)
		}

		text, err := SliceRanges(absPath, linesAttr, LanguageEllipsis(lang))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("include %q: %v", incPath, err))
			for k := i; k <= closer; k++ {
				out = append(out, lines[k])
			}
			i = closer
			continue
		}
		text = Dedent(text)
		// Trim a single trailing newline so the rendered code block
		// doesn't end with a blank line. Multi-line trailing blanks
		// from the source are preserved.
		text = strings.TrimSuffix(text, "\n")

		// Emit a fresh fence with the same fence/lang and the body
		// replaced. We deliberately drop include="..." and lines="..."
		// from the rendered fence info so the post-render code block
		// looks identical to a hand-authored one — clean Prism class,
		// clean DOM, no leaked authoring metadata.
		cleanInfo := rest
		cleanInfo = strings.ReplaceAll(cleanInfo, incMatch[0], "")
		if l := linesAttrRe.FindString(rest); l != "" {
			cleanInfo = strings.ReplaceAll(cleanInfo, l, "")
		}
		if r := regionAttrRe.FindString(rest); r != "" {
			cleanInfo = strings.ReplaceAll(cleanInfo, r, "")
		}
		if h := highlightAttrRe.FindString(rest); h != "" {
			cleanInfo = strings.ReplaceAll(cleanInfo, h, "")
		}
		cleanInfo = strings.TrimSpace(cleanInfo)
		// Emit raw HTML for every include — gives us the <pre> handle
		// that Prism's Line Numbers plugin (always on) and Line
		// Highlight plugin (when highlight= is set) read their config
		// from. Goldmark passes raw HTML through under allowRawHTML
		// for trusted file-based content.
		//
		// data-start matches the first line of the rendered snippet
		// to its file-absolute position, so the gutter numbers line
		// up with what an editor shows for the source file. Multi-
		// range and whole-file includes still get sequential gutter
		// numbers starting from data-start, which is good enough for
		// v1 (perfect non-contiguous numbering would need a custom
		// gutter renderer).
		dataStart := ""
		if linesAttr != "" {
			if ranges, perr := parseLineRanges(linesAttr); perr == nil && len(ranges) > 0 {
				dataStart = fmt.Sprintf("%d", ranges[0][0])
			}
		}
		var sb strings.Builder
		sb.WriteString(`<pre class="language-`)
		sb.WriteString(escapeAttr(lang))
		sb.WriteString(` line-numbers"`)
		if highlightAttr != "" {
			sb.WriteString(` data-line="`)
			sb.WriteString(escapeAttr(highlightAttr))
			sb.WriteString(`"`)
		}
		if dataStart != "" && dataStart != "1" {
			sb.WriteString(` data-start="` + dataStart + `"`)
			// Tell Prism Line Highlight that data-line values are
			// already in the same number space as the gutter (which
			// runs from data-start). Without this offset, the
			// highlight plugin treats data-line as snippet-relative
			// and the overlay misaligns with the visible numbers.
			if highlightAttr != "" {
				if start, err := strconv.Atoi(dataStart); err == nil && start > 1 {
					sb.WriteString(fmt.Sprintf(` data-line-offset="%d"`, start-1))
				}
			}
		}
		sb.WriteString(`><code class="language-`)
		sb.WriteString(escapeAttr(lang))
		sb.WriteString(`">`)
		sb.WriteString(escapeText(text))
		sb.WriteString(`</code></pre>`)
		out = append(out, sb.String())

		// Append a "View on GitHub →" footer linking back to the real
		// file at the cited line range. Skipped silently when the
		// caller didn't provide enough info to construct a URL — the
		// snippet alone is still useful.
		if footer := renderSourceFooter(link, baseDir, absPath, linesAttr); footer != "" {
			out = append(out, "")
			out = append(out, footer)
		}

		included = append(included, absPath)
		i = closer
	}

	return []byte(strings.Join(out, "\n")), included, warnings
}

// renderSourceFooter returns the raw-HTML markdown line that links
// back to the included file's location in its source repository, or
// an empty string when the caller didn't supply enough info.
//
// The path passed back to the link is computed relative to the
// markdown page's directory and joined onto link.PagePathInRepo —
// matches what tinkerdown already does for "Edit this page on
// GitHub" links elsewhere in the codebase.
func renderSourceFooter(link LinkOptions, pageDir, absPath, lineRange string) string {
	if link.RepoURL == "" {
		return ""
	}
	relToPage, err := filepath.Rel(pageDir, absPath)
	if err != nil || strings.HasPrefix(relToPage, "..") {
		// Included file lives outside the page dir — we don't have a
		// confident way to map it to the repo path, so skip the link.
		return ""
	}
	relToPage = filepath.ToSlash(relToPage)

	repoPath := relToPage
	if link.PagePathInRepo != "" {
		repoPath = strings.TrimRight(link.PagePathInRepo, "/") + "/" + relToPage
	}

	branch := link.Branch
	if branch == "" {
		branch = "main"
	}
	url := strings.TrimRight(link.RepoURL, "/") + "/blob/" + branch + "/" + repoPath
	if frag := lineFragment(lineRange); frag != "" {
		url += frag
	}

	label := filepath.Base(absPath)
	if r := lineLabel(lineRange); r != "" {
		label += ":" + r
	}
	return `<p class="tinkerdown-include-source"><a href="` + escapeAttr(url) +
		`" target="_blank" rel="noopener">` + escapeText(label) + `</a></p>`
}

// lineLabel returns the human-readable suffix for the link label —
// "13" for a single line, "13-35" for a range, "5-8,15-22" for
// multi-range, empty for whole-file.
func lineLabel(lineRange string) string {
	if lineRange == "" {
		return ""
	}
	ranges, err := parseLineRanges(lineRange)
	if err != nil || len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r[0] == r[1] {
			parts = append(parts, fmt.Sprintf("%d", r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return strings.Join(parts, ",")
}

// lineFragment turns "20-32" into "#L20-L32" and "20" into "#L20".
// For comma-separated multi-range like "5-8,15-22", uses the FIRST
// range only — GitHub's URL fragments don't support multi-range
// (the second range would just scroll past the first). Empty /
// invalid ranges drop the fragment so the link points at the whole
// file.
func lineFragment(lineRange string) string {
	if lineRange == "" {
		return ""
	}
	ranges, err := parseLineRanges(lineRange)
	if err != nil || len(ranges) == 0 {
		return ""
	}
	start, end := ranges[0][0], ranges[0][1]
	if start == end {
		return fmt.Sprintf("#L%d", start)
	}
	return fmt.Sprintf("#L%d-L%d", start, end)
}

// escapeAttr / escapeText keep the footer safe for raw-HTML pass-
// through — the URL and label flow from authored frontmatter and
// resolved file paths, both trustable here, but escaping costs
// nothing and keeps the output well-formed.
func escapeAttr(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`"`, "&quot;",
		`<`, "&lt;",
		`>`, "&gt;",
	)
	return r.Replace(s)
}

func escapeText(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
	)
	return r.Replace(s)
}

// Resolve canonicalizes an include path supplied in markdown. It
// resolves relative paths against baseDir (the directory of the
// markdown file), then ensures the result lives inside root (the
// page-discovery root) so traversal attacks are rejected.
//
// Symlinks are followed on both candidate and root before the
// containment check so a symlinked tempdir (e.g. macOS /tmp →
// /private/tmp) doesn't false-positive as an escape.
func Resolve(baseDir, root, includePath string) (string, error) {
	if includePath == "" {
		return "", fmt.Errorf("include path is empty")
	}
	candidate := includePath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", includePath, err)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(absCandidate)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", includePath, err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", root, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", root, err)
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("include %q escapes the page root", includePath)
	}
	return resolvedCandidate, nil
}

// Slice reads absPath and returns either its full content (when
// lineRange is empty) or the 1-indexed inclusive [start, end] range.
// Multiple comma-separated ranges are supported (e.g. "5-8,15-22");
// they're joined with a single blank line. To insert an ellipsis
// comment between joined ranges, use SliceRanges with a non-empty
// separator instead.
//
// The end of each range is clamped to the file's actual line count
// so a stale range that overruns produces a useful snippet rather
// than an error. The start must be in-range.
//
// Trailing newlines from the source file are preserved on full-file
// reads but stripped from sliced ranges, so the rendered code block
// doesn't end with a stray blank line.
func Slice(absPath, lineRange string) (string, error) {
	return SliceRanges(absPath, lineRange, "")
}

// SliceRanges is Slice with an explicit separator inserted between
// joined ranges. Useful for multi-range includes — pass
// LanguageEllipsis(lang) to get a syntax-highlighted "// ..." line
// (or its language equivalent) between ranges.
func SliceRanges(absPath, lineRange, separator string) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", absPath, err)
	}
	if lineRange == "" {
		return string(data), nil
	}

	ranges, err := parseLineRanges(lineRange)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		start, end := r[0], r[1]
		if start > len(lines) {
			return "", fmt.Errorf("lines=%s starts past end of %s (%d lines)", lineRange, absPath, len(lines))
		}
		if end > len(lines) {
			end = len(lines)
		}
		parts = append(parts, strings.Join(lines[start-1:end], "\n"))
	}
	if separator == "" {
		return strings.Join(parts, "\n\n"), nil
	}
	return strings.Join(parts, "\n\n"+separator+"\n\n"), nil
}

// FindRegion scans absPath for the named-region markers
// `>>> region:NAME` (start) and `<<< region:NAME` (end), and returns
// the 1-indexed inclusive [start, end] line range of the content
// strictly between them. The markers themselves are excluded.
//
// Markers can be wrapped in any single-line comment style — the
// matcher only looks for the literal `>>> region:NAME` / `<<<
// region:NAME` substring on a line, so:
//
//	// >>> region:state          // Go / C-family
//	# >>> region:state           # Python / shell / yaml
//	<!-- >>> region:state -->    HTML
//
// all work. Errors when either marker is missing or appears in the
// wrong order.
func FindRegion(absPath, name string) (start, end int, err error) {
	if name == "" {
		return 0, 0, fmt.Errorf("region name is empty")
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", absPath, err)
	}
	startMarker := ">>> region:" + name
	endMarker := "<<< region:" + name
	startIdx, endIdx := -1, -1
	for i, line := range strings.Split(string(data), "\n") {
		switch {
		case startIdx == -1 && strings.Contains(line, startMarker):
			startIdx = i
		case startIdx != -1 && endIdx == -1 && strings.Contains(line, endMarker):
			endIdx = i
		}
		if startIdx != -1 && endIdx != -1 {
			break
		}
	}
	if startIdx == -1 {
		return 0, 0, fmt.Errorf("region %q: start marker %q not found in %s", name, startMarker, absPath)
	}
	if endIdx == -1 {
		return 0, 0, fmt.Errorf("region %q: end marker %q not found in %s", name, endMarker, absPath)
	}
	if endIdx <= startIdx+1 {
		return 0, 0, fmt.Errorf("region %q: end marker not after start marker (or empty region)", name)
	}
	return startIdx + 2, endIdx, nil
}

// LanguageEllipsis returns the comment-style ellipsis line that fits
// the named fence language — Prism will syntax-highlight it as a
// proper comment so it visually separates joined ranges without
// breaking the surrounding code's style.
func LanguageEllipsis(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "c", "cpp", "c++", "java", "ts", "tsx", "js", "jsx", "rust", "swift", "kotlin", "css", "scss", "less":
		return "// ..."
	case "python", "py", "ruby", "rb", "shell", "sh", "bash", "zsh", "yaml", "yml", "toml", "makefile", "make", "dockerfile", "ini", "conf":
		return "# ..."
	case "html", "xml", "svg", "vue", "svelte":
		return "<!-- ... -->"
	case "sql", "lua", "haskell", "hs":
		return "-- ..."
	default:
		return "// ..."
	}
}

// Dedent strips the longest common leading whitespace across all
// non-blank lines, preserving the relative indentation of the
// snippet. Blank lines are ignored when computing the prefix and
// also stripped of leading whitespace in the output.
func Dedent(text string) string {
	lines := strings.Split(text, "\n")
	prefix := ""
	prefixSet := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		current := leadingWhitespace(line)
		if !prefixSet {
			prefix = current
			prefixSet = true
			continue
		}
		prefix = commonPrefix(prefix, current)
		if prefix == "" {
			break
		}
	}
	if prefix == "" {
		return text
	}
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = line[len(prefix):]
		}
	}
	return strings.Join(lines, "\n")
}

// parseLineRanges parses comma-separated ranges like "5-8,15-22" or
// the single-range "5-8" form. Whitespace around commas and dashes
// is tolerated. Single integers ("5") are accepted as one-line
// ranges. Returns an error on any malformed segment.
func parseLineRanges(s string) ([][2]int, error) {
	parts := strings.Split(s, ",")
	out := make([][2]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		start, end, err := parseLineRange(p)
		if err != nil {
			return nil, err
		}
		out = append(out, [2]int{start, end})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("lines=%q: no valid ranges", s)
	}
	return out, nil
}

// parseLineRange parses "N-M" into a 1-indexed inclusive [start, end]
// pair. Single integers ("N") are accepted as a one-line range.
// Invalid forms (non-numeric, end < start, zero, negative) all error.
func parseLineRange(s string) (int, int, error) {
	parts := strings.SplitN(s, "-", 2)
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("lines=%q: %w", s, err)
	}
	if start < 1 {
		return 0, 0, fmt.Errorf("lines=%q: start must be >= 1", s)
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("lines=%q: %w", s, err)
		}
	}
	if end < start {
		return 0, 0, fmt.Errorf("lines=%q: end (%d) is before start (%d)", s, end, start)
	}
	return start, end, nil
}

// leadingWhitespace returns the run of whitespace at the start of s.
// Tabs and spaces are returned verbatim — Dedent treats them as
// literal characters, so mixed-indent snippets only collapse to the
// common prefix that's identical across lines.
func leadingWhitespace(s string) string {
	for i, r := range s {
		if !unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

// commonPrefix returns the longest string that's a prefix of both a
// and b. Operates on bytes — for whitespace prefixes (ASCII tabs and
// spaces) that's equivalent to runes.
func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}
