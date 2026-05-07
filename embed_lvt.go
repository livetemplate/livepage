package tinkerdown

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// defaultEmbedTimeout caps how long the docs page is allowed to wait
// for a single upstream LiveTemplate app while assembling the page.
// Slow upstreams degrade docs render speed proportionally — keep this
// tight by default, let authors override per-block when needed.
const defaultEmbedTimeout = 2 * time.Second

// embedLvtRe matches the placeholder div emitted by injectBlockAttributes
// for ` ```embed-lvt ` blocks. The placeholder is self-closing; it carries
// only data-* attributes and is replaced wholesale by ProcessEmbedLvt.
var embedLvtRe = regexp.MustCompile(`<div class="tinkerdown-embed-lvt"[^>]*></div>`)

// dataAttrRe captures one data-* (or style=) attribute name and its
// double-quoted value. Used to read placeholder attributes without
// pulling in a full HTML parser for the placeholder itself.
var dataAttrRe = regexp.MustCompile(`\b(data-[a-z-]+|style)="([^"]*)"`)

// embedAttrs is the parsed subset of placeholder attributes that drive
// fetcher behavior. All fields are optional except via parser-side
// invariants (server defaults to current origin, path defaults to "/").
type embedAttrs struct {
	blockID    string
	server     string
	upstream   string
	path       string
	session    string
	style      string
	timeout    time.Duration
	showSource bool
}

// ProcessEmbedLvt scans rendered page HTML for `embed-lvt` placeholders
// and replaces each with the wrapper HTML returned by the upstream
// LiveTemplate app. Forwards the docs reader's Cookie and Accept-Language
// headers so cookie-based auth keeps working when apps share a cookie
// domain.
//
// On error or timeout, the placeholder becomes a small "live demo
// unavailable" badge and the page renders without the embed.
//
// Same-origin embeds (path-only, no `server` attribute) resolve against
// the request's host so the docs site can embed apps proxied through
// itself with zero CORS or origin configuration.
func ProcessEmbedLvt(htmlStr string, req *http.Request) string {
	return embedLvtRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		attrs := parseEmbedAttrs(match)
		upstreamURL, err := buildUpstreamURL(attrs, req)
		if err != nil {
			return embedUnavailableBadge(attrs, "configuration error")
		}

		body, err := fetchUpstream(upstreamURL, req, attrs.timeout)
		if err != nil {
			return embedUnavailableBadge(attrs, err.Error())
		}

		wrapperHTML := extractLvtWrapper(body)
		if wrapperHTML == "" {
			return embedUnavailableBadge(attrs, "no LiveTemplate wrapper in upstream response")
		}

		return renderEmbed(attrs, wrapperHTML)
	})
}

// parseEmbedAttrs reads the placeholder's data-* and style attributes
// into an embedAttrs struct. Unknown attributes are ignored.
func parseEmbedAttrs(placeholder string) embedAttrs {
	a := embedAttrs{path: "/", timeout: defaultEmbedTimeout}
	for _, m := range dataAttrRe.FindAllStringSubmatch(placeholder, -1) {
		switch m[1] {
		case "data-block-id":
			a.blockID = m[2]
		case "data-embed-server":
			a.server = m[2]
		case "data-embed-path":
			if m[2] != "" {
				a.path = m[2]
			}
		case "data-embed-session":
			a.session = m[2]
		case "data-embed-timeout":
			if d, err := time.ParseDuration(m[2]); err == nil && d > 0 {
				a.timeout = d
			}
		case "data-embed-upstream":
			a.upstream = m[2]
		case "data-show-source":
			a.showSource = m[2] == "true"
		case "style":
			a.style = m[2]
		}
	}
	return a
}

// buildUpstreamURL resolves the placeholder's coordinates into the URL
// the server-side fetch GETs. Precedence:
//
//  1. `upstream` (auto-proxy mode): fetch directly from the upstream
//     origin, skipping the self-proxy hop. The proxy still handles the
//     browser-side WebSocket via the registered route.
//  2. `server` (cross-origin mode): use the explicit remote origin.
//  3. Same-origin fallback: derive the origin from the docs request,
//     which is the right choice when the operator manually configured a
//     `routes:` entry in tinkerdown.yaml — the GET reaches the upstream
//     through tinkerdown's own proxy.
func buildUpstreamURL(a embedAttrs, req *http.Request) (string, error) {
	if a.upstream != "" {
		return strings.TrimRight(a.upstream, "/") + a.path, nil
	}
	if a.server == "" {
		if req == nil {
			return "", fmt.Errorf("no server/upstream attribute and no request to derive origin")
		}
		scheme := "http"
		if req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		return scheme + "://" + req.Host + a.path, nil
	}
	server := strings.TrimRight(a.server, "/")
	return server + a.path, nil
}

// fetchUpstream performs the GET. Forwards Cookie and Accept-Language
// from the docs request when present so the upstream sees the same
// auth context.
func fetchUpstream(upstreamURL string, req *http.Request, timeout time.Duration) (string, error) {
	ctx := context.Background()
	if req != nil {
		ctx = req.Context()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	if req != nil {
		if v := req.Header.Get("Cookie"); v != "" {
			httpReq.Header.Set("Cookie", v)
		}
		if v := req.Header.Get("Accept-Language"); v != "" {
			httpReq.Header.Set("Accept-Language", v)
		}
	}
	httpReq.Header.Set("User-Agent", "tinkerdown/embed-lvt")
	httpReq.Header.Set("Accept", "text/html")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", upstreamURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// extractLvtWrapper parses the upstream response and returns the outer
// HTML of the first `<div data-lvt-id="...">` element it finds, with
// the `data-lvt-id` attribute renamed to `data-lvt-id-pending` so the
// LiveTemplateClient module-level autoInit (in @livetemplate/client)
// does not race with our EmbedLvtBlock by grabbing the wrapper and
// trying to connect with default URLs. The client-side EmbedLvtBlock
// renames it back to `data-lvt-id` before invoking connect().
//
// The choice of "first wrapper" matches livetemplate's current
// one-app-per-page constraint: a LiveTemplate HTTP response contains
// exactly one wrapper. If a future livetemplate version returns
// multiple wrappers we'd need to surface them all and let the embed
// block pick by id; not relevant today.
//
// Returns the empty string if no wrapper is found — the caller renders
// the unavailable badge in that case.
func extractLvtWrapper(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	wrapper := findLvtWrapper(doc)
	if wrapper == nil {
		return ""
	}
	for i, a := range wrapper.Attr {
		if a.Key == "data-lvt-id" {
			wrapper.Attr[i].Key = "data-lvt-id-pending"
			break
		}
	}
	var sb strings.Builder
	if err := html.Render(&sb, wrapper); err != nil {
		return ""
	}
	return sb.String()
}

// findLvtWrapper walks the parsed document and returns the first node
// that has a `data-lvt-id` attribute, or nil if none exists.
func findLvtWrapper(n *html.Node) *html.Node {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "data-lvt-id" {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findLvtWrapper(c); found != nil {
			return found
		}
	}
	return nil
}

// renderEmbed wraps the inlined upstream HTML in the tinkerdown
// embed-lvt container so the client-side discovery scan picks it up.
// The container carries the upstream coordinates the client needs to
// open the WebSocket against the right origin.
//
// When the placeholder requested show-source, the output is wrapped in
// the same tinkerdown-lvt-demo card used by literate `lvt` blocks: a
// syntax-highlighted view of the upstream HTML on top, the live
// embed wrapper underneath. The source view is a snapshot of the
// fetched HTML — subsequent WebSocket-driven DOM updates do not
// re-write it (intentional: the source documents structure, the live
// view shows current state).
func renderEmbed(a embedAttrs, wrapperHTML string) string {
	embed := buildEmbedContainer(a, wrapperHTML)
	if !a.showSource {
		return embed
	}
	var sb strings.Builder
	sb.WriteString(`<div class="tinkerdown-lvt-demo tinkerdown-lvt-demo-stacked">`)
	sb.WriteString(`<pre><code class="language-html">`)
	sb.WriteString(escapeHTML(wrapperHTML))
	sb.WriteString(`</code></pre>`)
	sb.WriteString(embed)
	sb.WriteString(`</div>`)
	return sb.String()
}

func buildEmbedContainer(a embedAttrs, wrapperHTML string) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tinkerdown-embed-lvt" data-tinkerdown-block`)
	if a.blockID != "" {
		fmt.Fprintf(&sb, ` data-block-id=%q`, a.blockID)
	}
	sb.WriteString(` data-block-type="embed-lvt"`)
	if a.server != "" {
		fmt.Fprintf(&sb, ` data-embed-server=%q`, a.server)
	}
	if a.path != "" && a.path != "/" {
		fmt.Fprintf(&sb, ` data-embed-path=%q`, a.path)
	}
	if a.session != "" {
		fmt.Fprintf(&sb, ` data-embed-session=%q`, a.session)
	}
	if a.style != "" {
		fmt.Fprintf(&sb, ` style=%q`, a.style)
	}
	sb.WriteByte('>')
	sb.WriteString(wrapperHTML)
	sb.WriteString(`</div>`)
	return sb.String()
}

// embedUnavailableBadge returns the HTML shown in place of the embed
// when the upstream fetch fails. The reason is rendered as a title
// attribute (visible on hover) but not in the page text — keeps the
// docs page reading cleanly even when a demo is down.
func embedUnavailableBadge(a embedAttrs, reason string) string {
	style := `padding:0.75rem 1rem;border:1px dashed var(--card-border,#bbb);` +
		`border-radius:8px;color:var(--muted-color,#666);font-size:0.9em;`
	if a.style != "" {
		style = a.style + ";" + style
	}
	return fmt.Sprintf(
		`<div class="tinkerdown-embed-lvt unavailable" data-block-type="embed-lvt" style=%q title=%q>live demo unavailable</div>`,
		style,
		reason,
	)
}

// ParseTimeout parses a duration string like "2s" / "1500ms" and returns
// the duration; falls back to defaultEmbedTimeout for empty or invalid
// input. Exported so other tinkerdown packages (e.g. tests, future
// proxy integration) can share the same parsing rules.
func ParseTimeout(s string) time.Duration {
	if s == "" {
		return defaultEmbedTimeout
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultEmbedTimeout
}
